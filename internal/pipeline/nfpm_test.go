package pipeline

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/logx"
	"github.com/karolsko/gobunshin/internal/runctx"
)

func TestPackagesDebRpmApk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "postinstall.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(dir, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	project := config.Project{
		ProjectName: "demo",
		NFPMs: []config.NFPM{{
			PackageName: "demo",
			Maintainer:  "Dev <dev@example.com>",
			Description: "Demo {{ .Version }}",
			Homepage:    "https://example.com",
			License:     "MIT",
			Depends:     []string{"libc6"},
			Contents: []config.NFPMFile{{
				Src: "LICENSE",
				Dst: "/usr/share/doc/{{ .PackageName }}/LICENSE",
			}},
			Scripts: config.NFPMScripts{PostInstall: "postinstall.sh"},
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
		Name: "demo", Path: filepath.Join(dir, "demo"), Type: artifact.TypeBinary,
		Goos: "linux", Goarch: "amd64", Binary: "demo", BuildID: "demo",
	})
	rc.AddBinary(&artifact.Artifact{
		Name: "demo", Path: filepath.Join(dir, "demo"), Type: artifact.TypeBinary,
		Goos: "darwin", Goarch: "arm64", Binary: "demo", BuildID: "demo",
	})
	if err := Packages(rc); err != nil {
		t.Fatal(err)
	}
	arts := rc.ArtifactsCopy()
	if len(arts) != 3 {
		t.Fatalf("got %d packages", len(arts))
	}
	byFormat := map[string]string{}
	for _, a := range arts {
		if a.Type != artifact.TypePackage || a.Goarch != "amd64" {
			t.Fatalf("artifact %+v", a)
		}
		byFormat[a.Format] = a.Path
	}
	deb := mustRead(t, byFormat["deb"])
	if !bytes.HasPrefix(deb, []byte("!<arch>\n")) {
		t.Fatal("deb is not an ar archive")
	}
	control := debMember(t, deb, "control.tar.gz")
	controlText := tarFile(t, control, "control")
	for _, want := range []string{"Package: demo", "Version: 1.2.3", "Architecture: amd64", "Depends: libc6", "Maintainer: Dev <dev@example.com>", "Description: Demo 1.2.3"} {
		if !strings.Contains(controlText, want) {
			t.Fatalf("control missing %q:\n%s", want, controlText)
		}
	}
	data := debMember(t, deb, "data.tar.gz")
	if !tarHas(t, data, "usr/bin/demo") || !tarHas(t, data, "usr/share/doc/demo/LICENSE") {
		t.Fatal("deb payload is missing files")
	}

	rpm := mustRead(t, byFormat["rpm"])
	if !bytes.HasPrefix(rpm, []byte{0xed, 0xab, 0xee, 0xdb}) {
		t.Fatal("rpm magic missing")
	}
	for _, want := range [][]byte{[]byte("demo"), []byte("/usr/bin/"), []byte("x86_64"), []byte("/usr/share/doc/demo/")} {
		if !bytes.Contains(rpm, want) {
			t.Fatalf("rpm is missing %q", want)
		}
	}

	apkBody := mustRead(t, byFormat["apk"])
	if !bytes.HasPrefix(apkBody, []byte{0x1f, 0x8b}) {
		t.Fatal("apk is not gzip")
	}
	pkginfo := tarFile(t, apkBody, ".PKGINFO")
	if !strings.Contains(pkginfo, "pkgname = demo") || !strings.Contains(pkginfo, "arch = x86_64") {
		t.Fatalf("pkginfo:\n%s", pkginfo)
	}
	if !tarHas(t, apkBody, "usr/bin/demo") {
		t.Fatal("apk is missing the binary")
	}
}

func TestPackagesSkipWithoutLinux(t *testing.T) {
	dir := t.TempDir()
	rc := &runctx.Context{
		Go:   context.Background(),
		Dir:  dir,
		Dist: dir,
		Project: config.Project{NFPMs: []config.NFPM{{
			ID: "demo", PackageName: "demo", Maintainer: "Dev <dev@example.com>", Formats: []string{"deb"},
		}}},
		Log: logx.New(io.Discard, false),
	}
	rc.AddBinary(&artifact.Artifact{Goos: "darwin", Goarch: "arm64", Binary: "demo", BuildID: "demo", Path: dir})
	if err := Packages(rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.ArtifactsCopy()) != 0 {
		t.Fatal("darwin binary was packaged")
	}
}

func TestPackageArch(t *testing.T) {
	arch, err := packageArch("deb", "arm", "7")
	if err != nil || arch != "armhf" {
		t.Fatalf("deb arm: %s %v", arch, err)
	}
	arch, err = packageArch("rpm", "arm64", "")
	if err != nil || arch != "aarch64" {
		t.Fatalf("rpm arm64: %s %v", arch, err)
	}
	arch, err = packageArch("apk", "386", "")
	if err != nil || arch != "x86" {
		t.Fatalf("apk 386: %s %v", arch, err)
	}
	arch, err = packageArch("apk", "mips64le", "")
	if err != nil || arch != "mips64" {
		t.Fatalf("apk mips64le: %s %v", arch, err)
	}
	arch, err = packageArch("rpm", "mips64le", "")
	if err != nil || arch != "mips64el" {
		t.Fatalf("rpm mips64le: %s %v", arch, err)
	}
	if _, err := packageArch("deb", "wasm", ""); err == nil {
		t.Fatal("expected unsupported arch")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func debMember(t *testing.T, ar []byte, name string) []byte {
	t.Helper()
	rest := ar[8:]
	for len(rest) >= 60 {
		hdr := rest[:60]
		rest = rest[60:]
		member := strings.TrimRight(strings.TrimSpace(string(hdr[0:16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(hdr[48:58])))
		if err != nil {
			t.Fatal(err)
		}
		if size > len(rest) {
			t.Fatalf("ar member %s overruns the file", member)
		}
		data := rest[:size]
		rest = rest[size:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		if member == name {
			return data
		}
	}
	t.Fatalf("deb has no %s", name)
	return nil
}

func tarFile(t *testing.T, gzipped []byte, name string) string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(gzipped))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name == name || strings.HasSuffix(hdr.Name, "/"+name) {
			body, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			return string(body)
		}
	}
	t.Fatalf("tar has no %s", name)
	return ""
}

func tarHas(t *testing.T, gzipped []byte, name string) bool {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(gzipped))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return false
		}
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimPrefix(hdr.Name, "./")
		if got == name || strings.HasSuffix(got, "/"+name) {
			return true
		}
	}
}
