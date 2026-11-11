package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/karolsko/gobunshin/internal/archive"
	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/fsys"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Archive packs built binaries into the formats configured for the project.
func Archive(c *runctx.Context) error {
	bins := c.BinariesCopy()
	c.Log.Step("archiving")
	produced := 0
	for _, cfg := range c.Project.Archives {
		n, err := oneArchive(c, cfg, bins)
		if err != nil {
			return err
		}
		produced += n
	}
	if produced == 0 && len(bins) > 0 && len(c.Project.Archives) > 0 {
		return fmt.Errorf("archives produced no files")
	}
	return nil
}

func oneArchive(c *runctx.Context, cfg config.Archive, bins []*artifact.Artifact) (int, error) {
	n := 0
	for _, bin := range bins {
		if len(cfg.IDs) > 0 && !contains(cfg.IDs, bin.BuildID) {
			continue
		}
		data := c.TargetData(bin.Goos, bin.Goarch, bin.Goarm, bin.Binary)
		if cfg.If != "" {
			cond, err := tmpl.Render(cfg.If, data)
			if err != nil {
				return 0, fmt.Errorf("archive %s: %w", cfg.ID, err)
			}
			if !tmpl.Truthy(cond) {
				continue
			}
		}
		base, err := tmpl.Render(cfg.NameTemplate, data)
		if err != nil {
			return 0, fmt.Errorf("archive %s: %w", cfg.ID, err)
		}
		for _, format := range formatsFor(cfg, bin.Goos, bin.Goarch) {
			ext, normalized, err := formatExt(format, bin.Goos)
			if err != nil {
				return 0, fmt.Errorf("archive %s: %w", cfg.ID, err)
			}
			name := base
			if ext != "" && !strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
				name += ext
			}
			if strings.ContainsAny(name, `/\`) || name == "" {
				return 0, fmt.Errorf("archive %s: name %q must be a file name", cfg.ID, name)
			}
			if taken(c, name) {
				return 0, fmt.Errorf("duplicate artifact name %s", name)
			}
			wrap := cfg.Wrapped() && normalized != "binary"
			binName := bin.Binary
			if bin.Goos == "windows" && !strings.HasSuffix(strings.ToLower(binName), ".exe") {
				binName += ".exe"
			}
			inner := binName
			if wrap {
				inner = base + "/" + binName
			}
			entries := []archive.Entry{{Name: inner, Source: bin.Path, Mode: 0o755}}
			if normalized != "binary" {
				extra, err := extraFiles(c, cfg, base, wrap)
				if err != nil {
					return 0, err
				}
				entries = append(entries, extra...)
			}
			dst := filepath.Join(c.Dist, name)
			if err := archive.Create(dst, normalized, entries, c.CommitTime); err != nil {
				return 0, fmt.Errorf("archive %s: %w", name, err)
			}
			kind := artifact.TypeArchive
			if normalized == "binary" {
				kind = artifact.TypeBinary
			}
			if err := c.Add(&artifact.Artifact{
				Name:      name,
				Path:      dst,
				Type:      kind,
				Goos:      bin.Goos,
				Goarch:    bin.Goarch,
				Goarm:     bin.Goarm,
				Binary:    bin.Binary,
				BuildID:   bin.BuildID,
				ArchiveID: cfg.ID,
				Format:    normalized,
				Wrapped:   wrap,
				InnerPath: inner,
			}); err != nil {
				return 0, err
			}
			c.Log.Sub("%s", name)
			n++
		}
	}
	if n == 0 {
		c.Log.Sub("skipping archive %s: no matching binaries", cfg.ID)
	}
	return n, nil
}

func extraFiles(c *runctx.Context, cfg config.Archive, base string, wrap bool) ([]archive.Entry, error) {
	var entries []archive.Entry
	for _, f := range cfg.Files {
		rel, err := fsys.Rel(f.Src)
		if err != nil {
			return nil, fmt.Errorf("archive %s: %w", cfg.ID, err)
		}
		opened, err := fsys.OpenUnder(c.Dir, rel)
		if err != nil {
			return nil, fmt.Errorf("archive %s: %w", cfg.ID, err)
		}
		opened.Close()
		dst := f.Dst
		if dst == "" {
			dst = filepath.Base(rel)
		}
		dst = filepath.ToSlash(dst)
		if dst == "" || strings.Contains(dst, "..") || strings.HasPrefix(dst, "/") {
			return nil, fmt.Errorf("archive %s: file destination %q is not allowed", cfg.ID, f.Dst)
		}
		if wrap {
			dst = base + "/" + dst
		}
		entries = append(entries, archive.Entry{
			Name:   dst,
			Source: filepath.Join(c.Dir, rel),
		})
	}
	return entries, nil
}

func formatsFor(a config.Archive, goos, goarch string) []string {
	formats := a.Formats
	for _, o := range a.FormatOverrides {
		if o.Goos == "" && o.Goarch == "" {
			continue
		}
		if o.Goos != "" && o.Goos != goos {
			continue
		}
		if o.Goarch != "" && o.Goarch != goarch {
			continue
		}
		if len(o.Formats) > 0 {
			formats = o.Formats
		}
	}
	return formats
}

func formatExt(format, goos string) (ext, normalized string, err error) {
	switch format {
	case "tar.gz":
		return ".tar.gz", "tar.gz", nil
	case "tgz":
		return ".tgz", "tar.gz", nil
	case "zip":
		return ".zip", "zip", nil
	case "binary":
		if goos == "windows" {
			return ".exe", "binary", nil
		}
		return "", "binary", nil
	default:
		return "", "", fmt.Errorf("unknown format %q", format)
	}
}

func taken(c *runctx.Context, name string) bool {
	for _, a := range c.ArtifactsCopy() {
		if a.Name == name {
			return true
		}
	}
	return false
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
