package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/rancherlabs/dep-fetch/internal/config"
)

var (
	configFile string
	binDir     string
	skipCache  bool
)

var rootCmd = &cobra.Command{
	Use:   "dep-fetch",
	Short: "Fetch versioned binary dependencies from GitHub Releases",
	Long: `dep-fetch fetches versioned binary dependencies from GitHub Releases.

It replaces ad-hoc per-tool fetch scripts with a single declarative config
(.bin-deps.yaml), checksum verification on every download, local caching,
and a Renovate-compatible schema so versions stay current automatically.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if skipCache {
			return os.Setenv(config.EnvSkipCache, "1")
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default: .bin-deps.yaml, or $DEP_FETCH_CONFIG)")
	rootCmd.PersistentFlags().StringVar(&binDir, "bin-dir", "", "output directory for binaries (default: ./bin, or $DEP_FETCH_BIN_DIR)")
	rootCmd.PersistentFlags().BoolVar(&skipCache, "skip-cache", false, "bypass the version cache (equivalent to $DEP_FETCH_SKIP_CACHE=1)")
}
