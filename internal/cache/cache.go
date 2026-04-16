package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"

	"github.com/rancher/dep-fetch/internal/config"
)

const (
	cacheDir = ".dep-fetch/cache"
	ttl      = 24 * time.Hour
)

// LatestVersion returns the cached latest version tag for owner/repo, if it exists and
// is younger than 24 hours. Returns ("", false, nil) on a cache miss.
func LatestVersion(fs billy.Filesystem, owner, repo string) (string, bool, error) {
	if skip() {
		return "", false, nil
	}

	path := filepath.Join(cacheDir, owner+"-"+repo)
	f, err := fs.Open(path)
	if err != nil {
		// Missing cache file is a normal miss.
		return "", false, nil
	}
	defer f.Close() //nolint:errcheck // read-only; close error is not actionable

	data, err := io.ReadAll(f)
	if err != nil {
		return "", false, err
	}

	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) != 2 {
		return "", false, nil
	}

	ts, err := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	if err != nil {
		return "", false, nil
	}

	if time.Since(time.Unix(ts, 0)) > ttl {
		return "", false, nil
	}

	return strings.TrimSpace(lines[1]), true, nil
}

// WriteLatestVersion stores the resolved latest version tag for owner/repo.
func WriteLatestVersion(fs billy.Filesystem, owner, repo, version string) error {
	if err := fs.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	path := filepath.Join(cacheDir, owner+"-"+repo)
	f, err := fs.Create(path)
	if err != nil {
		return fmt.Errorf("writing version cache: %w", err)
	}

	_, err = fmt.Fprintf(f, "%d\n%s\n", time.Now().Unix(), version)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func skip() bool {
	return os.Getenv(config.EnvSkipCache) == "1"
}
