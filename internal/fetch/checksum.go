package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	ghrelease "github.com/mallardduck/ghreleases"

	"github.com/rancherlabs/dep-fetch/internal/config"
	"github.com/rancherlabs/dep-fetch/internal/release"
)

// verifyReader computes the SHA-256 of all data read from r and compares it to expected.
func verifyReader(r io.Reader, expected string) ([]byte, error) {
	h := sha256.New()
	data, err := io.ReadAll(io.TeeReader(r, h))
	if err != nil {
		return nil, err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return nil, fmt.Errorf("checksum mismatch:\n  expected: %s\n  actual:   %s", expected, actual)
	}
	return data, nil
}

// sha256Hex returns the hex-encoded SHA-256 digest of data.
func sha256Hex(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// FetchChecksums retrieves the SHA-256 checksums for all platforms declared in tool.Checksums
// at the given version. It first attempts to download and parse the tool's checksum file; if
// that fails or is incomplete, it falls back to downloading each asset individually.
func FetchChecksums(tool *config.Tool, version string) (map[string]string, error) {
	baseVars := release.Vars{
		Name:    tool.Name,
		Version: version,
	}
	checksumAsset := release.Render(tool.ChecksumTemplate(), baseVars)
	checksumURL := release.AssetURL(tool.Owner(), tool.Repo(), version, checksumAsset)

	fmt.Printf("  Attempting to use checksum file %s...\n", checksumAsset)
	var checksumBuf bytes.Buffer
	_, err := ghClient.Download(checksumURL, &checksumBuf, ghrelease.DownloadOptions{Context: context.Background()})
	if err == nil {
		allFound := true
		tempChecksums := make(map[string]string)
		for plat := range tool.Checksums {
			vars := baseVars
			parts := strings.Split(plat, "/")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid platform format: %s", plat)
			}
			vars.OS, vars.Arch = parts[0], parts[1]
			vars.Ext = tool.Ext(vars.OS)
			assetName := release.Render(tool.DownloadTemplate(), vars)
			sum, err := parseChecksumFile(checksumBuf.Bytes(), assetName)
			if err != nil {
				allFound = false
				fmt.Printf("    %s not found in checksum file\n", assetName)
				break
			}
			tempChecksums[plat] = sum
		}
		if allFound {
			fmt.Printf("  Found all checksums in %s\n", checksumAsset)
			return tempChecksums, nil
		}
	} else {
		fmt.Printf("  Could not download checksum file %s: %v\n", checksumAsset, err)
	}

	fmt.Printf("  Falling back to downloading individual assets...\n")
	checksums := make(map[string]string)
	for plat := range tool.Checksums {
		parts := strings.Split(plat, "/")
		vars := baseVars
		vars.OS, vars.Arch = parts[0], parts[1]
		vars.Ext = tool.Ext(vars.OS)
		assetName := release.Render(tool.DownloadTemplate(), vars)
		assetURL := release.AssetURL(tool.Owner(), tool.Repo(), version, assetName)

		fmt.Printf("  Fetching %s/%s (%s)...\n", vars.OS, vars.Arch, assetName)
		var buf bytes.Buffer
		if _, err := ghClient.Download(assetURL, &buf, ghrelease.DownloadOptions{Context: context.Background()}); err != nil {
			return nil, fmt.Errorf("downloading %s: %w", assetName, err)
		}
		checksums[plat] = sha256Hex(buf.Bytes())
	}
	return checksums, nil
}

// parseChecksumFile finds the SHA-256 for assetName in a checksums.txt-style file.
// Each line is expected to be: "<hex>  <filename>" (two spaces, GNU coreutils sha256sum format).
func parseChecksumFile(data []byte, assetName string) (string, error) {
	checksums, err := ghrelease.ParseChecksumFile(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("parsing checksum file: %w", err)
	}

	// Try exact match first
	if sum, ok := checksums[assetName]; ok {
		return sum, nil
	}

	// Try matching with various prefix/path variations
	for name, sum := range checksums {
		// Strip common prefixes for comparison
		cleanName := strings.TrimPrefix(strings.TrimPrefix(name, "./"), "*")

		if cleanName == assetName {
			return sum, nil
		}

		// Try matching basename (for path-prefixed entries)
		if strings.HasSuffix(cleanName, "/"+assetName) {
			return sum, nil
		}
	}

	return "", fmt.Errorf("no checksum entry for %q in checksum file", assetName)
}
