package config

import "fmt"

func validate(cfg *Config) error {
	for i, t := range cfg.Tools {
		if t.Name == "" {
			return fmt.Errorf("tool[%d]: name is required", i)
		}
		if t.Version == "" {
			return fmt.Errorf("tool %q: version is required", t.Name)
		}
		if t.Source == "" {
			return fmt.Errorf("tool %q: source is required", t.Name)
		}
		if t.Owner() == "" || t.Repo() == "" {
			return fmt.Errorf("tool %q: source must be in owner/repo format", t.Name)
		}

		switch t.Mode {
		case ModePinned:
			if t.Version == "latest" {
				return fmt.Errorf("tool %q: version: latest is not valid with mode: pinned", t.Name)
			}
			if len(t.Checksums) == 0 {
				return fmt.Errorf("tool %q: mode: pinned requires checksums", t.Name)
			}
		case ModeReleaseChecksums:
			if !inReleaseChecksumAllowlist(t.Source) {
				return fmt.Errorf(
					"tool %q: source %q is not in the release-checksums allowlist;\n"+
						"  open a PR to add it or switch to mode: pinned with committed checksums",
					t.Name, t.Source,
				)
			}
			if t.Version == "latest" && !inLatestPermitted(t.Source) {
				return fmt.Errorf(
					"tool %q: version: latest is only valid for allowlisted internal tool repos;\n"+
						"  use an explicit version tag for %q",
					t.Name, t.Source,
				)
			}
		case "":
			return fmt.Errorf("tool %q: mode is required (pinned or release-checksums)", t.Name)
		default:
			return fmt.Errorf("tool %q: unknown mode %q (must be pinned or release-checksums)", t.Name, t.Mode)
		}
	}
	return nil
}
