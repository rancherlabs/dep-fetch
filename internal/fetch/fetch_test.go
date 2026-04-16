package fetch

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"

	"github.com/rancher/dep-fetch/internal/cache"
	"github.com/rancher/dep-fetch/internal/config"
	"github.com/rancher/dep-fetch/internal/platform"
	"github.com/rancher/dep-fetch/internal/receipt"
)

// roundTripFunc lets tests stub http.DefaultTransport without starting a server.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// mockHTTP replaces http.DefaultTransport for the test, always returning status+body.
func mockHTTP(t *testing.T, status int, body []byte) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// mockHTTPDispatch replaces http.DefaultTransport with a per-request handler.
func mockHTTPDispatch(t *testing.T, fn func(*http.Request) *http.Response) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return fn(req), nil
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

func hashBytes(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// --- selectTools ---

func TestSelectTools_NoFilter(t *testing.T) {
	tools := []config.Tool{{Name: "a"}, {Name: "b"}}
	got, err := selectTools(tools, nil)
	if err != nil {
		t.Fatalf("selectTools() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("selectTools() len = %d, want 2", len(got))
	}
}

func TestSelectTools_WithFilter(t *testing.T) {
	tools := []config.Tool{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got, err := selectTools(tools, []string{"a", "c"})
	if err != nil {
		t.Fatalf("selectTools() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("selectTools() len = %d, want 2", len(got))
	}
	names := map[string]bool{}
	for _, tool := range got {
		names[tool.Name] = true
	}
	if !names["a"] || !names["c"] {
		t.Errorf("selectTools() names = %v, want a and c", names)
	}
}

func TestSelectTools_UnknownTool(t *testing.T) {
	tools := []config.Tool{{Name: "a"}}
	if _, err := selectTools(tools, []string{"unknown"}); err == nil {
		t.Error("selectTools() expected error for unknown tool, got nil")
	}
}

func TestSelectTools_EmptyList(t *testing.T) {
	got, err := selectTools(nil, nil)
	if err != nil {
		t.Fatalf("selectTools() unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("selectTools() = %v, want nil", got)
	}
}

// --- ToolStatus ---

func TestToolStatus_IsInstalled(t *testing.T) {
	if !(ToolStatus{InstalledVersion: "v1.0.0"}).IsInstalled() {
		t.Error("IsInstalled() = false, want true when version is set")
	}
	if (ToolStatus{}).IsInstalled() {
		t.Error("IsInstalled() = true, want false when version is empty")
	}
}

func TestToolStatus_IsUpToDate(t *testing.T) {
	tests := []struct {
		name   string
		status ToolStatus
		want   bool
	}{
		{
			name:   "declared version matches",
			status: ToolStatus{DeclaredVersion: "v1.0.0", InstalledVersion: "v1.0.0"},
			want:   true,
		},
		{
			name:   "declared version mismatch",
			status: ToolStatus{DeclaredVersion: "v2.0.0", InstalledVersion: "v1.0.0"},
			want:   false,
		},
		{
			name:   "resolved version used when set",
			status: ToolStatus{DeclaredVersion: "latest", ResolvedVersion: "v1.0.0", InstalledVersion: "v1.0.0"},
			want:   true,
		},
		{
			name:   "resolved version mismatch",
			status: ToolStatus{DeclaredVersion: "latest", ResolvedVersion: "v2.0.0", InstalledVersion: "v1.0.0"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsUpToDate(); got != tt.want {
				t.Errorf("IsUpToDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- resolveVersion ---

func TestResolveVersion_Pinned(t *testing.T) {
	fs := memfs.New()
	tool := config.Tool{Name: "tool", Version: "v1.2.3", Source: "a/b"}
	v, err := resolveVersion(fs, tool)
	if err != nil {
		t.Fatalf("resolveVersion() unexpected error: %v", err)
	}
	if v != "v1.2.3" {
		t.Errorf("resolveVersion() = %q, want %q", v, "v1.2.3")
	}
}

func TestResolveVersion_LatestCacheHit(t *testing.T) {
	fs := memfs.New()
	if err := cache.WriteLatestVersion(fs, "owner", "repo", "v9.9.9"); err != nil {
		t.Fatal(err)
	}
	tool := config.Tool{Name: "tool", Version: "latest", Source: "owner/repo"}
	v, err := resolveVersion(fs, tool)
	if err != nil {
		t.Fatalf("resolveVersion() unexpected error: %v", err)
	}
	if v != "v9.9.9" {
		t.Errorf("resolveVersion() = %q, want %q", v, "v9.9.9")
	}
}

func TestResolveVersion_LatestHTTP(t *testing.T) {
	fs := memfs.New()
	mockHTTP(t, 200, []byte(`{"tag_name":"v5.0.0"}`))

	tool := config.Tool{Name: "tool", Version: "latest", Source: "owner/repo"}
	v, err := resolveVersion(fs, tool)
	if err != nil {
		t.Fatalf("resolveVersion() unexpected error: %v", err)
	}
	if v != "v5.0.0" {
		t.Errorf("resolveVersion() = %q, want %q", v, "v5.0.0")
	}
	// Should also write to cache.
	cached, hit, _ := cache.LatestVersion(fs, "owner", "repo")
	if !hit || cached != "v5.0.0" {
		t.Errorf("cache after resolveVersion: hit=%v version=%q, want true/v5.0.0", hit, cached)
	}
}

func TestResolveVersion_LatestHTTPError(t *testing.T) {
	fs := memfs.New()
	mockHTTP(t, 404, []byte("not found"))

	tool := config.Tool{Name: "tool", Version: "latest", Source: "owner/repo"}
	_, err := resolveVersion(fs, tool)
	if err == nil {
		t.Error("resolveVersion() expected error for HTTP 404, got nil")
	}
}

// --- downloadAndInstall ---

func TestDownloadAndInstall_DownloadError(t *testing.T) {
	fs := memfs.New()
	mockHTTP(t, 404, []byte("not found"))

	_, err := downloadAndInstall(fs, "./bin", "tool", "owner", "repo", "v1.0.0", "tool", "", "abc")
	if err == nil {
		t.Error("downloadAndInstall() expected error for HTTP 404")
	}
}

func TestDownloadAndInstall_ChecksumMismatch(t *testing.T) {
	fs := memfs.New()
	mockHTTP(t, 200, []byte("binary content"))

	_, err := downloadAndInstall(fs, "./bin", "tool", "owner", "repo", "v1.0.0", "tool", "", "wrongchecksum")
	if err == nil {
		t.Error("downloadAndInstall() expected error for checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("downloadAndInstall() error = %q, want checksum mismatch", err.Error())
	}
}

func TestDownloadAndInstall_ExtractError(t *testing.T) {
	fs := memfs.New()
	data := []byte("not a valid tar archive")
	mockHTTP(t, 200, data)

	_, err := downloadAndInstall(fs, "./bin", "tool", "owner", "repo", "v1.0.0", "tool.tar.gz", "tool", hashBytes(data))
	if err == nil {
		t.Error("downloadAndInstall() expected error for invalid tar.gz")
	}
}

func TestDownloadAndInstall_OK(t *testing.T) {
	tmpDir := t.TempDir()
	fs := osfs.New("/")

	binContent := []byte("fake binary content")
	checksum := hashBytes(binContent)
	mockHTTP(t, 200, binContent)

	gotChecksum, err := downloadAndInstall(fs, tmpDir, "tool", "owner", "repo", "v1.0.0", "tool", "", checksum)
	if err != nil {
		t.Fatalf("downloadAndInstall() unexpected error: %v", err)
	}
	if gotChecksum != checksum {
		t.Errorf("downloadAndInstall() checksum = %q, want %q", gotChecksum, checksum)
	}
	info, err := os.Stat(filepath.Join(tmpDir, "tool"))
	if err != nil {
		t.Fatalf("binary not found after install: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("binary is not executable after install")
	}
}

// --- syncPinned ---

func TestSyncPinned_NoPlatformChecksum(t *testing.T) {
	fs := memfs.New()
	goos, goarch := platform.Current()
	tool := config.Tool{
		Name:      "tool",
		Source:    "a/b",
		Checksums: map[string]string{"fakeos/fakearch": "abc"},
	}
	_, _, err := syncPinned(fs, "./bin", tool, "v1.0.0", goos, goarch, "tool", "")
	if err == nil {
		t.Error("syncPinned() expected error for missing platform checksum")
	}
	if !strings.Contains(err.Error(), "no checksum for platform") {
		t.Errorf("syncPinned() error = %q, want 'no checksum for platform'", err.Error())
	}
}

func TestSyncPinned_OK(t *testing.T) {
	tmpDir := t.TempDir()
	fs := osfs.New("/")

	goos, goarch := platform.Current()
	binContent := []byte("pinned binary")
	checksum := hashBytes(binContent)
	checksumFileContent := []byte("some checksum file content")

	// First request → checksum file, second → binary asset.
	callCount := 0
	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		callCount++
		var body []byte
		if strings.Contains(req.URL.String(), "checksums") {
			body = checksumFileContent
		} else {
			body = binContent
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}
	})

	tool := config.Tool{
		Name:      "tool",
		Source:    "owner/repo",
		Checksums: map[string]string{goos + "/" + goarch: checksum},
	}
	gotChecksum, gotFileHash, err := syncPinned(fs, tmpDir, tool, "v1.0.0", goos, goarch, "tool", "")
	if err != nil {
		t.Fatalf("syncPinned() unexpected error: %v", err)
	}
	if gotChecksum != checksum {
		t.Errorf("syncPinned() binChecksum = %q, want %q", gotChecksum, checksum)
	}
	if gotFileHash != hashBytes(checksumFileContent) {
		t.Errorf("syncPinned() checksumFileHash = %q, want %q", gotFileHash, hashBytes(checksumFileContent))
	}
}

// TestSyncPinned_ChecksumFileUnavailable confirms that a failed checksum-file download is
// a soft failure: sync still succeeds and checksumFileHash is empty.
func TestSyncPinned_ChecksumFileUnavailable(t *testing.T) {
	tmpDir := t.TempDir()
	fs := osfs.New("/")

	goos, goarch := platform.Current()
	binContent := []byte("pinned binary")
	checksum := hashBytes(binContent)

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		// Checksum file 404s; binary download succeeds.
		if strings.Contains(req.URL.String(), "checksums") {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(binContent)), Header: make(http.Header)}
	})

	tool := config.Tool{
		Name:      "tool",
		Source:    "owner/repo",
		Checksums: map[string]string{goos + "/" + goarch: checksum},
	}
	gotChecksum, gotFileHash, err := syncPinned(fs, tmpDir, tool, "v1.0.0", goos, goarch, "tool", "")
	if err != nil {
		t.Fatalf("syncPinned() unexpected error: %v", err)
	}
	if gotChecksum != checksum {
		t.Errorf("syncPinned() binChecksum = %q, want %q", gotChecksum, checksum)
	}
	if gotFileHash != "" {
		t.Errorf("syncPinned() checksumFileHash = %q, want empty when checksum file is unavailable", gotFileHash)
	}
}

// --- syncReleaseChecksums ---

func TestSyncReleaseChecksums_ChecksumDownloadError(t *testing.T) {
	fs := memfs.New()
	mockHTTP(t, 500, []byte("error"))

	tool := config.Tool{Name: "tool", Source: "owner/repo"}
	_, _, err := syncReleaseChecksums(fs, "./bin", tool, "v1.0.0", "tool", "", "checksums.txt")
	if err == nil {
		t.Error("syncReleaseChecksums() expected error for HTTP 500 on checksum file")
	}
	if !strings.Contains(err.Error(), "downloading checksum file") {
		t.Errorf("syncReleaseChecksums() error = %q, want 'downloading checksum file'", err.Error())
	}
}

func TestSyncReleaseChecksums_ParseError(t *testing.T) {
	fs := memfs.New()
	// Checksum file has no entry for our asset.
	mockHTTP(t, 200, []byte("abc123  othertool\n"))

	tool := config.Tool{Name: "tool", Source: "owner/repo"}
	_, _, err := syncReleaseChecksums(fs, "./bin", tool, "v1.0.0", "tool", "", "checksums.txt")
	if err == nil {
		t.Error("syncReleaseChecksums() expected error for missing checksum entry")
	}
}

func TestSyncReleaseChecksums_OK(t *testing.T) {
	tmpDir := t.TempDir()
	fs := osfs.New("/")

	binContent := []byte("release binary")
	checksum := hashBytes(binContent)
	checksumFile := []byte(checksum + "  tool\n")

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		var body []byte
		if strings.Contains(req.URL.String(), "checksums") {
			body = checksumFile
		} else {
			body = binContent
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}
	})

	tool := config.Tool{Name: "tool", Source: "owner/repo"}
	gotChecksum, gotFileHash, err := syncReleaseChecksums(fs, tmpDir, tool, "v1.0.0", "tool", "", "checksums.txt")
	if err != nil {
		t.Fatalf("syncReleaseChecksums() unexpected error: %v", err)
	}
	if gotChecksum != checksum {
		t.Errorf("syncReleaseChecksums() binChecksum = %q, want %q", gotChecksum, checksum)
	}
	if gotFileHash != hashBytes(checksumFile) {
		t.Errorf("syncReleaseChecksums() checksumFileHash = %q, want %q", gotFileHash, hashBytes(checksumFile))
	}
}

// --- syncTool ---

func TestSyncTool_AlreadyUpToDate(t *testing.T) {
	fs := memfs.New()
	binDir := "./bin"
	binContent := []byte("binary")

	if err := fs.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.Create(binDir + "/tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(binContent); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Write(fs, "tool", "v1.0.0", "", hashBytes(binContent)); err != nil {
		t.Fatal(err)
	}

	tool := config.Tool{
		Name: "tool", Version: "v1.0.0",
		Source: "rancher/charts-build-scripts", Mode: config.ModeReleaseChecksums,
	}
	if err := syncTool(fs, binDir, tool); err != nil {
		t.Errorf("syncTool() unexpected error: %v", err)
	}
}

func TestSyncTool_UnknownMode(t *testing.T) {
	fs := memfs.New()
	tool := config.Tool{Name: "tool", Version: "v1.0.0", Source: "a/b", Mode: "invalid"}
	if err := syncTool(fs, "./bin", tool); err == nil {
		t.Error("syncTool() expected error for unknown mode")
	}
}

// --- Sync ---

func TestSync_AlreadyUpToDate(t *testing.T) {
	fs := memfs.New()
	binDir := "./bin"
	binContent := []byte("binary")

	if err := fs.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.Create(binDir + "/tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(binContent); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Write(fs, "tool", "v1.0.0", "", hashBytes(binContent)); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Tools: []config.Tool{
		{Name: "tool", Version: "v1.0.0", Source: "rancher/charts-build-scripts", Mode: config.ModeReleaseChecksums},
	}}
	if err := Sync(fs, cfg, binDir, nil); err != nil {
		t.Errorf("Sync() unexpected error: %v", err)
	}
}

func TestSync_FilterError(t *testing.T) {
	fs := memfs.New()
	cfg := &config.Config{Tools: []config.Tool{{Name: "a"}}}
	if err := Sync(fs, cfg, "./bin", []string{"unknown"}); err == nil {
		t.Error("Sync() expected error for unknown filter tool")
	}
}

func TestSync_NoPlatformChecksum(t *testing.T) {
	fs := memfs.New()
	// 404 for the checksum file (soft fail), then error from missing platform checksum.
	mockHTTP(t, 404, []byte("not found"))
	cfg := &config.Config{Tools: []config.Tool{{
		Name: "tool", Version: "v1.0.0", Source: "a/b", Mode: config.ModePinned,
		Checksums: map[string]string{"fakeos/fakearch": "abc"},
	}}}
	err := Sync(fs, cfg, "./bin", nil)
	if err == nil {
		t.Error("Sync() expected error for missing platform checksum")
	}
	if !strings.Contains(err.Error(), "no checksum for platform") {
		t.Errorf("Sync() error = %q, want 'no checksum for platform'", err.Error())
	}
}

// --- Verify ---

func TestVerify_AllValid(t *testing.T) {
	fs := memfs.New()
	binDir := "./bin"
	binContent := []byte("binary")

	if err := fs.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.Create(binDir + "/tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(binContent); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	// Empty ChecksumFileHash — checksum file re-verification is skipped.
	if err := receipt.Write(fs, "tool", "v1.0.0", "", hashBytes(binContent)); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Tools: []config.Tool{
		{Name: "tool", Version: "v1.0.0", Source: "rancher/charts-build-scripts", Mode: config.ModeReleaseChecksums},
	}}
	if err := Verify(fs, cfg, binDir); err != nil {
		t.Errorf("Verify() unexpected error: %v", err)
	}
}

// TestVerify_ChecksumFileChanged confirms that a changed upstream checksum file produces a warning
// (printed to stdout) but does not cause Verify to return an error.
func TestVerify_ChecksumFileChanged(t *testing.T) {
	fs := memfs.New()
	binDir := "./bin"
	binContent := []byte("binary")

	if err := fs.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.Create(binDir + "/tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(binContent); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Receipt records a checksum file hash that will not match what the mock serves.
	if err := receipt.Write(fs, "tool", "v1.0.0", "originalfilechecksum", hashBytes(binContent)); err != nil {
		t.Fatal(err)
	}

	// Mock serves a different checksum file than what was recorded.
	mockHTTP(t, 200, []byte("different checksum file content"))

	cfg := &config.Config{Tools: []config.Tool{
		{Name: "tool", Version: "v1.0.0", Source: "rancher/charts-build-scripts", Mode: config.ModeReleaseChecksums},
	}}
	// Verify must succeed (no error returned) — the changed file is a warning only.
	if err := Verify(fs, cfg, binDir); err != nil {
		t.Errorf("Verify() unexpected error for changed checksum file (should warn, not fail): %v", err)
	}
}

func TestVerify_MissingReceipt_SyncFails(t *testing.T) {
	fs := memfs.New()
	// No receipt — Verify will try to sync, which fails (no platform checksum).
	// The checksum file 404s (soft fail), then platform check fails (hard fail).
	mockHTTP(t, 404, []byte("not found"))
	cfg := &config.Config{Tools: []config.Tool{{
		Name: "tool", Version: "v1.0.0", Source: "a/b", Mode: config.ModePinned,
		Checksums: map[string]string{"fakeos/fakearch": "abc"},
	}}}
	if err := Verify(fs, cfg, "./bin"); err == nil {
		t.Error("Verify() expected error when sync fails")
	}
}

// --- List ---

func TestList_NoReceipts(t *testing.T) {
	fs := memfs.New()
	cfg := &config.Config{Tools: []config.Tool{
		{Name: "tool1", Version: "v1.0.0", Source: "rancher/charts-build-scripts", Mode: config.ModeReleaseChecksums},
	}}
	statuses, err := List(fs, cfg, "./bin")
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("List() len = %d, want 1", len(statuses))
	}
	if statuses[0].IsInstalled() {
		t.Error("List() IsInstalled() = true, want false for tool with no receipt")
	}
}

func TestList_WithReceipt(t *testing.T) {
	fs := memfs.New()
	cfg := &config.Config{Tools: []config.Tool{
		{Name: "tool1", Version: "v2.0.0", Source: "rancher/charts-build-scripts", Mode: config.ModeReleaseChecksums},
	}}
	if err := receipt.Write(fs, "tool1", "v2.0.0", "", "somechecksum"); err != nil {
		t.Fatal(err)
	}
	statuses, err := List(fs, cfg, "./bin")
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if !statuses[0].IsInstalled() {
		t.Error("List() IsInstalled() = false, want true")
	}
	if !statuses[0].IsUpToDate() {
		t.Error("List() IsUpToDate() = false, want true")
	}
}

func TestList_LatestVersionCacheHit(t *testing.T) {
	fs := memfs.New()
	if err := cache.WriteLatestVersion(fs, "rancher", "charts-build-scripts", "v3.0.0"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Tools: []config.Tool{
		{Name: "tool1", Version: "latest", Source: "rancher/charts-build-scripts", Mode: config.ModeReleaseChecksums},
	}}
	statuses, err := List(fs, cfg, "./bin")
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if statuses[0].ResolvedVersion != "v3.0.0" {
		t.Errorf("List() ResolvedVersion = %q, want %q", statuses[0].ResolvedVersion, "v3.0.0")
	}
}

// --- extractBinary error paths ---

func TestGunzip_InvalidData(t *testing.T) {
	if _, err := gunzip([]byte("not gzip data")); err == nil {
		t.Error("gunzip() expected error for invalid gzip data")
	}
}

func TestExtractFromTarGz_InvalidGzip(t *testing.T) {
	if _, err := extractFromTarGz([]byte("not gzip"), "file"); err == nil {
		t.Error("extractFromTarGz() expected error for invalid gzip")
	}
}

func TestExtractFromTarGz_InvalidTar(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte("this is not a tar archive")); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := extractFromTarGz(buf.Bytes(), "file"); err == nil {
		t.Error("extractFromTarGz() expected error for invalid tar content")
	}
}

func TestVerifyReader_ReadError(t *testing.T) {
	if _, err := verifyReader(&errReader{}, "anychecksum"); err == nil {
		t.Error("verifyReader() expected error from reader failure")
	}
}

type errReader struct{}

func (e *errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("simulated read error") }

// --- extractBinary ---

func TestExtractBinary_DirectBinary(t *testing.T) {
	data := []byte("binary content")
	got, err := extractBinary(data, "mytool", "")
	if err != nil {
		t.Fatalf("extractBinary() unexpected error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("extractBinary() data mismatch")
	}
}

func TestExtractBinary_GzipDecompress(t *testing.T) {
	original := []byte("binary data")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(original); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractBinary(buf.Bytes(), "mytool.gz", "")
	if err != nil {
		t.Fatalf("extractBinary() unexpected error: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("extractBinary() = %q, want %q", got, original)
	}
}

func TestExtractBinary_TarGz(t *testing.T) {
	content := []byte("the binary")
	got, err := extractBinary(makeTarGz(t, "bin/mytool", content), "archive.tar.gz", "bin/mytool")
	if err != nil {
		t.Fatalf("extractBinary() unexpected error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("extractBinary() = %q, want %q", got, content)
	}
}

func TestExtractBinary_TarGz_DotSlashPath(t *testing.T) {
	content := []byte("the binary")
	got, err := extractBinary(makeTarGz(t, "./bin/mytool", content), "archive.tar.gz", "bin/mytool")
	if err != nil {
		t.Fatalf("extractBinary() unexpected error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("extractBinary() = %q, want %q", got, content)
	}
}

func TestExtractBinary_TarGz_NoExtractPath(t *testing.T) {
	if _, err := extractBinary(makeTarGz(t, "bin/mytool", []byte("data")), "archive.tar.gz", ""); err == nil {
		t.Error("extractBinary() expected error for .tar.gz with no extract path")
	}
}

func TestExtractBinary_TarGz_NotFound(t *testing.T) {
	if _, err := extractBinary(makeTarGz(t, "bin/othertool", []byte("data")), "archive.tar.gz", "bin/missing"); err == nil {
		t.Error("extractBinary() expected error when file not in tar")
	}
}

func TestExtractBinary_Tgz(t *testing.T) {
	content := []byte("tgz content")
	got, err := extractBinary(makeTarGz(t, "mytool", content), "archive.tgz", "mytool")
	if err != nil {
		t.Fatalf("extractBinary() unexpected error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("extractBinary() = %q, want %q", got, content)
	}
}

func TestExtractBinary_Zip(t *testing.T) {
	content := []byte("zip content")
	got, err := extractBinary(makeZip(t, "bin/mytool", content), "archive.zip", "bin/mytool")
	if err != nil {
		t.Fatalf("extractBinary() unexpected error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("extractBinary() = %q, want %q", got, content)
	}
}

func TestExtractBinary_Zip_NoExtractPath(t *testing.T) {
	if _, err := extractBinary(makeZip(t, "mytool", []byte("data")), "archive.zip", ""); err == nil {
		t.Error("extractBinary() expected error for .zip with no extract path")
	}
}

func TestExtractBinary_Zip_NotFound(t *testing.T) {
	if _, err := extractBinary(makeZip(t, "othertool", []byte("data")), "archive.zip", "missing"); err == nil {
		t.Error("extractBinary() expected error when file not in zip")
	}
}

// --- archive helpers ---

func makeTarGz(t *testing.T, path string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(content)), Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, path string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
