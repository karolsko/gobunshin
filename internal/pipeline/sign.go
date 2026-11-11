package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// Sign runs the configured external signing commands.
func Sign(c *runctx.Context) error {
	if len(c.Project.Signs) == 0 {
		return nil
	}
	c.Log.Step("signing")
	for _, spec := range c.Project.Signs {
		for _, a := range c.ArtifactsCopy() {
			if !signWant(spec.Artifacts, a.Type) {
				continue
			}
			data := c.TargetData(a.Goos, a.Goarch, a.Goarm, a.Binary)
			if spec.If != "" {
				cond, err := tmpl.Render(spec.If, data)
				if err != nil {
					return fmt.Errorf("sign %s: %w", spec.ID, err)
				}
				if !tmpl.Truthy(cond) {
					continue
				}
			}
			sig, args, cmdName, err := signCommand(spec, a, data)
			if err != nil {
				return err
			}
			c.Log.Sub("%s %s", spec.ID, a.Name)
			c.Log.Verbose("%s %s", cmdName, strings.Join(args, " "))
			cmd := exec.CommandContext(c.Go, cmdName, args...)
			cmd.Dir = c.Dir
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("sign %s: %w\n%s", a.Name, err, strings.TrimSpace(string(out)))
			}
			if _, err := os.Stat(sig); err != nil {
				return fmt.Errorf("sign %s: %s did not create %s", spec.ID, cmdName, sig)
			}
			if err := c.Add(&artifact.Artifact{
				Name:    filepath.Base(sig),
				Path:    sig,
				Type:    artifact.TypeSignature,
				Goos:    a.Goos,
				Goarch:  a.Goarch,
				Binary:  a.Binary,
				BuildID: a.BuildID,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func signCommand(spec config.Sign, a *artifact.Artifact, data tmpl.Data) (string, []string, string, error) {
	replacer := strings.NewReplacer("${artifact}", a.Path, "${artifactName}", a.Name)
	sig, err := tmpl.Render(replacer.Replace(spec.Signature), data)
	if err != nil {
		return "", nil, "", fmt.Errorf("sign %s: %w", spec.ID, err)
	}
	cmdName, err := tmpl.Render(replacer.Replace(spec.Cmd), data)
	if err != nil {
		return "", nil, "", fmt.Errorf("sign %s: %w", spec.ID, err)
	}
	args := make([]string, len(spec.Args))
	for i, arg := range spec.Args {
		args[i], err = tmpl.Render(replacer.Replace(arg), data)
		if err != nil {
			return "", nil, "", fmt.Errorf("sign %s: %w", spec.ID, err)
		}
	}
	return sig, args, cmdName, nil
}

func signWant(kind string, t artifact.Type) bool {
	switch kind {
	case "", "archives":
		return t == artifact.TypeArchive || t == artifact.TypeBinary || t == artifact.TypePackage
	case "checksum":
		return t == artifact.TypeChecksum
	case "binary":
		return t == artifact.TypeBinary
	case "package":
		return t == artifact.TypePackage
	case "all":
		return t == artifact.TypeArchive || t == artifact.TypeBinary || t == artifact.TypePackage || t == artifact.TypeChecksum || t == artifact.TypeFile
	default:
		return false
	}
}
