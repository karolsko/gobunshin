package semver

import (
	"regexp"
	"strings"
)

// Tag is a git tag that parsed as a semantic version.
type Tag struct {
	Name    string
	Version Version
}

// MatchTags keeps tags with the given prefix, drops ignored tags, and parses the rest.
// prefix is removed before parsing. A leading v on the remainder is accepted by Parse.
func MatchTags(tags []string, prefix string, ignore []*regexp.Regexp) []Tag {
	var out []Tag
	for _, tag := range tags {
		if prefix != "" && !strings.HasPrefix(tag, prefix) {
			continue
		}
		skip := false
		for _, re := range ignore {
			if re.MatchString(tag) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		v, err := Parse(strings.TrimPrefix(tag, prefix))
		if err != nil {
			continue
		}
		out = append(out, Tag{Name: tag, Version: v})
	}
	return out
}

// Highest returns the greatest tag. When stableOnly is set, prereleases are skipped.
func Highest(tags []Tag, stableOnly bool) (Tag, bool) {
	var best Tag
	found := false
	for _, t := range tags {
		if stableOnly && t.Version.IsPre() {
			continue
		}
		if !found || Compare(t.Version, best.Version) > 0 {
			best = t
			found = true
		}
	}
	return best, found
}

// Previous returns the greatest tag strictly older than current.
func Previous(tags []Tag, current Version, stableOnly bool) (Tag, bool) {
	var best Tag
	found := false
	for _, t := range tags {
		if Compare(t.Version, current) >= 0 {
			continue
		}
		if stableOnly && t.Version.IsPre() {
			continue
		}
		if !found || Compare(t.Version, best.Version) > 0 {
			best = t
			found = true
		}
	}
	return best, found
}
