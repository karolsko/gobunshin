// Package fsys opens project files and rejects paths that leave the project root.
package fsys

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Rel cleans name and reports whether it stays inside the project.
func Rel(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("path must be relative: %s", name)
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the project: %s", name)
	}
	return clean, nil
}

// OpenUnder opens a file that must live inside root.
func OpenUnder(root, name string) (*os.File, error) {
	rel, err := Rel(name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(root, rel))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if st.IsDir() {
		f.Close()
		return nil, fmt.Errorf("%s is a directory", name)
	}
	return f, nil
}

// ReadUnder reads a file that must live inside root.
func ReadUnder(root, name string) ([]byte, error) {
	f, err := OpenUnder(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return b, nil
}
