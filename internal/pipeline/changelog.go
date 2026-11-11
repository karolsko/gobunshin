package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/karolsko/gobunshin/internal/config"
	"github.com/karolsko/gobunshin/internal/gitx"
	"github.com/karolsko/gobunshin/internal/runctx"
)

// Changelog fills c.Changelog from git history and writes dist/changelog.md when dist is set.
func Changelog(c *runctx.Context) error {
	if c.Project.Changelog.Disable {
		c.Changelog = ""
		return nil
	}
	var commits []gitx.Commit
	if gitx.IsRepo(c.Dir) {
		rev := ""
		if c.PreviousTag != "" {
			rev = c.PreviousTag + "..HEAD"
		}
		var err error
		commits, err = gitx.Log(c.Dir, rev, c.Project.Changelog.Filters.IncludePaths, c.Project.Changelog.Limit)
		if err != nil {
			return err
		}
	}
	text, err := formatChangelog(commits, c.Project.Changelog)
	if err != nil {
		return err
	}
	c.Changelog = text
	if c.Dist != "" {
		if err := os.WriteFile(filepath.Join(c.Dist, "changelog.md"), []byte(text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func formatChangelog(commits []gitx.Commit, cfg config.Changelog) (string, error) {
	list := commits
	if cfg.Sort == "asc" {
		list = reverseCommits(commits)
	}
	exclude, err := compileAll(cfg.Filters.Exclude)
	if err != nil {
		return "", err
	}
	include, err := compileAll(cfg.Filters.Include)
	if err != nil {
		return "", err
	}
	var filtered []gitx.Commit
	for _, commit := range list {
		if commit.Subject == "" {
			continue
		}
		if matchesAny(exclude, commit.Subject) {
			continue
		}
		if len(include) > 0 && !matchesAny(include, commit.Subject) {
			continue
		}
		filtered = append(filtered, commit)
	}

	var b strings.Builder
	title := cfg.TitleText()
	if title != "" {
		b.WriteString(title)
		b.WriteString("\n")
	}
	if len(filtered) == 0 {
		if title != "" {
			b.WriteString("\n")
		}
		b.WriteString("No commits found.\n")
		return b.String(), nil
	}
	if len(cfg.Groups) == 0 {
		if title != "" {
			b.WriteString("\n")
		}
		for _, commit := range filtered {
			fmt.Fprintf(&b, "* %s (%s)\n", commit.Subject, shortHash(commit.Hash))
		}
		return b.String(), nil
	}

	regs := make([]*regexp.Regexp, len(cfg.Groups))
	for i, g := range cfg.Groups {
		regs[i], err = regexp.Compile(g.Regexp)
		if err != nil {
			return "", fmt.Errorf("changelog group %q: %w", g.Title, err)
		}
	}
	lines := make([][]string, len(cfg.Groups))
	var other []string
	for _, commit := range filtered {
		line := fmt.Sprintf("* %s (%s)", commit.Subject, shortHash(commit.Hash))
		matched := false
		for i, re := range regs {
			if re.MatchString(commit.Subject) {
				lines[i] = append(lines[i], line)
				matched = true
				break
			}
		}
		if !matched {
			other = append(other, line)
		}
	}
	first := true
	write := func(heading string, items []string) {
		if len(items) == 0 {
			return
		}
		if !first && cfg.Divider != "" {
			b.WriteString("\n")
			b.WriteString(cfg.Divider)
			b.WriteString("\n")
		}
		first = false
		b.WriteString("\n### ")
		b.WriteString(heading)
		b.WriteString("\n\n")
		for _, item := range items {
			b.WriteString(item)
			b.WriteString("\n")
		}
	}
	for i, g := range cfg.Groups {
		write(g.Title, lines[i])
	}
	write("Other", other)
	return b.String(), nil
}

func reverseCommits(in []gitx.Commit) []gitx.Commit {
	out := make([]gitx.Commit, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func compileAll(patterns []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("changelog filter %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func matchesAny(regs []*regexp.Regexp, s string) bool {
	for _, re := range regs {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
