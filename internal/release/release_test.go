package release

import (
	"testing"
)

var defaultVars = Vars{
	Name:    "mytool",
	OS:      "linux",
	Arch:    "amd64",
	Version: "v1.2.3",
}

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		vars    Vars
		want    string
	}{
		{
			name:    "basic substitutions",
			pattern: "{name}_{os}_{arch}",
			vars:    defaultVars,
			want:    "mytool_linux_amd64",
		},
		{
			name:    "replace modifier maps matching value",
			pattern: "{arch|replace:amd64=x86_64}",
			vars:    defaultVars,
			want:    "x86_64",
		},
		{
			name:    "replace modifier no match is noop",
			pattern: "{arch|replace:arm64=aarch64}",
			vars:    defaultVars,
			want:    "amd64",
		},
		{
			name:    "replace modifier single",
			pattern: "{os|replace:darwin=macOS}",
			vars:    Vars{OS: "darwin"},
			want:    "macOS",
		},
		{
			name:    "replace modifier chained for multiple mappings",
			pattern: "{os|replace:darwin=macOS|replace:linux=Linux}",
			vars:    Vars{OS: "darwin"},
			want:    "macOS",
		},
		{
			name:    "replace modifier chained for multiple mappings - linux input",
			pattern: "{os|replace:darwin=macOS|replace:linux=Linux}",
			vars:    Vars{OS: "linux"},
			want:    "Linux",
		},
		{
			name:    "replace modifier goreleaser arch pattern",
			pattern: "{name}_{version}_{arch|replace:amd64=x86_64}",
			vars:    defaultVars,
			want:    "mytool_v1.2.3_x86_64",
		},
		{
			name:    "full archive name",
			pattern: "{name}_{version}_{os}_{arch}.tar.gz",
			vars:    defaultVars,
			want:    "mytool_v1.2.3_linux_amd64.tar.gz",
		},
		{
			name:    "upper modifier",
			pattern: "{os|upper}",
			vars:    defaultVars,
			want:    "LINUX",
		},
		{
			name:    "lower modifier",
			pattern: "{name|lower}",
			vars:    Vars{Name: "MyTool"},
			want:    "mytool",
		},
		{
			name:    "title modifier",
			pattern: "{os|title}",
			vars:    defaultVars,
			want:    "Linux",
		},
		{
			name:    "title modifier on empty string",
			pattern: "{os|title}",
			vars:    Vars{OS: ""},
			want:    "",
		},
		{
			name:    "trimprefix modifier strips v prefix",
			pattern: "{version|trimprefix:v}",
			vars:    defaultVars,
			want:    "1.2.3",
		},
		{
			name:    "trimprefix modifier no match is noop",
			pattern: "{version|trimprefix:x}",
			vars:    defaultVars,
			want:    "v1.2.3",
		},
		{
			name:    "trimsuffix modifier",
			pattern: "{name|trimsuffix:tool}",
			vars:    defaultVars,
			want:    "my",
		},
		{
			name:    "chained modifiers upper then trimprefix",
			pattern: "{os|upper|trimprefix:LIN}",
			vars:    defaultVars,
			want:    "UX",
		},
		{
			name:    "chained trimprefix then trimsuffix",
			pattern: "{version|trimprefix:v|trimsuffix:.3}",
			vars:    defaultVars,
			want:    "1.2",
		},
		{
			name:    "darwin title case",
			pattern: "{os|title}",
			vars:    Vars{OS: "darwin"},
			want:    "Darwin",
		},
		{
			name:    "ext variable linux tar.gz",
			pattern: "{name}_{version}_{os}_{arch}.{ext}",
			vars:    Vars{Name: "gh", OS: "linux", Arch: "amd64", Version: "v2.0.0", Ext: "tar.gz"},
			want:    "gh_v2.0.0_linux_amd64.tar.gz",
		},
		{
			name:    "ext variable darwin zip",
			pattern: "{name}_{version}_{os}_{arch}.{ext}",
			vars:    Vars{Name: "gh", OS: "darwin", Arch: "arm64", Version: "v2.0.0", Ext: "zip"},
			want:    "gh_v2.0.0_darwin_arm64.zip",
		},
		{
			name:    "ext variable windows zip",
			pattern: "{name}_{version}_{os}_{arch}.{ext}",
			vars:    Vars{Name: "gh", OS: "windows", Arch: "amd64", Version: "v2.0.0", Ext: "zip"},
			want:    "gh_v2.0.0_windows_amd64.zip",
		},
		{
			name:    "default modifier uses fallback when ext is empty",
			pattern: "{name}.{ext|default:zip}",
			vars:    Vars{Name: "gh", Ext: ""},
			want:    "gh.zip",
		},
		{
			name:    "default modifier does not override a set value",
			pattern: "{name}.{ext|default:zip}",
			vars:    Vars{Name: "gh", Ext: "tar.gz"},
			want:    "gh.tar.gz",
		},
		{
			name:    "default modifier on non-ext variable",
			pattern: "{os|default:linux}",
			vars:    Vars{OS: ""},
			want:    "linux",
		},
		{
			name:    "unknown variable left as-is",
			pattern: "{unknown_var}",
			vars:    defaultVars,
			want:    "{unknown_var}",
		},
		{
			name:    "unknown modifier still substitutes value",
			pattern: "{os|bogusmod}",
			vars:    defaultVars,
			want:    "linux",
		},
		{
			name:    "empty pattern",
			pattern: "",
			vars:    defaultVars,
			want:    "",
		},
		{
			name:    "no template tokens",
			pattern: "checksums.txt",
			vars:    defaultVars,
			want:    "checksums.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.pattern, tt.vars)
			if got != tt.want {
				t.Errorf("Render(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestAssetURL(t *testing.T) {
	tests := []struct {
		owner, repo, tag, asset string
		want                    string
	}{
		{
			owner: "rancher", repo: "dep-fetch", tag: "v1.0.0", asset: "dep-fetch_linux_amd64",
			want: "https://github.com/rancher/dep-fetch/releases/download/v1.0.0/dep-fetch_linux_amd64",
		},
		{
			owner: "owner", repo: "repo", tag: "v0.1.2", asset: "checksums.txt",
			want: "https://github.com/owner/repo/releases/download/v0.1.2/checksums.txt",
		},
	}
	for _, tt := range tests {
		got := AssetURL(tt.owner, tt.repo, tt.tag, tt.asset)
		if got != tt.want {
			t.Errorf("AssetURL(%q,%q,%q,%q) = %q, want %q", tt.owner, tt.repo, tt.tag, tt.asset, got, tt.want)
		}
	}
}
