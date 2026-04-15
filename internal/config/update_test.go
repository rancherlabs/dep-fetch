package config

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"go.yaml.in/yaml/v3"
)

// --- helpers -----------------------------------------------------------------

func writeConfig(t *testing.T, content string) (*Config, billy.Filesystem) {
	t.Helper()
	fs := memfs.New()
	f, err := fs.Create(DefaultConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprint(f, content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(fs, "", "")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	return cfg, fs
}

func readConfig(t *testing.T, fs billy.Filesystem) string {
	t.Helper()
	f, err := fs.Open(DefaultConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// --- locateToolFields --------------------------------------------------------

func TestLocateToolFields_Version(t *testing.T) {
	const yaml = `tools:
  - name: mytool
    version: v1.0.0
    source: rancher/mytool
    mode: pinned
    checksums:
      linux/amd64: aaaa
`
	locs, err := locateToolFields([]byte(yaml), "mytool")
	if err != nil {
		t.Fatalf("locateToolFields() unexpected error: %v", err)
	}
	if locs.version.value != "v1.0.0" {
		t.Errorf("version value = %q, want v1.0.0", locs.version.value)
	}
	if locs.version.line == 0 {
		t.Error("version line should be non-zero")
	}
}

func TestLocateToolFields_Checksums(t *testing.T) {
	const yaml = `tools:
  - name: mytool
    version: v1.0.0
    source: rancher/mytool
    mode: pinned
    checksums:
      linux/amd64: aaaa
      darwin/amd64: bbbb
`
	locs, err := locateToolFields([]byte(yaml), "mytool")
	if err != nil {
		t.Fatalf("locateToolFields() unexpected error: %v", err)
	}
	if len(locs.checksums) != 2 {
		t.Fatalf("checksums count = %d, want 2", len(locs.checksums))
	}
	if locs.checksums["linux/amd64"].value != "aaaa" {
		t.Errorf("linux/amd64 = %q, want aaaa", locs.checksums["linux/amd64"].value)
	}
	if locs.checksums["darwin/amd64"].value != "bbbb" {
		t.Errorf("darwin/amd64 = %q, want bbbb", locs.checksums["darwin/amd64"].value)
	}
}

func TestLocateToolFields_ToolNotFound(t *testing.T) {
	const yaml = `tools:
  - name: mytool
    version: v1.0.0
    source: rancher/mytool
    mode: pinned
    checksums:
      linux/amd64: aaaa
`
	_, err := locateToolFields([]byte(yaml), "notexist")
	if err == nil {
		t.Fatal("locateToolFields() expected error for unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "notexist") {
		t.Errorf("error should mention tool name, got: %v", err)
	}
}

func TestLocateToolFields_MultiTool(t *testing.T) {
	const yaml = `tools:
  - name: tool-a
    version: v1.0.0
    source: rancher/tool-a
    mode: pinned
    checksums:
      linux/amd64: aaaa
  - name: tool-b
    version: v3.0.0
    source: rancher/tool-b
    mode: pinned
    checksums:
      linux/amd64: bbbb
`
	locs, err := locateToolFields([]byte(yaml), "tool-b")
	if err != nil {
		t.Fatalf("locateToolFields() unexpected error: %v", err)
	}
	if locs.version.value != "v3.0.0" {
		t.Errorf("version value = %q, want v3.0.0", locs.version.value)
	}
}

// --- applyEdits --------------------------------------------------------------

func TestApplyEdits_Version(t *testing.T) {
	lines := strings.Split("tools:\n  - name: mytool\n    version: v1.0.0\n", "\n")
	locs := &toolLocations{
		version:   scalarLocation{line: 3, value: "v1.0.0", style: yaml.Style(0)},
		checksums: map[string]scalarLocation{},
	}

	applyEdits(lines, locs, "v2.0.0", nil)

	if !strings.Contains(lines[2], "v2.0.0") {
		t.Errorf("expected v2.0.0 on line 3, got: %q", lines[2])
	}
}

func TestApplyEdits_DoubleQuotedVersion(t *testing.T) {
	lines := strings.Split(`tools:
  - name: mytool
    version: "v1.0.0"
`, "\n")
	locs := &toolLocations{
		version:   scalarLocation{line: 3, value: "v1.0.0", style: yaml.DoubleQuotedStyle},
		checksums: map[string]scalarLocation{},
	}

	applyEdits(lines, locs, "v2.0.0", nil)

	if !strings.Contains(lines[2], `"v2.0.0"`) {
		t.Errorf("expected double-quoted v2.0.0 on line 3, got: %q", lines[2])
	}
}

func TestApplyEdits_Checksums(t *testing.T) {
	lines := strings.Split("tools:\n  - name: mytool\n    version: v1.0.0\n    checksums:\n      linux/amd64: aaaa\n", "\n")
	locs := &toolLocations{
		version: scalarLocation{line: 3, value: "v1.0.0"},
		checksums: map[string]scalarLocation{
			"linux/amd64": {line: 5, value: "aaaa"},
		},
	}

	applyEdits(lines, locs, "v2.0.0", map[string]string{"linux/amd64": "1111"})

	if !strings.Contains(lines[4], "1111") {
		t.Errorf("expected updated checksum on line 5, got: %q", lines[4])
	}
}

func TestApplyEdits_RenovateComment(t *testing.T) {
	lines := strings.Split("tools:\n  - name: mytool\n    version: v1.0.0\n    checksums:\n      linux/amd64: aaaa # renovate-local: mytool=v1.0.0\n", "\n")
	locs := &toolLocations{
		version: scalarLocation{line: 3, value: "v1.0.0"},
		checksums: map[string]scalarLocation{
			"linux/amd64": {line: 5, value: "aaaa"},
		},
	}

	applyEdits(lines, locs, "v2.0.0", map[string]string{"linux/amd64": "1111"})

	if !strings.Contains(lines[4], "renovate-local: mytool=v2.0.0") {
		t.Errorf("expected renovate-local comment updated, got: %q", lines[4])
	}
}

func TestApplyEdits_SkipsMissingChecksums(t *testing.T) {
	lines := strings.Split("tools:\n  - name: mytool\n    version: v1.0.0\n    checksums:\n      linux/amd64: aaaa\n      darwin/amd64: bbbb\n", "\n")
	locs := &toolLocations{
		version: scalarLocation{line: 3, value: "v1.0.0"},
		checksums: map[string]scalarLocation{
			"linux/amd64":  {line: 5, value: "aaaa"},
			"darwin/amd64": {line: 6, value: "bbbb"},
		},
	}

	// Only supply linux; darwin should be untouched.
	applyEdits(lines, locs, "v2.0.0", map[string]string{"linux/amd64": "1111"})

	if !strings.Contains(lines[4], "1111") {
		t.Errorf("expected linux/amd64 updated, got: %q", lines[4])
	}
	if !strings.Contains(lines[5], "bbbb") {
		t.Errorf("expected darwin/amd64 untouched, got: %q", lines[5])
	}
}

// --- UpdateToolVersion (integration) ----------------------------------------

func TestUpdateToolVersion_Version(t *testing.T) {
	const input = `tools:
  - name: mytool
    version: v1.0.0
    source: rancher/mytool
    mode: pinned
    checksums:
      linux/amd64: aaaa
`
	cfg, fs := writeConfig(t, input)

	if err := UpdateToolVersion(fs, cfg, "mytool", "v2.0.0", nil); err != nil {
		t.Fatalf("UpdateToolVersion() unexpected error: %v", err)
	}

	got := readConfig(t, fs)
	if !strings.Contains(got, "version: v2.0.0") {
		t.Errorf("expected version updated to v2.0.0, got:\n%s", got)
	}
	if strings.Contains(got, "v1.0.0") {
		t.Errorf("expected old version removed, got:\n%s", got)
	}
}

func TestUpdateToolVersion_QuotedVersion(t *testing.T) {
	const input = `tools:
  - name: mytool
    version: "v1.0.0"
    source: rancher/mytool
    mode: pinned
    checksums:
      linux/amd64: aaaa
`
	cfg, fs := writeConfig(t, input)

	if err := UpdateToolVersion(fs, cfg, "mytool", "v2.0.0", nil); err != nil {
		t.Fatalf("UpdateToolVersion() unexpected error: %v", err)
	}

	got := readConfig(t, fs)
	if !strings.Contains(got, `version: "v2.0.0"`) {
		t.Errorf("expected double-quoted version updated, got:\n%s", got)
	}
}

func TestUpdateToolVersion_PreservesOtherTools(t *testing.T) {
	const input = `tools:
  - name: tool-a
    version: v1.0.0
    source: rancher/tool-a
    mode: pinned
    checksums:
      linux/amd64: aaaa
  - name: tool-b
    version: v3.0.0
    source: rancher/tool-b
    mode: pinned
    checksums:
      linux/amd64: bbbb
`
	cfg, fs := writeConfig(t, input)

	if err := UpdateToolVersion(fs, cfg, "tool-a", "v2.0.0", map[string]string{"linux/amd64": "1111"}); err != nil {
		t.Fatalf("UpdateToolVersion() unexpected error: %v", err)
	}

	got := readConfig(t, fs)
	if !strings.Contains(got, "linux/amd64: 1111") {
		t.Errorf("expected tool-a checksum updated, got:\n%s", got)
	}
	if !strings.Contains(got, "version: v3.0.0") || !strings.Contains(got, "linux/amd64: bbbb") {
		t.Errorf("expected tool-b untouched, got:\n%s", got)
	}
}

func TestUpdateToolVersion_ToolNotFound(t *testing.T) {
	const input = `tools:
  - name: mytool
    version: v1.0.0
    source: rancher/mytool
    mode: pinned
    checksums:
      linux/amd64: aaaa
`
	cfg, fs := writeConfig(t, input)

	err := UpdateToolVersion(fs, cfg, "notexist", "v2.0.0", nil)
	if err == nil {
		t.Fatal("UpdateToolVersion() expected error for unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "notexist") {
		t.Errorf("error should mention tool name, got: %v", err)
	}
}
