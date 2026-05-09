package format

import (
	"testing"

	"github.com/rancherlabs/dep-fetch/internal/fetch"
)

func TestStatusLabel_NotInstalled(t *testing.T) {
	s := fetch.ToolStatus{
		Name:             "tool1",
		DeclaredVersion:  "v1.0.0",
		InstalledVersion: "",
	}
	got := StatusLabel(s)
	want := "not installed"
	if got != want {
		t.Errorf("StatusLabel() = %q, want %q", got, want)
	}
}

func TestStatusLabel_UpToDate(t *testing.T) {
	s := fetch.ToolStatus{
		Name:             "tool1",
		DeclaredVersion:  "v1.0.0",
		InstalledVersion: "v1.0.0",
	}
	got := StatusLabel(s)
	want := "current (v1.0.0)"
	if got != want {
		t.Errorf("StatusLabel() = %q, want %q", got, want)
	}
}

func TestStatusLabel_LatestNoCache(t *testing.T) {
	s := fetch.ToolStatus{
		Name:             "tool1",
		DeclaredVersion:  "latest",
		ResolvedVersion:  "",
		InstalledVersion: "v1.0.0",
	}
	got := StatusLabel(s)
	want := "installed (v1.0.0)"
	if got != want {
		t.Errorf("StatusLabel() = %q, want %q", got, want)
	}
}

func TestStatusLabel_LatestOutdated(t *testing.T) {
	s := fetch.ToolStatus{
		Name:             "tool1",
		DeclaredVersion:  "latest",
		ResolvedVersion:  "v2.0.0",
		InstalledVersion: "v1.0.0",
	}
	got := StatusLabel(s)
	want := "outdated (installed v1.0.0, latest v2.0.0)"
	if got != want {
		t.Errorf("StatusLabel() = %q, want %q", got, want)
	}
}

func TestStatusLabel_PinnedOutdated(t *testing.T) {
	s := fetch.ToolStatus{
		Name:             "tool1",
		DeclaredVersion:  "v2.0.0",
		InstalledVersion: "v1.0.0",
	}
	got := StatusLabel(s)
	want := "outdated (installed v1.0.0)"
	if got != want {
		t.Errorf("StatusLabel() = %q, want %q", got, want)
	}
}
