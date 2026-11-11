package pipeline

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/githubx"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Homebrew writes formula files and, when publishing, commits them to the tap.
func Homebrew(c *runctx.Context) error {
	if len(c.Project.Homebrews) == 0 {
		return nil
	}
	if c.Dist == "" {
		return fmt.Errorf("homebrew: dist directory is not set")
	}
	c.Log.Step("homebrew")
	if err := os.MkdirAll(filepath.Join(c.Dist, "homebrew"), 0o755); err != nil {
		return err
	}
	var client *githubx.Client
	for _, h := range c.Project.Homebrews {
		body, err := renderProjectFormula(c, h)
		if err != nil {
			return err
		}
		path := filepath.Join(c.Dist, "homebrew", h.Name+".rb")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
		if err := c.Add(&artifact.Artifact{
			Name: h.Name + ".rb",
			Path: path,
			Type: artifact.TypeBrew,
		}); err != nil {
			return err
		}
		if !c.DoPublish || h.Repository.Owner == "" || h.Repository.Name == "" {
			c.Log.Sub("%s", path)
			continue
		}
		if c.Token == "" {
			return fmt.Errorf("homebrew %s: set GITHUB_TOKEN or GH_TOKEN to publish the tap", h.Name)
		}
		if client == nil {
			client = githubClient(c)
		}
		repoPath := filepath.ToSlash(filepath.Join(h.Folder, h.Name+".rb"))
		msg := fmt.Sprintf("gobunshin: update %s to %s", h.Name, c.Version)
		if err := client.PutFile(c.Go, h.Repository.Owner, h.Repository.Name, repoPath, h.Branch, msg, []byte(body)); err != nil {
			return fmt.Errorf("homebrew %s: %w", h.Name, err)
		}
		c.Log.Sub("%s/%s", h.Repository.Name, repoPath)
	}
	return nil
}

func renderProjectFormula(c *runctx.Context, h config.Homebrew) (string, error) {
	if c.Owner == "" || c.Name == "" {
		return "", fmt.Errorf("homebrew %s: release.github owner and name are required", h.Name)
	}
	assets, binary, wrapped, err := pickBrewAssets(c, h)
	if err != nil {
		return "", err
	}
	desc := h.Description
	if desc == "" {
		desc = c.Project.Metadata.Description
	}
	if desc == "" {
		desc = c.Project.ProjectName
	}
	home := h.Homepage
	if home == "" {
		home = c.Project.Metadata.Homepage
	}
	license := h.License
	if license == "" {
		license = c.Project.Metadata.License
	}
	desc, err = c.Render(desc)
	if err != nil {
		return "", err
	}
	if home != "" {
		home, err = c.Render(home)
		if err != nil {
			return "", err
		}
	}
	install := h.Install
	if install != "" {
		install, err = tmpl.Render(install, c.Data())
		if err != nil {
			return "", err
		}
	} else if wrapped {
		install = fmt.Sprintf(`bin.install Dir["*/%s"].first`, binary)
	} else {
		install = fmt.Sprintf("bin.install %s", rubyQuote(binary))
	}
	testBody := h.Test
	if testBody != "" {
		testBody, err = c.Render(testBody)
		if err != nil {
			return "", err
		}
	}
	return renderFormula(formulaInput{
		Class:    className(h.Name),
		Desc:     desc,
		Homepage: home,
		License:  license,
		Version:  c.Version,
		Install:  install,
		Test:     testBody,
		Assets:   assets,
	}), nil
}

type brewAsset struct {
	Goos   string
	Goarch string
	URL    string
	SHA    string
}

