// Package cli is the gobunshin command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/release"
)

// BuildInfo is stamped into the binary with -X.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Main runs gobunshin and returns the process exit code.
func Main(args []string, info BuildInfo) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, usage)
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		if info.Version == "" {
			info.Version = "dev"
		}
		if info.Commit == "" {
			info.Commit = "none"
		}
		if info.Date == "" {
			info.Date = "unknown"
		}
		fmt.Printf("gobunshin %s commit %s built %s\n", info.Version, info.Commit, info.Date)
		return 0
	}
	cmd := args[0]
	switch cmd {
	case "init", "check", "build", "release", "publish", "changelog":
	default:
		fmt.Fprintf(os.Stderr, "gobunshin: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		configPath  string
		snapshot    bool
		nightly     bool
		single      bool
		skipPublish bool
		skipHooks   bool
		skipClean   bool
		force       bool
		verbose     bool
		parallel    int
		timeout     time.Duration
		ids         stringList
	)
	fs.StringVar(&configPath, "config", "", "config file")
	fs.BoolVar(&snapshot, "snapshot", false, "build an untagged snapshot; snapshots are never published")
	fs.BoolVar(&nightly, "nightly", false, "publish a moving nightly release")
	fs.BoolVar(&single, "single-target", false, "build only for this machine")
	fs.BoolVar(&skipPublish, "skip-publish", false, "build artifacts without uploading them")
	fs.BoolVar(&skipHooks, "skip-hooks", false, "skip before and after hooks")
	fs.BoolVar(&skipClean, "skip-clean", false, "do not remove dist/ first")
	fs.BoolVar(&force, "force", false, "overwrite an existing config (init)")
	fs.BoolVar(&verbose, "verbose", false, "log commands")
	fs.IntVar(&parallel, "parallel", 0, "parallel builds (default: the number of CPUs)")
	fs.DurationVar(&timeout, "timeout", 30*time.Minute, "maximum time for the command")
	fs.Var(&ids, "id", "build id to include (repeatable)")
	if err := fs.Parse(normalize(args[1:])); err != nil {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "gobunshin: unexpected argument %q\n\n%s", fs.Arg(0), usage)
		return 2
	}
	if cmd == "init" {
		if err := runInit(force); err != nil {
			fmt.Fprintf(os.Stderr, "gobunshin: %s\n", err)
			return 1
		}
		return 0
	}
	err := release.Execute(context.Background(), release.Options{
		Config:       configPath,
		Snapshot:     snapshot,
		Nightly:      nightly,
		SingleTarget: single,
		IDs:          []string(ids),
		SkipPublish:  skipPublish,
		SkipHooks:    skipHooks,
		SkipClean:    skipClean,
		Timeout:      timeout,
		Verbose:      verbose,
		Parallel:     parallel,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
	}, cmd)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "gobunshin: timed out after %s\n", timeout)
			return 1
		}
		fmt.Fprintf(os.Stderr, "gobunshin: %s\n", err)
		return 1
	}
	return 0
}

func runInit(force bool) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ".gobunshin.yaml")
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists; pass --force to overwrite it", path)
	}
	if err := os.WriteFile(path, []byte(config.Scaffold(dir)), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "wrote %s\n", path)
	return nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func normalize(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "-f":
			out = append(out, "--config")
		case strings.HasPrefix(a, "-f="):
			out = append(out, "--config="+strings.TrimPrefix(a, "-f="))
		default:
			out = append(out, a)
		}
	}
	return out
}

const usage = `gobunshin is a fully free release tool for Go projects.

Usage:
  gobunshin <command> [flags]

Commands:
  init         Write a starter .gobunshin.yaml
  check        Validate the configuration
  build        Cross-compile binaries
  release      Build, package, and publish a release
  publish      Publish artifacts already built in dist/
  changelog    Print the changelog for the current or next release
  version      Print the gobunshin version

Flags:
  -f, --config string    Config file
      --snapshot         Untagged snapshot build; never published
      --nightly          Moving nightly release
      --single-target    Build only for this machine
      --id string        Build id to include (repeatable)
      --skip-publish     Build artifacts without uploading them
      --skip-hooks       Skip before and after hooks
      --skip-clean       Do not remove dist/ first
      --timeout duration Maximum time (default 30m)
      --parallel int     Parallel builds (default: the number of CPUs)
      --verbose          Log commands
      --force            Overwrite an existing config (init)

Set GITHUB_TOKEN or GH_TOKEN to publish. There is no license key and no phone-home.
`
