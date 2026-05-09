package cmd

import (
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/rancherlabs/dep-fetch/internal/config"
	"github.com/rancherlabs/dep-fetch/internal/fetch"
	"github.com/rancherlabs/dep-fetch/internal/format"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the current state of all declared tools",
	Long:  `List all tools declared in the config, showing their declared version, installed version, and status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fs := osfs.New(".")

		cfg, resolvedBinDir, err := config.Load(fs, configFile, binDir)
		if err != nil {
			return err
		}

		statuses, err := fetch.List(fs, cfg, resolvedBinDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

		fmt.Printf("%-28s %-12s %-20s %s\n", "NAME", "VERSION", "MODE", "STATUS")
		fmt.Printf("%-28s %-12s %-20s %s\n", "----", "-------", "----", "------")
		for _, s := range statuses {
			fmt.Printf("%-28s %-12s %-20s %s\n", s.Name, s.DeclaredVersion, s.Mode, format.StatusLabel(s))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
