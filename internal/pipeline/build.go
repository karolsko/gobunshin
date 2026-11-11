package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/fsys"
	"github.com/karolsko/gobunshin/internal/runctx"
	"github.com/karolsko/gobunshin/internal/tmpl"
)

type buildJob struct {
	build  config.Build
	target artifact.Target
	binary string
	out    string
}

// Targets expands a build into GOOS/GOARCH pairs. single keeps only hostOS/hostArch.
func Targets(b config.Build, single bool, hostOS, hostArch string) ([]artifact.Target, error) {
	var out []artifact.Target
	for _, goos := range b.Goos {
		for _, goarch := range b.Goarch {
			arms := []string{""}
			if goarch == "arm" {
				arms = b.Goarm
				if len(arms) == 0 {
					arms = []string{"7"}
				}
			}
			for _, arm := range arms {
				t := artifact.Target{Goos: goos, Goarch: goarch, Goarm: arm}
				if ignored(b.Ignore, t) {
					continue
				}
				out = append(out, t)
			}
		}
	}
	if !single {
		return out, nil
	}
	var kept []artifact.Target
	for _, t := range out {
		if t.Goos == hostOS && t.Goarch == hostArch {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("build %s has no %s/%s target", b.ID, hostOS, hostArch)
	}
	return kept, nil
}

func ignored(list []config.Ignore, t artifact.Target) bool {
	for _, ig := range list {
		if ig.Goos == "" && ig.Goarch == "" && ig.Goarm == "" {
			continue
		}
		if ig.Goos != "" && ig.Goos != t.Goos {
			continue
		}
		if ig.Goarch != "" && ig.Goarch != t.Goarch {
			continue
		}
		if ig.Goarm != "" && ig.Goarm != t.Goarm {
			continue
		}
		return true
	}
	return false
}

// Build compiles or copies every selected binary into dist/build.
func Build(c *runctx.Context) error {
	builds, err := selectedBuilds(c)
	if err != nil {
		return err
	}
	var jobs []buildJob
	for _, b := range builds {
		targets, err := Targets(b, c.SingleTarget, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return err
		}
		for _, t := range targets {
			data := c.TargetData(t.Goos, t.Goarch, t.Goarm, b.Binary)
			if b.If != "" {
				cond, err := tmpl.Render(b.If, data)
				if err != nil {
					return fmt.Errorf("build %s: %w", b.ID, err)
				}
				if !tmpl.Truthy(cond) {
					continue
				}
			}
			binary, err := tmpl.Render(b.Binary, data)
			if err != nil {
				return fmt.Errorf("build %s: %w", b.ID, err)
			}
			if binary == "" || strings.ContainsAny(binary, `/\`) {
				return fmt.Errorf("build %s: binary name %q is not a single path segment", b.ID, binary)
			}
			folder := fmt.Sprintf("%s_%s_%s", b.ID, t.Goos, t.Goarch)
			if t.Goarch == "arm" && t.Goarm != "" {
				folder += "v" + t.Goarm
			}
			jobs = append(jobs, buildJob{
				build:  b,
				target: t,
				binary: binary,
				out:    filepath.Join(c.Dist, "build", folder, binary+t.Ext()),
			})
		}
	}
	if len(jobs) == 0 {
		return fmt.Errorf("no build targets selected")
	}
	c.Log.Step("building %d binaries", len(jobs))
	return runParallel(c.Go, c.Parallel, len(jobs), func(i int) error {
		j := jobs[i]
		c.Log.Sub("%s %s", j.build.ID, j.target)
		if err := os.MkdirAll(filepath.Dir(j.out), 0o755); err != nil {
			return err
		}
		switch j.build.Builder {
		case "prebuilt":
			if err := copyPrebuilt(c, j); err != nil {
				return err
			}
		default:
			if err := goBuild(c, j); err != nil {
				return err
			}
		}
		c.AddBinary(&artifact.Artifact{
			Name:    filepath.Base(j.out),
			Path:    j.out,
			Type:    artifact.TypeBinary,
			Goos:    j.target.Goos,
			Goarch:  j.target.Goarch,
			Goarm:   j.target.Goarm,
			Binary:  j.binary,
			BuildID: j.build.ID,
		})
		return nil
	})
}

func selectedBuilds(c *runctx.Context) ([]config.Build, error) {
	if len(c.IDs) == 0 {
		return c.Project.Builds, nil
	}
	want := map[string]bool{}
	for _, id := range c.IDs {
		want[id] = true
	}
	var out []config.Build
	for _, b := range c.Project.Builds {
		if want[b.ID] {
			out = append(out, b)
			delete(want, b.ID)
		}
	}
	for id := range want {
		return nil, fmt.Errorf("unknown build id %q", id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no builds match --id")
	}
	return out, nil
}

func goBuild(c *runctx.Context, j buildJob) error {
	data := c.TargetData(j.target.Goos, j.target.Goarch, j.target.Goarm, j.binary)
	args := []string{"build", "-o", j.out}
	args = append(args, j.build.Flags...)
	if len(j.build.Ldflags) > 0 {
		var flags []string
		for _, s := range j.build.Ldflags {
			rendered, err := tmpl.Render(s, data)
			if err != nil {
				return fmt.Errorf("build %s: %w", j.build.ID, err)
			}
			flags = append(flags, rendered)
		}
		args = append(args, "-ldflags", strings.Join(flags, " "))
	}
	args = append(args, j.build.Main)
	cmd := exec.CommandContext(c.Go, "go", args...)
	cmd.Dir = workingDir(c, j.build)
	cmd.Env = append(os.Environ(), "GOOS="+j.target.Goos, "GOARCH="+j.target.Goarch)
	if j.target.Goarm != "" {
		cmd.Env = append(cmd.Env, "GOARM="+j.target.Goarm)
	}
	for _, e := range j.build.Env {
		rendered, err := tmpl.Render(e, data)
		if err != nil {
			return fmt.Errorf("build %s: %w", j.build.ID, err)
		}
		if !strings.Contains(rendered, "=") {
			return fmt.Errorf("build %s: env %q is not KEY=value", j.build.ID, rendered)
		}
		cmd.Env = append(cmd.Env, rendered)
	}
	c.Log.Verbose("go %s", strings.Join(args, " "))
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 4000 {
			msg = msg[len(msg)-4000:]
		}
		return fmt.Errorf("build %s %s: %w\n%s", j.build.ID, j.target, err, msg)
	}
	return nil
}

func copyPrebuilt(c *runctx.Context, j buildJob) error {
	data := c.TargetData(j.target.Goos, j.target.Goarch, j.target.Goarm, j.binary)
	src, err := tmpl.Render(j.build.Prebuilt.Path, data)
	if err != nil {
		return fmt.Errorf("build %s: %w", j.build.ID, err)
	}
	in, err := fsys.OpenUnder(c.Dir, src)
	if err != nil {
		return fmt.Errorf("build %s: %w", j.build.ID, err)
	}
	defer in.Close()
	out, err := os.OpenFile(j.out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		_ = out.Close()
		return fmt.Errorf("build %s: %w", j.build.ID, err)
	}
	return out.Close()
}

func workingDir(c *runctx.Context, b config.Build) string {
	if b.Dir != "" {
		return filepath.Join(c.Dir, b.Dir)
	}
	if c.Project.Monorepo.Dir != "" {
		return filepath.Join(c.Dir, c.Project.Monorepo.Dir)
	}
	return c.Dir
}
