package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIncludeMerge(t *testing.T) {
	dir := t.TempDir()
	base := []byte(`
variables:
  owner: base
builds:
  - id: app
    env: ["CGO_ENABLED=0"]
    ldflags: ["-s -w"]
    goos: [linux]
`)
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), base, 0o644); err != nil {
		t.Fatal(err)
	}
	root := []byte(`
version: 1
includes:
  - base.yaml
project_name: demo
variables:
  channel: stable
builds:
  - id: app
    main: ./cmd/demo
    binary: demo
    ldflags: ["-s -w -X main.version={{ .Version }}"]
  - id: extra
    main: .
    binary: extra
    goos: [linux]
    goarch: [amd64]
`)
	path := filepath.Join(dir, ".gobunshin.yaml")
	if err := os.WriteFile(path, root, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ApplyDefaults(p, dir)
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	if p.Variables["owner"] != "base" || p.Variables["channel"] != "stable" {
		t.Fatalf("variables: %+v", p.Variables)
	}
	if len(p.Builds) != 2 {
		t.Fatalf("builds: %+v", p.Builds)
	}
	app := p.Builds[0]
	if app.Main != "./cmd/demo" || app.Binary != "demo" {
		t.Fatalf("overlay lost: %+v", app)
	}
	if len(app.Env) != 1 || app.Env[0] != "CGO_ENABLED=0" {
		t.Fatalf("env not kept: %+v", app.Env)
	}
	if len(app.Ldflags) != 1 || app.Ldflags[0] == "-s -w" {
		t.Fatalf("ldflags not replaced: %+v", app.Ldflags)
	}
	if len(app.Goos) != 1 || app.Goos[0] != "linux" {
		t.Fatalf("goos not kept: %+v", app.Goos)
	}
	if p.Builds[1].ID != "extra" {
		t.Fatalf("extra build missing")
	}
}

func TestIncludeCycleAndUnknownField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("includes: [b.yaml]\nproject_name: a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("includes: [a.yaml]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(dir, "a.yaml")); err == nil {
		t.Fatal("expected cycle error")
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("project_name: demo\nnot_a_field: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestDefaultsDetectCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "demo", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/acme/demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Project{}
	ApplyDefaults(p, dir)
	if p.ProjectName != "demo" {
		t.Fatalf("name %s", p.ProjectName)
	}
	if p.Builds[0].Main != "./cmd/demo" {
		t.Fatalf("main %s", p.Builds[0].Main)
	}
	if p.Release.GitHub.Owner != "acme" || p.Release.GitHub.Name != "demo" {
		t.Fatalf("github %+v", p.Release.GitHub)
	}
	if !p.Archives[0].Wrapped() {
		t.Fatal("default wrap")
	}
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
}
