// Package release runs the gobunshin commands.
package release

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/gitx"
	"github.com/karolsko/gobunshin/internal/logx"
	"github.com/karolsko/gobunshin/internal/pipeline"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/semver"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Options controls one command.
type Options struct {
	Dir          string
	Config       string
	Snapshot     bool
	Nightly      bool
	SingleTarget bool
	IDs          []string
	SkipPublish  bool
	SkipHooks    bool
	SkipClean    bool
	Timeout      time.Duration
	Verbose      bool
	Parallel     int
	Stdout       io.Writer
	Stderr       io.Writer
	Token        string
	APIBase      string
}

// Execute runs check, build, release, publish, or changelog.
func Execute(parent context.Context, opt Options, cmd string) error {
	if opt.Timeout <= 0 {
		opt.Timeout = 30 * time.Minute
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, opt.Timeout)
	defer cancel()
	started := time.Now()
	rc, err := prepare(ctx, opt)
	if err != nil {
		return err
	}
	if cmd == "check" {
		return check(rc)
	}
	if cmd == "publish" {
		fillRemote(rc)
		dist, err := safeDist(rc.Dir, rc.Project.Dist)
		if err != nil {
			return err
		}
		rc.Dist = dist
		if err := pipeline.LoadManifest(rc); err != nil {
			return err
		}
		if err := renderVariables(rc); err != nil {
			return err
		}
		rc.DoPublish = true
		if err := pipeline.Publish(rc); err != nil {
			return err
		}
		if err := pipeline.Homebrew(rc); err != nil {
			return err
		}
		rc.Log.Step("publish finished in %s", time.Since(started).Round(time.Millisecond))
		return nil
	}
	if err := resolveVersion(rc, opt, cmd); err != nil {
		return err
	}
	if err := renderVariables(rc); err != nil {
		return err
	}
	if cmd == "changelog" {
		if err := pipeline.Changelog(rc); err != nil {
			return err
		}
		if opt.Stdout == nil {
			opt.Stdout = os.Stdout
		}
		_, err := io.WriteString(opt.Stdout, rc.Changelog)
		return err
	}

	dist, err := safeDist(rc.Dir, rc.Project.Dist)
	if err != nil {
		return err
	}
	rc.Dist = dist
	if !opt.SkipClean {
		if err := os.RemoveAll(dist); err != nil {
			return fmt.Errorf("clean %s: %w", dist, err)
		}
	}
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return err
	}
	if err := pipeline.Hooks(rc, rc.Project.Before.Hooks, "running before hooks"); err != nil {
		return err
	}
	if err := pipeline.Build(rc); err != nil {
		return err
	}
	if cmd == "build" {
		rc.Log.Step("build finished in %s", time.Since(started).Round(time.Millisecond))
		return nil
	}
	if err := pipeline.Archive(rc); err != nil {
		return err
	}
	if err := pipeline.Packages(rc); err != nil {
		return err
	}
	if err := pipeline.Installers(rc); err != nil {
		return err
	}
	if err := pipeline.TemplateFiles(rc); err != nil {
		return err
	}
	if err := pipeline.Changelog(rc); err != nil {
		return err
	}
	if err := pipeline.Checksum(rc); err != nil {
		return err
	}
	if err := pipeline.Sign(rc); err != nil {
		return err
	}
	if err := pipeline.WriteManifest(rc); err != nil {
		return err
	}
	switch {
	case rc.Snapshot:
		rc.Log.Step("skipping publish for snapshot")
	case opt.SkipPublish || rc.Project.Release.Disable:
		rc.Log.Step("skipping publish")
	default:
		rc.DoPublish = true
		if err := pipeline.Publish(rc); err != nil {
			return err
		}
	}
	if err := pipeline.Homebrew(rc); err != nil {
		return err
	}
	if err := pipeline.Hooks(rc, rc.Project.After.Hooks, "running after hooks"); err != nil {
		return err
	}
	rc.Log.Step("release finished in %s", time.Since(started).Round(time.Millisecond))
	return nil
}

