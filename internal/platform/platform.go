package platform

import "runtime"

// Current returns the OS and architecture strings used in template substitution.
// GOOS and GOARCH are the sole source of platform information — no uname fallback.
func Current() (os, arch string) {
	return runtime.GOOS, runtime.GOARCH
}
