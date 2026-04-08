package fetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"

	"github.com/mallardduck/dep-fetch/internal/cache"
	"github.com/mallardduck/dep-fetch/internal/config"
	gh "github.com/mallardduck/dep-fetch/internal/github"
	"github.com/mallardduck/dep-fetch/internal/platform"
	"github.com/mallardduck/dep-fetch/internal/receipt"
	"github.com/mallardduck/dep-fetch/internal/release"
)

// ToolStatus describes the installed state of a single tool.
type ToolStatus struct {
	Name             string
	DeclaredVersion  string
	ResolvedVersion  string // for version:latest, the cached tag; empty if unknown
	InstalledVersion string
	Source           string
	Mode             string
}

func (s ToolStatus) IsInstalled() bool { return s.InstalledVersion != "" }
func (s ToolStatus) IsUpToDate() bool {
	want := s.ResolvedVersion
	if want == "" {
		want = s.DeclaredVersion
	}
	return s.InstalledVersion == want
}

// Sync fetches and verifies all tools (or only those named in filter).
func Sync(fs billy.Filesystem, cfg *config.Config, binDir string, filter []string) error {
	tools := selectTools(cfg.Tools, filter)
	for _, t := range tools {
		if err := syncTool(fs, cfg, binDir, t); err != nil {
			return fmt.Errorf("sync %s: %w", t.Name, err)
		}
	}
	return nil
}

// Verify checks installed tool receipts and binary integrity; missing or invalid tools are synced.
func Verify(fs billy.Filesystem, cfg *config.Config, binDir string) error {
	receipts := receipt.NewManager(fs, binDir)
	for _, t := range cfg.Tools {
		version, err := resolveVersion(fs, t)
		if err != nil {
			return fmt.Errorf("verify %s: %w", t.Name, err)
		}

		ok, err := receipts.Verify(t.Name, version)
		if err != nil {
			return fmt.Errorf("verify %s: %w", t.Name, err)
		}
		if !ok {
			fmt.Printf("  %s: receipt invalid or missing — syncing\n", t.Name)
			if err := syncTool(fs, cfg, binDir, t); err != nil {
				return fmt.Errorf("verify %s: %w", t.Name, err)
			}
			continue
		}

		fmt.Printf("  %s: ok (%s)\n", t.Name, version)
	}
	return nil
}

// List returns the status of all declared tools.
func List(fs billy.Filesystem, cfg *config.Config, binDir string) ([]ToolStatus, error) {
	receipts := receipt.NewManager(fs, binDir)
	var statuses []ToolStatus
	for _, t := range cfg.Tools {
		r, err := receipts.Read(t.Name)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", t.Name, err)
		}

		var resolved string
		if t.Version == "latest" {
			// Best-effort: use cached tag so we can show meaningful up-to-date status.
			// A cache miss just leaves ResolvedVersion empty — the label handles that.
			resolved, _, _ = cache.LatestVersion(fs, t.Owner(), t.Repo())
		}

		statuses = append(statuses, ToolStatus{
			Name:             t.Name,
			DeclaredVersion:  t.Version,
			ResolvedVersion:  resolved,
			InstalledVersion: r.Version,
			Source:           t.Source,
			Mode:             t.Mode,
		})
	}
	return statuses, nil
}

// syncTool fetches, verifies, and installs a single tool.
func syncTool(fs billy.Filesystem, cfg *config.Config, binDir string, t config.Tool) error {
	goos, goarch := platform.Current()
	receipts := receipt.NewManager(fs, binDir)

	version, err := resolveVersion(fs, t)
	if err != nil {
		return err
	}

	// Use receipt verification as the skip check — not just version equality.
	// This catches a stale version match where the binary itself has changed on disk.
	ok, err := receipts.Verify(t.Name, version)
	if err != nil {
		return err
	}
	if ok {
		fmt.Printf("  %s: already at %s\n", t.Name, version)
		return nil
	}

	fmt.Printf("  %s: fetching %s\n", t.Name, version)

	vars := release.Vars{
		Name:    t.Name,
		OS:      goos,
		Arch:    goarch,
		Version: version,
	}

	binTmpl := release.Render(t.BinaryTemplate(), vars)

	var binChecksum string
	switch t.Mode {
	case config.ModePinned:
		binChecksum, err = syncPinned(fs, binDir, t, version, goos, goarch, binTmpl)
	case config.ModeReleaseChecksums:
		checksumAsset := release.Render(t.ChecksumTemplate(), vars)
		binChecksum, err = syncReleaseChecksums(fs, binDir, t, version, binTmpl, checksumAsset)
	}
	if err != nil {
		return err
	}

	return receipts.Write(t.Name, version, binChecksum)
}

