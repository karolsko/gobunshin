package semver

import (
	"regexp"
	"testing"
)

func TestParseAndCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.2", "1.2.0", 0},
		{"1", "1.0.0", 0},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-beta.2", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0+build.1", "1.0.0+build.2", 0},
	}
	for _, tc := range cases {
		av, err := Parse(tc.a)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.a, err)
		}
		bv, err := Parse(tc.b)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.b, err)
		}
		if got := Compare(av, bv); got != tc.want {
			t.Fatalf("Compare(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, s := range []string{"", "01.2.3", "1.2.3-01", "1.2.3-rc..1", "nope", "1.2.3.4"} {
		if _, err := Parse(s); err == nil {
			t.Fatalf("Parse(%q) succeeded", s)
		}
	}
}

func TestIncPatch(t *testing.T) {
	v, err := Parse("1.2.3-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	if got := IncPatch(v).String(); got != "1.2.4" {
		t.Fatalf("got %s", got)
	}
}

func TestMatchAndPrevious(t *testing.T) {
	ignore := []*regexp.Regexp{regexp.MustCompile(`^nightly$`)}
	tags := MatchTags([]string{
		"v0.9.0",
		"v1.0.0-rc.1",
		"v1.0.0",
		"v1.1.0-rc.1",
		"nightly",
		"not-a-version",
		"api/v2.0.0",
	}, "", ignore)
	if len(tags) != 4 {
		t.Fatalf("matched %d tags: %+v", len(tags), tags)
	}
	cur, err := Parse("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	prev, ok := Previous(tags, cur, true)
	if !ok || prev.Name != "v0.9.0" {
		t.Fatalf("previous stable = %+v ok=%v", prev, ok)
	}
	prev, ok = Previous(tags, cur, false)
	if !ok || prev.Name != "v1.0.0-rc.1" {
		t.Fatalf("previous any = %+v", prev)
	}
	high, ok := Highest(tags, true)
	if !ok || high.Name != "v1.0.0" {
		t.Fatalf("highest stable = %+v", high)
	}

	prefixed := MatchTags([]string{"api/v1.2.3", "api/v1.2.4-rc.1", "other/v9.0.0"}, "api/", nil)
	if len(prefixed) != 2 {
		t.Fatalf("prefixed: %+v", prefixed)
	}
	if prefixed[0].Version.String() != "1.2.3" && prefixed[1].Version.String() != "1.2.3" {
		t.Fatalf("strip prefix: %+v", prefixed)
	}
}
