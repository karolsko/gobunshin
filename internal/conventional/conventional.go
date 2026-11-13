// Package conventional reads Conventional Commits and picks the next release.
package conventional

import (
	"regexp"
	"strings"

	"github.com/karolsko/gobunshin/internal/gitx"
	"github.com/karolsko/gobunshin/internal/semver"
)

// Level is how far the version should move.
type Level int

const (
	None Level = iota
	Patch
	Minor
	Major
)

// String returns major, minor, patch, or none.
func (l Level) String() string {
	switch l {
	case Major:
		return "major"
	case Minor:
		return "minor"
	case Patch:
		return "patch"
	default:
		return "none"
	}
}

// Commit is a parsed conventional commit header.
type Commit struct {
	Type        string
	Scope       string
	Description string
	Breaking    bool
}

var (
	headerRE   = regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^)]+)\))?(!)?:[[:space:]]+(.+)$`)
	breakingRE = regexp.MustCompile(`(?m)^BREAKING[[:space:]]+CHANGE:`)
)

// Parse reads a conventional commit. ok is false when the subject is not conventional.
func Parse(subject, body string) (Commit, bool) {
	m := headerRE.FindStringSubmatch(strings.TrimSpace(subject))
	if m == nil {
		return Commit{}, false
	}
	c := Commit{
		Type:        strings.ToLower(m[1]),
		Scope:       m[2],
		Description: strings.TrimSpace(m[4]),
		Breaking:    m[3] == "!" || breakingRE.MatchString(body),
	}
	return c, true
}

// Next applies the highest bump in commits to current.
// The boolean is false when no commit requests a release.
func Next(current semver.Version, commits []gitx.Commit) (semver.Version, Level, bool) {
	level := None
	for _, commit := range commits {
		parsed, ok := Parse(commit.Subject, commit.Body)
		if !ok {
			continue
		}
		if next := bump(parsed); next > level {
			level = next
		}
	}
	if level == None {
		return current, None, false
	}
	return apply(current, level), level, true
}

func bump(c Commit) Level {
	if c.Breaking {
		return Major
	}
	switch c.Type {
	case "feat":
		return Minor
	case "fix", "perf", "revert":
		return Patch
	default:
		return None
	}
}

func apply(v semver.Version, level Level) semver.Version {
	switch level {
	case Major:
		return semver.Version{Major: v.Major + 1}
	case Minor:
		return semver.Version{Major: v.Major, Minor: v.Minor + 1}
	case Patch:
		return semver.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
	default:
		return v
	}
}
