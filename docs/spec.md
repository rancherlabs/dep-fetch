# dep-fetch — Tool Specification

## Overview

`dep-fetch` is a Go CLI that fetches versioned binary dependencies from GitHub Releases. It is designed to unify the local developer experience and CI pipelines, replacing ad-hoc per-tool fetch scripts with a single declarative config.

---

## Invocation

```
dep-fetch sync               # fetch/verify all tools in config
dep-fetch sync <name>        # fetch/verify a single tool by name
dep-fetch list               # show current state of all declared tools
dep-fetch verify             # verify checksums of already-downloaded binaries without re-fetching
dep-fetch update <name> [version]  # update tool version and checksums in config (default version: latest)
```

`dep-fetch verify` checks the SHA-256 of each binary against the checksum recorded in its receipt at install time. Using the receipt checksum (rather than the originally declared or release-provided checksum) is intentionally more defensive: for archive assets the receipt stores the extracted binary's checksum, which differs from the asset checksum. This means `verify` catches corruption or replacement of the extracted binary even when the original asset checksum is no longer available. If a binary is missing entirely, it is downloaded and verified (i.e. `verify` falls back to sync semantics for absent tools). If a download fails, the command exits non-zero.

Config file resolution order:
1. `--config <path>` flag
2. `DEP_FETCH_CONFIG` env var
3. `.bin-deps.yaml` in the working directory

Output directory defaults to `./bin/` and is overridable via `--bin-dir` or `DEP_FETCH_BIN_DIR`.

---

## Config Schema

```yaml
# .bin-deps.yaml

# Optional: override the output directory for all tools (default: ./bin)
bin_dir: ./bin

tools:
  # release-checksums mode: version bumped by Renovate via github-releases datasource.
  # No checksum management in config — checksums are fetched from the release at runtime.
  - name: charts-build-scripts
    # renovate: datasource=github-releases depName=rancher/charts-build-scripts
    version: latest                         # "latest" valid only for allowlisted internal tool repos
    source: rancher/charts-build-scripts
    mode: release-checksums

  - name: ob-charts-tool
    # renovate: datasource=github-releases depName=rancher/ob-charts-tool
    version: v0.3.1
    source: rancher/ob-charts-tool
    mode: release-checksums
    # Optional overrides for release artifact naming conventions:
    release:
      download_template: "ob-charts-tool_{os}_{arch}"
      checksum_template: "ob-charts-tool_{version|trimprefix:v}_checksums.txt"

  # pinned mode: version AND per-platform checksums are both managed by Renovate.
  - name: golangci-lint
    # renovate: datasource=github-releases depName=golangci/golangci-lint
    version: v1.57.2
    source: golangci/golangci-lint
    mode: pinned
    checksums:
      darwin/amd64: "abc123...64char-hex..."  # renovate-local: golangci-lint=v1.57.2
      darwin/arm64: "def456...64char-hex..."  # renovate-local: golangci-lint=v1.57.2
      linux/amd64:  "ghi789...64char-hex..."  # renovate-local: golangci-lint=v1.57.2
      linux/arm64:  "jkl012...64char-hex..."  # renovate-local: golangci-lint=v1.57.2
    release:
      download_template: "golangci-lint-{version}-{os}-{arch}.tar.gz"
      extract: "golangci-lint-{version}-{os}-{arch}/golangci-lint"

  # pinned mode with per-OS archive formats via {ext}:
  - name: mytool
    # renovate: datasource=github-releases depName=owner/mytool
    version: v1.0.0
    source: owner/mytool
    mode: pinned
    checksums:
      linux/amd64:  "abc123...64char-hex..."
      darwin/amd64: "def456...64char-hex..."
      darwin/arm64: "789abc...64char-hex..."
    release:
      download_template: "mytool_{version}_{os}_{arch}.{ext|default:tar.gz}"
      extract: "mytool"
      extensions:
        linux: "tar.gz"
        darwin: "zip"
```

### Field Reference

| Field | Required | Default | Description |
|---|---|---|---|
| `name` | yes | — | Output filename under `bin_dir` |
| `version` | yes | — | Release tag (e.g. `v1.2.3`) or `"latest"`. `"latest"` is only valid with `mode: release-checksums` on an allowlisted internal tool repo — it is a hard error in all other cases. |
| `source` | yes | — | GitHub `owner/repo` |
| `mode` | yes | — | `release-checksums` or `pinned` |
| `release.download_template` | no | `{name}_{os}_{arch}` | Release asset filename to download (include extension, e.g. `.tar.gz`) |
| `release.checksum_template` | no | `checksums.txt` | Checksum file asset name. Used at runtime by `release-checksums` mode to verify downloads, and by `dep-fetch update` to fetch new checksums when updating a `pinned` mode tool. |
| `release.extract` | no | — | Path within archive to use as the binary. Required when `download_template` is an archive. Omit for direct binary assets. |
| `release.extensions` | no | — | Map of OS name to file extension string (e.g. `linux: tar.gz`). Populates the `{ext}` template variable. |
| `checksums` | required for `pinned` | — | Map of `{os}/{arch}` to SHA-256 hex digest of the **downloaded asset** (archive or binary) |

