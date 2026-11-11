package pipeline

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	_ "github.com/goreleaser/nfpm/v2/apk" // registers the apk packager
	_ "github.com/goreleaser/nfpm/v2/deb" // registers the deb packager
	"github.com/goreleaser/nfpm/v2/files"
	_ "github.com/goreleaser/nfpm/v2/rpm" // registers the rpm packager

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/fsys"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Packages builds deb, rpm, and apk packages from Linux binaries.
func Packages(c *runctx.Context) error {
	if len(c.Project.NFPMs) == 0 {
		return nil
	}
	c.Log.Step("system packages")
	var linux []*artifact.Artifact
	for _, bin := range c.BinariesCopy() {
		if bin.Goos == "linux" {
			linux = append(linux, bin)
		}
	}
	if len(linux) == 0 {
		c.Log.Sub("no linux binaries in this build")
		return nil
	}
	n := 0
	for _, spec := range c.Project.NFPMs {
		for _, bin := range linux {
			if len(spec.IDs) > 0 && !contains(spec.IDs, bin.BuildID) {
				continue
			}
			data := c.TargetData(bin.Goos, bin.Goarch, bin.Goarm, bin.Binary)
			if spec.If != "" {
				cond, err := tmpl.Render(spec.If, data)
				if err != nil {
					return fmt.Errorf("nfpm %s: %w", spec.ID, err)
				}
				if !tmpl.Truthy(cond) {
					continue
				}
			}
			for _, format := range spec.Formats {
				if err := onePackage(c, spec, bin, format); err != nil {
					return err
				}
				n++
			}
		}
	}
	if n == 0 {
		c.Log.Sub("no packages produced")
	}
	return nil
}

