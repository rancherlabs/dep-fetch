package receipt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
)

// Receipt holds the recorded state of an installed binary.
type Receipt struct {
	Version  string // e.g. v1.9.14
	Checksum string // SHA-256 hex of the binary at install time
}

func fileName(name string) string {
	return "." + name + ".receipt"
}

// Read returns the stored receipt for a tool, or a zero Receipt if not present or malformed.
func Read(fs billy.Filesystem, binDir, name string) (Receipt, error) {
	path := filepath.Join(binDir, fileName(name))
	f, err := fs.Open(path)
	if err != nil {
		return Receipt{}, nil
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return Receipt{}, fmt.Errorf("reading receipt for %s: %w", name, err)
	}

	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return Receipt{}, nil // malformed or old format — treat as missing
	}
	return Receipt{
		Version:  strings.TrimSpace(lines[0]),
		Checksum: strings.TrimSpace(lines[1]),
	}, nil
}

// Write atomically writes a receipt recording the installed version and binary checksum.
func Write(fs billy.Filesystem, binDir, name, version, checksum string) error {
	tmp, err := fs.TempFile(binDir, "."+name+".receipt.tmp")
	if err != nil {
		return fmt.Errorf("creating receipt temp file for %s: %w", name, err)
	}
	tmpName := tmp.Name()

	_, err = fmt.Fprintf(tmp, "%s\n%s\n", version, checksum)
	tmp.Close()
	if err != nil {
		_ = fs.Remove(tmpName)
		return fmt.Errorf("writing receipt for %s: %w", name, err)
	}

	dest := filepath.Join(binDir, fileName(name))
	if err := fs.Rename(tmpName, dest); err != nil {
		_ = fs.Remove(tmpName)
		return fmt.Errorf("installing receipt for %s: %w", name, err)
	}
	return nil
}

// Verify checks whether the installed binary matches the stored receipt for wantVersion.
// Returns (true, nil) only when the receipt version matches AND the binary on disk hashes
// to the checksum recorded at install time.
// Returns (false, nil) when the receipt is missing, malformed, or records a different version.
// Returns (false, error) when the version matches but the binary checksum does not — this
// indicates corruption or tampering and must be treated as a hard error by the caller.
func Verify(fs billy.Filesystem, binDir, name, wantVersion string) (bool, error) {
	r, err := Read(fs, binDir, name)
	if err != nil {
		return false, err
	}
	if r.Version == "" || r.Version != wantVersion {
		return false, nil
	}

	// Version matches — verify the binary's current checksum against what was recorded.
	actual, err := hashFile(fs, filepath.Join(binDir, name))
	if err != nil {
		// Binary missing or unreadable — not verified.
		return false, nil
	}
	if !strings.EqualFold(actual, r.Checksum) {
		return false, fmt.Errorf(
			"%s: binary checksum does not match receipt (expected %s, got %s) — binary may be corrupted or tampered with",
			name, r.Checksum, actual,
		)
	}
	return true, nil
}

// Manager provides scoped receipt operations for a fixed filesystem and bin directory.
// Construct one with [NewManager] and call Read, Write, and Verify without repeating
// the filesystem and bin directory on every call.
type Manager struct {
	fs     billy.Filesystem
	binDir string
}

// NewManager returns a Manager scoped to the given filesystem and bin directory.
func NewManager(fs billy.Filesystem, binDir string) *Manager {
	return &Manager{fs: fs, binDir: binDir}
}

func (m *Manager) Read(name string) (Receipt, error) {
	return Read(m.fs, m.binDir, name)
}

func (m *Manager) Write(name, version, checksum string) error {
	return Write(m.fs, m.binDir, name, version, checksum)
}

func (m *Manager) Verify(name, wantVersion string) (bool, error) {
	return Verify(m.fs, m.binDir, name, wantVersion)
}

func hashFile(fs billy.Filesystem, path string) (string, error) {
	f, err := fs.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
