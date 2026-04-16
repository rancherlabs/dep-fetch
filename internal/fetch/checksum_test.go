package fetch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rancher/dep-fetch/internal/config"
)

func TestVerifyReader_Match(t *testing.T) {
	data := []byte("hello world")
	h := sha256.New()
	h.Write(data)
	expected := hex.EncodeToString(h.Sum(nil))

	got, err := verifyReader(strings.NewReader(string(data)), expected)
	if err != nil {
		t.Fatalf("verifyReader() unexpected error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("verifyReader() data mismatch")
	}
}

func TestVerifyReader_CaseInsensitive(t *testing.T) {
	data := []byte("test")
	h := sha256.New()
	h.Write(data)
	lower := hex.EncodeToString(h.Sum(nil))
	upper := strings.ToUpper(lower)

	_, err := verifyReader(strings.NewReader("test"), upper)
	if err != nil {
		t.Errorf("verifyReader() should accept uppercase checksum, got error: %v", err)
	}
}

func TestVerifyReader_Mismatch(t *testing.T) {
	_, err := verifyReader(strings.NewReader("hello"), "wrongchecksum")
	if err == nil {
		t.Error("verifyReader() expected error for checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("verifyReader() error = %q, want to contain 'checksum mismatch'", err.Error())
	}
}

func TestSha256Hex(t *testing.T) {
	data := []byte("deterministic")
	h := sha256.New()
	h.Write(data)
	want := hex.EncodeToString(h.Sum(nil))

	got := sha256Hex(data)
	if got != want {
		t.Errorf("sha256Hex() = %q, want %q", got, want)
	}

	// Calling again should return the same result.
	if sha256Hex(data) != got {
		t.Error("sha256Hex() is not deterministic")
	}
}

func TestParseChecksumFile_Found(t *testing.T) {
	content := []byte("abc123  myfile.tar.gz\ndef456  other.tar.gz\n")
	checksum, err := parseChecksumFile(content, "myfile.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumFile() unexpected error: %v", err)
	}
	if checksum != "abc123" {
		t.Errorf("parseChecksumFile() = %q, want %q", checksum, "abc123")
	}
}

func TestParseChecksumFile_DotSlashPrefix(t *testing.T) {
	content := []byte("abc123  ./myfile.tar.gz\n")
	checksum, err := parseChecksumFile(content, "myfile.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumFile() unexpected error for ./ prefix: %v", err)
	}
	if checksum != "abc123" {
		t.Errorf("parseChecksumFile() = %q, want %q", checksum, "abc123")
	}
}

func TestParseChecksumFile_StarPrefix(t *testing.T) {
	content := []byte("abc123  *myfile.tar.gz\n")
	checksum, err := parseChecksumFile(content, "myfile.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumFile() unexpected error for * prefix: %v", err)
	}
	if checksum != "abc123" {
		t.Errorf("parseChecksumFile() = %q, want %q", checksum, "abc123")
	}
}

func TestParseChecksumFile_PathSuffix(t *testing.T) {
	content := []byte("abc123  dist/myfile.tar.gz\n")
	checksum, err := parseChecksumFile(content, "myfile.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumFile() unexpected error for path suffix match: %v", err)
	}
	if checksum != "abc123" {
		t.Errorf("parseChecksumFile() = %q, want %q", checksum, "abc123")
	}
}

func TestParseChecksumFile_NotFound(t *testing.T) {
	content := []byte("abc123  otherfile.tar.gz\n")
	_, err := parseChecksumFile(content, "missing.tar.gz")
	if err == nil {
		t.Error("parseChecksumFile() expected error for missing entry, got nil")
	}
}

func TestParseChecksumFile_Empty(t *testing.T) {
	_, err := parseChecksumFile([]byte{}, "myfile.tar.gz")
	if err == nil {
		t.Error("parseChecksumFile() expected error for empty file, got nil")
	}
}

