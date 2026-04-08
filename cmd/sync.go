package cmd

import (
	"fmt"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/mallardduck/dep-fetch/internal/config"
	"github.com/mallardduck/dep-fetch/internal/fetch"
)

var syncCmd = &cobra.Command{
	Use:   "sync [name...]",
	Short: "Fetch and verify all tools (or specific tools by name)",
	Long: `Fetch and verify all tools declared in the config file.

Pass one or more tool names to sync only those tools:

  dep-fetch sync golangci-lint
  dep-fetch sync golangci-lint charts-build-scripts`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fs := osfs.New(".")

		cfg, resolvedBinDir, err := config.Load(fs, configFile, binDir)
		if err != nil {
			return err
		}

		fmt.Println("Syncing tools...")
		if err := fetch.Sync(fs, cfg, resolvedBinDir, args); err != nil {
			return err
		}
		fmt.Println("Done.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