### Template Variables

| Variable | Example |
|---|---|
| `{name}` | `charts-build-scripts` |
| `{os}` | `linux`, `darwin` |
| `{arch}` | `amd64`, `arm64` |
| `{version}` | `v0.18.0` |
| `{ext}` | `tar.gz`, `zip` (per-OS value from `release.extensions`) |

### Template Modifiers

Modifiers transform a variable's value. Apply with `|` after the variable name; chain multiple modifiers with additional `|` separators (applied left to right):

| Modifier | Description | Example |
|---|---|---|
| `upper` | Uppercase | `{os\|upper}` → `LINUX` |
| `lower` | Lowercase | `{os\|lower}` → `linux` |
| `title` | Capitalise first character | `{os\|title}` → `Linux` |
| `trimprefix:X` | Remove leading string X | `{version\|trimprefix:v}` → `0.18.0` |
| `trimsuffix:X` | Remove trailing string X | `{name\|trimsuffix:-tool}` → `charts-build-scripts` |
| `replace:FROM=TO` | Replace exact value | `{arch\|replace:amd64=x86_64}` → `x86_64` |
| `default:X` | Use X if the value is empty | `{ext\|default:tar.gz}` → `tar.gz` |

Chain example: `{version|trimprefix:v|trimsuffix:.0}` strips the `v` prefix then the `.0` patch suffix (e.g. `v1.2.0` → `1.2`).

