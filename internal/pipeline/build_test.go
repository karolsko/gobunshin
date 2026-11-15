package pipeline

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/logx"
	"github.com/karolsko/gobunshin/internal/runctx"
)

func TestWindowsIconEmbedded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.26.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var pngBuf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	img.Set(0, 0, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icon.png"), pngBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	project := config.Project{
		ProjectName: "demo",
		Builds: []config.Build{{
			Main:   ".",
			Icon:   "icon.png",
			Goos:   []string{"windows"},
			Goarch: []string{"amd64"},
			Env:    []string{"CGO_ENABLED=0"},
		}},
	}
	config.ApplyDefaults(&project, dir)
	if err := config.Validate(&project); err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(dir, "dist")
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
	if err := Build(rc); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dist, "build", "demo_windows_amd64", "demo.exe")
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRsrc(body) {
		t.Fatal("windows executable has no .rsrc section")
	}
	if _, err := os.Stat(filepath.Join(dir, "gobunshin_windows_amd64.syso")); !os.IsNotExist(err) {
		t.Fatal("icon object file was left in the package directory")
	}
}

func hasRsrc(b []byte) bool {
	if len(b) < 0x40 || string(b[:2]) != "MZ" {
		return false
	}
	pe := int(binary.LittleEndian.Uint32(b[0x3c:0x40]))
	if pe < 0 || pe+24 > len(b) || string(b[pe:pe+4]) != "PE\x00\x00" {
		return false
	}
	sections := int(binary.LittleEndian.Uint16(b[pe+6 : pe+8]))
	opt := int(binary.LittleEndian.Uint16(b[pe+20 : pe+22]))
	off := pe + 24 + opt
	for i := 0; i < sections; i++ {
		start := off + i*40
		if start+8 > len(b) {
			return false
		}
		name := strings.TrimRight(string(b[start:start+8]), "\x00")
		if name == ".rsrc" {
			return true
		}
	}
	return false
}
