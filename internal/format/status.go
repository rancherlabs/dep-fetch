package format

import (
	"fmt"

	"github.com/rancherlabs/dep-fetch/internal/fetch"
)

// StatusLabel converts a ToolStatus to a human-readable status string.
// Logic:
// - Not installed → "not installed"
// - Up to date → "current (version)"
// - Latest with no cache → "installed (version)"
// - Latest with cache mismatch → "outdated (installed X, latest Y)"
// - Pinned version mismatch → "outdated (installed X)"
func StatusLabel(s fetch.ToolStatus) string {
	if !s.IsInstalled() {
		return "not installed"
	}
	if s.IsUpToDate() {
		return fmt.Sprintf("current (%s)", s.InstalledVersion)
	}
	// version: latest with no cache — we don't know what "latest" is, so just report installed.
	if s.DeclaredVersion == "latest" && s.ResolvedVersion == "" {
		return fmt.Sprintf("installed (%s)", s.InstalledVersion)
	}
	// version: latest with a cached tag that differs from installed.
	if s.DeclaredVersion == "latest" {
		return fmt.Sprintf("outdated (installed %s, latest %s)", s.InstalledVersion, s.ResolvedVersion)
	}
	return fmt.Sprintf("outdated (installed %s)", s.InstalledVersion)
}
