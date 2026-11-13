package conventional

import (
	"testing"

	"github.com/karolsko/gobunshin/internal/gitx"
	"github.com/karolsko/gobunshin/internal/semver"
)

func TestParse(t *testing.T) {
	c, ok := Parse("feat(pkg)!: add installers", "")
	if !ok || c.Type != "feat" || c.Scope != "pkg" || !c.Breaking {
		t.Fatalf("%+v ok=%v", c, ok)
	}
	c, ok = Parse("fix: nil tag", "BREAKING CHANGE: drops the old flag")
	if !ok || !c.Breaking || c.Type != "fix" {
		t.Fatalf("%+v", c)
	}
	if _, ok := Parse("Merge pull request #2", ""); ok {
		t.Fatal("merge commits are not conventional")
	}
}

func TestNext(t *testing.T) {
	base, err := semver.Parse("0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		msgs  []string
		want  string
		level Level
		ok    bool
	}{
		{"feat", []string{"feat: add packages"}, "0.2.0", Minor, true},
		{"fix", []string{"fix: checksum order"}, "0.1.1", Patch, true},
		{"breaking", []string{"feat!: new config"}, "1.0.0", Major, true},
		{"highest", []string{"fix: a", "feat: b", "chore: c"}, "0.2.0", Minor, true},
		{"none", []string{"docs: readme", "ci: actions"}, "0.1.0", None, false},
	}
	for _, tc := range cases {
		var commits []gitx.Commit
		for _, msg := range tc.msgs {
			commits = append(commits, gitx.Commit{Subject: msg})
		}
		got, level, ok := Next(base, commits)
		if ok != tc.ok || level != tc.level || got.String() != tc.want {
			t.Fatalf("%s: got %s %s ok=%v", tc.name, got, level, ok)
		}
	}
}
