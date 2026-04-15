package fetch

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/mallardduck/dep-fetch/internal/config"
	gh "github.com/mallardduck/dep-fetch/internal/github"
	"github.com/mallardduck/dep-fetch/internal/release"
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
	vars := release.Vars{
		Name:    tool.Name,
		Version: version,
	}
	checksumAsset := release.Render(tool.ChecksumTemplate(), vars)
	checksumURL := release.AssetURL(tool.Owner(), tool.Repo(), version, checksumAsset)

	fmt.Printf("  Attempting to use checksum file %s...\n", checksumAsset)
	var checksumBuf bytes.Buffer
	err := gh.DownloadAsset(checksumURL, &checksumBuf)
	if err == nil {
		allFound := true
		tempChecksums := make(map[string]string)
		for plat := range tool.Checksums {
			parts := strings.Split(plat, "/")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid platform format: %s", plat)
			}
			v := vars
			v.OS, v.Arch = parts[0], parts[1]
			assetName := release.Render(tool.BinaryTemplate(), v)
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
		v := vars
		v.OS, v.Arch = parts[0], parts[1]
		assetName := release.Render(tool.BinaryTemplate(), v)
		assetURL := release.AssetURL(tool.Owner(), tool.Repo(), version, assetName)

		fmt.Printf("  Fetching %s/%s (%s)...\n", v.OS, v.Arch, assetName)
		var buf bytes.Buffer
		if err := gh.DownloadAsset(assetURL, &buf); err != nil {
			return nil, fmt.Errorf("downloading %s: %w", assetName, err)
		}
		checksums[plat] = sha256Hex(buf.Bytes())
	}
	return checksums, nil
}

// parseChecksumFile finds the SHA-256 for assetName in a checksums.txt-style file.
// Each line is expected to be: "<hex>  <filename>" (two spaces, GNU coreutils sha256sum format).
func parseChecksumFile(data []byte, assetName string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		// Support both one-space and two-space separators.
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		// Some checksum files prefix the filename with "./" or "*".
		name := strings.TrimPrefix(strings.TrimPrefix(parts[1], "./"), "*")
		if name == assetName || strings.HasSuffix(name, "/"+assetName) {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %q in checksum file", assetName)
}
