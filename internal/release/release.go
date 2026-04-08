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
		"{version}", v.Version,
		"{version_no_v}", v.VersionWithoutV(),
	)
	return r.Replace(pattern)
}

// AssetURL returns the download URL for a named asset in a GitHub release.
func AssetURL(owner, repo, tag, assetName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, assetName)
}

// AssetFilename returns the release asset filename for a rendered binary template.
// When the template contains a "/" (archive path), the asset is the first path
// component with ".tar.gz" appended. Otherwise it is the template value itself.
func AssetFilename(renderedBinaryTemplate string) string {
	if strings.Contains(renderedBinaryTemplate, "/") {
		return strings.SplitN(renderedBinaryTemplate, "/", 2)[0] + ".tar.gz"
	}
	return renderedBinaryTemplate
}
