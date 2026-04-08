package config

// allowlistEntry describes a repo permitted to use release-checksums mode.
type allowlistEntry struct {
	source        string
	latestAllowed bool // whether version: latest is permitted for this repo
}

// releaseChecksumAllowlist is the compile-time list of GitHub repos permitted to use
// release-checksums mode. Adding an entry requires a PR to dep-fetch itself.
// Set latestAllowed only for internal tool repos we own and release ourselves.
var releaseChecksumAllowlist = []allowlistEntry{
	{"rancher/dep-fetch", false},
	{"rancher/charts-build-scripts", true},
	{"rancher/ob-charts-tool", true},
}

func inReleaseChecksumAllowlist(source string) bool {
	for _, e := range releaseChecksumAllowlist {
		if e.source == source {
			return true
		}
	}
	return false
}

func inLatestPermitted(source string) bool {
	for _, e := range releaseChecksumAllowlist {
		if e.source == source {
			return e.latestAllowed
		}
	}
	return false
}
