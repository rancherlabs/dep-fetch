package fetch

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"

	"github.com/rancher/dep-fetch/internal/cache"
	"github.com/rancher/dep-fetch/internal/config"
	gh "github.com/rancher/dep-fetch/internal/github"
	"github.com/rancher/dep-fetch/internal/platform"
	"github.com/rancher/dep-fetch/internal/receipt"
	"github.com/rancher/dep-fetch/internal/release"
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
	tools, err := selectTools(cfg.Tools, filter)
	if err != nil {
		return err
	}
	for _, t := range tools {
		if err := syncTool(fs, binDir, t); err != nil {
			return fmt.Errorf("sync %s: %w", t.Name, err)
		}
	}
	return nil
}

// Verify checks installed tool receipts and binary integrity; missing or invalid tools are synced.
// After confirming the binary is intact, it re-fetches the upstream checksum file and warns if
// it has changed since the tool was installed.
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
			if err := syncTool(fs, binDir, t); err != nil {
				return fmt.Errorf("verify %s: %w", t.Name, err)
			}
			continue
		}

		// Binary is intact. Now verify the upstream checksum file hasn't changed.
		r, err := receipts.Read(t.Name)
		if err != nil {
			return fmt.Errorf("verify %s: reading receipt: %w", t.Name, err)
		}
		if r.ChecksumFileHash != "" {
			if warnErr := verifyChecksumFileHash(t, version, r.ChecksumFileHash); warnErr != nil {
				fmt.Printf("  WARNING: %s: %v\n", t.Name, warnErr)
			}
		}

		fmt.Printf("  %s: ok (%s)\n", t.Name, version)
	}
	return nil
}

// verifyChecksumFileHash re-fetches the upstream checksum file and compares its hash to the
// value recorded in the receipt. Returns an error if the file has changed or cannot be fetched.
func verifyChecksumFileHash(t config.Tool, version, expectedHash string) error {
	vars := release.Vars{Name: t.Name, Version: version}
	checksumAsset := release.Render(t.ChecksumTemplate(), vars)
	checksumURL := release.AssetURL(t.Owner(), t.Repo(), version, checksumAsset)

	var buf bytes.Buffer
	if err := gh.DownloadAsset(checksumURL, &buf); err != nil {
		return fmt.Errorf("re-downloading checksum file %s: %w", checksumAsset, err)
	}

	actualHash := sha256Hex(buf.Bytes())
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf(
			"checksum file %s has changed since installation (receipt: %s, upstream: %s) — upstream may have modified release assets",
			checksumAsset, expectedHash, actualHash,
		)
	}
	return nil
}

// InvalidateReceipt removes the stored receipt for a tool, forcing re-sync on the next run.
func InvalidateReceipt(fs billy.Filesystem, name string) error {
	return receipt.Delete(fs, name)
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
func syncTool(fs billy.Filesystem, binDir string, t config.Tool) error {
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
		// Binary is intact — still check the upstream checksum file hasn't changed.
		r, err := receipts.Read(t.Name)
		if err != nil {
			return err
		}
		if r.ChecksumFileHash != "" {
			if warnErr := verifyChecksumFileHash(t, version, r.ChecksumFileHash); warnErr != nil {
				fmt.Printf("  WARNING: %s: %v\n", t.Name, warnErr)
			}
		}
		fmt.Printf("  %s: already at %s\n", t.Name, version)
		return nil
	}

	fmt.Printf("  %s: fetching %s\n", t.Name, version)

	vars := release.Vars{
		Name:    t.Name,
		OS:      goos,
		Arch:    goarch,
		Version: version,
		Ext:     t.Ext(goos),
	}

	binTmpl := release.Render(t.DownloadTemplate(), vars)
	extractPath := release.Render(t.ExtractPath(), vars)

	var binChecksum, checksumFileHash string
	switch t.Mode {
	case config.ModePinned:
		binChecksum, checksumFileHash, err = syncPinned(fs, binDir, t, version, goos, goarch, binTmpl, extractPath)
	case config.ModeReleaseChecksums:
		checksumAsset := release.Render(t.ChecksumTemplate(), vars)
		binChecksum, checksumFileHash, err = syncReleaseChecksums(fs, binDir, t, version, binTmpl, extractPath, checksumAsset)
	default:
		return fmt.Errorf("unknown mode %q for tool %s", t.Mode, t.Name)
	}
	if err != nil {
		return err
	}

	return receipts.Write(t.Name, version, checksumFileHash, binChecksum)
}

func syncPinned(fs billy.Filesystem, binDir string, t config.Tool, version, goos, goarch, assetName, extractPath string) (binChecksum, checksumFileHash string, err error) {
	plat := goos + "/" + goarch
	expected, ok := t.Checksums[plat]
	if !ok {
		platforms := make([]string, 0, len(t.Checksums))
		for p := range t.Checksums {
			platforms = append(platforms, p)
		}
		return "", "", fmt.Errorf("no checksum for platform %s; available: %s", plat, strings.Join(platforms, ", "))
	}

	// Attempt to download the checksum file so its hash can be recorded in the receipt.
	// This is a soft step — failure means we can't track the chain but sync still proceeds.
	vars := release.Vars{Name: t.Name, Version: version}
	checksumAsset := release.Render(t.ChecksumTemplate(), vars)
	checksumURL := release.AssetURL(t.Owner(), t.Repo(), version, checksumAsset)

	var checksumBuf bytes.Buffer
	if dlErr := gh.DownloadAsset(checksumURL, &checksumBuf); dlErr != nil {
		fmt.Printf("  %s: checksum file unavailable, chain tracking skipped (%v)\n", t.Name, dlErr)
	} else {
		checksumFileHash = sha256Hex(checksumBuf.Bytes())
	}

	binChecksum, err = downloadAndInstall(fs, binDir, t.Name, t.Owner(), t.Repo(), version, assetName, extractPath, expected)
	return binChecksum, checksumFileHash, err
}

