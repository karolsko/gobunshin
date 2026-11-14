package pipeline

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/installer"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Installers builds macOS .pkg and .dmg images and Windows NSIS installers.
func Installers(c *runctx.Context) error {
	if len(c.Project.Installers) == 0 {
		return nil
	}
	c.Log.Step("installers")
	n := 0
	for _, spec := range c.Project.Installers {
		for _, bin := range c.BinariesCopy() {
			if len(spec.IDs) > 0 && !contains(spec.IDs, bin.BuildID) {
				continue
			}
			data := c.TargetData(bin.Goos, bin.Goarch, bin.Goarm, bin.Binary)
			if spec.If != "" {
				cond, err := tmpl.Render(spec.If, data)
				if err != nil {
					return fmt.Errorf("installer %s: %w", spec.ID, err)
				}
				if !tmpl.Truthy(cond) {
					continue
				}
			}
			for _, format := range spec.Formats {
				if !installerWants(format, bin.Goos) {
					continue
				}
				if err := oneInstaller(c, spec, bin, format); err != nil {
					return err
				}
				n++
			}
		}
	}
	if n == 0 {
		c.Log.Sub("no macOS or Windows binaries in this build")
	}
	return nil
}

func installerWants(format, goos string) bool {
	switch format {
	case "pkg", "dmg":
		return goos == "darwin"
	case "nsis":
		return goos == "windows"
	default:
		return false
	}
}

func oneInstaller(c *runctx.Context, spec config.Installer, bin *artifact.Artifact, format string) error {
	data := c.TargetData(bin.Goos, bin.Goarch, bin.Goarm, bin.Binary)
	name, err := tmpl.Render(spec.Name, data)
	if err != nil {
		return fmt.Errorf("installer %s: %w", spec.ID, err)
	}
	base, err := tmpl.Render(spec.FileNameTemplate, data)
	if err != nil {
		return fmt.Errorf("installer %s: %w", spec.ID, err)
	}
	switch format {
	case "pkg":
		return writePkg(c, spec, bin, name, base)
	case "dmg":
		return writeDmg(c, spec, bin, name, base)
	case "nsis":
		return writeNSIS(c, spec, bin, name, base)
	default:
		return fmt.Errorf("installer %s: unknown format %q", spec.ID, format)
	}
}

func writePkg(c *runctx.Context, spec config.Installer, bin *artifact.Artifact, name, base string) error {
	data := c.TargetData(bin.Goos, bin.Goarch, bin.Goarm, bin.Binary)
	data.PackageName = name
	ident, err := tmpl.Render(spec.Identifier, data)
	if err != nil {
		return fmt.Errorf("installer %s: %w", spec.ID, err)
	}
	bindir, err := tmpl.Render(spec.Bindir, data)
	if err != nil {
		return fmt.Errorf("installer %s: %w", spec.ID, err)
	}
	if !strings.HasPrefix(bindir, "/") {
		return fmt.Errorf("installer %s: bindir %q must be absolute", spec.ID, bindir)
	}
	body, err := os.ReadFile(bin.Path)
	if err != nil {
		return err
	}
	dest := path.Join(bindir, bin.Binary)
	pkg, err := installer.Pkg(ident, c.Version, []installer.File{{
		Path: dest,
		Data: body,
	}}, c.CommitTime)
	if err != nil {
		return fmt.Errorf("installer %s: %w", spec.ID, err)
	}
	filename := base + ".pkg"
	return addInstaller(c, spec, bin, "pkg", filename, pkg)
}

func writeDmg(c *runctx.Context, spec config.Installer, bin *artifact.Artifact, name, base string) error {
	body, err := os.ReadFile(bin.Path)
	if err != nil {
		return err
	}
	img, err := installer.Dmg(name, []installer.File{{
		Path: bin.Binary,
		Data: body,
		Mode: 0o755,
	}}, c.CommitTime)
	if err != nil {
		return fmt.Errorf("installer %s: %w", spec.ID, err)
	}
	return addInstaller(c, spec, bin, "dmg", base+".dmg", img)
}

func writeNSIS(c *runctx.Context, spec config.Installer, bin *artifact.Artifact, name, base string) error {
	filename := base + "-setup.exe"
	if strings.ContainsAny(filename, `/\`) || filename == "-setup.exe" {
		return fmt.Errorf("installer %s: name %q is not a file name", spec.ID, filename)
	}
	dst := filepath.Join(c.Dist, filename)
	script := installer.NSISScript(name, c.Version, bin.Goarch, bin.Path, dst)
	if err := installer.NSIS(script, dst); err != nil {
		return fmt.Errorf("installer %s: %w", spec.ID, err)
	}
	if err := c.Add(&artifact.Artifact{
		Name:      filename,
		Path:      dst,
		Type:      artifact.TypePackage,
		Goos:      bin.Goos,
		Goarch:    bin.Goarch,
		Goarm:     bin.Goarm,
		Binary:    bin.Binary,
		BuildID:   bin.BuildID,
		ArchiveID: spec.ID,
		Format:    "nsis",
	}); err != nil {
		return err
	}
	c.Log.Sub("%s", filename)
	return nil
}

func addInstaller(c *runctx.Context, spec config.Installer, bin *artifact.Artifact, format, filename string, body []byte) error {
	if strings.ContainsAny(filename, `/\`) || filename == "" {
		return fmt.Errorf("installer %s: name %q is not a file name", spec.ID, filename)
	}
	dst := filepath.Join(c.Dist, filename)
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return err
	}
	if err := c.Add(&artifact.Artifact{
		Name:      filename,
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
	c.Log.Sub("%s", filename)
	return nil
}
