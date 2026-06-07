package ignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseHonorsIgnoreSectionCommentsAndNegation(t *testing.T) {
	rules, err := Parse(strings.NewReader(`
node_modules
[IGNORE]
# comment
node_modules
*.log
!keep.log
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 active rules, got %d", len(rules))
	}
	if rules[0].Pattern != "node_modules" || rules[2].Pattern != "keep.log" || !rules[2].Negate {
		t.Fatalf("unexpected rules: %#v", rules)
	}
}

func TestRuleSetMatchesGlobalAndProjectOverride(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, ".ignore")
	if err := os.WriteFile(global, []byte("[IGNORE]\n*.log\nnode_modules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(dir, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".ignore"), []byte("[IGNORE]\n!keep.log\nbuild\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs := NewRuleSet(global)
	if err := rs.Reload(); err != nil {
		t.Fatal(err)
	}
	if !rs.Match(filepath.Join(project, "debug.log"), false).Ignored {
		t.Fatal("expected global wildcard to ignore debug.log")
	}
	if rs.Match(filepath.Join(project, "keep.log"), false).Ignored {
		t.Fatal("expected project negation to override global wildcard")
	}
	if !rs.Match(filepath.Join(project, "src", "node_modules"), true).Ignored {
		t.Fatal("expected folder name to match recursively")
	}
}

func TestRuleSetReloadsChangedProjectIgnore(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.ignore")
	projectIgnore := filepath.Join(dir, ".ignore")
	if err := os.WriteFile(global, []byte("[IGNORE]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectIgnore, []byte("[IGNORE]\ncache\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs := NewRuleSet(global)
	if err := rs.Reload(); err != nil {
		t.Fatal(err)
	}
	if !rs.Match(filepath.Join(dir, "cache"), true).Ignored {
		t.Fatal("expected initial project rule to ignore cache")
	}
	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(projectIgnore, []byte("[IGNORE]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rs.Match(filepath.Join(dir, "cache"), true).Ignored {
		t.Fatal("expected changed project ignore file to invalidate cached rules")
	}
}
