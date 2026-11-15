package installer

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karolsko/gobunshin/internal/icon"
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
	}}, nil, when)
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

func TestDmgVolumeIcon(t *testing.T) {
	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.NRGBA{R: 20, G: 120, B: 200, A: 255})
		}
	}
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	icns, err := icon.ICNS("icon.png", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	payload := []byte("#!/bin/sh\necho hi\n")
	dmg, err := Dmg("gobunshin", []File{{
		Path: "gobunshin",
		Data: payload,
	}}, icns, when)
	if err != nil {
		t.Fatal(err)
	}
	raw := inflateDMG(dmg)
	if !bytes.Contains(raw, utf16BE(".VolumeIcon.icns")) {
		t.Fatal("volume icon name missing")
	}
	if !bytes.Contains(raw, icns) {
		t.Fatal("icns payload missing")
	}
	if bytes.Count(raw, []byte("icns")) < 3 {
		t.Fatal("file resource fork icon missing")
	}
	rec, err := fileRecord(raw, "gobunshin", len(payload))
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(raw[rec+56:rec+58])&kHasCustomIcon == 0 {
		t.Fatal("file is missing the custom icon flag")
	}
	if binary.BigEndian.Uint32(raw[rec+168+12:rec+168+16]) == 0 {
		t.Fatal("file resource fork has no blocks")
	}
	path := filepath.Join(t.TempDir(), "gobunshin.dmg")
	if err := os.WriteFile(path, dmg, 0o644); err != nil {
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

func utf16BE(s string) []byte {
	buf := make([]byte, 0, len(s)*2)
	for _, r := range s {
		var pair [2]byte
		binary.BigEndian.PutUint16(pair[:], uint16(r))
		buf = append(buf, pair[:]...)
	}
	return buf
}

func inflateDMG(dmg []byte) []byte {
	var out bytes.Buffer
	for i := 0; i+2 < len(dmg); i++ {
		if dmg[i] != 0x78 {
			continue
		}
		switch dmg[i+1] {
		case 0x01, 0x5e, 0x9c, 0xda:
		default:
			continue
		}
		r, err := zlib.NewReader(bytes.NewReader(dmg[i:]))
		if err != nil {
			continue
		}
		n, copyErr := io.Copy(&out, r)
		r.Close()
		if n == 0 || copyErr != nil {
			continue
		}
	}
	return out.Bytes()
}

func TestNSISScript(t *testing.T) {
	script := NSISScript("gobunshin", "1.2.3", "amd64", "/tmp/gobunshin.exe", "/tmp/out.exe", "")
	if strings.Contains(script, "Icon ") {
		t.Fatalf("unexpected icon line:\n%s", script)
	}
	withIcon := NSISScript("gobunshin", "1.2.3", "amd64", "/tmp/gobunshin.exe", "/tmp/out.exe", "/tmp/app.ico")
	for _, want := range []string{`Icon "/tmp/app.ico"`, `UninstallIcon "/tmp/app.ico"`} {
		if !strings.Contains(withIcon, want) {
			t.Fatalf("script missing %q\n%s", want, withIcon)
		}
	}
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
	script = NSISScript("gobunshin", "1.2.3", "amd64", src, out, "")
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