func syncReleaseChecksums(fs billy.Filesystem, binDir string, t config.Tool, version, assetName, extractPath, checksumAsset string) (binChecksum, checksumFileHash string, err error) {
	checksumURL := release.AssetURL(t.Owner(), t.Repo(), version, checksumAsset)
	var checksumBuf bytes.Buffer
	if err := gh.DownloadAsset(checksumURL, &checksumBuf); err != nil {
		return "", "", fmt.Errorf("downloading checksum file: %w", err)
	}

	checksumFileHash = sha256Hex(checksumBuf.Bytes())

	expected, err := parseChecksumFile(checksumBuf.Bytes(), assetName)
	if err != nil {
		return "", "", err
	}

	binChecksum, err = downloadAndInstall(fs, binDir, t.Name, t.Owner(), t.Repo(), version, assetName, extractPath, expected)
	return binChecksum, checksumFileHash, err
}

// downloadAndInstall downloads an asset, verifies its checksum against expectedChecksum,
// extracts the binary (if extractPath is set), atomically installs it, and returns the
// SHA-256 of the installed binary for recording in the receipt.
func downloadAndInstall(fs billy.Filesystem, binDir, name, owner, repo, version, assetName, extractPath, expectedChecksum string) (string, error) {
	if err := fs.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("creating bin dir: %w", err)
	}

	assetURL := release.AssetURL(owner, repo, version, assetName)

	// Download to an in-memory buffer so we can verify before writing.
	var buf bytes.Buffer
	if err := gh.DownloadAsset(assetURL, &buf); err != nil {
		return "", fmt.Errorf("downloading %s: %w", assetName, err)
	}

	// Verify the downloaded asset's checksum (archive or binary — whatever was declared).
	data, err := verifyReader(bytes.NewReader(buf.Bytes()), expectedChecksum)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}

	// Extract the binary from the asset.
	binData, err := extractBinary(data, assetName, extractPath)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}

	// Compute the binary's own SHA-256 — this is what goes into the receipt.
	// For archives this will differ from the asset checksum above, which is expected.
	binChecksum := sha256Hex(binData)

	// Atomically write to bin_dir.
	tmp, err := fs.TempFile(binDir, name+".tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp file in %s: %w", binDir, err)
	}
	tmpName := tmp.Name()

	_, err = tmp.Write(binData)
	if closeErr := tmp.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
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
	if err := os.Chmod(dest, 0755); err != nil { //nolint:gosec // 0755 is intentional for installed executables
		return "", fmt.Errorf("chmod %s: %w", name, err)
	}

	return binChecksum, nil
}

// extractBinary extracts the binary from data based on the asset filename extension.
// If extractPath is empty, the asset itself is the binary (possibly gunzipped if .gz).
// Supported archive formats: .tar.gz / .tgz, .zip, .gz (gunzip only).
func extractBinary(data []byte, assetName, extractPath string) ([]byte, error) {
	switch {
	case strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz"):
		if extractPath == "" {
			return nil, fmt.Errorf("asset %q is a tar archive but no extract path was specified", assetName)
		}
		return extractFromTarGz(data, extractPath)

	case strings.HasSuffix(assetName, ".zip"):
		if extractPath == "" {
			return nil, fmt.Errorf("asset %q is a zip archive but no extract path was specified", assetName)
		}
		return extractFromZip(data, extractPath)

	case strings.HasSuffix(assetName, ".gz"):
		// Standalone gzip — no extract path needed, just decompress.
		return gunzip(data)

	default:
		// Direct binary — use as-is.
		return data, nil
	}
}

func extractFromTarGz(data []byte, path string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close() //nolint:errcheck // read-only decompression; close error is not actionable

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
	return nil, fmt.Errorf("%q not found in tar archive", path)
}

func extractFromZip(data []byte, path string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("opening zip: %w", err)
	}
	for _, f := range zr.File {
		if f.Name == path || strings.TrimPrefix(f.Name, "./") == path {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("opening %q in zip: %w", path, err)
			}
			defer rc.Close() //nolint:errcheck // read-only; close error is not actionable
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%q not found in zip archive", path)
}

func gunzip(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close() //nolint:errcheck // read-only decompression; close error is not actionable
	return io.ReadAll(gz)
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

func selectTools(tools []config.Tool, filter []string) ([]config.Tool, error) {
	if len(filter) == 0 {
		return tools, nil
	}
	names := make(map[string]bool, len(filter))
	for _, n := range filter {
		names[n] = true
	}
	var out []config.Tool
	for _, t := range tools {
		if names[t.Name] {
			out = append(out, t)
			delete(names, t.Name)
		}
	}
	if len(names) > 0 {
		unknown := make([]string, 0, len(names))
		for n := range names {
			unknown = append(unknown, n)
		}
		return nil, fmt.Errorf("unknown tool(s): %s", strings.Join(unknown, ", "))
	}
	return out, nil
}
