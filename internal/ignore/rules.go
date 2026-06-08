package ignore

import (
	"bufio"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type Rule struct {
	Pattern  string
	Negate   bool
	DirOnly  bool
	Anchored bool
	HasSlash bool
	Root     string
}

type RuleSet struct {
	mu          sync.RWMutex
	globalPath  string
	globalRules []Rule
	dirCache    map[string]cacheEntry
	generation  uint64
}

type cacheEntry struct {
	rules       []Rule
	projectPath string
	projectMod  int64
	generation  uint64
}

type Decision struct {
	Ignored bool
	Rule    string
}

func NewRuleSet(globalPath string) *RuleSet {
	return &RuleSet{globalPath: globalPath, dirCache: make(map[string]cacheEntry)}
}

func (r *RuleSet) Reload() error {
	rules, err := ParseFile(r.globalPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	r.mu.Lock()
	r.globalRules = rules
	r.dirCache = make(map[string]cacheEntry)
	r.generation++
	r.mu.Unlock()
	return nil
}

func (r *RuleSet) Match(path string, isDir bool) Decision {
	dir := path
	if !isDir {
		dir = filepath.Dir(path)
	}
	rules := r.rulesFor(dir)
	ignored := false
	var matched string
	for _, rule := range rules {
		if matchRule(rule, path, isDir) {
			ignored = !rule.Negate
			matched = rule.Pattern
		}
	}
	return Decision{Ignored: ignored, Rule: matched}
}

func (r *RuleSet) rulesFor(dir string) []Rule {
	dir = filepath.Clean(dir)
	r.mu.RLock()
	if entry, ok := r.dirCache[dir]; ok && entry.generation == r.generation && projectIgnoreUnchanged(entry.projectPath, entry.projectMod) {
		cp := append([]Rule(nil), entry.rules...)
		r.mu.RUnlock()
		return cp
	}
	global := append([]Rule(nil), r.globalRules...)
	generation := r.generation
	r.mu.RUnlock()

	project := findProjectIgnore(dir)
	projectMod := int64(0)
	if project != "" {
		projectMod = modTimeUnixNano(project)
		if rules, err := ParseFile(project); err == nil {
			global = append(global, rules...)
		}
	}

	r.mu.Lock()
	r.dirCache[dir] = cacheEntry{
		rules:       append([]Rule(nil), global...),
		projectPath: project,
		projectMod:  projectMod,
		generation:  generation,
	}
	r.mu.Unlock()
	return global
}

func projectIgnoreUnchanged(path string, cachedMod int64) bool {
	if path == "" {
		return true
	}
	return modTimeUnixNano(path) == cachedMod
}

func modTimeUnixNano(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func findProjectIgnore(start string) string {
	dir := filepath.Clean(start)
	for {
		candidate := filepath.Join(dir, ".ignore")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func ParseFile(path string) ([]Rule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rules, err := Parse(f)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(path)
	for i := range rules {
		rules[i].Root = root
	}
	return rules, nil
}

func Parse(rd io.Reader) ([]Rule, error) {
	scanner := bufio.NewScanner(rd)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	lines := make([]string, 0, 64)
	hasIgnoreSection := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(line, "[IGNORE]") {
			hasIgnoreSection = true
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	active := !hasIgnoreSection
	rules := make([]Rule, 0, 64)
	for _, line := range lines {
		if strings.EqualFold(line, "[IGNORE]") {
			active = true
			continue
		}
		if !active || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := parseLine(line)
		if rule.Pattern != "" {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func parseLine(line string) Rule {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	rule := Rule{}
	if strings.HasPrefix(line, `\#`) {
		line = strings.TrimPrefix(line, `\`)
	}
	if strings.HasPrefix(line, "!") {
		rule.Negate = true
		line = strings.TrimPrefix(line, "!")
		if strings.HasPrefix(line, `\#`) {
			line = strings.TrimPrefix(line, `\`)
		}
	}
	line = strings.TrimSpace(filepath.ToSlash(line))
	if line == "" || line == "." {
		return Rule{}
	}
	if strings.HasSuffix(line, "/") {
		rule.DirOnly = true
		line = strings.TrimRight(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		rule.Anchored = true
		line = strings.TrimLeft(line, "/")
	}
	line = path.Clean(line)
	line = strings.Trim(line, "/")
	if line == "." || line == "" {
		return Rule{}
	}
	rule.HasSlash = strings.Contains(line, "/")
	rule.Pattern = line
	return rule
}

func matchRule(rule Rule, filePath string, isDir bool) bool {
	if rule.Pattern == "" || (rule.DirOnly && !isDir) {
		return false
	}
	rel := normalizedRulePath(rule, filePath)
	base := path.Base(rel)
	pattern := strings.ToLower(filepath.ToSlash(rule.Pattern))

	if rule.Anchored {
		return globMatch(pattern, rel)
	}
	if !rule.HasSlash && !strings.ContainsAny(pattern, "*?[") {
		return base == pattern
	}
	if !rule.HasSlash {
		return globMatch(pattern, base)
	}
	if globMatch(pattern, rel) {
		return true
	}
	return globMatch("**/"+pattern, rel)
}

func normalizedRulePath(rule Rule, filePath string) string {
	clean := filepath.Clean(filePath)
	if rule.Root != "" {
		if rel, err := filepath.Rel(rule.Root, clean); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			return strings.ToLower(filepath.ToSlash(rel))
		}
	}
	return strings.ToLower(filepath.ToSlash(clean))
}

func globMatch(pattern, value string) bool {
	if ok, _ := path.Match(pattern, value); ok {
		return true
	}
	rx, err := globRegex(pattern)
	if err != nil {
		return false
	}
	return rx.MatchString(value)
}

func globRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(ch)
		default:
			b.WriteByte(ch)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
