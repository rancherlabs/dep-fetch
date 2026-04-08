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
	BinDir string `yaml:"bin_dir"`
	Tools  []Tool `yaml:"tools"`
}

type Tool struct {
	Name      string            `yaml:"name"`
	Version   string            `yaml:"version"`
	Source    string            `yaml:"source"`
	Mode      string            `yaml:"mode"`
	Checksums map[string]string `yaml:"checksums"`
	Release   *ReleaseConfig    `yaml:"release"`
}

type ReleaseConfig struct {
	BinaryTemplate   string `yaml:"binary_template"`
	ChecksumTemplate string `yaml:"checksum_template"`
}

func (t *Tool) Owner() string {
	owner, _ := splitSource(t.Source)
	return owner
}

func (t *Tool) Repo() string {
	_, repo := splitSource(t.Source)
	return repo
}

func (t *Tool) BinaryTemplate() string {
	if t.Release != nil && t.Release.BinaryTemplate != "" {
		return t.Release.BinaryTemplate
	}
	return "{name}_{os}_{arch}"
}

func (t *Tool) ChecksumTemplate() string {
	if t.Release != nil && t.Release.ChecksumTemplate != "" {
		return t.Release.ChecksumTemplate
	}
	return "checksums.txt"
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
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, "", fmt.Errorf("parsing config %s: %w", path, err)
	}

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
