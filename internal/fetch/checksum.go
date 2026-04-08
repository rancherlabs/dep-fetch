package fetch

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
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
