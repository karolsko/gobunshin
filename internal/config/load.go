package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigNames is the lookup order for a project file in the working directory.
var ConfigNames = []string{
	".gobunshin.yaml",
	".gobunshin.yml",
	".config/gobunshin.yaml",
	".config/gobunshin.yml",
	"gobunshin.yaml",
	"gobunshin.yml",
}

// Find returns the config path. An explicit path is used as given.
func Find(dir, explicit string) (string, error) {
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Join(dir, explicit)
		}
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("config %s: %w", explicit, err)
		}
		return explicit, nil
	}
	for _, name := range ConfigNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no config file in %s (looked for %s)", dir, ConfigNames[0])
}

// Load reads a project file and merges includes.
// Included files are applied first. The including file wins on scalar fields.
// Slices of mappings that all have an id (builds, archives) merge by that id.
// Other slices are replaced when the including file sets them.
func Load(path string) (*Project, error) {
	merged, err := loadFile(path, map[string]struct{}{}, 0)
	if err != nil {
		return nil, err
	}
	if merged == nil {
		merged = map[string]any{}
	}
	stringifyGoarm(merged)
	raw, err := yaml.Marshal(merged)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var p Project
	if err := dec.Decode(&p); err != nil {
		if err == io.EOF {
			return &Project{}, nil
		}
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return &p, nil
}

func loadFile(path string, seen map[string]struct{}, depth int) (map[string]any, error) {
	if depth > 16 {
		return nil, fmt.Errorf("include depth exceeded at %s", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, ok := seen[abs]; ok {
		return nil, fmt.Errorf("include cycle at %s", path)
	}
	seen[abs] = struct{}{}
	defer delete(seen, abs)

	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, nil
	}
	var raw any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: config must be a mapping", path)
	}
	includes, err := includeList(m["includes"])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	delete(m, "includes")

	base := map[string]any{}
	dir := filepath.Dir(path)
	for _, inc := range includes {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(dir, inc)
		}
		child, err := loadFile(incPath, seen, depth+1)
		if err != nil {
			return nil, err
		}
		base = merge(base, child)
	}
	return merge(base, m), nil
}

func includeList(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		if t == "" {
			return nil, nil
		}
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, fmt.Errorf("includes entries must be paths")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("includes must be a path or a list of paths")
	}
}

func merge(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, sv := range src {
		dv, ok := dst[k]
		if !ok || dv == nil {
			dst[k] = sv
			continue
		}
		dm, dok := dv.(map[string]any)
		sm, sok := sv.(map[string]any)
		if dok && sok {
			dst[k] = merge(dm, sm)
			continue
		}
		ds, dok := dv.([]any)
		ss, sok := sv.([]any)
		if dok && sok && isIDSlice(ds) && isIDSlice(ss) {
			dst[k] = mergeByID(ds, ss)
			continue
		}
		dst[k] = sv
	}
	return dst
}

func isIDSlice(s []any) bool {
	if len(s) == 0 {
		return false
	}
	for _, item := range s {
		m, ok := item.(map[string]any)
		if !ok {
			return false
		}
		id, ok := m["id"].(string)
		if !ok || id == "" {
			return false
		}
	}
	return true
}

func mergeByID(base, overlay []any) []any {
	index := map[string]int{}
	out := make([]any, 0, len(base)+len(overlay))
	for _, item := range base {
		m := item.(map[string]any)
		id := m["id"].(string)
		index[id] = len(out)
		out = append(out, cloneMap(m))
	}
	for _, item := range overlay {
		m := item.(map[string]any)
		id := m["id"].(string)
		if i, ok := index[id]; ok {
			out[i] = merge(out[i].(map[string]any), m)
		} else {
			index[id] = len(out)
			out = append(out, cloneMap(m))
		}
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
