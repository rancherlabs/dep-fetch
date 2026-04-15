package release

import (
	"fmt"
	"regexp"
	"strings"
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

// tokenRe matches {variable} and {variable|modifier1,modifier2,...} template tokens.
var tokenRe = regexp.MustCompile(`\{([^}|]+)(?:\|([^}]*))?\}`)

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
// Design restriction: modifier arguments (the part after ':') must not contain a
// pipe character, because pipes are the modifier separator. This is unlikely to
// be a problem in practice since asset names do not contain pipes.
//
// Unknown variables or modifiers are left as-is.
func Render(pattern string, v Vars) string {
	vars := map[string]string{
		"name":    v.Name,
		"os":      v.OS,
		"arch":    v.Arch,
		"version": v.Version,
		"ext":     v.Ext,
	}
	return tokenRe.ReplaceAllStringFunc(pattern, func(token string) string {
		m := tokenRe.FindStringSubmatch(token)
		val, ok := vars[m[1]]
		if !ok {
			return token
		}
		if m[2] == "" {
			return val
		}
		for mod := range strings.SplitSeq(m[2], "|") {
			name, arg, _ := strings.Cut(mod, ":")
			switch name {
			case "upper":
				val = strings.ToUpper(val)
			case "lower":
				val = strings.ToLower(val)
			case "title":
				if val != "" {
					val = strings.ToUpper(val[:1]) + val[1:]
				}
			case "trimprefix":
				val = strings.TrimPrefix(val, arg)
			case "trimsuffix":
				val = strings.TrimSuffix(val, arg)
			case "replace":
				from, to, ok := strings.Cut(arg, "=")
				if ok && val == from {
					val = to
				}
			case "default":
				if val == "" {
					val = arg
				}
			}
		}
		return val
	})
}

// AssetURL returns the download URL for a named asset in a GitHub release.
func AssetURL(owner, repo, tag, assetName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, assetName)
}
