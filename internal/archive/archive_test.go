package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateStableAndReadable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "demo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(dir, "README.md")
	if err := os.WriteFile(note, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	entries := []Entry{
		{Name: "demo_1.0.0_linux_amd64/demo", Source: bin, Mode: 0o755},
		{Name: "demo_1.0.0_linux_amd64/README.md", Source: note, Mode: 0o644},
	}
	a := filepath.Join(dir, "a.tar.gz")
	b := filepath.Join(dir, "b.tar.gz")
	if err := Create(a, "tar.gz", entries, when); err != nil {
		t.Fatal(err)
	}
	if err := Create(b, "tar.gz", entries, when); err != nil {
		t.Fatal(err)
	}
	ab, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ab, bb) {
		t.Fatal("tar.gz bytes differ between identical inputs")
	}
	gz, err := gzip.NewReader(bytes.NewReader(ab))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, hdr.Name)
	}
	if len(names) != 2 {
		t.Fatalf("names %+v", names)
	}

	zpath := filepath.Join(dir, "a.zip")
	if err := Create(zpath, "zip", entries, when); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(zpath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 {
		t.Fatalf("zip files %d", len(zr.File))
	}

	raw := filepath.Join(dir, "demo.out")
	if err := Create(raw, "binary", []Entry{{Name: "demo", Source: bin, Mode: 0o755}}, when); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("echo hi")) {
		t.Fatalf("raw %q", got)
	}
}
