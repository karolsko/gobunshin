// Package semver parses and orders a practical subset of Semantic Versioning.
package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed semantic version. Build metadata is ignored.
type Version struct {
	Major, Minor, Patch int
	Pre                 []Ident
	Original            string
}

// Ident is one prerelease identifier.
type Ident struct {
	Num   int
	Str   string
	IsNum bool
}

// Parse accepts 1, 1.2, 1.2.3, an optional leading v, prerelease, and build metadata.
func Parse(raw string) (Version, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	original := s
	if s[0] == 'v' || s[0] == 'V' {
		s = s[1:]
	}
	main, _, _ := strings.Cut(s, "+")
	var pre string
	if i := strings.IndexByte(main, '-'); i >= 0 {
		pre = main[i+1:]
		main = main[:i]
	}
	parts := strings.Split(main, ".")
	if len(parts) == 0 || len(parts) > 3 || main == "" {
		return Version{}, fmt.Errorf("invalid version %q", raw)
	}
	nums := []int{0, 0, 0}
	for i, p := range parts {
		n, err := parseNum(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q", raw)
		}
		nums[i] = n
	}
	var idents []Ident
	if pre != "" {
		for _, p := range strings.Split(pre, ".") {
			id, err := parseIdent(p)
			if err != nil {
				return Version{}, fmt.Errorf("invalid version %q", raw)
			}
			idents = append(idents, id)
		}
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: idents, Original: original}, nil
}

func parseNum(p string) (int, error) {
	if p == "" || (len(p) > 1 && p[0] == '0') {
		return 0, fmt.Errorf("bad number")
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 0 {
		return 0, err
	}
	return n, nil
}

func parseIdent(p string) (Ident, error) {
	if p == "" {
		return Ident{}, fmt.Errorf("empty ident")
	}
	for _, r := range p {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
			return Ident{}, fmt.Errorf("bad ident")
		}
	}
	id := Ident{Str: p}
	if n, err := strconv.Atoi(p); err == nil {
		if len(p) > 1 && p[0] == '0' {
			return Ident{}, fmt.Errorf("leading zero")
		}
		id.IsNum = true
		id.Num = n
	}
	return id, nil
}

// String renders the version without build metadata.
func (v Version) String() string {
	b := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) == 0 {
		return b
	}
	parts := make([]string, len(v.Pre))
	for i, id := range v.Pre {
		if id.IsNum {
			parts[i] = strconv.Itoa(id.Num)
		} else {
			parts[i] = id.Str
		}
	}
	return b + "-" + strings.Join(parts, ".")
}

// Prerelease is the prerelease segment without the leading dash.
func (v Version) Prerelease() string {
	s := v.String()
	_, pre, ok := strings.Cut(s, "-")
	if !ok {
		return ""
	}
	return pre
}

// IsPre reports whether the version has a prerelease segment.
func (v Version) IsPre() bool { return len(v.Pre) > 0 }

// Compare returns -1, 0, or 1 following SemVer precedence.
func Compare(a, b Version) int {
	if c := cmpInt(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpInt(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpInt(a.Patch, b.Patch); c != 0 {
		return c
	}
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}
	n := len(a.Pre)
	if len(b.Pre) < n {
		n = len(b.Pre)
	}
	for i := 0; i < n; i++ {
		if c := compareIdent(a.Pre[i], b.Pre[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a.Pre), len(b.Pre))
}

func compareIdent(a, b Ident) int {
	switch {
	case a.IsNum && b.IsNum:
		return cmpInt(a.Num, b.Num)
	case a.IsNum:
		return -1
	case b.IsNum:
		return 1
	case a.Str < b.Str:
		return -1
	case a.Str > b.Str:
		return 1
	default:
		return 0
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// IncPatch returns the next patch release and drops any prerelease.
func IncPatch(v Version) Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
}
