// Package githubx talks to the GitHub release API.
package githubx

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrNotFound is returned when GitHub answers 404.
var ErrNotFound = errors.New("not found")

// Client is a small GitHub REST client.
type Client struct {
	API    string
	Upload string
	Token  string
	HTTP   *http.Client
}

// Release is a GitHub release.
type Release struct {
	ID        int64  `json:"id"`
	TagName   string `json:"tag_name"`
	UploadURL string `json:"upload_url"`
	HTMLURL   string `json:"html_url"`
}

// ReleaseRequest creates a release.
type ReleaseRequest struct {
	TagName         string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish,omitempty"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	Draft           bool   `json:"draft"`
	Prerelease      bool   `json:"prerelease"`
}

// New builds a client for github.com or a GitHub Enterprise host.
func New(host, token string) *Client {
	api := "https://api.github.com"
	upload := "https://uploads.github.com"
	if host != "" && host != "github.com" {
		api = "https://" + host + "/api/v3"
		upload = api
	}
	return &Client{
		API:    strings.TrimRight(api, "/"),
		Upload: strings.TrimRight(upload, "/"),
		Token:  token,
		HTTP:   &http.Client{Timeout: 2 * time.Minute},
	}
}

// ReleaseByTag fetches the release for a tag.
func (c *Client) ReleaseByTag(ctx context.Context, owner, repo, tag string) (Release, bool, error) {
	var rel Release
	_, err := c.do(ctx, http.MethodGet, c.API+"/repos/"+owner+"/"+repo+"/releases/tags/"+url.PathEscape(tag), nil, &rel)
	if errors.Is(err, ErrNotFound) {
		return Release{}, false, nil
	}
	if err != nil {
		return Release{}, false, err
	}
	return rel, true, nil
}

// DeleteRelease deletes a release. The git tag is left in place.
func (c *Client) DeleteRelease(ctx context.Context, owner, repo string, id int64) error {
	_, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("%s/repos/%s/%s/releases/%d", c.API, owner, repo, id), nil, nil)
	return err
}

// CreateRelease creates a release and returns it.
func (c *Client) CreateRelease(ctx context.Context, owner, repo string, req ReleaseRequest) (Release, error) {
	var rel Release
	_, err := c.do(ctx, http.MethodPost, c.API+"/repos/"+owner+"/"+repo+"/releases", req, &rel)
	if err != nil {
		return Release{}, err
	}
	rel.UploadURL = strings.Split(rel.UploadURL, "{")[0]
	return rel, nil
}

// UploadAsset uploads a file to a release. uploadURL is the release upload endpoint without the query string.
func (c *Client) UploadAsset(ctx context.Context, uploadURL, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(uploadURL, "/") + "?name=" + url.QueryEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, f)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size()
	c.decorate(req)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return statusError(resp)
	}
	return nil
}

// EnsureRef moves refs/tags/tag to sha, creating the ref when it is missing.
func (c *Client) EnsureRef(ctx context.Context, owner, repo, tag, sha string) error {
	patch := c.API + "/repos/" + owner + "/" + repo + "/git/refs/tags/" + url.PathEscape(tag)
	_, err := c.do(ctx, http.MethodPatch, patch, map[string]string{"sha": sha}, nil)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	body := map[string]string{"ref": "refs/tags/" + tag, "sha": sha}
	_, err = c.do(ctx, http.MethodPost, c.API+"/repos/"+owner+"/"+repo+"/git/refs", body, nil)
	return err
}

// PutFile creates or updates a file via the contents API.
// A file whose blob is already identical is left untouched.
func (c *Client) PutFile(ctx context.Context, owner, repo, path, branch, message string, content []byte) error {
	escaped := escapePath(path)
	getURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", c.API, owner, repo, escaped, url.QueryEscape(branch))
	var current struct {
		SHA string `json:"sha"`
	}
	_, err := c.do(ctx, http.MethodGet, getURL, nil, &current)
	sha := ""
	switch {
	case err == nil:
		if current.SHA == gitBlobSHA(content) {
			return nil
		}
		sha = current.SHA
	case errors.Is(err, ErrNotFound):
	default:
		return err
	}
	body := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
	}
	if sha != "" {
		body["sha"] = sha
	}
	putURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.API, owner, repo, escaped)
	_, err = c.do(ctx, http.MethodPut, putURL, body, nil)
	return err
}

func (c *Client) do(ctx context.Context, method, endpoint string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return 0, err
	}
	c.decorate(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, statusError(resp)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (c *Client) decorate(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gobunshin")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

func statusError(resp *http.Response) error {
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	msg := strings.TrimSpace(string(buf))
	if msg == "" {
		return fmt.Errorf("github %s %s: %s", resp.Request.Method, resp.Request.URL.Path, resp.Status)
	}
	return fmt.Errorf("github %s %s: %s: %s", resp.Request.Method, resp.Request.URL.Path, resp.Status, msg)
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func gitBlobSHA(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	_, _ = h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))
}
