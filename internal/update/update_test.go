package update

import (
	"context"
	"strings"
	"testing"

	"github.com/rancherlabs/dep-fetch/internal/config"
)

func TestValidateUpdateable_Success(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.Tool{
			{
				Name:    "mytool",
				Version: "v1.0.0",
				Source:  "owner/repo",
				Mode:    config.ModePinned,
				Checksums: map[string]string{
					"linux/amd64": "abc123",
				},
			},
		},
	}

	tool, err := ValidateUpdateable(cfg, "mytool")
	if err != nil {
		t.Fatalf("ValidateUpdateable() unexpected error: %v", err)
	}
	if tool.Name != "mytool" {
		t.Errorf("ValidateUpdateable() tool.Name = %q, want %q", tool.Name, "mytool")
	}
}

func TestValidateUpdateable_ToolNotFound(t *testing.T) {
	cfg := &config.Config{Tools: []config.Tool{}}

	_, err := ValidateUpdateable(cfg, "missing")
	if err == nil {
		t.Fatal("ValidateUpdateable() expected error for missing tool, got nil")
	}
}

func TestValidateUpdateable_NotPinned(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.Tool{
			{
				Name:   "mytool",
				Mode:   config.ModeReleaseChecksums,
				Source: "owner/repo",
			},
		},
	}

	_, err := ValidateUpdateable(cfg, "mytool")
	if err == nil {
		t.Fatal("ValidateUpdateable() expected error for non-pinned tool, got nil")
	}
	if !strings.Contains(err.Error(), "not pinned") {
		t.Errorf("ValidateUpdateable() error = %q, want 'not pinned' message", err.Error())
	}
}

func TestResolveVersion_Concrete(t *testing.T) {
	tool := config.Tool{Source: "owner/repo"}

	version, err := ResolveVersion(context.Background(), tool, "v1.5.0")
	if err != nil {
		t.Fatalf("ResolveVersion() unexpected error: %v", err)
	}
	if version != "v1.5.0" {
		t.Errorf("ResolveVersion() = %q, want %q", version, "v1.5.0")
	}
}
