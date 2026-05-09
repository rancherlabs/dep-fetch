package update

import (
	"context"
	"fmt"

	"github.com/go-git/go-billy/v5"
	ghrelease "github.com/mallardduck/ghreleases"

	"github.com/rancherlabs/dep-fetch/internal/config"
	"github.com/rancherlabs/dep-fetch/internal/fetch"
)

// ghClient is the shared GitHub client. Matches the pattern in internal/fetch.
var ghClient = ghrelease.NewClient("")

// Options configures an update operation.
type Options struct {
	ToolName   string
	NewVersion string // "latest" or concrete version like "v1.2.3"
}

// Result captures the outcome of an update operation.
type Result struct {
	ToolName        string
	ResolvedVersion string // Concrete version after resolving "latest"
	Checksums       map[string]string
}

// ValidateUpdateable checks if a tool can be updated.
// Returns error if tool doesn't exist or is not in pinned mode.
func ValidateUpdateable(cfg *config.Config, toolName string) (config.Tool, error) {
	tool, err := cfg.GetTool(toolName)
	if err != nil {
		return config.Tool{}, err
	}

	if tool.Mode != config.ModePinned {
		return config.Tool{}, fmt.Errorf("tool %q is not pinned, cannot update", toolName)
	}

	return tool, nil
}

// ResolveVersion converts "latest" to concrete version by querying GitHub.
// Returns the input unchanged if not "latest".
func ResolveVersion(ctx context.Context, tool config.Tool, version string) (string, error) {
	if version != "latest" {
		return version, nil
	}

	v, err := ghClient.LatestRelease(ctx, tool.Owner(), tool.Repo())
	if err != nil {
		return "", fmt.Errorf("fetching latest release for %s/%s: %w", tool.Owner(), tool.Repo(), err)
	}

	return v, nil
}

// Update performs a complete update workflow for a tool.
// Steps:
// 1. Validate tool is updateable
// 2. Resolve version (if "latest")
// 3. Fetch checksums for all platforms
// 4. Update config file
// 5. Invalidate receipt
func Update(ctx context.Context, fs billy.Filesystem, cfg *config.Config, opts Options) (*Result, error) {
	// Step 1: Validate
	tool, err := ValidateUpdateable(cfg, opts.ToolName)
	if err != nil {
		return nil, err
	}

	// Step 2: Resolve version
	resolvedVersion, err := ResolveVersion(ctx, tool, opts.NewVersion)
	if err != nil {
		return nil, err
	}

	// Step 3: Fetch checksums
	checksums, err := fetch.FetchChecksums(&tool, resolvedVersion)
	if err != nil {
		return nil, err
	}

	// Step 4: Update config file
	if err := config.UpdateToolVersion(fs, cfg, opts.ToolName, resolvedVersion, checksums); err != nil {
		return nil, err
	}

	// Step 5: Invalidate receipt to force re-sync
	if err := fetch.InvalidateReceipt(fs, opts.ToolName); err != nil {
		return nil, fmt.Errorf("invalidating receipt for %s: %w", opts.ToolName, err)
	}

	return &Result{
		ToolName:        opts.ToolName,
		ResolvedVersion: resolvedVersion,
		Checksums:       checksums,
	}, nil
}
