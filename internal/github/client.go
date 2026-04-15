package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var apiBase = "https://api.github.com"

type release struct {
	TagName string `json:"tag_name"`
}

// LatestRelease returns the tag name of the latest release for owner/repo.
func LatestRelease(owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBase, owner, repo)
	body, err := doGet(url, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	defer body.Close() //nolint:errcheck // read-only response body; close error is not actionable

	var r release
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		return "", fmt.Errorf("decoding latest release response for %s/%s: %w", owner, repo, err)
	}
	if r.TagName == "" {
		return "", fmt.Errorf("no tag found in latest release for %s/%s", owner, repo)
	}
	return r.TagName, nil
}

// DownloadAsset downloads the release asset at assetURL and writes it to w.
func DownloadAsset(assetURL string, w io.Writer) error {
	body, err := doGet(assetURL, "application/octet-stream")
	if err != nil {
		return err
	}
	defer body.Close() //nolint:errcheck // read-only response body; close error is not actionable

	if _, err := io.Copy(w, body); err != nil {
		return fmt.Errorf("downloading %s: %w", assetURL, err)
	}
	return nil
}

func doGet(url, accept string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}
	req.Header.Set("Accept", accept)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if !strings.HasSuffix(req.URL.Host, "github.com") {
		return nil, fmt.Errorf("unauthorized host: %s", req.URL.Host)
	}

	// #nosec G704 - Host is validated against github.com domain suffix
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close() //nolint:errcheck,gosec // already in error path
		return nil, fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}
