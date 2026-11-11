package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionAndUnknown(t *testing.T) {
	if code := Main([]string{"version"}, BuildInfo{Version: "1.2.3", Commit: "abc", Date: "2026-01-02"}); code != 0 {
		t.Fatalf("version exit %d", code)
	}
	if code := Main([]string{"nope"}, BuildInfo{}); code != 2 {
		t.Fatalf("unknown exit %d", code)
	}
}

func TestInitThenCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/acme/demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "demo", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if code := Main(nil, BuildInfo{}); code != 0 {
		t.Fatalf("help %d", code)
	}
	if code := Main([]string{"init"}, BuildInfo{}); code != 0 {
		t.Fatal("init failed")
	}
	if code := Main([]string{"init"}, BuildInfo{}); code != 1 {
		t.Fatal("second init should fail")
	}
	if code := Main([]string{"check"}, BuildInfo{}); code != 0 {
		t.Fatal("check failed")
	}
}
