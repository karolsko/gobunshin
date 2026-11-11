// Package runctx carries one release run.
package runctx

import (
	"context"
	"maps"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/logx"
	"github.com/karolsko/gobunshin/internal/semver"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Context is the state shared by every stage of a run.
type Context struct {
	Go           context.Context
	Dir          string
	Dist         string
	Project      config.Project
	RawVars      map[string]string
	Version      string
	Tag          string
	Commit       string
	ShortCommit  string
	CommitTime   time.Time
	Now          time.Time
	PreviousTag  string
	Changelog    string
	Snapshot     bool
	Nightly      bool
	Binaries     []*artifact.Artifact
	Artifacts    []*artifact.Artifact
	Log          *logx.Logger
	Parallel     int
	IDs          []string
	SingleTarget bool
	SkipHooks    bool
	DoPublish    bool
	Token        string
	Owner        string
	Name         string
	Host         string
	APIBase      string
	Env          map[string]string
	mu           sync.Mutex
}

// AddBinary records an unpackaged build output.
func (c *Context) AddBinary(a *artifact.Artifact) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Binaries = append(c.Binaries, a)
}

// Add records a file that can be published.
func (c *Context) Add(a *artifact.Artifact) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, have := range c.Artifacts {
		if have.Name == a.Name {
			return errDuplicate(a.Name)
		}
	}
	c.Artifacts = append(c.Artifacts, a)
	return nil
}

func errDuplicate(name string) error {
	return &duplicateName{name}
}

type duplicateName struct{ name string }

func (e *duplicateName) Error() string {
	return "duplicate artifact name " + e.name
}

// BinariesCopy returns the build outputs.
func (c *Context) BinariesCopy() []*artifact.Artifact {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*artifact.Artifact, len(c.Binaries))
	copy(out, c.Binaries)
	return out
}

// ArtifactsCopy returns the shippable files.
func (c *Context) ArtifactsCopy() []*artifact.Artifact {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*artifact.Artifact, len(c.Artifacts))
	copy(out, c.Artifacts)
	return out
}

// Data builds the template view for project-level text.
func (c *Context) Data() tmpl.Data {
	return c.view("", "", "", "")
}

// TargetData builds the template view for one compiled binary.
func (c *Context) TargetData(goos, goarch, goarm, binary string) tmpl.Data {
	return c.view(goos, goarch, goarm, binary)
}

func (c *Context) view(goos, goarch, goarm, binary string) tmpl.Data {
	env := c.Env
	if env == nil {
		env = map[string]string{}
	}
	vars := c.Project.Variables
	if vars == nil {
		vars = map[string]string{}
	}
	d := tmpl.Data{
		Version:     c.Version,
		Tag:         c.Tag,
		Commit:      c.Commit,
		ShortCommit: c.ShortCommit,
		Date:        c.CommitTime.UTC().Format("2006-01-02"),
		Timestamp:   c.CommitTime.UTC().Unix(),
		Now:         c.Now.UTC().Unix(),
		ProjectName: c.Project.ProjectName,
		Os:          goos,
		Arch:        goarch,
		Arm:         goarm,
		Binary:      binary,
		IsSnapshot:  c.Snapshot,
		IsNightly:   c.Nightly,
		Changelog:   c.Changelog,
		Env:         env,
		Var:         vars,
		Artifacts:   c.artifactRefs(),
	}
	if goos == "windows" {
		d.Ext = ".exe"
	}
	if v, err := semver.Parse(c.Version); err == nil {
		d.Major = v.Major
		d.Minor = v.Minor
		d.Patch = v.Patch
		d.Prerelease = v.Prerelease()
	}
	return d
}

func (c *Context) artifactRefs() []tmpl.ArtifactRef {
	arts := c.ArtifactsCopy()
	refs := make([]tmpl.ArtifactRef, 0, len(arts))
	for _, a := range arts {
		refs = append(refs, tmpl.ArtifactRef{
			Name:   a.Name,
			Path:   a.Path,
			Type:   string(a.Type),
			Os:     a.Goos,
			Arch:   a.Goarch,
			Sha256: a.SHA256,
			Binary: a.Binary,
		})
	}
	return refs
}

// Render executes a template against the project-level view.
func (c *Context) Render(src string) (string, error) {
	return tmpl.Render(src, c.Data())
}

// EnvMap reads the process environment.
func EnvMap() map[string]string {
	out := map[string]string{}
	for _, pair := range os.Environ() {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

// CloneVars copies the variable table so later renders can start from the raw text.
func CloneVars(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	return maps.Clone(in)
}
