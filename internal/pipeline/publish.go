package pipeline

import (
	"fmt"
	"strings"

	"github.com/karolsko/gobunshin/internal/artifact"
	"github.com/karolsko/gobunshin/internal/fsys"
	"github.com/karolsko/gobunshin/internal/githubx"
	"github.com/karolsko/gobunshin/internal/runctx"
)

// Publish creates the GitHub release and uploads shippable artifacts.
func Publish(c *runctx.Context) error {
	if c.Snapshot {
		return fmt.Errorf("refusing to publish a snapshot")
	}
	if c.Owner == "" || c.Name == "" {
		return fmt.Errorf("release.github.owner and name are required to publish")
	}
	if c.Token == "" {
		return fmt.Errorf("set GITHUB_TOKEN or GH_TOKEN to publish")
	}
	where := c.Host
	if where == "" {
		where = "github.com"
	}
	c.Log.Step("publishing to %s/%s/%s", where, c.Owner, c.Name)
	client := githubClient(c)
	replace := c.Nightly || c.Project.Release.Mode == "replace"
	if replace {
		if err := client.EnsureRef(c.Go, c.Owner, c.Name, c.Tag, c.Commit); err != nil {
			return fmt.Errorf("update tag %s: %w", c.Tag, err)
		}
	}
	existing, found, err := client.ReleaseByTag(c.Go, c.Owner, c.Name, c.Tag)
	if err != nil {
		return err
	}
	if found {
		if !replace {
			return fmt.Errorf("release %s already exists; set release.mode: replace to overwrite it", c.Tag)
		}
		if err := client.DeleteRelease(c.Go, c.Owner, c.Name, existing.ID); err != nil {
			return fmt.Errorf("delete release %s: %w", c.Tag, err)
		}
	}
	body, err := releaseNotes(c)
	if err != nil {
		return err
	}
	name, err := releaseName(c)
	if err != nil {
		return err
	}
	req := githubx.ReleaseRequest{
		TagName:    c.Tag,
		Name:       name,
		Body:       body,
		Draft:      c.Project.Release.Draft,
		Prerelease: prerelease(c),
	}
	if looksLikeCommit(c.Commit) {
		req.TargetCommitish = c.Commit
	}
	rel, err := client.CreateRelease(c.Go, c.Owner, c.Name, req)
	if err != nil {
		return err
	}
	if rel.HTMLURL != "" {
		c.Log.Sub("%s", rel.HTMLURL)
	}
	uploadURL := rel.UploadURL
	if uploadURL == "" {
		uploadURL = fmt.Sprintf("%s/repos/%s/%s/releases/%d/assets", client.Upload, c.Owner, c.Name, rel.ID)
	}
	for _, a := range uploadable(c.ArtifactsCopy()) {
		if err := client.UploadAsset(c.Go, uploadURL, a.Name, a.Path); err != nil {
			return fmt.Errorf("upload %s: %w", a.Name, err)
		}
		c.Log.Sub("%s", a.Name)
	}
	return nil
}

func githubClient(c *runctx.Context) *githubx.Client {
	client := githubx.New(c.Host, c.Token)
	if c.APIBase != "" {
		client.API = strings.TrimRight(c.APIBase, "/")
		client.Upload = client.API
	}
	return client
}

func releaseNotes(c *runctx.Context) (string, error) {
	header, err := section(c, c.Project.Release.Header, c.Project.Release.HeaderFile)
	if err != nil {
		return "", err
	}
	footer, err := section(c, c.Project.Release.Footer, c.Project.Release.FooterFile)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if strings.TrimSpace(header) != "" {
		b.WriteString(strings.TrimSpace(header))
		b.WriteString("\n\n")
	}
	b.WriteString(strings.TrimSpace(c.Changelog))
	if strings.TrimSpace(footer) != "" {
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(footer))
	}
	b.WriteString("\n")
	return b.String(), nil
}

func section(c *runctx.Context, inline, file string) (string, error) {
	var parts []string
	if strings.TrimSpace(inline) != "" {
		rendered, err := c.Render(inline)
		if err != nil {
			return "", err
		}
		parts = append(parts, rendered)
	}
	if file != "" {
		body, err := fsys.ReadUnder(c.Dir, file)
		if err != nil {
			return "", err
		}
		rendered, err := c.Render(string(body))
		if err != nil {
			return "", err
		}
		parts = append(parts, rendered)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n")), nil
}

func releaseName(c *runctx.Context) (string, error) {
	src := c.Project.Release.NameTemplate
	if c.Nightly {
		src = c.Project.Nightly.NameTemplate
	}
	return c.Render(src)
}

func prerelease(c *runctx.Context) bool {
	switch c.Project.Release.Prerelease {
	case "true":
		return true
	case "false":
		return false
	default:
		return c.Nightly || strings.Contains(strings.Split(c.Version, "+")[0], "-")
	}
}

func uploadable(arts []*artifact.Artifact) []*artifact.Artifact {
	var out []*artifact.Artifact
	for _, a := range arts {
		switch a.Type {
		case artifact.TypeArchive, artifact.TypeBinary, artifact.TypePackage, artifact.TypeChecksum, artifact.TypeSignature, artifact.TypeFile:
			out = append(out, a)
		}
	}
	return out
}

func looksLikeCommit(s string) bool {
	if len(s) < 7 || s == "none" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
