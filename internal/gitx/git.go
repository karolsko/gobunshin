// Package gitx reads the git facts a release needs.
package gitx

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Commit is one line of history.
type Commit struct {
	Hash    string
	Subject string
	Body    string
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// Head returns the full hash of HEAD.
func Head(dir string) (string, error) {
	return run(dir, "rev-parse", "HEAD")
}

// CommitTime returns the committer time of HEAD.
func CommitTime(dir string) (time.Time, error) {
	out, err := run(dir, "log", "-1", "--format=%cI")
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, out)
}

// Tags lists every tag.
func Tags(dir string) ([]string, error) {
	return lines(dir, "tag", "--list")
}

// TagsAtHead lists tags that point at HEAD.
func TagsAtHead(dir string) ([]string, error) {
	return lines(dir, "tag", "--list", "--points-at", "HEAD")
}

// RemoteURL returns origin, or the first remote when origin is missing.
func RemoteURL(dir string) (string, error) {
	out, err := run(dir, "remote", "get-url", "origin")
	if err == nil {
		return out, nil
	}
	remotes, rerr := lines(dir, "remote")
	if rerr != nil || len(remotes) == 0 {
		return "", err
	}
	return run(dir, "remote", "get-url", remotes[0])
}

// Log returns commits reachable from rev, newest first.
// rev is empty for the whole history, or a range such as "v1.0.0..HEAD".
// paths limit the log to those pathspecs. limit 0 means no limit.
func Log(dir, rev string, paths []string, limit int) ([]Commit, error) {
	args := []string{"log", "--pretty=format:%H%x1f%s%x1f%b%x1e"}
	if limit > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", limit))
	}
	if rev != "" {
		args = append(args, rev)
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if strings.Contains(msg, "does not have any commits") {
			return nil, nil
		}
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(msg))
	}
	text := strings.Trim(string(out), "\n\x1e")
	if text == "" {
		return nil, nil
	}
	var commits []Commit
	for _, record := range strings.Split(text, "\x1e") {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\x1f", 3)
		if len(parts) < 2 || parts[0] == "" {
			continue
		}
		commit := Commit{Hash: parts[0], Subject: strings.TrimSpace(parts[1])}
		if len(parts) == 3 {
			commit.Body = strings.TrimSpace(parts[2])
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

// CreateAnnotatedTag creates a tag at HEAD without GPG signing.
func CreateAnnotatedTag(dir, name, message string) error {
	cmd := exec.Command("git", "-c", "tag.gpgsign=false", "tag", "-a", name, "-m", message)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git tag %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// EnvTag returns the tag CI checked out, when git itself has no local tag.
func EnvTag() string {
	if ref := os.Getenv("GITHUB_REF"); strings.HasPrefix(ref, "refs/tags/") {
		return strings.TrimPrefix(ref, "refs/tags/")
	}
	return os.Getenv("CI_COMMIT_TAG")
}

func lines(dir string, args ...string) ([]string, error) {
	out, err := run(dir, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
