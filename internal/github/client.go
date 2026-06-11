package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var (
	apiBase           = "https://api.github.com"
	allowedHostSuffix = "github.com"
)

type release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// LatestRelease returns the tag name of the latest release for owner/repo.
// If tagPrefix is empty, returns any release (uses GitHub's /releases/latest API).
// If tagPrefix is non-empty, lists releases and returns the first matching tag.
func LatestRelease(owner, repo, tagPrefix string) (string, error) {
	// Fast path: use GitHub's /releases/latest endpoint when no prefix filtering needed
	if tagPrefix == "" {
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

	// Prefix filtering: list releases and find the first match
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", apiBase, owner, repo)
	body, err := doGet(url, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	defer body.Close() //nolint:errcheck // read-only response body; close error is not actionable

	var releases []release
	if err := json.NewDecoder(body).Decode(&releases); err != nil {
		return "", fmt.Errorf("decoding releases list for %s/%s: %w", owner, repo, err)
	}

	for _, r := range releases {
		// Skip drafts and prereleases
		if r.Draft || r.Prerelease {
			continue
		}
		// Check tag prefix
		if strings.HasPrefix(r.TagName, tagPrefix) && r.TagName != "" {
			return r.TagName, nil
		}
	}

	return "", fmt.Errorf("no release found with tag prefix %q for %s/%s", tagPrefix, owner, repo)
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

func validateHost(host string) error {
	if strings.HasSuffix(host, allowedHostSuffix) {
		return nil
	}
	return fmt.Errorf("unauthorized host: %s", host)
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

	if err := validateHost(req.URL.Host); err != nil {
		return nil, err
	}

	// #nosec G107 - Host is validated against github.com domain suffix or test apiBase
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