func pickBrewAssets(c *runctx.Context, h config.Homebrew) ([]brewAsset, string, bool, error) {
	rank := map[string]int{"tar.gz": 0, "tgz": 1, "zip": 2, "binary": 3}
	best := map[string]*artifact.Artifact{}
	for _, a := range c.ArtifactsCopy() {
		if a.Type != artifact.TypeArchive && a.Type != artifact.TypeBinary {
			continue
		}
		if a.Goos != "darwin" && a.Goos != "linux" {
			continue
		}
		if a.Goarch != "amd64" && a.Goarch != "arm64" {
			continue
		}
		if len(h.IDs) > 0 && !contains(h.IDs, a.ArchiveID) {
			continue
		}
		key := a.Goos + "/" + a.Goarch
		if old := best[key]; old == nil || rank[a.Format] < rank[old.Format] {
			best[key] = a
		}
	}
	if len(best) == 0 {
		return nil, "", false, fmt.Errorf("homebrew %s: no darwin or linux archives", h.Name)
	}
	keys := make([]string, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	binary := ""
	wrapped := false
	seenWrap := false
	var assets []brewAsset
	for _, key := range keys {
		a := best[key]
		if a.SHA256 == "" {
			return nil, "", false, fmt.Errorf("homebrew %s: %s has no sha256", h.Name, a.Name)
		}
		if binary == "" {
			binary = a.Binary
		} else if binary != a.Binary {
			return nil, "", false, fmt.Errorf("homebrew %s: archives contain different binary names", h.Name)
		}
		if !seenWrap {
			wrapped = a.Wrapped
			seenWrap = true
		} else if wrapped != a.Wrapped && h.Install == "" {
			return nil, "", false, fmt.Errorf("homebrew %s: mixed wrap_in_directory; set install", h.Name)
		}
		assets = append(assets, brewAsset{
			Goos:   a.Goos,
			Goarch: a.Goarch,
			URL:    downloadURL(c.Owner, c.Name, c.Tag, a.Name),
			SHA:    a.SHA256,
		})
	}
	return assets, binary, wrapped, nil
}

func downloadURL(owner, repo, tag, file string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, url.PathEscape(tag), url.PathEscape(file))
}

type formulaInput struct {
	Class    string
	Desc     string
	Homepage string
	License  string
	Version  string
	Install  string
	Test     string
	Assets   []brewAsset
}

func renderFormula(in formulaInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "class %s < Formula\n", in.Class)
	fmt.Fprintf(&b, "  desc %s\n", rubyQuote(in.Desc))
	if in.Homepage != "" {
		fmt.Fprintf(&b, "  homepage %s\n", rubyQuote(in.Homepage))
	}
	fmt.Fprintf(&b, "  version %s\n", rubyQuote(in.Version))
	if in.License != "" {
		fmt.Fprintf(&b, "  license %s\n", rubyQuote(in.License))
	}
	b.WriteString("\n")
	byOS := map[string]map[string]brewAsset{}
	for _, a := range in.Assets {
		if byOS[a.Goos] == nil {
			byOS[a.Goos] = map[string]brewAsset{}
		}
		byOS[a.Goos][a.Goarch] = a
	}
	writeOS := func(goos, label string) {
		arches := byOS[goos]
		if len(arches) == 0 {
			return
		}
		fmt.Fprintf(&b, "  on_%s do\n", label)
		if a, ok := arches["arm64"]; ok {
			b.WriteString("    on_arm do\n")
			writeBrewURL(&b, a, 6)
			b.WriteString("    end\n")
		}
		if a, ok := arches["amd64"]; ok {
			b.WriteString("    on_intel do\n")
			writeBrewURL(&b, a, 6)
			b.WriteString("    end\n")
		}
		b.WriteString("  end\n\n")
	}
	writeOS("darwin", "macos")
	writeOS("linux", "linux")
	b.WriteString("  def install\n")
	b.WriteString(indentRuby(in.Install, 4))
	b.WriteString("\n  end\n")
	if strings.TrimSpace(in.Test) != "" {
		b.WriteString("\n  test do\n")
		b.WriteString(indentRuby(in.Test, 4))
		b.WriteString("\n  end\n")
	}
	b.WriteString("end\n")
	return b.String()
}

func writeBrewURL(b *strings.Builder, a brewAsset, indent int) {
	pad := strings.Repeat(" ", indent)
	fmt.Fprintf(b, "%surl %s\n", pad, rubyQuote(a.URL))
	fmt.Fprintf(b, "%ssha256 %s\n", pad, rubyQuote(a.SHA))
}

func indentRuby(body string, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.Trim(body, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = pad + strings.TrimLeft(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func rubyQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func className(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	})
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	s := b.String()
	if s == "" {
		return "Formula"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "App" + s
	}
	return s
}
