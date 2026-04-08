package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	configFile string
	binDir     string
)

var rootCmd = &cobra.Command{
	Use:   "dep-fetch",
	Short: "Fetch versioned binary dependencies from GitHub Releases",
	Long: `dep-fetch fetches versioned binary dependencies from GitHub Releases.

It replaces ad-hoc per-tool fetch scripts with a single declarative config
(.bin-deps.yaml), checksum verification on every download, local caching,
and a Renovate-compatible schema so versions stay current automatically.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default: .bin-deps.yaml, or $DEP_FETCH_CONFIG)")
	rootCmd.PersistentFlags().StringVar(&binDir, "bin-dir", "", "output directory for binaries (default: ./bin, or $DEP_FETCH_BIN_DIR)")
}
