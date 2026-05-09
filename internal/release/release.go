package release

import (
	"fmt"

	ghrelease "github.com/mallardduck/ghreleases"
)

// Vars holds the substitution values for release asset name patterns declared
// in .bin-deps.yaml (download_template, checksum_template).
type Vars struct {
	Name    string
	OS      string
	Arch    string
	Version string // e.g. v0.18.0
	Ext     string // file extension for the platform, e.g. "tar.gz" or "zip"
}

// Render substitutes all template variables in a release asset name pattern.
// Tokens take the form {variable} or {variable|modifier1,modifier2,...}.
// Modifiers are applied left-to-right and chained with additional `|` separators.
//
// Supported modifiers:
//   - upper              — strings.ToUpper
//   - lower              — strings.ToLower
//   - title              — capitalise first character only (e.g. darwin → Darwin)
//   - trimprefix:ARG     — strings.TrimPrefix(val, ARG)
//   - trimsuffix:ARG     — strings.TrimSuffix(val, ARG)
//   - replace:FROM=TO    — replace exact value (e.g. amd64 → x86_64); noop if no match
//
// Unknown variables or modifiers are left as-is.
func Render(pattern string, v Vars) string {
	vars := ghrelease.TemplateVars{
		Name:    v.Name,
		OS:      v.OS,
		Arch:    v.Arch,
		Version: v.Version,
		Ext:     v.Ext,
	}
	// Use TemplatePermissive to maintain backward compatibility (unknown vars pass through)
	result, err := ghrelease.Render(pattern, vars, ghrelease.TemplateFailsafe)
	if err != nil {
		// Fallback to original pattern if render fails (shouldn't happen in permissive mode)
		return pattern
	}
	return result
}

// AssetURL returns the download URL for a named asset in a GitHub release.
func AssetURL(owner, repo, tag, assetName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, assetName)
}
