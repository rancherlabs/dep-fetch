package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-billy/v5"
	"go.yaml.in/yaml/v3"
)

const (
	ModePinned           = "pinned"
	ModeReleaseChecksums = "release-checksums"

	DefaultConfigFile = ".bin-deps.yaml"
	DefaultBinDir     = "./bin"

	EnvConfig    = "DEP_FETCH_CONFIG"
	EnvBinDir    = "DEP_FETCH_BIN_DIR"
	EnvSkipCache = "DEP_FETCH_SKIP_CACHE"
)

type Config struct {
	filePath string `yaml:"-"`
	BinDir   string `yaml:"bin_dir,omitempty"`
	Tools    []Tool `yaml:"tools"`
}

func (c *Config) FilePath() string {
	return c.filePath
}

func (c *Config) GetTool(name string) (Tool, error) {
	for _, t := range c.Tools {
		if t.Name == name {
			return t, nil
		}
	}
	return Tool{}, fmt.Errorf("tool %q not found", name)
}

type Tool struct {
	Name      string            `yaml:"name"`
	Version   string            `yaml:"version"`
	Source    string            `yaml:"source"`
	Mode      string            `yaml:"mode"`
	Checksums map[string]string `yaml:"checksums,omitempty"`
	Release   *ReleaseConfig    `yaml:"release,omitempty"`
}

type ReleaseConfig struct {
	TagPrefix        *string           `yaml:"tag_prefix,omitempty"` // filter releases by tag prefix; defaults to "v" if nil
	DownloadTemplate string            `yaml:"download_template,omitempty"`
	ChecksumTemplate string            `yaml:"checksum_template,omitempty"`
	Extract          string            `yaml:"extract,omitempty"`    // path within archive to use as the binary; empty = asset is the binary
	Extensions       map[string]string `yaml:"extensions,omitempty"` // per-OS file extension, e.g. linux: tar.gz, darwin: zip
}

func (t *Tool) Owner() string {
	owner, _ := splitSource(t.Source)
	return owner
}

func (t *Tool) Repo() string {
	_, repo := splitSource(t.Source)
	return repo
}

func (t *Tool) DownloadTemplate() string {
	if t.Release != nil && t.Release.DownloadTemplate != "" {
		return t.Release.DownloadTemplate
	}
	return "{name}_{os}_{arch}"
}

func (t *Tool) ChecksumTemplate() string {
	if t.Release != nil && t.Release.ChecksumTemplate != "" {
		return t.Release.ChecksumTemplate
	}
	return "checksums.txt"
}

// ExtractPath returns the path within the downloaded archive to use as the binary.
// An empty string means the downloaded asset is the binary itself.
func (t *Tool) ExtractPath() string {
	if t.Release != nil {
		return t.Release.Extract
	}
	return ""
}

// Ext returns the configured file extension for the given OS.
// Falls back to the "default" key in extensions if the OS is not explicitly listed.
func (t *Tool) Ext(goos string) string {
	if t.Release != nil {
		if ext, ok := t.Release.Extensions[goos]; ok {
			return ext
		}
		return t.Release.Extensions["default"]
	}
	return ""
}

// TagPrefix returns the configured tag prefix for filtering releases.
// Returns "v" by default (filters to semantic version tags like v1.2.3).
// Returns empty string if explicitly configured to "" (no filtering).
// Returns the configured prefix for monorepo sub-packages (e.g. "database/v").
func (t *Tool) TagPrefix() string {
	if t.Release != nil && t.Release.TagPrefix != nil {
		return *t.Release.TagPrefix
	}
	return "v"
}

func splitSource(source string) (owner, repo string) {
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// Load reads and validates the config. configPath and binDir may be empty (uses defaults/env).
func Load(fs billy.Filesystem, configPath, binDir string) (*Config, string, error) {
	path := resolveConfigPath(configPath)

	f, err := fs.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading config %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only; close error is not actionable

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, "", fmt.Errorf("parsing config %s: %w", path, err)
	}
	cfg.filePath = path

	resolved := resolveBinDir(binDir, cfg.BinDir)

	if err := validate(&cfg); err != nil {
		return nil, "", err
	}

	return &cfg, resolved, nil
}

func resolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv(EnvConfig); env != "" {
		return env
	}
	return DefaultConfigFile
}

func resolveBinDir(flagValue, configValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv(EnvBinDir); env != "" {
		return env
	}
	if configValue != "" {
		return configValue
	}
	return DefaultBinDir
}