func onePackage(c *runctx.Context, spec config.NFPM, bin *artifact.Artifact, format string) error {
	arch, err := packageArch(format, bin.Goarch, bin.Goarm)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	data := c.TargetData(bin.Goos, bin.Goarch, bin.Goarm, bin.Binary)
	pkgName, err := tmpl.Render(spec.PackageName, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	data.PackageName = pkgName
	data.Release = spec.Release
	data.Arch = arch

	maintainer, err := tmpl.Render(spec.Maintainer, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	description, err := tmpl.Render(spec.Description, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	homepage, err := tmpl.Render(spec.Homepage, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	license, err := tmpl.Render(spec.License, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	vendor, err := tmpl.Render(spec.Vendor, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	bindir, err := tmpl.Render(spec.Bindir, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	bindir, err = absSlash(bindir)
	if err != nil {
		return fmt.Errorf("nfpm %s: bindir: %w", spec.ID, err)
	}
	contents, err := packageContents(c, spec, data, bin, bindir)
	if err != nil {
		return err
	}
	scripts, err := packageScripts(c, spec, data)
	if err != nil {
		return err
	}
	depends, err := renderList(spec.Depends, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	recommends, err := renderList(spec.Recommends, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	suggests, err := renderList(spec.Suggests, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	conflicts, err := renderList(spec.Conflicts, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	replaces, err := renderList(spec.Replaces, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	provides, err := renderList(spec.Provides, data)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}

	info := &nfpm.Info{
		Name:            pkgName,
		Arch:            arch,
		Platform:        "linux",
		Version:         c.Version,
		VersionSchema:   "semver",
		Release:         spec.Release,
		Epoch:           spec.Epoch,
		Section:         spec.Section,
		Priority:        spec.Priority,
		Maintainer:      maintainer,
		Description:     description,
		Homepage:        homepage,
		License:         license,
		Vendor:          vendor,
		MTime:           c.CommitTime,
		DisableGlobbing: true,
		Overridables: nfpm.Overridables{
			Depends:    depends,
			Recommends: recommends,
			Suggests:   suggests,
			Conflicts:  conflicts,
			Replaces:   replaces,
			Provides:   provides,
			Contents:   contents,
			Scripts:    scripts,
		},
	}
	switch format {
	case "deb":
		info.Deb.Arch = arch
		info.Deb.Compression = "gzip"
	case "rpm":
		info.RPM.Arch = arch
		info.RPM.Compression = "gzip"
	case "apk":
		info.APK.Arch = arch
	}
	info = nfpm.WithDefaults(info)
	if err := nfpm.PrepareForPackager(info, format); err != nil {
		return fmt.Errorf("nfpm %s %s: %w", spec.ID, format, err)
	}
	packager, err := nfpm.Get(format)
	if err != nil {
		return fmt.Errorf("nfpm %s: %w", spec.ID, err)
	}
	name := packager.ConventionalFileName(info)
	if spec.FileNameTemplate != "" {
		name, err = tmpl.Render(spec.FileNameTemplate, data)
		if err != nil {
			return fmt.Errorf("nfpm %s: %w", spec.ID, err)
		}
		if !strings.HasSuffix(name, "."+format) {
			name += "." + format
		}
	}
	name = filepath.Base(name)
	if name == "" || name == "." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("nfpm %s: package name %q is not a file name", spec.ID, name)
	}
	dst := filepath.Join(c.Dist, name)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	if err := packager.Package(info, out); err != nil {
		return fmt.Errorf("nfpm %s %s: %w", spec.ID, format, err)
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	if err := c.Add(&artifact.Artifact{
		Name:      name,
		Path:      dst,
		Type:      artifact.TypePackage,
		Goos:      bin.Goos,
		Goarch:    bin.Goarch,
		Goarm:     bin.Goarm,
		Binary:    bin.Binary,
		BuildID:   bin.BuildID,
		ArchiveID: spec.ID,
		Format:    format,
	}); err != nil {
		return err
	}
	c.Log.Sub("%s", name)
	return nil
}

func packageContents(c *runctx.Context, spec config.NFPM, data tmpl.Data, bin *artifact.Artifact, bindir string) (files.Contents, error) {
	when := c.CommitTime
	dest := path.Join(bindir, bin.Binary)
	contents := files.Contents{{
		Source:      bin.Path,
		Destination: dest,
		Type:        files.TypeFile,
		FileInfo: &files.ContentFileInfo{
			Owner: "root",
			Group: "root",
			Mode:  0o755,
			MTime: when,
		},
	}}
	for _, file := range spec.Contents {
		src, err := tmpl.Render(file.Src, data)
		if err != nil {
			return nil, fmt.Errorf("nfpm %s: %w", spec.ID, err)
		}
		dst, err := tmpl.Render(file.Dst, data)
		if err != nil {
			return nil, fmt.Errorf("nfpm %s: %w", spec.ID, err)
		}
		dst, err = absSlash(dst)
		if err != nil {
			return nil, fmt.Errorf("nfpm %s: content dst: %w", spec.ID, err)
		}
		kind := file.Type
		if kind == "" {
			kind = files.TypeFile
		}
		mode := os.FileMode(0o644)
		if kind == files.TypeDir || kind == files.TypeTree {
			mode = 0o755
		}
		if kind == files.TypeSymlink {
			mode = 0o777
		}
		if file.Mode != "" {
			parsed, err := strconv.ParseUint(file.Mode, 8, 32)
			if err != nil {
				return nil, fmt.Errorf("nfpm %s: mode %q: %w", spec.ID, file.Mode, err)
			}
			mode = os.FileMode(parsed)
		}
		entry := &files.Content{
			Destination: dst,
			Type:        kind,
			FileInfo: &files.ContentFileInfo{
				Owner: "root",
				Group: "root",
				Mode:  mode,
				MTime: when,
			},
		}
		switch kind {
		case files.TypeSymlink:
			entry.Source = src
		case files.TypeDir:
		default:
			rel, err := fsys.Rel(src)
			if err != nil {
				return nil, fmt.Errorf("nfpm %s: %w", spec.ID, err)
			}
			full := filepath.Join(c.Dir, rel)
			st, err := os.Stat(full)
			if err != nil {
				return nil, fmt.Errorf("nfpm %s: %w", spec.ID, err)
			}
			if kind == files.TypeTree && !st.IsDir() {
				return nil, fmt.Errorf("nfpm %s: %s is not a directory", spec.ID, src)
			}
			if kind != files.TypeTree && st.IsDir() {
				return nil, fmt.Errorf("nfpm %s: %s is a directory; set type: tree", spec.ID, src)
			}
			entry.Source = full
		}
		contents = append(contents, entry)
	}
	return contents, nil
}

func packageScripts(c *runctx.Context, spec config.NFPM, data tmpl.Data) (nfpm.Scripts, error) {
	var scripts nfpm.Scripts
	var err error
	if scripts.PreInstall, err = scriptPath(c, spec.ID, spec.Scripts.PreInstall, data); err != nil {
		return scripts, err
	}
	if scripts.PostInstall, err = scriptPath(c, spec.ID, spec.Scripts.PostInstall, data); err != nil {
		return scripts, err
	}
	if scripts.PreRemove, err = scriptPath(c, spec.ID, spec.Scripts.PreRemove, data); err != nil {
		return scripts, err
	}
	if scripts.PostRemove, err = scriptPath(c, spec.ID, spec.Scripts.PostRemove, data); err != nil {
		return scripts, err
	}
	return scripts, nil
}

func scriptPath(c *runctx.Context, id, src string, data tmpl.Data) (string, error) {
	if src == "" {
		return "", nil
	}
	rendered, err := tmpl.Render(src, data)
	if err != nil {
		return "", fmt.Errorf("nfpm %s: %w", id, err)
	}
	rel, err := fsys.Rel(rendered)
	if err != nil {
		return "", fmt.Errorf("nfpm %s: %w", id, err)
	}
	f, err := fsys.OpenUnder(c.Dir, rel)
	if err != nil {
		return "", fmt.Errorf("nfpm %s: %w", id, err)
	}
	f.Close()
	return filepath.Join(c.Dir, rel), nil
}

func renderList(items []string, data tmpl.Data) ([]string, error) {
	var out []string
	for _, item := range items {
		rendered, err := tmpl.Render(item, data)
		if err != nil {
			return nil, err
		}
		rendered = strings.TrimSpace(rendered)
		if rendered != "" {
			out = append(out, rendered)
		}
	}
	return out, nil
}

func absSlash(p string) (string, error) {
	if p == "" || !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%q must be an absolute path", p)
	}
	clean := path.Clean(p)
	if clean == "/" {
		return "/", nil
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("%q escapes its root", p)
	}
	return clean, nil
}

func packageArch(format, goarch, goarm string) (string, error) {
	table := map[string]map[string]string{
		"deb": {
			"386":      "i386",
			"amd64":    "amd64",
			"arm64":    "arm64",
			"ppc64le":  "ppc64el",
			"s390x":    "s390x",
			"riscv64":  "riscv64",
			"mips64le": "mips64el",
			"loong64":  "loong64",
		},
		"rpm": {
			"386":      "i386",
			"amd64":    "x86_64",
			"arm64":    "aarch64",
			"ppc64le":  "ppc64le",
			"s390x":    "s390x",
			"riscv64":  "riscv64",
			"mips64le": "mips64el",
			"loong64":  "loongarch64",
		},
		"apk": {
			"386":      "x86",
			"amd64":    "x86_64",
			"arm64":    "aarch64",
			"ppc64le":  "ppc64le",
			"s390x":    "s390x",
			"riscv64":  "riscv64",
			"mips64le": "mips64",
			"loong64":  "loongarch64",
		},
	}
	if goarch == "arm" {
		return armArch(format, goarm), nil
	}
	arch, ok := table[format][goarch]
	if !ok {
		return "", fmt.Errorf("no %s architecture for %s", format, goarch)
	}
	return arch, nil
}

func armArch(format, goarm string) string {
	switch format {
	case "deb":
		if goarm == "5" {
			return "armel"
		}
		return "armhf"
	case "rpm":
		if goarm == "5" {
			return "armv5tel"
		}
		return "armv7hl"
	default:
		switch goarm {
		case "5":
			return "armel"
		case "6":
			return "armhf"
		default:
			return "armv7"
		}
	}
}
