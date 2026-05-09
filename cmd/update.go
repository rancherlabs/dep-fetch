package cmd

import (
	"context"
	"fmt"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/rancherlabs/dep-fetch/internal/config"
	"github.com/rancherlabs/dep-fetch/internal/update"
)

var updateCmd = &cobra.Command{
	Use:   "update [tool] [version (default: latest)]",
	Short: "Update a tool's version and checksums in the configuration file",
	Long: `Update a tool's version and checksums in the configuration file.
The command first attempts to download the checksum file (using checksum_template if provided).
If the checksum file is missing or incomplete, it falls back to downloading each asset
individually and calculating its SHA-256 checksum.
If version is "latest", the latest release tag is fetched from GitHub.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		newVersion := "latest"
		if len(args) == 2 {
			newVersion = args[1]
		}

		fs := osfs.New(".")
		cfg, _, err := config.Load(fs, configFile, "")
		if err != nil {
			return err
		}

		fmt.Printf("Updating %s to %s...\n", toolName, newVersion)

		result, err := update.Update(context.Background(), fs, cfg, update.Options{
			ToolName:   toolName,
			NewVersion: newVersion,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Successfully updated %s to %s in %s\n",
			result.ToolName, result.ResolvedVersion, cfg.FilePath())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