func syncPinned(fs billy.Filesystem, binDir string, t config.Tool, version, goos, goarch, binTmpl string) (string, error) {
	plat := goos + "/" + goarch
	expected, ok := t.Checksums[plat]
	if !ok {
		platforms := make([]string, 0, len(t.Checksums))
		for p := range t.Checksums {
			platforms = append(platforms, p)
		}
		return "", fmt.Errorf("no checksum for platform %s; available: %s", plat, strings.Join(platforms, ", "))
	}

	return downloadAndInstall(fs, binDir, t.Name, t.Owner(), t.Repo(), version, binTmpl, expected)
}

func syncReleaseChecksums(fs billy.Filesystem, binDir string, t config.Tool, version, binTmpl, checksumAsset string) (string, error) {
	// Download the checksum file first.
	checksumURL := release.AssetURL(t.Owner(), t.Repo(), version, checksumAsset)
	var checksumBuf bytes.Buffer
	if err := gh.DownloadAsset(checksumURL, &checksumBuf); err != nil {
		return "", fmt.Errorf("downloading checksum file: %w", err)
	}

	assetName := release.AssetFilename(binTmpl)
	expected, err := parseChecksumFile(checksumBuf.Bytes(), assetName)
	if err != nil {
		return "", err
	}

	return downloadAndInstall(fs, binDir, t.Name, t.Owner(), t.Repo(), version, binTmpl, expected)
}

// downloadAndInstall downloads an asset, verifies its checksum, atomically installs it,
// and returns the SHA-256 of the installed binary for recording in the receipt.
func downloadAndInstall(fs billy.Filesystem, binDir, name, owner, repo, version, binTmpl, expectedChecksum string) (string, error) {
	if err := fs.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("creating bin dir: %w", err)
	}

	hasTarPath := strings.Contains(binTmpl, "/")
	var assetName string
	if hasTarPath {
		assetName = strings.SplitN(binTmpl, "/", 2)[0] + ".tar.gz"
	} else {
		assetName = binTmpl
	}

	assetURL := release.AssetURL(owner, repo, version, assetName)

	// Download to an in-memory buffer so we can verify before writing.
	var buf bytes.Buffer
	if err := gh.DownloadAsset(assetURL, &buf); err != nil {
		return "", fmt.Errorf("downloading %s: %w", assetName, err)
	}

	// Verify the downloaded asset's checksum.
	data, err := verifyReader(bytes.NewReader(buf.Bytes()), expectedChecksum)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}

	// Extract the binary from the archive if needed.
	var binData []byte
	if hasTarPath {
		binData, err = extractFromTarGz(data, binTmpl)
		if err != nil {
			return "", fmt.Errorf("extracting %s from archive: %w", binTmpl, err)
		}
	} else {
		binData = data
	}

	// Compute the binary's own SHA-256 — this is what goes into the receipt.
	binChecksum := sha256Hex(binData)

	// Atomically write to bin_dir.
	tmp, err := fs.TempFile(binDir, name+".tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp file in %s: %w", binDir, err)
	}
	tmpName := tmp.Name()

	_, err = tmp.Write(binData)
	tmp.Close()
	if err != nil {
		_ = fs.Remove(tmpName)
		return "", fmt.Errorf("writing binary for %s: %w", name, err)
	}

	dest := filepath.Join(binDir, name)
	if err := fs.Rename(tmpName, dest); err != nil {
		_ = fs.Remove(tmpName)
		return "", fmt.Errorf("installing %s: %w", name, err)
	}

	// Set executable bit. go-billy's osfs does not expose chmod, so fall back to os.Chmod.
	if err := os.Chmod(dest, 0755); err != nil {
		return "", fmt.Errorf("chmod %s: %w", name, err)
	}

	return binChecksum, nil
}

// resolveVersion returns the concrete version tag for a tool, consulting the cache for "latest".
func resolveVersion(fs billy.Filesystem, t config.Tool) (string, error) {
	if t.Version != "latest" {
		return t.Version, nil
	}

	if v, hit, err := cache.LatestVersion(fs, t.Owner(), t.Repo()); err != nil {
		return "", err
	} else if hit {
		return v, nil
	}

	v, err := gh.LatestRelease(t.Owner(), t.Repo())
	if err != nil {
		return "", err
	}

	_ = cache.WriteLatestVersion(fs, t.Owner(), t.Repo(), v)
	return v, nil
}

// extractFromTarGz extracts a named file from a .tar.gz archive and returns its contents.
func extractFromTarGz(data []byte, path string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if hdr.Name == path || strings.TrimPrefix(hdr.Name, "./") == path {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("file %q not found in archive", path)
}

func selectTools(tools []config.Tool, filter []string) []config.Tool {
	if len(filter) == 0 {
		return tools
	}
	names := make(map[string]bool, len(filter))
	for _, n := range filter {
		names[n] = true
	}
	var out []config.Tool
	for _, t := range tools {
		if names[t.Name] {
			out = append(out, t)
		}
	}
	return out
}
