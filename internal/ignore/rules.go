package ignore

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Rule struct {
	Pattern string
	Negate  bool
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
	relName := filepath.Base(path)
	normalized := strings.ToLower(filepath.Clean(path))
	ignored := false
	var matched string
	for _, rule := range rules {
		if matchRule(rule.Pattern, relName, normalized) {
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
	return Parse(f)
}

func Parse(rd io.Reader) ([]Rule, error) {
	scanner := bufio.NewScanner(rd)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	active := false
	rules := make([]Rule, 0, 64)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(line, "[IGNORE]") {
			active = true
			continue
		}
		if !active || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := Rule{Pattern: filepath.Clean(line)}
		if strings.HasPrefix(rule.Pattern, "!") {
			rule.Negate = true
			rule.Pattern = strings.TrimPrefix(rule.Pattern, "!")
		}
		rule.Pattern = strings.Trim(rule.Pattern, `/\`)
		if rule.Pattern != "" {
			rules = append(rules, rule)
		}
	}
	return rules, scanner.Err()
}

func matchRule(pattern, base, normalizedPath string) bool {
	pattern = strings.ToLower(strings.Trim(pattern, `/\`))
	base = strings.ToLower(base)
	if pattern == base {
		return true
	}
	if strings.ContainsAny(pattern, "*?[") {
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
	}
	if strings.Contains(pattern, string(filepath.Separator)) {
		return strings.HasSuffix(normalizedPath, strings.ToLower(filepath.Clean(pattern)))
	}
	return false
}
