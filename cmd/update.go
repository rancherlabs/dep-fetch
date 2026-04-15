package cmd

import (
	"fmt"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/mallardduck/dep-fetch/internal/config"
	"github.com/mallardduck/dep-fetch/internal/fetch"
	gh "github.com/mallardduck/dep-fetch/internal/github"
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

		targetTool, err := cfg.GetTool(toolName)
		if err != nil {
			return err
		}

		if targetTool.Mode != config.ModePinned {
			return fmt.Errorf("tool %q is not pinned, cannot update", toolName)
		}

		if newVersion == "latest" {
			v, err := gh.LatestRelease(targetTool.Owner(), targetTool.Repo())
			if err != nil {
				return fmt.Errorf("fetching latest release for %s/%s: %w", targetTool.Owner(), targetTool.Repo(), err)
			}
			newVersion = v
		}

		fmt.Printf("Updating %s to %s...\n", toolName, newVersion)

		checksums, err := fetch.FetchChecksums(&targetTool, newVersion)
		if err != nil {
			return err
		}

		if err := config.UpdateToolVersion(fs, cfg, toolName, newVersion, checksums); err != nil {
			return err
		}

		fmt.Printf("Successfully updated %s to %s in %s\n", toolName, newVersion, cfg.FilePath())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
