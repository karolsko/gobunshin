package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/fsys"
	"github.com/karolsko/gobunshin/internal/runctx"
)

// Hooks runs shell commands. label is the log line for the group.
func Hooks(c *runctx.Context, hooks []config.Hook, label string) error {
	if c.SkipHooks || len(hooks) == 0 {
		return nil
	}
	c.Log.Step("%s", label)
	for _, h := range hooks {
		command, err := c.Render(h.Cmd)
		if err != nil {
			return fmt.Errorf("hook: %w", err)
		}
		dir := c.Dir
		if h.Dir == "" && c.Project.Monorepo.Dir != "" {
			dir = filepath.Join(c.Dir, c.Project.Monorepo.Dir)
		}
		if h.Dir != "" {
			rel, err := fsys.Rel(h.Dir)
			if err != nil {
				return err
			}
			dir = filepath.Join(c.Dir, rel)
		}
		c.Log.Sub("%s", command)
		if err := runShell(c, dir, command); err != nil {
			return fmt.Errorf("hook %q: %w", command, err)
		}
	}
	return nil
}

func runShell(c *runctx.Context, dir, command string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(c.Go, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(c.Go, "sh", "-c", command)
	}
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOBUNSHIN_VERSION="+c.Version,
		"GOBUNSHIN_TAG="+c.Tag,
		"GOBUNSHIN_COMMIT="+c.Commit,
	)
	out, err := cmd.CombinedOutput()
	if len(bytesTrim(out)) > 0 {
		c.Log.Verbose("%s", strings.TrimSpace(string(out)))
	}
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func bytesTrim(b []byte) string {
	return strings.TrimSpace(string(b))
}
