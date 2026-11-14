package pipeline

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/logx"
	"github.com/karolsko/gobunshin/internal/runctx"
)

func TestInstallersDmg(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "demo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(dir, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	project := config.Project{
		ProjectName: "demo",
		Installers: []config.Installer{{
			Name:    "demo",
			Formats: []string{"dmg"},
		}},
	}
	config.ApplyDefaults(&project, dir)
	if err := config.Validate(&project); err != nil {
		t.Fatal(err)
	}
	rc := &runctx.Context{
		Go:         context.Background(),
		Dir:        dir,
		Dist:       dist,
		Project:    project,
		Version:    "1.2.3",
		CommitTime: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
		Now:        time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
		Log:        logx.New(io.Discard, false),
		Env:        map[string]string{},
	}
	rc.AddBinary(&artifact.Artifact{
		Name: "demo", Path: bin, Type: artifact.TypeBinary,
		Goos: "darwin", Goarch: "amd64", Binary: "demo", BuildID: "demo",
	})
	rc.AddBinary(&artifact.Artifact{
		Name: "demo-linux", Path: bin, Type: artifact.TypeBinary,
		Goos: "linux", Goarch: "amd64", Binary: "demo", BuildID: "demo",
	})
	if err := Installers(rc); err != nil {
		t.Fatal(err)
	}
	arts := rc.ArtifactsCopy()
	if len(arts) != 1 {
		t.Fatalf("got %d artifacts", len(arts))
	}
	got := arts[0]
	if got.Format != "dmg" || got.Goos != "darwin" || got.Name != "demo_1.2.3_darwin_amd64.dmg" {
		t.Fatalf("artifact %+v", got)
	}
	body := mustRead(t, got.Path)
	if len(body) < 512 || string(body[len(body)-512:len(body)-508]) != "koly" {
		t.Fatal("missing UDIF koly trailer")
	}
}
