package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rancher/dep-fetch/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dep-fetch %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.BuildTime)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
