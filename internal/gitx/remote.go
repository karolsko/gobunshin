package gitx

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseRemote splits a git remote URL into host, owner, and repository.
func ParseRemote(raw string) (host, owner, name string, err error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".git")
	if raw == "" {
		return "", "", "", fmt.Errorf("empty git remote")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", "", err
		}
		parts := splitPath(u.Path)
		if len(parts) < 2 {
			return "", "", "", fmt.Errorf("cannot parse remote %q", raw)
		}
		return u.Hostname(), parts[0], strings.Join(parts[1:], "/"), nil
	}
	// git@github.com:owner/repo
	if at := strings.Index(raw, "@"); at >= 0 {
		rest := raw[at+1:]
		host, path, ok := strings.Cut(rest, ":")
		if !ok {
			return "", "", "", fmt.Errorf("cannot parse remote %q", raw)
		}
		parts := splitPath(path)
		if len(parts) < 2 {
			return "", "", "", fmt.Errorf("cannot parse remote %q", raw)
		}
		return host, parts[0], strings.Join(parts[1:], "/"), nil
	}
	return "", "", "", fmt.Errorf("cannot parse remote %q", raw)
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
