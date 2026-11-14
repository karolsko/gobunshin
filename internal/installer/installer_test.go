package installer

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPkgExpands(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	body := []byte("#!/bin/sh\necho hi\n")
	pkg, err := Pkg("com.example.demo", "1.2.3", []File{{
		Path: "/usr/local/bin/demo",
		Data: body,
	}}, when)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pkg, []byte("xar!")) {
		t.Fatal("missing xar magic")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.pkg")
	if err := os.WriteFile(path, pkg, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("pkgutil"); err != nil {
		t.Skip("pkgutil is not available")
	}
	expanded := filepath.Join(dir, "expanded")
	cmd := exec.Command("pkgutil", "--expand", path, expanded)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pkgutil --expand: %v\n%s", err, out)
	}
	info, err := os.ReadFile(filepath.Join(expanded, "PackageInfo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`identifier="com.example.demo"`, `version="1.2.3"`, "<payload "} {
		if !strings.Contains(string(info), want) {
			t.Fatalf("PackageInfo missing %s:\n%s", want, info)
		}
	}
	if _, err := exec.LookPath("lsbom"); err == nil {
		ls := exec.Command("lsbom", filepath.Join(expanded, "Bom"))
		text, err := ls.CombinedOutput()
		if err != nil {
			t.Fatalf("lsbom: %v\n%s", err, text)
		}
		if !bytes.Contains(text, []byte("usr/local/bin/demo")) && !bytes.Contains(text, []byte("demo")) {
			t.Fatalf("bom listing:\n%s", text)
		}
	}
}

func TestDmgVerifies(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	body := []byte("#!/bin/sh\necho hi\n")
	img, err := Dmg("gobunshin", []File{{
		Path: "/usr/local/bin/gobunshin",
		Data: body,
	}}, when)
	if err != nil {
		t.Fatal(err)
	}
	if len(img) < 512 || string(img[len(img)-512:len(img)-508]) != "koly" {
		t.Fatal("missing UDIF koly trailer")
	}
	path := filepath.Join(t.TempDir(), "gobunshin.dmg")
	if err := os.WriteFile(path, img, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil is not available")
	}
	cmd := exec.Command("hdiutil", "verify", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hdiutil verify: %v\n%s", err, out)
	}
}

func TestNSISScript(t *testing.T) {
	script := NSISScript("gobunshin", "1.2.3", "amd64", "/tmp/gobunshin.exe", "/tmp/out.exe")
	for _, want := range []string{`Name "gobunshin 1.2.3"`, `$PROGRAMFILES64\gobunshin`, `File "/oname=gobunshin.exe"`, "DisplayVersion"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q\n%s", want, script)
		}
	}
	if _, err := exec.LookPath("makensis"); err != nil {
		t.Skip("makensis is not installed")
	}
	out := filepath.Join(t.TempDir(), "setup.exe")
	src := filepath.Join(t.TempDir(), "gobunshin.exe")
	if err := os.WriteFile(src, []byte("MZ fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	script = NSISScript("gobunshin", "1.2.3", "amd64", src, out)
	if err := NSIS(script, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("MZ")) {
		t.Fatal("installer is not a Windows executable")
	}
}
