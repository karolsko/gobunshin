package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/karolsko/gobunshin/internal/pipeline"
)

func TestUntaggedReleaseFails(t *testing.T) {
	t.Setenv("GITHUB_REF", "")
	t.Setenv("CI_COMMIT_TAG", "")
	dir := initRepo(t)
	writeProject(t, dir, false)
	err := Execute(context.Background(), Options{Dir: dir, Stderr: io.Discard, Stdout: io.Discard}, "release")
	if err == nil || !strings.Contains(err.Error(), "not tagged") {
		t.Fatalf("got %v", err)
	}
}

func TestSnapshotRelease(t *testing.T) {
	t.Setenv("GITHUB_REF", "")
	t.Setenv("CI_COMMIT_TAG", "")
	t.Setenv("GOTOOLCHAIN", "local")
	dir := initRepo(t)
	writeProject(t, dir, true)
	var log bytes.Buffer
	err := Execute(context.Background(), Options{
		Dir:          dir,
		Snapshot:     true,
		SingleTarget: true,
		Stderr:       &log,
		Stdout:       io.Discard,
	}, "release")
	if err != nil {
		t.Fatalf("release: %v\n%s", err, log.String())
	}
	if !strings.Contains(log.String(), "skipping publish for snapshot") {
		t.Fatalf("log:\n%s", log.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "hook.txt")); err != nil {
		t.Fatal("before hook did not run")
	}
	bin := filepath.Join(dir, "dist", "build", "demo_"+runtime.GOOS+"_"+runtime.GOARCH, "demo")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run binary: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SNAPSHOT") {
		t.Fatalf("version %q", out)
	}
	archives, err := filepath.Glob(filepath.Join(dir, "dist", "*.tar.gz"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives %+v err %v", archives, err)
	}
	if !archiveContains(t, archives[0], "README.md") {
		t.Fatal("archive missing README.md")
	}
	sums, err := os.ReadFile(filepath.Join(dir, "dist", "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sums), filepath.Base(archives[0])) {
		t.Fatalf("checksums:\n%s", sums)
	}
	if _, err := os.Stat(archives[0] + ".sig"); err != nil {
		t.Fatal("missing signature")
	}
	hello, err := os.ReadFile(filepath.Join(dir, "dist", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hello), "SNAPSHOT") {
		t.Fatalf("template %q", hello)
	}
	formula, err := os.ReadFile(filepath.Join(dir, "dist", "homebrew", "demo.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(formula), "sha256") || !strings.Contains(string(formula), "class Demo") {
		t.Fatalf("formula:\n%s", formula)
	}
	notes, err := os.ReadFile(filepath.Join(dir, "dist", "changelog.md"))
	if err != nil || !strings.Contains(string(notes), "feat: start") {
		t.Fatalf("changelog %s err %v", notes, err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "dist", "artifacts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded pipeline.Manifest
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Snapshot || decoded.Version == "" {
		t.Fatalf("manifest %+v", decoded)
	}
}

func TestTaggedReleaseSkipPublish(t *testing.T) {
	t.Setenv("GITHUB_REF", "")
	t.Setenv("CI_COMMIT_TAG", "")
	t.Setenv("GOTOOLCHAIN", "local")
	dir := initRepo(t)
	writeProject(t, dir, true)
	git(t, dir, "tag", "v1.4.0")
	err := Execute(context.Background(), Options{
		Dir:          dir,
		SingleTarget: true,
		SkipPublish:  true,
		SkipHooks:    true,
		Stderr:       io.Discard,
		Stdout:       io.Discard,
	}, "release")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "dist", "build", "demo_"+runtime.GOOS+"_"+runtime.GOARCH, "demo")
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "1.4.0" {
		t.Fatalf("version %q", out)
	}
}

func TestPublishPrebuilt(t *testing.T) {
	t.Setenv("GITHUB_REF", "")
	t.Setenv("CI_COMMIT_TAG", "")
	var uploads []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("token %s", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/releases/tags/"):
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releases"):
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":3,"tag_name":"v1.2.3","html_url":"https://github.com/acme/demo/releases/v1.2.3","upload_url":"`+serverUpload(r)+`"}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/assets"):
			body, _ := io.ReadAll(r.Body)
			uploads = append(uploads, r.URL.Query().Get("name")+":"+string(body))
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := initRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "seed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seed", "demo"), []byte("prebuilt"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := []byte(`
version: 1
project_name: demo
builds:
  - id: demo
    builder: prebuilt
    binary: demo
    prebuilt:
      path: seed/demo
    goos: [linux]
    goarch: [amd64]
archives:
  - formats: [tar.gz]
    wrap_in_directory: false
checksum:
  name_template: checksums.txt
changelog:
  disable: true
release:
  github:
    owner: acme
    name: demo
`)
	if err := os.WriteFile(filepath.Join(dir, ".gobunshin.yaml"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "feat: seed")
	git(t, dir, "tag", "v1.2.3")

	err := Execute(context.Background(), Options{
		Dir:     dir,
		Token:   "test-token",
		APIBase: srv.URL,
		Stderr:  io.Discard,
		Stdout:  io.Discard,
	}, "release")
	if err != nil {
		t.Fatal(err)
	}
	if len(uploads) < 2 {
		t.Fatalf("uploads %+v", uploads)
	}
	var sawArchive bool
	for _, item := range uploads {
		if strings.Contains(item, "demo_1.2.3_linux_amd64.tar.gz") {
			sawArchive = true
		}
	}
	if !sawArchive {
		t.Fatalf("uploads %+v", uploads)
	}
}

func serverUpload(r *http.Request) string {
	return "http://" + r.Host + "/repos/acme/demo/releases/3/assets{?name,label}"
}

func writeProject(t *testing.T, dir string, withHooks bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainSrc := "package main\n\nimport \"fmt\"\n\nvar version = \"dev\"\n\nfunc main() { fmt.Println(version) }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hello.tmpl"), []byte("version {{ .Version }}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := ""
	if withHooks {
		hook = "before:\n  hooks:\n    - echo hooked > hook.txt\n"
	}
	cfg := `version: 1
project_name: demo
` + hook + `builds:
  - id: demo
    main: .
    binary: demo
    env: ["CGO_ENABLED=0"]
    ldflags:
      - '-X main.version={{ .Version }}'
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
archives:
  - formats: [tar.gz]
    files: [README.md]
checksum:
  name_template: checksums.txt
template_files:
  - src: hello.tmpl
    dst: hello.txt
signs:
  - cmd: cp
    args: ["${artifact}", "${artifact}.sig"]
homebrews:
  - name: demo
    description: Demo app
    license: MIT
release:
  github:
    owner: acme
    name: demo
`
	if err := os.WriteFile(filepath.Join(dir, ".gobunshin.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "feat: start")
}

func archiveContains(t *testing.T, path, needle string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
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
		if strings.HasSuffix(hdr.Name, needle) {
			return true
		}
	}
}

func TestNextVersion(t *testing.T) {
	t.Setenv("GITHUB_REF", "")
	t.Setenv("CI_COMMIT_TAG", "")
	dir := initRepo(t)
	cfg := []byte("version: 1\nproject_name: demo\nversioning:\n  conventional_commits: true\n  tag_prefix: v\nbuilds:\n  - main: .\n    goos: [linux]\n    goarch: [amd64]\n")
	if err := os.WriteFile(filepath.Join(dir, ".gobunshin.yaml"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "chore: init")
	git(t, dir, "tag", "v0.1.0")
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "note.txt")
	git(t, dir, "commit", "-m", "feat: packages")

	var out bytes.Buffer
	err := Execute(context.Background(), Options{Dir: dir, Stdout: &out, Stderr: io.Discard}, "next")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "v0.2.0" {
		t.Fatalf("next = %q", out.String())
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "dev@example.com")
	git(t, dir, "config", "user.name", "Dev")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "tag.gpgsign", "false")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", args, err, out)
	}
}
