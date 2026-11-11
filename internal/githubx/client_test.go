package githubx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestReleaseUploadAndReplace(t *testing.T) {
	var uploaded string
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing token")
		}
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/releases/tags/"):
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releases"):
			var req ReleaseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.TagName != "v1.2.3" || req.Body == "" {
				t.Errorf("request %+v", req)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(Release{
				ID:        7,
				TagName:   req.TagName,
				HTMLURL:   "https://github.com/acme/app/releases/v1.2.3",
				UploadURL: "http://" + r.Host + "/repos/acme/app/releases/7/assets{?name,label}",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/assets"):
			body, _ := io.ReadAll(r.Body)
			uploaded = string(body)
			if r.URL.Query().Get("name") != "app.tar.gz" {
				t.Errorf("name %s", r.URL.Query().Get("name"))
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/git/refs/tags/"):
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/refs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("github.com", "test-token")
	c.API = srv.URL
	c.HTTP = srv.Client()
	ctx := context.Background()

	_, found, err := c.ReleaseByTag(ctx, "acme", "app", "v1.2.3")
	if err != nil || found {
		t.Fatalf("found %v err %v", found, err)
	}
	rel, err := c.CreateRelease(ctx, "acme", "app", ReleaseRequest{TagName: "v1.2.3", Name: "v1.2.3", Body: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/app.tar.gz"
	if err := os.WriteFile(path, []byte("archive-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.UploadAsset(ctx, rel.UploadURL, "app.tar.gz", path); err != nil {
		t.Fatal(err)
	}
	if uploaded != "archive-bytes" {
		t.Fatalf("uploaded %q", uploaded)
	}
	if err := c.DeleteRelease(ctx, "acme", "app", rel.ID); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("not deleted")
	}
	if err := c.EnsureRef(ctx, "acme", "app", "nightly", "abc123"); err != nil {
		t.Fatal(err)
	}
}

func TestPutFileSkipsIdentical(t *testing.T) {
	content := []byte("class Demo < Formula\nend\n")
	puts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"sha": gitBlobSHA(content)})
		case http.MethodPut:
			puts++
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New("", "tok")
	c.API = srv.URL
	c.HTTP = srv.Client()
	if err := c.PutFile(context.Background(), "acme", "tap", "Formula/demo.rb", "main", "update", content); err != nil {
		t.Fatal(err)
	}
	if puts != 0 {
		t.Fatalf("puts %d", puts)
	}
}
