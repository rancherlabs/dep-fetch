package cmd

import (
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/rancher/dep-fetch/internal/config"
	"github.com/rancher/dep-fetch/internal/fetch"
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
			fmt.Printf("%-28s %-12s %-20s %s\n", s.Name, s.DeclaredVersion, s.Mode, statusLabel(s))
		}
		return nil
	},
}

func statusLabel(s fetch.ToolStatus) string {
	if !s.IsInstalled() {
		return "not installed"
	}
	if s.IsUpToDate() {
		return fmt.Sprintf("current (%s)", s.InstalledVersion)
	}
	// version: latest with no cache — we don't know what "latest" is, so just report installed.
	if s.DeclaredVersion == "latest" && s.ResolvedVersion == "" {
		return fmt.Sprintf("installed (%s)", s.InstalledVersion)
	}
	// version: latest with a cached tag that differs from installed.
	if s.DeclaredVersion == "latest" {
		return fmt.Sprintf("outdated (installed %s, latest %s)", s.InstalledVersion, s.ResolvedVersion)
	}
	return fmt.Sprintf("outdated (installed %s)", s.InstalledVersion)
}

func init() {
	rootCmd.AddCommand(listCmd)
}
