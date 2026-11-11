package pipeline

import (
	"strings"
	"testing"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/gitx"
)

func TestTargetsIgnoreAndSingle(t *testing.T) {
	b := config.Build{
		ID:     "app",
		Goos:   []string{"linux", "darwin"},
		Goarch: []string{"amd64", "arm64"},
		Ignore: []config.Ignore{{Goos: "darwin", Goarch: "amd64"}},
	}
	got, err := Targets(b, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("targets %+v", got)
	}
	kept, err := Targets(b, true, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0].String() != "darwin/arm64" {
		t.Fatalf("single %+v", kept)
	}
	if _, err := Targets(b, true, "windows", "amd64"); err == nil {
		t.Fatal("expected missing host target")
	}

	arm := config.Build{ID: "arm", Goos: []string{"linux"}, Goarch: []string{"arm"}, Goarm: []string{"6", "7"}}
	arms, err := Targets(arm, false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(arms) != 2 || arms[0].Goarm != "6" {
		t.Fatalf("arms %+v", arms)
	}
}

func TestFormatChangelogGroups(t *testing.T) {
	title := "## Changelog"
	text, err := formatChangelog([]gitx.Commit{
		{Hash: "aaaaaaaa", Subject: "feat: add"},
		{Hash: "bbbbbbbb", Subject: "chore: noise"},
		{Hash: "cccccccc", Subject: "fix: bug"},
		{Hash: "dddddddd", Subject: "docs: readme"},
	}, config.Changelog{
		Sort:  "asc",
		Title: &title,
		Groups: []config.Group{
			{Title: "Features", Regexp: "^feat"},
			{Title: "Fixes", Regexp: "^fix"},
		},
		Filters: config.Filters{Exclude: []string{"^docs:"}},
		Divider: "---",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "### Features") || !strings.Contains(text, "feat: add (aaaaaaa)") {
		t.Fatalf("features missing:\n%s", text)
	}
	if !strings.Contains(text, "### Fixes") || !strings.Contains(text, "fix: bug") {
		t.Fatalf("fixes missing:\n%s", text)
	}
	if strings.Contains(text, "docs:") {
		t.Fatalf("docs not filtered:\n%s", text)
	}
	if !strings.Contains(text, "### Other") || !strings.Contains(text, "chore: noise") {
		t.Fatalf("other missing:\n%s", text)
	}
	feat := strings.Index(text, "feat: add")
	fix := strings.Index(text, "fix: bug")
	if feat > fix {
		t.Fatalf("expected oldest first:\n%s", text)
	}
	if !strings.Contains(text, "---") {
		t.Fatal("missing divider")
	}
}

func TestRenderFormula(t *testing.T) {
	body := renderFormula(formulaInput{
		Class:    "Demo",
		Desc:     "Demo app",
		Homepage: "https://example.com",
		License:  "MIT",
		Version:  "1.2.3",
		Install:  `bin.install Dir["*/demo"].first`,
		Test:     `system "#{bin}/demo", "version"`,
		Assets: []brewAsset{
			{Goos: "darwin", Goarch: "arm64", URL: "https://github.com/acme/demo/releases/download/v1.2.3/demo.tar.gz", SHA: "abc"},
			{Goos: "linux", Goarch: "amd64", URL: "https://github.com/acme/demo/releases/download/v1.2.3/demo_linux.tar.gz", SHA: "def"},
		},
	})
	for _, want := range []string{
		"class Demo < Formula",
		`desc "Demo app"`,
		"on_macos do",
		"on_arm do",
		`sha256 "abc"`,
		"on_linux do",
		"on_intel do",
		`sha256 "def"`,
		`bin.install Dir["*/demo"].first`,
		`system "#{bin}/demo", "version"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}
	if className("my-app") != "MyApp" || className("1tool") != "App1tool" {
		t.Fatalf("class names %s %s", className("my-app"), className("1tool"))
	}
}

func TestSignWant(t *testing.T) {
	if !signWant("archives", artifact.TypeArchive) || signWant("archives", artifact.TypeChecksum) {
		t.Fatal("archives selector")
	}
	if !signWant("all", artifact.TypeFile) || !signWant("archives", artifact.TypePackage) || !signWant("package", artifact.TypePackage) {
		t.Fatal("all selector")
	}
}