func prepare(ctx context.Context, opt Options) (*runctx.Context, error) {
	if opt.Dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		opt.Dir = wd
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}
	log := logx.New(opt.Stderr, opt.Verbose)
	path, err := config.Find(opt.Dir, opt.Config)
	if err != nil {
		return nil, err
	}
	shown := path
	if rel, err := filepath.Rel(opt.Dir, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		shown = rel
	}
	log.Step("loading %s", shown)
	project, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	config.ApplyDefaults(project, opt.Dir)
	if err := config.Validate(project); err != nil {
		return nil, err
	}
	token := opt.Token
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	parallel := opt.Parallel
	if parallel < 1 {
		parallel = runtime.NumCPU()
	}
	return &runctx.Context{
		Go:           ctx,
		Dir:          opt.Dir,
		Project:      *project,
		RawVars:      runctx.CloneVars(project.Variables),
		Log:          log,
		Parallel:     parallel,
		IDs:          append([]string(nil), opt.IDs...),
		SingleTarget: opt.SingleTarget,
		SkipHooks:    opt.SkipHooks,
		Now:          time.Now().UTC(),
		Env:          runctx.EnvMap(),
		Token:        token,
		Owner:        project.Release.GitHub.Owner,
		Name:         project.Release.GitHub.Name,
		Host:         project.Release.GitHub.Host,
		APIBase:      opt.APIBase,
	}, nil
}

func check(rc *runctx.Context) error {
	n := 0
	for _, b := range rc.Project.Builds {
		targets, err := pipeline.Targets(b, false, "", "")
		if err != nil {
			return err
		}
		n += len(targets)
	}
	rc.Log.Step("config is valid")
	rc.Log.Sub("project %s", rc.Project.ProjectName)
	rc.Log.Sub("%d builds, %d targets, %d archives, %d system packages, %d installers", len(rc.Project.Builds), n, len(rc.Project.Archives), len(rc.Project.NFPMs), len(rc.Project.Installers))
	return nil
}