func TestParseChecksumFile_SkipsShortLines(t *testing.T) {
	content := []byte("abc123\nfull-hash  myfile.tar.gz\n")
	checksum, err := parseChecksumFile(content, "myfile.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumFile() unexpected error: %v", err)
	}
	if checksum != "full-hash" {
		t.Errorf("parseChecksumFile() = %q, want %q", checksum, "full-hash")
	}
}

// --- FetchChecksums ---

func makeTool(platforms []string) *config.Tool {
	checksums := make(map[string]string, len(platforms))
	for _, p := range platforms {
		checksums[p] = ""
	}
	return &config.Tool{
		Name:      "mytool",
		Source:    "owner/repo",
		Checksums: checksums,
	}
}

func makeToolWithExt(platforms []string, extensions map[string]string, downloadTmpl string) *config.Tool {
	checksums := make(map[string]string, len(platforms))
	for _, p := range platforms {
		checksums[p] = ""
	}
	return &config.Tool{
		Name:      "mytool",
		Source:    "owner/repo",
		Checksums: checksums,
		Release: &config.ReleaseConfig{
			DownloadTemplate: downloadTmpl,
			Extensions:       extensions,
		},
	}
}

func TestFetchChecksums_FromChecksumFile(t *testing.T) {
	tool := makeTool([]string{"linux/amd64"})
	// Default template renders to "mytool_linux_amd64"; checksum file lists it.
	checksumFile := []byte("abc123  mytool_linux_amd64\n")

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		var body []byte
		if strings.Contains(req.URL.Path, "checksums.txt") {
			body = checksumFile
		} else {
			body = []byte("should not be fetched")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}
	})

	got, err := FetchChecksums(tool, "v1.0.0")
	if err != nil {
		t.Fatalf("FetchChecksums() unexpected error: %v", err)
	}
	if got["linux/amd64"] != "abc123" {
		t.Errorf("FetchChecksums() linux/amd64 = %q, want %q", got["linux/amd64"], "abc123")
	}
}

func TestFetchChecksums_MultiplePlatforms_FromChecksumFile(t *testing.T) {
	tool := makeTool([]string{"linux/amd64", "darwin/arm64"})
	checksumFile := []byte("aaa111  mytool_linux_amd64\nbbb222  mytool_darwin_arm64\n")

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(checksumFile)), Header: make(http.Header)}
	})

	got, err := FetchChecksums(tool, "v1.0.0")
	if err != nil {
		t.Fatalf("FetchChecksums() unexpected error: %v", err)
	}
	if got["linux/amd64"] != "aaa111" {
		t.Errorf("linux/amd64 = %q, want %q", got["linux/amd64"], "aaa111")
	}
	if got["darwin/arm64"] != "bbb222" {
		t.Errorf("darwin/arm64 = %q, want %q", got["darwin/arm64"], "bbb222")
	}
}

func TestFetchChecksums_ChecksumFileMissingEntry_FallsBackToIndividual(t *testing.T) {
	tool := makeTool([]string{"linux/amd64"})
	// Checksum file does not contain our asset — triggers fallback.
	checksumFile := []byte("abc123  unrelated_tool\n")
	assetContent := []byte("binary data")
	assetChecksum := sha256Hex(assetContent)

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		var body []byte
		if strings.Contains(req.URL.Path, "checksums.txt") {
			body = checksumFile
		} else {
			body = assetContent
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}
	})

	got, err := FetchChecksums(tool, "v1.0.0")
	if err != nil {
		t.Fatalf("FetchChecksums() unexpected error: %v", err)
	}
	if got["linux/amd64"] != assetChecksum {
		t.Errorf("FetchChecksums() linux/amd64 = %q, want %q", got["linux/amd64"], assetChecksum)
	}
}

