// Package config loads a gobunshin project file.
package config

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Project is a .gobunshin.yaml file.
type Project struct {
	Version       int               `yaml:"version,omitempty"`
	ProjectName   string            `yaml:"project_name,omitempty"`
	Dist          string            `yaml:"dist,omitempty"`
	Variables     map[string]string `yaml:"variables,omitempty"`
	Metadata      Metadata          `yaml:"metadata,omitempty"`
	Monorepo      Monorepo          `yaml:"monorepo,omitempty"`
	Git           Git               `yaml:"git,omitempty"`
	Before        HookGroup         `yaml:"before,omitempty"`
	After         HookGroup         `yaml:"after,omitempty"`
	Builds        []Build           `yaml:"builds,omitempty"`
	Archives      []Archive         `yaml:"archives,omitempty"`
	Checksum      Checksum          `yaml:"checksum,omitempty"`
	Signs         []Sign            `yaml:"signs,omitempty"`
	Changelog     Changelog         `yaml:"changelog,omitempty"`
	Snapshot      Snapshot          `yaml:"snapshot,omitempty"`
	Nightly       Nightly           `yaml:"nightly,omitempty"`
	Release       Release           `yaml:"release,omitempty"`
	Homebrews     []Homebrew        `yaml:"homebrews,omitempty"`
	NFPMs         []NFPM            `yaml:"nfpms,omitempty"`
	Installers    []Installer       `yaml:"installers,omitempty"`
	TemplateFiles []TemplateFile    `yaml:"template_files,omitempty"`
}

// Metadata holds defaults reused by publishers. Every field is free.
type Metadata struct {
	Description string `yaml:"description,omitempty"`
	Homepage    string `yaml:"homepage,omitempty"`
	License     string `yaml:"license,omitempty"`
}

// Monorepo scopes tags and the changelog to one module in a shared repository.
type Monorepo struct {
	Dir       string `yaml:"dir,omitempty"`
	TagPrefix string `yaml:"tag_prefix,omitempty"`
}

// Git controls which tags count as releases.
type Git struct {
	IgnoreTags []string `yaml:"ignore_tags,omitempty"`
	TagSort    string   `yaml:"tag_sort,omitempty"`
	TagPrefix  string   `yaml:"tag_prefix,omitempty"`
}

// HookGroup is a list of shell commands.
type HookGroup struct {
	Hooks []Hook `yaml:"hooks,omitempty"`
}

// Hook is a shell command. A bare string is accepted in YAML.
type Hook struct {
	Cmd string `yaml:"cmd"`
	Dir string `yaml:"dir,omitempty"`
}

// UnmarshalYAML accepts either a string or a mapping with cmd and dir.
func (h *Hook) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		h.Cmd = value.Value
		h.Dir = ""
		return nil
	}
	var raw struct {
		Cmd string `yaml:"cmd"`
		Dir string `yaml:"dir"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if raw.Cmd == "" {
		return fmt.Errorf("hook cmd is required")
	}
	*h = Hook{Cmd: raw.Cmd, Dir: raw.Dir}
	return nil
}

// Build is one binary, compiled with Go or copied from a prebuilt path.
type Build struct {
	ID       string   `yaml:"id,omitempty"`
	Builder  string   `yaml:"builder,omitempty"`
	Main     string   `yaml:"main,omitempty"`
	Binary   string   `yaml:"binary,omitempty"`
	Dir      string   `yaml:"dir,omitempty"`
	Env      []string `yaml:"env,omitempty"`
	Flags    []string `yaml:"flags,omitempty"`
	Ldflags  []string `yaml:"ldflags,omitempty"`
	Goos     []string `yaml:"goos,omitempty"`
	Goarch   []string `yaml:"goarch,omitempty"`
	Goarm    []string `yaml:"goarm,omitempty"`
	Ignore   []Ignore `yaml:"ignore,omitempty"`
	If       string   `yaml:"if,omitempty"`
	Prebuilt Prebuilt `yaml:"prebuilt,omitempty"`
}

// Prebuilt copies an existing binary instead of compiling.
type Prebuilt struct {
	Path string `yaml:"path,omitempty"`
}

// Ignore drops one GOOS/GOARCH pair. Empty fields match anything.
type Ignore struct {
	Goos   string `yaml:"goos,omitempty"`
	Goarch string `yaml:"goarch,omitempty"`
	Goarm  string `yaml:"goarm,omitempty"`
}

// Archive packages built binaries.
type Archive struct {
	ID              string           `yaml:"id,omitempty"`
	IDs             []string         `yaml:"ids,omitempty"`
	Formats         []string         `yaml:"formats,omitempty"`
	NameTemplate    string           `yaml:"name_template,omitempty"`
	FormatOverrides []FormatOverride `yaml:"format_overrides,omitempty"`
	Files           []File           `yaml:"files,omitempty"`
	WrapInDirectory *bool            `yaml:"wrap_in_directory,omitempty"`
	If              string           `yaml:"if,omitempty"`
}

// Wrapped reports whether the archive contains a top-level directory.
// The default is true.
func (a Archive) Wrapped() bool {
	if a.WrapInDirectory == nil {
		return true
	}
	return *a.WrapInDirectory
}

// FormatOverride replaces formats for one GOOS or GOARCH.
type FormatOverride struct {
	Goos    string   `yaml:"goos,omitempty"`
	Goarch  string   `yaml:"goarch,omitempty"`
	Formats []string `yaml:"formats,omitempty"`
}

// File is an extra file placed in an archive. A bare string names the source.
type File struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst,omitempty"`
}

