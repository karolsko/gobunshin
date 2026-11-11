// Package tmpl renders release templates.
package tmpl

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/karolsko/gobunshin/internal/semver"
)

// Data is the set of fields a template can read.
type Data struct {
	Version     string
	Tag         string
	Commit      string
	ShortCommit string
	Date        string
	Timestamp   int64
	Now         int64
	ProjectName string
	Os          string
	Arch        string
	Arm         string
	Ext         string
	Binary      string
	PackageName string
	Release     string
	Major       int
	Minor       int
	Patch       int
	Prerelease  string
	IsSnapshot  bool
	IsNightly   bool
	Changelog   string
	Env         map[string]string
	Var         map[string]string
	Artifacts   []ArtifactRef
}

// ArtifactRef is one produced file, as seen from a template.
type ArtifactRef struct {
	Name   string
	Path   string
	Type   string
	Os     string
	Arch   string
	Sha256 string
	Binary string
}

// Render executes src against data. Text without actions is returned as-is.
func Render(src string, data Data) (string, error) {
	if !strings.Contains(src, "{{") {
		return src, nil
	}
	t, err := parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", src, err)
	}
	return buf.String(), nil
}

// Parse checks that src is a valid template.
func Parse(src string) error {
	if !strings.Contains(src, "{{") {
		return nil
	}
	_, err := parse(src)
	return err
}

func parse(src string) (*template.Template, error) {
	t, err := template.New("t").Option("missingkey=error").Funcs(funcs()).Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", src, err)
	}
	return t, nil
}

// Truthy reports whether a rendered `if` condition keeps the artifact.
// Empty, "false", "no", and "0" skip it.
func Truthy(s string) bool {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "false", "no", "0":
		return false
	default:
		return true
	}
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"tolower":   strings.ToLower,
		"toupper":   strings.ToUpper,
		"trim":      strings.TrimSpace,
		"replace":   strings.ReplaceAll,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"env":       os.Getenv,
		"incpatch": func(v string) (string, error) {
			parsed, err := semver.Parse(v)
			if err != nil {
				return "", err
			}
			return semver.IncPatch(parsed).String(), nil
		},
		"time": func(layout string, unix int64) string {
			return time.Unix(unix, 0).UTC().Format(layout)
		},
	}
}