func TestFetchChecksums_ChecksumFileDownloadFails_FallsBackToIndividual(t *testing.T) {
	tool := makeTool([]string{"linux/amd64"})
	assetContent := []byte("binary data")
	assetChecksum := sha256Hex(assetContent)

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Path, "checksums.txt") {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(assetContent)), Header: make(http.Header)}
	})

	got, err := FetchChecksums(tool, "v1.0.0")
	if err != nil {
		t.Fatalf("FetchChecksums() unexpected error: %v", err)
	}
	if got["linux/amd64"] != assetChecksum {
		t.Errorf("FetchChecksums() linux/amd64 = %q, want %q", got["linux/amd64"], assetChecksum)
	}
}

func TestFetchChecksums_IndividualDownloadFails(t *testing.T) {
	tool := makeTool([]string{"linux/amd64"})
	mockHTTP(t, 500, []byte("error"))

	_, err := FetchChecksums(tool, "v1.0.0")
	if err == nil {
		t.Error("FetchChecksums() expected error when all downloads fail, got nil")
	}
}

func TestFetchChecksums_InvalidPlatformFormat(t *testing.T) {
	tool := &config.Tool{
		Name:      "mytool",
		Source:    "owner/repo",
		Checksums: map[string]string{"invalid": "abc"},
	}
	// Checksum file download will succeed but then platform parsing fails.
	mockHTTP(t, 200, []byte("abc123  mytool__\n"))

	_, err := FetchChecksums(tool, "v1.0.0")
	if err == nil {
		t.Error("FetchChecksums() expected error for invalid platform format, got nil")
	}
	if !strings.Contains(err.Error(), "invalid platform format") {
		t.Errorf("FetchChecksums() error = %q, want 'invalid platform format'", err.Error())
	}
}

// TestFetchChecksums_PerOSExtension verifies that per-OS extensions are correctly applied
// when rendering asset names. This was previously broken: Ext was never set in FetchChecksums,
// so templates using {ext} produced names like "mytool_linux_amd64." instead of
// "mytool_linux_amd64.tar.gz".
func TestFetchChecksums_PerOSExtension_FromChecksumFile(t *testing.T) {
	tool := makeToolWithExt(
		[]string{"linux/amd64", "darwin/arm64"},
		map[string]string{"linux": "tar.gz", "darwin": "zip", "default": "tar.gz"},
		"{name}_{os}_{arch}.{ext}",
	)
	checksumFile := []byte("aaa111  mytool_linux_amd64.tar.gz\nbbb222  mytool_darwin_arm64.zip\n")

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(checksumFile)), Header: make(http.Header)}
	})

	got, err := FetchChecksums(tool, "v1.0.0")
	if err != nil {
		t.Fatalf("FetchChecksums() unexpected error: %v", err)
	}
	if got["linux/amd64"] != "aaa111" {
		t.Errorf("linux/amd64 = %q, want %q", got["linux/amd64"], "aaa111")
	}
	if got["darwin/arm64"] != "bbb222" {
		t.Errorf("darwin/arm64 = %q, want %q", got["darwin/arm64"], "bbb222")
	}
}

func TestFetchChecksums_PerOSExtension_FallbackToIndividual(t *testing.T) {
	tool := makeToolWithExt(
		[]string{"linux/amd64"},
		map[string]string{"linux": "tar.gz", "default": "tar.gz"},
		"{name}_{os}_{arch}.{ext}",
	)
	assetContent := []byte("binary data")
	assetChecksum := sha256Hex(assetContent)

	mockHTTPDispatch(t, func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Path, "checksums.txt") {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}
		}
		// Verify the requested URL uses the correct extension.
		if !strings.HasSuffix(req.URL.Path, "mytool_linux_amd64.tar.gz") {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(assetContent)), Header: make(http.Header)}
	})

	got, err := FetchChecksums(tool, "v1.0.0")
	if err != nil {
		t.Fatalf("FetchChecksums() unexpected error: %v", err)
	}
	if got["linux/amd64"] != assetChecksum {
		t.Errorf("FetchChecksums() linux/amd64 = %q, want %q", got["linux/amd64"], assetChecksum)
	}
}
