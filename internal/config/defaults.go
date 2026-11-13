package config

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

const defaultNameTemplate = "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}"

// ApplyDefaults fills the fields a short config is allowed to omit.
func ApplyDefaults(p *Project, dir string) {
	if p.Variables == nil {
		p.Variables = map[string]string{}
	}
	modDir := dir
	if p.Monorepo.Dir != "" {
		candidate := filepath.Join(dir, p.Monorepo.Dir)
		if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
			modDir = candidate
		}
	}
	module := readModule(modDir)
	if p.ProjectName == "" {
		if module != "" {
			p.ProjectName = path.Base(module)
		} else {
			p.ProjectName = filepath.Base(dir)
		}
	}
	if p.Dist == "" {
		p.Dist = "dist"
	}
	if p.Git.TagSort == "" {
		p.Git.TagSort = "smartsemver"
	}
	if p.Snapshot.VersionTemplate == "" {
		p.Snapshot.VersionTemplate = "{{ incpatch .Version }}-SNAPSHOT-{{ .ShortCommit }}"
	}
	if p.Nightly.VersionTemplate == "" {
		p.Nightly.VersionTemplate = "0.0.0-nightly.{{ .ShortCommit }}"
	}
	if p.Nightly.TagName == "" {
		p.Nightly.TagName = "nightly"
	}
	if p.Nightly.NameTemplate == "" {
		p.Nightly.NameTemplate = "Nightly {{ .Date }}"
	}
	if p.Changelog.Sort == "" {
		p.Changelog.Sort = "asc"
	}
	if p.Changelog.Title == nil && !p.Changelog.Disable {
		title := "## Changelog"
		p.Changelog.Title = &title
	}
	if p.Monorepo.Dir != "" && len(p.Changelog.Filters.IncludePaths) == 0 {
		p.Changelog.Filters.IncludePaths = []string{filepath.ToSlash(p.Monorepo.Dir)}
	}
	if p.Checksum.NameTemplate == "" {
		p.Checksum.NameTemplate = "checksums.txt"
	}
	if p.Checksum.Algorithm == "" {
		p.Checksum.Algorithm = "sha256"
	}
	if p.Release.NameTemplate == "" {
		p.Release.NameTemplate = "{{ .Tag }}"
	}
	if p.Release.Mode == "" {
		p.Release.Mode = "fail"
	}
	if p.Release.Prerelease == "" {
		p.Release.Prerelease = "auto"
	}
	if p.Release.GitHub.Owner == "" || p.Release.GitHub.Name == "" {
		owner, name := githubFromModule(module)
		if p.Release.GitHub.Owner == "" {
			p.Release.GitHub.Owner = owner
		}
		if p.Release.GitHub.Name == "" {
			p.Release.GitHub.Name = name
		}
	}

	mainDir := dir
	if p.Monorepo.Dir != "" {
		mainDir = filepath.Join(dir, p.Monorepo.Dir)
	}
	if len(p.Builds) == 0 {
		p.Builds = []Build{{
			Env:   []string{"CGO_ENABLED=0"},
			Flags: []string{"-trimpath"},
		}}
	}
	used := map[string]bool{}
	for i := range p.Builds {
		b := &p.Builds[i]
		if b.ID == "" {
			b.ID = p.ProjectName
			if used[b.ID] {
				b.ID = fmtID(p.ProjectName, i+1)
			}
		}
		used[b.ID] = true
		if b.Builder == "" {
			b.Builder = "go"
		}
		if b.Binary == "" {
			b.Binary = p.ProjectName
		}
		if b.Builder == "go" && b.Main == "" {
			b.Main = detectMain(mainDir, p.ProjectName)
		}
		if len(b.Goos) == 0 {
			b.Goos = []string{"linux", "darwin", "windows"}
		}
		if len(b.Goarch) == 0 {
			b.Goarch = []string{"amd64", "arm64"}
		}
	}

	if len(p.Archives) == 0 {
		p.Archives = []Archive{{
			ID:      "default",
			Formats: []string{"tar.gz"},
			FormatOverrides: []FormatOverride{{
				Goos:    "windows",
				Formats: []string{"zip"},
			}},
		}}
	}
	archiveIDs := map[string]bool{}
	for i := range p.Archives {
		a := &p.Archives[i]
		if a.ID == "" {
			a.ID = "default"
			if archiveIDs[a.ID] {
				a.ID = fmtID("archive", i+1)
			}
		}
		archiveIDs[a.ID] = true
		if len(a.Formats) == 0 {
			a.Formats = []string{"tar.gz"}
		}
		if a.NameTemplate == "" {
			a.NameTemplate = defaultNameTemplate
		}
	}
	for i := range p.Homebrews {
		h := &p.Homebrews[i]
		if h.Name == "" {
			h.Name = p.ProjectName
		}
		if h.Folder == "" {
			h.Folder = "Formula"
		}
		if h.Branch == "" {
			h.Branch = "main"
		}
	}
	for i := range p.NFPMs {
		n := &p.NFPMs[i]
		if n.ID == "" {
			n.ID = "nfpm"
			if i > 0 {
				n.ID = fmtID("nfpm", i+1)
			}
		}
		if n.PackageName == "" {
			n.PackageName = strings.ToLower(p.ProjectName)
		}
		if n.Description == "" {
			n.Description = p.Metadata.Description
		}
		if n.Description == "" {
			n.Description = p.ProjectName
		}
		if n.Homepage == "" {
			n.Homepage = p.Metadata.Homepage
		}
		if n.License == "" {
			n.License = p.Metadata.License
		}
		if len(n.Formats) == 0 {
			n.Formats = []string{"deb", "rpm", "apk"}
		}
		if n.Bindir == "" {
			n.Bindir = "/usr/bin"
		}
		if n.Section == "" {
			n.Section = "default"
		}
		if n.Priority == "" {
			n.Priority = "extra"
		}
		if n.Release == "" {
			n.Release = "1"
		}
	}
	for i := range p.Installers {
		n := &p.Installers[i]
		if n.ID == "" {
			n.ID = "installer"
			if i > 0 {
				n.ID = fmtID("installer", i+1)
			}
		}
		if n.Name == "" {
			n.Name = p.ProjectName
		}
		if n.Identifier == "" {
			n.Identifier = "com.gobunshin." + strings.ToLower(n.Name)
		}
		if len(n.Formats) == 0 {
			n.Formats = []string{"pkg", "nsis"}
		}
		if n.Bindir == "" {
			n.Bindir = "/usr/local/bin"
		}
		if n.FileNameTemplate == "" {
			n.FileNameTemplate = "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}"
		}
	}
	for i := range p.Signs {
		if p.Signs[i].ID == "" {
			p.Signs[i].ID = fmtID("sign", i+1)
		}
		if p.Signs[i].Artifacts == "" {
			p.Signs[i].Artifacts = "archives"
		}
		if p.Signs[i].Signature == "" {
			p.Signs[i].Signature = "${artifact}.sig"
		}
	}
}

func fmtID(prefix string, n int) string {
	return prefix + "-" + strconvItoa(n)
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func readModule(dir string) string {
	body, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func githubFromModule(module string) (string, string) {
	parts := strings.Split(module, "/")
	if len(parts) >= 3 && parts[0] == "github.com" {
		return parts[1], parts[2]
	}
	return "", ""
}

func detectMain(dir, name string) string {
	if fileExists(filepath.Join(dir, "cmd", name, "main.go")) {
		return "./cmd/" + name
	}
	if fileExists(filepath.Join(dir, "main.go")) {
		return "."
	}
	entries, err := os.ReadDir(filepath.Join(dir, "cmd"))
	if err == nil && len(entries) == 1 && entries[0].IsDir() {
		if fileExists(filepath.Join(dir, "cmd", entries[0].Name(), "main.go")) {
			return "./cmd/" + entries[0].Name()
		}
	}
	return "."
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