func resolveVersion(rc *runctx.Context, opt Options, cmd string) error {
	if opt.Snapshot && opt.Nightly {
		return fmt.Errorf("pass only one of --snapshot and --nightly")
	}
	if !gitx.IsRepo(rc.Dir) {
		if cmd != "build" {
			return fmt.Errorf("%s is not a git repository", rc.Dir)
		}
		rc.Commit = "none"
		rc.ShortCommit = "none"
		rc.CommitTime = rc.Now
		rc.Version = "0.0.0"
		if err := renderVariables(rc); err != nil {
			return err
		}
		name, err := rc.Render(rc.Project.Snapshot.VersionTemplate)
		if err != nil {
			return err
		}
		rc.Version = name
		rc.Tag = name
		rc.Snapshot = true
		rc.Log.Step("snapshot %s", name)
		return nil
	}
	commit, err := gitx.Head(rc.Dir)
	if err != nil {
		return err
	}
	rc.Commit = commit
	rc.ShortCommit = short(commit)
	if when, err := gitx.CommitTime(rc.Dir); err == nil {
		rc.CommitTime = when
	} else {
		rc.CommitTime = rc.Now
	}
	fillRemote(rc)

	ignore, err := compileIgnore(rc.Project.Git.IgnoreTags)
	if err != nil {
		return err
	}
	tags, err := gitx.Tags(rc.Dir)
	if err != nil {
		return err
	}
	matched := semver.MatchTags(tags, rc.Project.TagPrefix(), ignore)
	head, err := gitx.TagsAtHead(rc.Dir)
	if err != nil {
		return err
	}
	headMatched := semver.MatchTags(head, rc.Project.TagPrefix(), ignore)
	var current semver.Tag
	has := false
	if len(headMatched) > 0 {
		current, has = semver.Highest(headMatched, false)
	} else if env := gitx.EnvTag(); env != "" {
		one := semver.MatchTags([]string{env}, rc.Project.TagPrefix(), ignore)
		if len(one) == 1 {
			current = one[0]
			has = true
			rc.Log.Sub("using tag %s from the CI environment", current.Name)
		}
	}
	base := "0.0.0"
	if high, ok := semver.Highest(matched, false); ok {
		base = high.Version.String()
	}

	switch {
	case opt.Nightly:
		if prev, ok := semver.Highest(matched, true); ok {
			rc.PreviousTag = prev.Name
		} else if prev, ok := semver.Highest(matched, false); ok {
			rc.PreviousTag = prev.Name
		}
		if err := applyTemplateVersion(rc, base, rc.Project.Nightly.VersionTemplate); err != nil {
			return err
		}
		rc.Tag = rc.Project.Nightly.TagName
		rc.Nightly = true
		rc.Log.Step("nightly %s (%s)", rc.Tag, rc.Version)
	case opt.Snapshot || (cmd == "build" && !has):
		if prev, ok := semver.Highest(matched, false); ok {
			rc.PreviousTag = prev.Name
		}
		if err := applyTemplateVersion(rc, base, rc.Project.Snapshot.VersionTemplate); err != nil {
			return err
		}
		rc.Tag = rc.Version
		rc.Snapshot = true
		rc.Log.Step("snapshot %s", rc.Version)
	case has:
		rc.Tag = current.Name
		rc.Version = current.Version.String()
		if prev, ok := semver.Previous(matched, current.Version, !current.Version.IsPre()); ok {
			rc.PreviousTag = prev.Name
		}
		rc.Log.Step("tag %s", rc.Tag)
	case cmd == "changelog":
		if prev, ok := semver.Highest(matched, false); ok {
			rc.PreviousTag = prev.Name
		}
		rc.Version = base
		rc.Log.Step("unreleased changes")
	default:
		return fmt.Errorf("current commit is not tagged; create a tag or pass --snapshot")
	}
	if rc.PreviousTag != "" {
		rc.Log.Sub("previous %s", rc.PreviousTag)
	}
	rc.Log.Sub("commit %s", rc.ShortCommit)
	return nil
}

func applyTemplateVersion(rc *runctx.Context, base, template string) error {
	rc.Version = base
	if err := renderVariables(rc); err != nil {
		return err
	}
	name, err := rc.Render(template)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("version template rendered empty")
	}
	rc.Version = name
	return nil
}

func renderVariables(rc *runctx.Context) error {
	if len(rc.RawVars) == 0 {
		rc.Project.Variables = map[string]string{}
		return nil
	}
	keys := make([]string, 0, len(rc.RawVars))
	for k := range rc.RawVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rendered := map[string]string{}
	for pass := 0; pass < 2; pass++ {
		data := rc.Data()
		if pass == 0 {
			data.Var = rc.RawVars
		} else {
			data.Var = rendered
		}
		for _, k := range keys {
			value, err := tmpl.Render(rc.RawVars[k], data)
			if err != nil {
				return fmt.Errorf("variable %s: %w", k, err)
			}
			rendered[k] = value
		}
	}
	rc.Project.Variables = rendered
	return nil
}

func fillRemote(rc *runctx.Context) {
	if rc.Owner != "" && rc.Name != "" {
		return
	}
	raw, err := gitx.RemoteURL(rc.Dir)
	if err != nil {
		return
	}
	host, owner, name, err := gitx.ParseRemote(raw)
	if err != nil {
		return
	}
	if rc.Owner == "" {
		rc.Owner = owner
	}
	if rc.Name == "" {
		rc.Name = name
	}
	if rc.Host == "" && host != "" && host != "github.com" {
		rc.Host = host
	}
}

func compileIgnore(patterns []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("git.ignore_tags %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func safeDist(root, dist string) (string, error) {
	if dist == "" {
		dist = "dist"
	}
	if !filepath.IsAbs(dist) {
		dist = filepath.Join(root, dist)
	}
	rel, err := filepath.Rel(root, dist)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("dist directory must stay inside the project")
	}
	return dist, nil
}