Windows is out of scope for the initial build (see [Platform Detection](#platform-detection)).

---

## Allowlist

The allowlist is a **compile-time** list of GitHub repos permitted to use `release-checksums` mode. It is not runtime-configurable.

**Rationale:** `release-checksums` delegates trust to the upstream repo's release process — `dep-fetch` downloads and verifies against whatever checksum file the release provides. Allowing arbitrary repos to use this mode would let any config (including one in a PR) introduce a new trusted binary source without review.

Restricting the allowlist to repos we own and control is what makes this delegation sound. Our pending plan to make releases immutable means a checksum file, once published, cannot be silently replaced — further hardening the trust anchor. Any external repo that cannot meet this bar must use `mode: pinned` instead. Exceptions for external repos require security team approval and a reviewed PR to `dep-fetch`.

`version: latest` is only permitted for allowlisted repos and is further restricted in practice to internal *tool* repos (currently `rancher/charts-build-scripts` and `rancher/ob-charts-tool`) — projects we own and release ourselves, where downstream product repos intentionally want to track the latest without Renovate churn. For all other allowlisted repos, pin an explicit tag.

Current allowlist: `rancher/dep-fetch` (self-hosting), `rancher/charts-build-scripts`, `rancher/ob-charts-tool`. Adding an entry requires a PR to `dep-fetch` itself.

Config validation at startup rejects any `release-checksums` tool whose `source` is not in this list, with a clear error directing the user to open that PR or switch to `mode: pinned` with committed checksums.

---

## Modes

### `release-checksums`

Downloads both the binary asset and the release's checksum file, then verifies the binary's SHA-256 against it.

**This mode requires the source repo to be in the compiled-in allowlist.** See [Allowlist](#allowlist) for the trust rationale.

Flow:
1. Resolve version (cache "latest" for 24h; pinned versions skip the cache)
2. Check receipt in `.dep-fetch/` — skip if version matches and binary checksum is intact
3. Download `{download_template}` asset from the GitHub release
4. Download `{checksum_template}` asset from the same release
5. Verify SHA-256 of downloaded asset against the checksum file entry
6. Extract binary if `release.extract` is set (archive assets); decompress if `.gz`
7. Move binary to `bin_dir/{name}`, set executable bit; write receipt to `.dep-fetch/`

### `pinned`

Checksums for each platform are declared directly in the config. No checksum file is fetched from the release at sync time; the config itself is the trust anchor.

This mode works for **any** `source` — no allowlist check is performed. `version: latest` is not valid in this mode — hard error at parse time.

`release.checksum_template` is not used during `sync` or `verify`, but **is** used by `dep-fetch update` to locate the upstream checksum file when refreshing checksums. If the tool's release doesn't publish a checksum file, or the file doesn't cover all declared platforms, `update` falls back to downloading each platform asset individually and computing its SHA-256.

Sync flow:
1. Check receipt in `.dep-fetch/` — skip if version matches and binary checksum is intact
2. Look up `{os}/{arch}` in `checksums` map — error if missing
3. Download `{download_template}` asset
4. Verify SHA-256 of downloaded asset against pinned value
5. Extract binary if `release.extract` is set (archive assets); decompress if `.gz`
6. Move binary to `bin_dir/{name}`, set executable bit; write receipt to `.dep-fetch/`

Update flow (`dep-fetch update <name> [version]`):
1. Resolve version (default: latest release tag from GitHub)
2. Attempt to download `{checksum_template}` asset from the release; parse SHA-256 entries for each declared platform
3. If the checksum file is missing or does not cover all platforms, download each `{download_template}` asset individually and compute its SHA-256
4. Rewrite the tool's `version` and `checksums` fields in the config file in-place, preserving all formatting and comments

---

## Caching

### Version cache (`.dep-fetch/cache/{owner}-{repo}`)

Stores the last resolved "latest" tag with a timestamp. TTL: 24 hours. Add `.dep-fetch/` to `.gitignore`. Set `DEP_FETCH_SKIP_CACHE=1` to bypass it.

### Receipt check (`.dep-fetch/{name}.receipt`)

Before downloading, the tool reads a receipt file from `.dep-fetch/` recording the installed version and the SHA-256 of the binary at install time. A tool is considered up-to-date only when both the version matches **and** the binary on disk still hashes to the recorded checksum. This detects corruption or replacement of the binary independently of the original download verification. Internal file format and atomic write behaviour are described in [implementation.md](./implementation.md).

---

## Error Handling

All errors exit non-zero. No partial state is left in `bin_dir/`.

| Situation | Behaviour |
|---|---|
| Source not in allowlist for `release-checksums` | Hard error at config parse time |
| `version: latest` with `mode: pinned` | Hard error at config parse time |
| `version: latest` with `mode: release-checksums` on non-tool repo | Hard error at config parse time |
| `checksums` missing an entry for current platform | Hard error with list of available platforms |
| HTTP non-200 on binary download | Hard error; nothing written to `bin_dir/` (download is buffered in memory) |
| HTTP non-200 on checksum download | Hard error; nothing written to `bin_dir/` |
| Checksum mismatch | Hard error; nothing written to `bin_dir/`, expected vs actual printed |
| Checksum file has no entry for the binary name | Hard error (do not fall through) |
| Binary missing during `verify` | Download and verify (sync semantics); hard error if download fails |

---

## Platform Detection

`dep-fetch` is a Go binary — `GOOS` and `GOARCH` are always available and are the sole source of platform information. No `uname` fallback is needed or used.

```
GOOS    →  os   (linux, darwin)
GOARCH  →  arch (amd64, arm64)
```

Windows is out of scope for the initial build. The `{os}` template variable does not map to `windows` and `.exe` is not appended. This can be revisited if there is a concrete need.

---

## CI Considerations

Recommended CI pattern:
1. Pin all versions explicitly (no `latest`) and commit checksums for `pinned` mode tools.
2. Set `DEP_FETCH_BIN_DIR` to a path that is cached between runs or recreated fresh each run.
3. Run `dep-fetch sync` as the first step of any job that needs the tools.

`dep-fetch` does not lock `bin_dir`. CI is safe because each job runs in an isolated container; avoid pointing multiple concurrent jobs at a shared writable `bin_dir`.

This replaces per-tool `pull-scripts` invocations in GHA workflows. See [implementation.md](./implementation.md) for the bootstrap and self-hosting strategy used by `dep-fetch`'s own repo.

---

## Future Scope

The following features are intentionally out of scope for the initial build. The tool must exist and work well before these are worth adding.

### `dep-fetch add` — onboarding subcommand

A guided subcommand for adding a new tool to an existing `.bin-deps.yaml`:

```
dep-fetch add k9s v0.50.18 --source derailed/k9s --mode pinned
```

This would:
1. Fetch checksums for the specified version across all configured platforms
2. Insert a new tool entry into `.bin-deps.yaml` — including the `# renovate:` version marker and `# renovate-local:` checksum markers
3. Run `dep-fetch sync <name>` to verify everything works before exiting

The primary implementation challenge is YAML modification that preserves existing formatting and correctly inserts annotation-style comments. Standard Go YAML libraries round-trip through a struct and lose comments entirely; this would require either a comment-preserving YAML library or a targeted append strategy.

Until this subcommand exists, onboarding a new tool is a manual edit to `.bin-deps.yaml` followed by `dep-fetch sync`.

---

## Migration from Existing Scripts

| Current script | Config equivalent |
|---|---|
| `scripts/pull-scripts` | `mode: release-checksums`, `source: rancher/charts-build-scripts` |
| `dev-scripts/pull-scripts` | `mode: release-checksums`, `source: rancher/ob-charts-tool`, `checksum_template: ob-charts-tool_{version|trimprefix:v}_checksums.txt` |

Once the tool is in place, both scripts can be deleted and Makefile/GHA targets updated to call `dep-fetch sync`.
