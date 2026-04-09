package release

import (
	"fmt"
	"strings"
)

// Vars holds the substitution values for release asset name patterns declared
// in .bin-deps.yaml (binary_template, checksum_template).
type Vars struct {
	Name    string
	OS      string
	Arch    string
	ArchAlt string // alternative arch name, e.g. x86_64 for amd64
	Version string // e.g. v0.18.0
}

// VersionWithoutV returns the version string without the leading "v".
func (v Vars) VersionWithoutV() string {
	return strings.TrimPrefix(v.Version, "v")
}

// Render substitutes all template variables in a release asset name pattern.
func Render(pattern string, v Vars) string {
	r := strings.NewReplacer(
		"{name}", v.Name,
		"{os}", v.OS,
		"{arch}", v.Arch,
		"{arch_alt}", v.ArchAlt,
		"{version}", v.Version,
		"{version_no_v}", v.VersionWithoutV(),
	)
	return r.Replace(pattern)
}

// AssetURL returns the download URL for a named asset in a GitHub release.
func AssetURL(owner, repo, tag, assetName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, assetName)
}
