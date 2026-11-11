package pipeline

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/runctx"
)

// Checksum writes the checksum file and records a sha256 for every shippable file.
func Checksum(c *runctx.Context) error {
	arts := hashable(c.ArtifactsCopy())
	sort.Slice(arts, func(i, j int) bool { return arts[i].Name < arts[j].Name })
	for _, a := range arts {
		sum, err := hashFile(a.Path, "sha256")
		if err != nil {
			return err
		}
		a.SHA256 = sum
	}
	if c.Project.Checksum.Disable {
		return nil
	}
	name, err := c.Render(c.Project.Checksum.NameTemplate)
	if err != nil {
		return fmt.Errorf("checksum name: %w", err)
	}
	if taken(c, name) {
		return fmt.Errorf("duplicate artifact name %s", name)
	}
	c.Log.Step("checksums")
	var lines []string
	for _, a := range arts {
		sum := a.SHA256
		if c.Project.Checksum.Algorithm != "sha256" {
			sum, err = hashFile(a.Path, c.Project.Checksum.Algorithm)
			if err != nil {
				return err
			}
		}
		lines = append(lines, fmt.Sprintf("%s  %s", sum, a.Name))
	}
	body := ""
	if len(lines) > 0 {
		body = strings.Join(lines, "\n") + "\n"
	}
	path := filepath.Join(c.Dist, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	if err := c.Add(&artifact.Artifact{
		Name: name,
		Path: path,
		Type: artifact.TypeChecksum,
	}); err != nil {
		return err
	}
	c.Log.Sub("%s", name)
	return nil
}

func hashable(arts []*artifact.Artifact) []*artifact.Artifact {
	var out []*artifact.Artifact
	for _, a := range arts {
		switch a.Type {
		case artifact.TypeArchive, artifact.TypeBinary, artifact.TypeFile, artifact.TypePackage:
			out = append(out, a)
		}
	}
	return out
}

func hashFile(path, algo string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var h hash.Hash
	switch algo {
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return "", fmt.Errorf("unknown checksum algorithm %s", algo)
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
