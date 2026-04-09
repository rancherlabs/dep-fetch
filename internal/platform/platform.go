package platform

import "runtime"

// Current returns the OS and architecture strings used in template substitution.
// GOOS and GOARCH are the sole source of platform information — no uname fallback.
func Current() (os, arch string) {
	return runtime.GOOS, runtime.GOARCH
}

// AltArch returns the alternative architecture name used by some release naming
// conventions (e.g. goreleaser). amd64 is mapped to x86_64; all other values are
// returned unchanged.
func AltArch(arch string) string {
	if arch == "amd64" {
		return "x86_64"
	}
	return arch
}
