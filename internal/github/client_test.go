package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// withTestServer temporarily redirects apiBase and allowedHostSuffix to the given
// httptest server and restores them after the test.
func withTestServer(t *testing.T, ts *httptest.Server) {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}
	origBase, origSuffix := apiBase, allowedHostSuffix
	apiBase = ts.URL
	allowedHostSuffix = u.Host
	t.Cleanup(func() {
		apiBase = origBase
		allowedHostSuffix = origSuffix
	})
}

func TestLatestRelease_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(release{TagName: "v1.2.3"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	withTestServer(t, ts)

	tag, err := LatestRelease("owner", "repo")
	if err != nil {
		t.Fatalf("LatestRelease() unexpected error: %v", err)
	}
	if tag != "v1.2.3" {
		t.Errorf("LatestRelease() = %q, want %q", tag, "v1.2.3")
	}
}

func TestLatestRelease_EmptyTag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(release{TagName: ""}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	withTestServer(t, ts)

	_, err := LatestRelease("owner", "repo")
	if err == nil {
		t.Error("LatestRelease() expected error for empty tag_name, got nil")
	}
}

func TestLatestRelease_HTTP404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()
	withTestServer(t, ts)

	_, err := LatestRelease("owner", "repo")
	if err == nil {
		t.Error("LatestRelease() expected error for HTTP 404, got nil")
	}
}

func TestLatestRelease_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, "not json{"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	withTestServer(t, ts)

	_, err := LatestRelease("owner", "repo")
	if err == nil {
		t.Error("LatestRelease() expected error for invalid JSON, got nil")
	}
}

func TestDownloadAsset_OK(t *testing.T) {
	content := []byte("binary asset data")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	withTestServer(t, ts)

	var buf bytes.Buffer
	if err := DownloadAsset(ts.URL+"/asset", &buf); err != nil {
		t.Fatalf("DownloadAsset() unexpected error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Errorf("DownloadAsset() wrote %q, want %q", buf.Bytes(), content)
	}
}

func TestDownloadAsset_HTTP500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer ts.Close()
	withTestServer(t, ts)

	var buf bytes.Buffer
	err := DownloadAsset(ts.URL+"/asset", &buf)
	if err == nil {
		t.Error("DownloadAsset() expected error for HTTP 500, got nil")
	}
}

func TestDownloadAsset_CopyError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("data")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	withTestServer(t, ts)

	err := DownloadAsset(ts.URL+"/asset", &failWriter{})
	if err == nil {
		t.Error("DownloadAsset() expected error for write failure, got nil")
	}
}

type failWriter struct{}

func (f *failWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("write failed") }

func TestGitHubTokenHeader(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token-123")

	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewEncoder(w).Encode(release{TagName: "v1.0.0"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer ts.Close()
	withTestServer(t, ts)

	_, err := LatestRelease("owner", "repo")
	if err != nil {
		t.Fatalf("LatestRelease() unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-token-123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token-123")
	}
}
