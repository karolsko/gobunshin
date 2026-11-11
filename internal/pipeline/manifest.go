package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/runctx"
)

// Manifest is the dist/artifacts.json file a later publish command reads.
type Manifest struct {
	Project   string             `json:"project"`
	Version   string             `json:"version"`
	Tag       string             `json:"tag"`
	Commit    string             `json:"commit"`
	Changelog string             `json:"changelog"`
	Snapshot  bool               `json:"snapshot"`
	Nightly   bool               `json:"nightly"`
	CreatedAt string             `json:"created_at"`
	Artifacts []ManifestArtifact `json:"artifacts"`
}

// ManifestArtifact is one file recorded in the manifest.
type ManifestArtifact struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	Goos      string `json:"goos,omitempty"`
	Goarch    string `json:"goarch,omitempty"`
	Goarm     string `json:"goarm,omitempty"`
	Binary    string `json:"binary,omitempty"`
	BuildID   string `json:"build_id,omitempty"`
	ArchiveID string `json:"archive_id,omitempty"`
	Format    string `json:"format,omitempty"`
	Wrapped   bool   `json:"wrapped,omitempty"`
	InnerPath string `json:"inner_path,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

// WriteManifest records the shippable artifacts.
func WriteManifest(c *runctx.Context) error {
	m := Manifest{
		Project:   c.Project.ProjectName,
		Version:   c.Version,
		Tag:       c.Tag,
		Commit:    c.Commit,
		Changelog: c.Changelog,
		Snapshot:  c.Snapshot,
		Nightly:   c.Nightly,
		CreatedAt: c.Now.UTC().Format(time.RFC3339),
	}
	for _, a := range c.ArtifactsCopy() {
		rel, err := filepath.Rel(c.Dir, a.Path)
		if err != nil {
			return err
		}
		m.Artifacts = append(m.Artifacts, ManifestArtifact{
			Name:      a.Name,
			Path:      filepath.ToSlash(rel),
			Type:      string(a.Type),
			Goos:      a.Goos,
			Goarch:    a.Goarch,
			Goarm:     a.Goarm,
			Binary:    a.Binary,
			BuildID:   a.BuildID,
			ArchiveID: a.ArchiveID,
			Format:    a.Format,
			Wrapped:   a.Wrapped,
			InnerPath: a.InnerPath,
			SHA256:    a.SHA256,
		})
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(filepath.Join(c.Dist, "artifacts.json"), body, 0o644)
}

// LoadManifest restores a previous dist/artifacts.json into the run.
func LoadManifest(c *runctx.Context) error {
	path := filepath.Join(c.Dist, "artifacts.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	c.Version = m.Version
	c.Tag = m.Tag
	c.Commit = m.Commit
	if len(m.Commit) >= 7 {
		c.ShortCommit = m.Commit[:7]
	} else {
		c.ShortCommit = m.Commit
	}
	c.Changelog = m.Changelog
	c.Snapshot = m.Snapshot
	c.Nightly = m.Nightly
	c.Artifacts = nil
	for _, item := range m.Artifacts {
		full := filepath.Join(c.Dir, filepath.FromSlash(item.Path))
		if _, err := os.Stat(full); err != nil {
			return fmt.Errorf("artifact %s: %w", item.Path, err)
		}
		c.Artifacts = append(c.Artifacts, &artifact.Artifact{
			Name:      item.Name,
			Path:      full,
			Type:      artifact.Type(item.Type),
			Goos:      item.Goos,
			Goarch:    item.Goarch,
			Goarm:     item.Goarm,
			Binary:    item.Binary,
			BuildID:   item.BuildID,
			ArchiveID: item.ArchiveID,
			Format:    item.Format,
			Wrapped:   item.Wrapped,
			InnerPath: item.InnerPath,
			SHA256:    item.SHA256,
		})
	}
	return nil
}
