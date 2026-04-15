package config

import (
	"fmt"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
)

func TestToolOwnerRepo(t *testing.T) {
	tests := []struct {
		source    string
		wantOwner string
		wantRepo  string
	}{
		{"rancher/dep-fetch", "rancher", "dep-fetch"},
		{"owner/repo", "owner", "repo"},
		{"", "", ""},
		{"noslash", "", ""},
	}
	for _, tt := range tests {
		tool := Tool{Source: tt.source}
		if got := tool.Owner(); got != tt.wantOwner {
			t.Errorf("Owner(%q) = %q, want %q", tt.source, got, tt.wantOwner)
		}
		if got := tool.Repo(); got != tt.wantRepo {
			t.Errorf("Repo(%q) = %q, want %q", tt.source, got, tt.wantRepo)
		}
	}
}

func TestToolBinaryTemplate(t *testing.T) {
	t.Run("default when release is nil", func(t *testing.T) {
		tool := Tool{}
		if got := tool.BinaryTemplate(); got != "{name}_{os}_{arch}" {
			t.Errorf("BinaryTemplate() = %q, want default", got)
		}
	})
	t.Run("default when release template is empty", func(t *testing.T) {
		tool := Tool{Release: &ReleaseConfig{}}
		if got := tool.BinaryTemplate(); got != "{name}_{os}_{arch}" {
			t.Errorf("BinaryTemplate() = %q, want default", got)
		}
	})
	t.Run("custom template", func(t *testing.T) {
		tool := Tool{Release: &ReleaseConfig{DownloadTemplate: "{name}_{version}_{os}"}}
		if got := tool.BinaryTemplate(); got != "{name}_{version}_{os}" {
			t.Errorf("BinaryTemplate() = %q, want custom", got)
		}
	})
}

func TestToolChecksumTemplate(t *testing.T) {
	t.Run("default when release is nil", func(t *testing.T) {
		tool := Tool{}
		if got := tool.ChecksumTemplate(); got != "checksums.txt" {
			t.Errorf("ChecksumTemplate() = %q, want default", got)
		}
	})
	t.Run("custom template", func(t *testing.T) {
		tool := Tool{Release: &ReleaseConfig{ChecksumTemplate: "{name}_{version}_checksums.txt"}}
		if got := tool.ChecksumTemplate(); got != "{name}_{version}_checksums.txt" {
			t.Errorf("ChecksumTemplate() = %q, want custom", got)
		}
	})
}

func TestToolExtractPath(t *testing.T) {
	t.Run("empty when release is nil", func(t *testing.T) {
		tool := Tool{}
		if got := tool.ExtractPath(); got != "" {
			t.Errorf("ExtractPath() = %q, want empty", got)
		}
	})
	t.Run("returns extract path", func(t *testing.T) {
		tool := Tool{Release: &ReleaseConfig{Extract: "bin/mytool"}}
		if got := tool.ExtractPath(); got != "bin/mytool" {
			t.Errorf("ExtractPath() = %q, want %q", got, "bin/mytool")
		}
	})
}

func TestToolExt(t *testing.T) {
	extensions := map[string]string{
		"linux":   "tar.gz",
		"darwin":  "zip",
		"windows": "zip",
	}
	tool := Tool{Release: &ReleaseConfig{Extensions: extensions}}

	tests := []struct {
		goos string
		want string
	}{
		{"linux", "tar.gz"},
		{"darwin", "zip"},
		{"windows", "zip"},
		{"freebsd", ""},
	}
	for _, tt := range tests {
		if got := tool.Ext(tt.goos); got != tt.want {
			t.Errorf("Ext(%q) = %q, want %q", tt.goos, got, tt.want)
		}
	}

	t.Run("nil release returns empty", func(t *testing.T) {
		empty := Tool{}
		if got := empty.Ext("linux"); got != "" {
			t.Errorf("Ext(linux) on nil release = %q, want empty", got)
		}
	})
}

func TestLoad(t *testing.T) {
	validYAML := `
tools:
  - name: mytool
    version: v1.0.0
    source: rancher/charts-build-scripts
    mode: release-checksums
`

	t.Run("loads valid config", func(t *testing.T) {
		fs := memfs.New()
		f, err := fs.Create(DefaultConfigFile)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprint(f, validYAML); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		cfg, binDir, err := Load(fs, "", "")
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if len(cfg.Tools) != 1 {
			t.Errorf("Load() tools count = %d, want 1", len(cfg.Tools))
		}
		if cfg.Tools[0].Name != "mytool" {
			t.Errorf("Load() tool name = %q, want %q", cfg.Tools[0].Name, "mytool")
		}
		if binDir != DefaultBinDir {
			t.Errorf("Load() binDir = %q, want %q", binDir, DefaultBinDir)
		}
	})

	t.Run("respects bin_dir from config", func(t *testing.T) {
		fs := memfs.New()
		f, _ := fs.Create(DefaultConfigFile)
		if _, err := fmt.Fprint(f, `
bin_dir: ./tools
tools:
  - name: mytool
    version: v1.0.0
    source: rancher/charts-build-scripts
    mode: release-checksums
`); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		_, binDir, err := Load(fs, "", "")
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if binDir != "./tools" {
			t.Errorf("Load() binDir = %q, want %q", binDir, "./tools")
		}
	})

	t.Run("flag binDir overrides config", func(t *testing.T) {
		fs := memfs.New()
		f, _ := fs.Create(DefaultConfigFile)
		if _, err := fmt.Fprint(f, `
bin_dir: ./tools
tools:
  - name: mytool
    version: v1.0.0
    source: rancher/charts-build-scripts
    mode: release-checksums
`); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		_, binDir, err := Load(fs, "", "./override")
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if binDir != "./override" {
			t.Errorf("Load() binDir = %q, want %q", binDir, "./override")
		}
	})

	t.Run("custom config path", func(t *testing.T) {
		fs := memfs.New()
		f, _ := fs.Create("custom.yaml")
		if _, err := fmt.Fprint(f, validYAML); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		cfg, _, err := Load(fs, "custom.yaml", "")
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if len(cfg.Tools) != 1 {
			t.Errorf("Load() tools count = %d, want 1", len(cfg.Tools))
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		fs := memfs.New()
		_, _, err := Load(fs, "", "")
		if err == nil {
			t.Error("Load() expected error for missing file, got nil")
		}
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		fs := memfs.New()
		f, _ := fs.Create(DefaultConfigFile)
		if _, err := fmt.Fprint(f, "key: {unclosed"); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		_, _, err := Load(fs, "", "")
		if err == nil {
			t.Error("Load() expected error for invalid YAML, got nil")
		}
	})

	t.Run("returns error for invalid config", func(t *testing.T) {
		fs := memfs.New()
		f, _ := fs.Create(DefaultConfigFile)
		if _, err := fmt.Fprint(f, `tools:
  - name: ""
    version: v1.0.0
    source: a/b
    mode: pinned
    checksums:
      linux/amd64: abc
`); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		_, _, err := Load(fs, "", "")
		if err == nil {
			t.Error("Load() expected validation error, got nil")
		}
	})
}
