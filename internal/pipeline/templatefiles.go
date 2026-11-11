package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/fsys"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

// TemplateFiles renders configured files into dist/ and publishes them.
func TemplateFiles(c *runctx.Context) error {
	if len(c.Project.TemplateFiles) == 0 {
		return nil
	}
	c.Log.Step("template files")
	for _, tf := range c.Project.TemplateFiles {
		body, err := fsys.ReadUnder(c.Dir, tf.Src)
		if err != nil {
			return err
		}
		rendered, err := tmpl.Render(string(body), c.Data())
		if err != nil {
			return fmt.Errorf("template file %s: %w", tf.Src, err)
		}
		dst := tf.Dst
		if dst == "" {
			dst = strings.TrimSuffix(filepath.Base(tf.Src), ".tmpl")
		}
		rel, err := fsys.Rel(dst)
		if err != nil {
			return fmt.Errorf("template file %s: %w", tf.Src, err)
		}
		out := filepath.Join(c.Dist, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, []byte(rendered), 0o644); err != nil {
			return err
		}
		name := filepath.Base(out)
		if err := c.Add(&artifact.Artifact{Name: name, Path: out, Type: artifact.TypeFile}); err != nil {
			return err
		}
		c.Log.Sub("%s", name)
	}
	return nil
}
