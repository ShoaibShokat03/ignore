package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateIgnoreFileCreatesDefaultFile(t *testing.T) {
	dir := t.TempDir()
	path, err := CreateIgnoreFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, ".ignore") {
		t.Fatalf("unexpected path: %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "node_modules/") {
		t.Fatalf("default .ignore content missing expected rule: %s", string(b))
	}
}

func TestCreateIgnoreFileDoesNotOverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".ignore")
	if err := os.WriteFile(path, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := CreateIgnoreFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("unexpected path: %s", got)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "custom\n" {
		t.Fatalf("existing .ignore was overwritten: %q", string(b))
	}
}

func TestCreateIgnoreFileUsesParentForFileTargets(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "project.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := CreateIgnoreFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, ".ignore") {
		t.Fatalf("unexpected path: %s", path)
	}
}
