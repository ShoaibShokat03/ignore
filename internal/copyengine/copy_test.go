package copyengine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"ignore/internal/ignore"
	"ignore/internal/metrics"
)

func TestCopyFilteredSkipsIgnoredDirectoriesBeforeTraversal(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.MkdirAll(filepath.Join(src, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "app", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "node_modules", "pkg", "big.js"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(root, ".ignore")
	if err := os.WriteFile(global, []byte("[IGNORE]\nnode_modules\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := ignore.NewRuleSet(global)
	if err := rules.Reload(); err != nil {
		t.Fatal(err)
	}
	engine := New(rules, metrics.New(), slog.New(slog.NewTextHandler(os.Stderr, nil)), 2, 32*1024)
	result, err := engine.CopyFiltered(context.Background(), src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesCopied != 1 || result.DirsSkipped != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("expected node_modules to be skipped, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "app", "main.go")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
}
