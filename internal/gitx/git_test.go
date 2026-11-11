package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseRemote(t *testing.T) {
	cases := []struct {
		raw, host, owner, name string
	}{
		{"git@github.com:karolsko/gobunshin.git", "github.com", "karolsko", "gobunshin"},
		{"https://github.com/karolsko/gobunshin.git", "github.com", "karolsko", "gobunshin"},
		{"https://github.com/karolsko/gobunshin", "github.com", "karolsko", "gobunshin"},
		{"ssh://git@github.com/karolsko/gobunshin.git", "github.com", "karolsko", "gobunshin"},
		{"git@gitlab.com:group/sub/repo.git", "gitlab.com", "group", "sub/repo"},
	}
	for _, tc := range cases {
		host, owner, name, err := ParseRemote(tc.raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if host != tc.host || owner != tc.owner || name != tc.name {
			t.Fatalf("%s -> %s %s %s", tc.raw, host, owner, name)
		}
	}
}

func TestLogAndTags(t *testing.T) {
	dir := initRepo(t)
	commit(t, dir, "feat: first", "a.txt")
	commit(t, dir, "fix: second", "b.txt")
	git(t, dir, "tag", "v0.1.0")
	commit(t, dir, "feat: third", "a.txt")
	git(t, dir, "tag", "v0.2.0")

	tags, err := Tags(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("tags %+v", tags)
	}
	at, err := TagsAtHead(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(at) != 1 || at[0] != "v0.2.0" {
		t.Fatalf("at head %+v", at)
	}
	commits, err := Log(dir, "v0.1.0..HEAD", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || commits[0].Subject != "feat: third" {
		t.Fatalf("log %+v", commits)
	}
	scoped, err := Log(dir, "", []string{"b.txt"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].Subject != "fix: second" {
		t.Fatalf("scoped %+v", scoped)
	}
	if _, err := CommitTime(dir); err != nil {
		t.Fatal(err)
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

func commit(t *testing.T, dir, msg, file string) {
	t.Helper()
	path := filepath.Join(dir, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(msg+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", file)
	git(t, dir, "commit", "-m", msg)
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
