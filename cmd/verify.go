package cmd

import (
	"fmt"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/cobra"

	"github.com/mallardduck/dep-fetch/internal/config"
	"github.com/mallardduck/dep-fetch/internal/fetch"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify checksums of installed binaries",
	Long: `Verify the SHA-256 checksum of each installed binary against its declared
or release-provided checksum.

If a binary is missing, it is downloaded and verified (sync semantics).
Exits non-zero if any verification fails.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fs := osfs.New(".")

		cfg, resolvedBinDir, err := config.Load(fs, configFile, binDir)
		if err != nil {
			return err
		}

		fmt.Println("Verifying tools...")
		if err := fetch.Verify(fs, cfg, resolvedBinDir); err != nil {
			return err
		}
		fmt.Println("Done.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}