// UnmarshalYAML accepts a string or a src/dst mapping.
func (f *File) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		f.Src = value.Value
		return nil
	}
	var raw struct {
		Src string `yaml:"src"`
		Dst string `yaml:"dst"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*f = File{Src: raw.Src, Dst: raw.Dst}
	return nil
}

// Checksum writes a checksum file for archives and extra files.
type Checksum struct {
	NameTemplate string `yaml:"name_template,omitempty"`
	Algorithm    string `yaml:"algorithm,omitempty"`
	Disable      bool   `yaml:"disable,omitempty"`
}

// Sign runs an external command to sign artifacts.
type Sign struct {
	ID        string   `yaml:"id,omitempty"`
	Cmd       string   `yaml:"cmd,omitempty"`
	Args      []string `yaml:"args,omitempty"`
	Artifacts string   `yaml:"artifacts,omitempty"`
	If        string   `yaml:"if,omitempty"`
	Signature string   `yaml:"signature,omitempty"`
}

// Changelog configures the generated release notes.
type Changelog struct {
	Sort    string  `yaml:"sort,omitempty"`
	Title   *string `yaml:"title,omitempty"`
	Divider string  `yaml:"divider,omitempty"`
	Filters Filters `yaml:"filters,omitempty"`
	Groups  []Group `yaml:"groups,omitempty"`
	Disable bool    `yaml:"disable,omitempty"`
	Limit   int     `yaml:"limit,omitempty"`
}

// TitleText returns the changelog heading. An explicit empty title removes it.
func (c Changelog) TitleText() string {
	if c.Title == nil {
		return ""
	}
	return *c.Title
}

// Filters select commit subjects and paths.
type Filters struct {
	Exclude      []string `yaml:"exclude,omitempty"`
	Include      []string `yaml:"include,omitempty"`
	IncludePaths []string `yaml:"include_paths,omitempty"`
}

// Group puts matching commits under one heading. The first match wins.
type Group struct {
	Title  string `yaml:"title,omitempty"`
	Regexp string `yaml:"regexp,omitempty"`
}

// Snapshot names untagged local releases. Snapshots are never published.
type Snapshot struct {
	VersionTemplate string `yaml:"version_template,omitempty"`
	NameTemplate    string `yaml:"name_template,omitempty"`
}

// Nightly names a moving release that is replaced on every run.
type Nightly struct {
	VersionTemplate string `yaml:"version_template,omitempty"`
	TagName         string `yaml:"tag_name,omitempty"`
	NameTemplate    string `yaml:"name_template,omitempty"`
}

// Release configures the GitHub release.
type Release struct {
	Disable      bool   `yaml:"disable,omitempty"`
	Draft        bool   `yaml:"draft,omitempty"`
	Prerelease   string `yaml:"prerelease,omitempty"`
	Mode         string `yaml:"mode,omitempty"`
	NameTemplate string `yaml:"name_template,omitempty"`
	Header       string `yaml:"header,omitempty"`
	Footer       string `yaml:"footer,omitempty"`
	HeaderFile   string `yaml:"header_file,omitempty"`
	FooterFile   string `yaml:"footer_file,omitempty"`
	GitHub       GitHub `yaml:"github,omitempty"`
}

// GitHub is a repository on github.com or a GitHub-compatible host.
type GitHub struct {
	Owner string `yaml:"owner,omitempty"`
	Name  string `yaml:"name,omitempty"`
	Host  string `yaml:"host,omitempty"`
}

// Homebrew is one formula published to a tap, and always written under dist/.
type Homebrew struct {
	Name        string   `yaml:"name,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty"`
	License     string   `yaml:"license,omitempty"`
	Folder      string   `yaml:"folder,omitempty"`
	Branch      string   `yaml:"branch,omitempty"`
	Install     string   `yaml:"install,omitempty"`
	Test        string   `yaml:"test,omitempty"`
	Repository  GitHub   `yaml:"repository,omitempty"`
	IDs         []string `yaml:"ids,omitempty"`
}

// NFPM builds Linux system packages from the compiled binaries.
type NFPM struct {
	ID               string      `yaml:"id,omitempty"`
	IDs              []string    `yaml:"ids,omitempty"`
	PackageName      string      `yaml:"package_name,omitempty"`
	Maintainer       string      `yaml:"maintainer,omitempty"`
	Description      string      `yaml:"description,omitempty"`
	Homepage         string      `yaml:"homepage,omitempty"`
	License          string      `yaml:"license,omitempty"`
	Vendor           string      `yaml:"vendor,omitempty"`
	Section          string      `yaml:"section,omitempty"`
	Priority         string      `yaml:"priority,omitempty"`
	Formats          []string    `yaml:"formats,omitempty"`
	Bindir           string      `yaml:"bindir,omitempty"`
	FileNameTemplate string      `yaml:"file_name_template,omitempty"`
	Epoch            string      `yaml:"epoch,omitempty"`
	Release          string      `yaml:"release,omitempty"`
	Depends          []string    `yaml:"depends,omitempty"`
	Recommends       []string    `yaml:"recommends,omitempty"`
	Suggests         []string    `yaml:"suggests,omitempty"`
	Conflicts        []string    `yaml:"conflicts,omitempty"`
	Replaces         []string    `yaml:"replaces,omitempty"`
	Provides         []string    `yaml:"provides,omitempty"`
	Contents         []NFPMFile  `yaml:"contents,omitempty"`
	Scripts          NFPMScripts `yaml:"scripts,omitempty"`
	If               string      `yaml:"if,omitempty"`
}

// NFPMFile is an extra file installed by a system package.
type NFPMFile struct {
	Src  string `yaml:"src,omitempty"`
	Dst  string `yaml:"dst"`
	Type string `yaml:"type,omitempty"`
	Mode string `yaml:"mode,omitempty"`
}

// NFPMScripts are maintainer scripts. Paths are relative to the project.
type NFPMScripts struct {
	PreInstall  string `yaml:"preinstall,omitempty"`
	PostInstall string `yaml:"postinstall,omitempty"`
	PreRemove   string `yaml:"preremove,omitempty"`
	PostRemove  string `yaml:"postremove,omitempty"`
}

// Installer builds a macOS .pkg or a Windows NSIS installer from one binary.
type Installer struct {
	ID               string   `yaml:"id,omitempty"`
	IDs              []string `yaml:"ids,omitempty"`
	Name             string   `yaml:"name,omitempty"`
	Identifier       string   `yaml:"identifier,omitempty"`
	Formats          []string `yaml:"formats,omitempty"`
	Bindir           string   `yaml:"bindir,omitempty"`
	FileNameTemplate string   `yaml:"file_name_template,omitempty"`
	If               string   `yaml:"if,omitempty"`
}

// TemplateFile is rendered into dist/ and uploaded with the release.
type TemplateFile struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst,omitempty"`
}

// UnmarshalYAML accepts a string or a src/dst mapping.
func (f *TemplateFile) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		f.Src = value.Value
		return nil
	}
	var raw struct {
		Src string `yaml:"src"`
		Dst string `yaml:"dst"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*f = TemplateFile{Src: raw.Src, Dst: raw.Dst}
	return nil
}

// TagPrefix is the git tag prefix, from git.tag_prefix or monorepo.tag_prefix.
func (p Project) TagPrefix() string {
	if p.Git.TagPrefix != "" {
		return p.Git.TagPrefix
	}
	return p.Monorepo.TagPrefix
}

// Flex helpers keep goarm values written as numbers working after a YAML round trip.
func stringifyGoarm(m map[string]any) {
	builds, ok := m["builds"].([]any)
	if !ok {
		return
	}
	for _, item := range builds {
		bm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		arms, ok := bm["goarm"].([]any)
		if !ok {
			continue
		}
		for i, arm := range arms {
			switch n := arm.(type) {
			case int:
				arms[i] = strconv.Itoa(n)
			case int64:
				arms[i] = strconv.FormatInt(n, 10)
			case uint64:
				arms[i] = strconv.FormatUint(n, 10)
			}
		}
	}
}
