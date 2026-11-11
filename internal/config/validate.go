package config

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Validate reports every problem found in a defaulted project.
func Validate(p *Project) error {
	var errs []error
	if p.Version != 0 && p.Version != 1 {
		errs = append(errs, fmt.Errorf("version must be 1"))
	}
	if p.ProjectName == "" || strings.ContainsAny(p.ProjectName, " /\\") {
		errs = append(errs, fmt.Errorf("project_name must be a single path segment"))
	}
	switch p.Git.TagSort {
	case "", "smartsemver", "semver":
	default:
		errs = append(errs, fmt.Errorf("git.tag_sort %q is not supported", p.Git.TagSort))
	}
	for _, pat := range p.Git.IgnoreTags {
		if _, err := regexp.Compile(pat); err != nil {
			errs = append(errs, fmt.Errorf("git.ignore_tags %q: %w", pat, err))
		}
	}
	if p.Changelog.Sort != "asc" && p.Changelog.Sort != "desc" {
		errs = append(errs, fmt.Errorf("changelog.sort must be asc or desc"))
	}
	for _, pat := range append(append([]string{}, p.Changelog.Filters.Exclude...), p.Changelog.Filters.Include...) {
		if _, err := regexp.Compile(pat); err != nil {
			errs = append(errs, fmt.Errorf("changelog filter %q: %w", pat, err))
		}
	}
	for _, g := range p.Changelog.Groups {
		if g.Title == "" || g.Regexp == "" {
			errs = append(errs, fmt.Errorf("changelog groups need a title and a regexp"))
			continue
		}
		if _, err := regexp.Compile(g.Regexp); err != nil {
			errs = append(errs, fmt.Errorf("changelog group %q: %w", g.Title, err))
		}
	}
	switch p.Checksum.Algorithm {
	case "sha256", "sha512":
	default:
		errs = append(errs, fmt.Errorf("checksum.algorithm must be sha256 or sha512"))
	}
	switch p.Release.Mode {
	case "fail", "replace":
	default:
		errs = append(errs, fmt.Errorf("release.mode must be fail or replace"))
	}
	switch p.Release.Prerelease {
	case "auto", "true", "false":
	default:
		errs = append(errs, fmt.Errorf("release.prerelease must be auto, true, or false"))
	}

	if len(p.Builds) == 0 {
		errs = append(errs, fmt.Errorf("at least one build is required"))
	}
	buildIDs := map[string]bool{}
	for _, b := range p.Builds {
		if b.ID == "" {
			errs = append(errs, fmt.Errorf("build id is required"))
			continue
		}
		if buildIDs[b.ID] {
			errs = append(errs, fmt.Errorf("duplicate build id %q", b.ID))
		}
		buildIDs[b.ID] = true
		switch b.Builder {
		case "go":
			if b.Main == "" {
				errs = append(errs, fmt.Errorf("build %s: main is required", b.ID))
			}
		case "prebuilt":
			if b.Prebuilt.Path == "" {
				errs = append(errs, fmt.Errorf("build %s: prebuilt.path is required", b.ID))
			}
		default:
			errs = append(errs, fmt.Errorf("build %s: builder must be go or prebuilt", b.ID))
		}
		if len(b.Goos) == 0 || len(b.Goarch) == 0 {
			errs = append(errs, fmt.Errorf("build %s: goos and goarch are required", b.ID))
		}
		errs = append(errs, parseField(b.If)...)
		errs = append(errs, parseField(b.Binary)...)
		errs = append(errs, parseField(b.Prebuilt.Path)...)
		for _, s := range append(append([]string{}, b.Ldflags...), b.Env...) {
			errs = append(errs, parseField(s)...)
		}
	}

	formats := map[string]bool{"tar.gz": true, "tgz": true, "zip": true, "binary": true}
	archiveIDs := map[string]bool{}
	for _, a := range p.Archives {
		if archiveIDs[a.ID] {
			errs = append(errs, fmt.Errorf("duplicate archive id %q", a.ID))
		}
		archiveIDs[a.ID] = true
		if len(a.Formats) == 0 {
			errs = append(errs, fmt.Errorf("archive %s: formats is required", a.ID))
		}
		for _, f := range a.Formats {
			if !formats[f] {
				errs = append(errs, fmt.Errorf("archive %s: unknown format %q", a.ID, f))
			}
		}
		for _, o := range a.FormatOverrides {
			for _, f := range o.Formats {
				if !formats[f] {
					errs = append(errs, fmt.Errorf("archive %s: unknown format %q", a.ID, f))
				}
			}
		}
		for _, id := range a.IDs {
			if !buildIDs[id] {
				errs = append(errs, fmt.Errorf("archive %s: unknown build id %q", a.ID, id))
			}
		}
		errs = append(errs, parseField(a.NameTemplate)...)
		errs = append(errs, parseField(a.If)...)
		for _, f := range a.Files {
			if f.Src == "" {
				errs = append(errs, fmt.Errorf("archive %s: file src is required", a.ID))
			}
		}
	}

	pkgFormats := map[string]bool{"deb": true, "rpm": true, "apk": true}
	pkgTypes := map[string]bool{"": true, "file": true, "config": true, "dir": true, "symlink": true, "tree": true}
	pkgName := regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*$`)
	seenNFPM := map[string]bool{}
	for _, n := range p.NFPMs {
		if n.ID == "" {
			errs = append(errs, fmt.Errorf("nfpm id is required"))
		} else if seenNFPM[n.ID] {
			errs = append(errs, fmt.Errorf("duplicate nfpm id %q", n.ID))
		}
		seenNFPM[n.ID] = true
		if !pkgName.MatchString(n.PackageName) {
			errs = append(errs, fmt.Errorf("nfpm %s: package_name %q must be a lowercase linux package name", n.ID, n.PackageName))
		}
		if strings.TrimSpace(n.Maintainer) == "" {
			errs = append(errs, fmt.Errorf("nfpm %s: maintainer is required", n.ID))
		}
		if len(n.Formats) == 0 {
			errs = append(errs, fmt.Errorf("nfpm %s: formats is required", n.ID))
		}
		for _, f := range n.Formats {
			if !pkgFormats[f] {
				errs = append(errs, fmt.Errorf("nfpm %s: unknown format %q", n.ID, f))
			}
		}
		for _, id := range n.IDs {
			if !buildIDs[id] {
				errs = append(errs, fmt.Errorf("nfpm %s: unknown build id %q", n.ID, id))
			}
		}
		if n.Bindir != "" && !strings.Contains(n.Bindir, "{{") && !strings.HasPrefix(n.Bindir, "/") {
			errs = append(errs, fmt.Errorf("nfpm %s: bindir must be an absolute path", n.ID))
		}
		for _, field := range []string{n.PackageName, n.Maintainer, n.Description, n.Homepage, n.License, n.Vendor, n.Bindir, n.FileNameTemplate, n.If} {
			errs = append(errs, parseField(field)...)
		}
		for _, list := range [][]string{n.Depends, n.Recommends, n.Suggests, n.Conflicts, n.Replaces, n.Provides} {
			for _, item := range list {
				errs = append(errs, parseField(item)...)
			}
		}
		for _, file := range n.Contents {
			if file.Dst == "" {
				errs = append(errs, fmt.Errorf("nfpm %s: content dst is required", n.ID))
			} else if !strings.Contains(file.Dst, "{{") && !strings.HasPrefix(file.Dst, "/") {
				errs = append(errs, fmt.Errorf("nfpm %s: content dst %q must be absolute", n.ID, file.Dst))
			}
			if !pkgTypes[file.Type] {
				errs = append(errs, fmt.Errorf("nfpm %s: content type %q is not supported", n.ID, file.Type))
			}
			if file.Type != "dir" && file.Type != "symlink" && file.Src == "" {
				errs = append(errs, fmt.Errorf("nfpm %s: content src is required", n.ID))
			}
			if file.Mode != "" {
				if _, err := strconv.ParseUint(file.Mode, 8, 32); err != nil {
					errs = append(errs, fmt.Errorf("nfpm %s: mode %q is not octal", n.ID, file.Mode))
				}
			}
			errs = append(errs, parseField(file.Src)...)
			errs = append(errs, parseField(file.Dst)...)
		}
		for _, script := range []string{n.Scripts.PreInstall, n.Scripts.PostInstall, n.Scripts.PreRemove, n.Scripts.PostRemove} {
			errs = append(errs, parseField(script)...)
		}
	}

	signKinds := map[string]bool{"": true, "archives": true, "checksum": true, "binary": true, "package": true, "all": true}
	for _, s := range p.Signs {
		if s.Cmd == "" {
			errs = append(errs, fmt.Errorf("sign %s: cmd is required", s.ID))
		}
		if !signKinds[s.Artifacts] {
			errs = append(errs, fmt.Errorf("sign %s: artifacts must be archives, checksum, binary, package, or all", s.ID))
		}
		errs = append(errs, parseField(s.If)...)
		for _, arg := range s.Args {
			errs = append(errs, parseField(arg)...)
		}
	}
	for _, h := range append(append([]Hook{}, p.Before.Hooks...), p.After.Hooks...) {
		if h.Cmd == "" {
			errs = append(errs, fmt.Errorf("hook cmd is required"))
		}
		errs = append(errs, parseField(h.Cmd)...)
	}
	errs = append(errs, parseField(p.Snapshot.VersionTemplate)...)
	errs = append(errs, parseField(p.Nightly.VersionTemplate)...)
	errs = append(errs, parseField(p.Nightly.NameTemplate)...)
	errs = append(errs, parseField(p.Release.NameTemplate)...)
	errs = append(errs, parseField(p.Release.Header)...)
	errs = append(errs, parseField(p.Release.Footer)...)
	errs = append(errs, parseField(p.Checksum.NameTemplate)...)
	for k, v := range p.Variables {
		if k == "" {
			errs = append(errs, fmt.Errorf("variable name is empty"))
		}
		errs = append(errs, parseField(v)...)
	}
	for _, tf := range p.TemplateFiles {
		if tf.Src == "" {
			errs = append(errs, fmt.Errorf("template_files.src is required"))
		}
	}
	for _, h := range p.Homebrews {
		if h.Name == "" {
			errs = append(errs, fmt.Errorf("homebrew name is required"))
		}
		errs = append(errs, parseField(h.Description)...)
		errs = append(errs, parseField(h.Homepage)...)
		errs = append(errs, parseField(h.Install)...)
		errs = append(errs, parseField(h.Test)...)
	}
	return errors.Join(errs...)
}

func parseField(s string) []error {
	if err := tmpl.Parse(s); err != nil {
		return []error{err}
	}
	return nil
}
