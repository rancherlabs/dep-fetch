# dep-fetch — Implementation Notes

Implementation details for building `dep-fetch`. For the user-facing spec, see [spec.md](./spec.md).

---

## Bootstrap and Self-Hosting

`dep-fetch` should bootstrap itself as early in its own development as possible. The goal is that the tool's own repo uses `.bin-deps.yaml` to manage its dev and CI tooling dependencies (golangci-lint, goreleaser, etc.), so that it exercises its own code in the most realistic way.

### The chicken-and-egg problem

Since `dep-fetch` cannot use itself before it is built, the repo needs a minimal bootstrap path:

1. **Initial bootstrap script** (`hack/bootstrap.sh`): a short shell script that downloads a known-good `dep-fetch` release and verifies its checksum manually (hardcoded for that one binary). This is the only ad-hoc fetch script the project will ever have.
2. **From that point on**, all tools (including future `dep-fetch` versions in dev) are declared in `.bin-deps.yaml` and fetched via `dep-fetch sync`.

```bash
# hack/bootstrap.sh — used only for the very first dep-fetch binary
set -euo pipefail
VERSION="v0.1.0"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
URL="https://github.com/rancher/dep-fetch/releases/download/${VERSION}/dep-fetch_${OS}_${ARCH}"
EXPECTED_SHA="<hardcoded-sha256-for-this-version>"

curl -fsSL "${URL}" -o bin/dep-fetch
echo "${EXPECTED_SHA}  bin/dep-fetch" | sha256sum -c -
chmod +x bin/dep-fetch
```

### GHA workflow

```yaml
- name: Bootstrap dep-fetch
  run: bash hack/bootstrap.sh

- name: Fetch dev tools
  run: bin/dep-fetch sync
```

Once a stable release exists and `rancher/dep-fetch` is in its own compiled-in allowlist, `bootstrap.sh` can be replaced with a simple `dep-fetch sync` that fetches `dep-fetch` itself — completing the self-hosting loop.

### Allowlist entry for self-hosting

`rancher/dep-fetch` must be in its own allowlist from the first release:

```go
// allowlist.go
type allowlistEntry struct {
    source        string
    latestAllowed bool
}

var releaseChecksumAllowlist = []allowlistEntry{
    {"rancher/dep-fetch", false},             // self-hosting; pin an explicit tag
    {"rancher/charts-build-scripts", true},   // internal tool repo; latest permitted
    {"rancher/ob-charts-tool", true},         // internal tool repo; latest permitted
}
```

`latestAllowed: false` on `rancher/dep-fetch` is intentional — even though it is on the allowlist for self-hosting, it should always be pinned to an explicit version in `.bin-deps.yaml`. This struct layout makes the two subsets (release-checksums-allowed, latest-allowed) structurally inseparable: adding a repo to the allowlist requires explicitly declaring whether `latest` is permitted, preventing the two sets from drifting independently.

---

## Internal Storage Formats

### Version cache

Location: `.dep-fetch/cache/{owner}-{repo}` (relative to working directory).

Two-line text file — unix timestamp on line 1, resolved tag on line 2:

```
1744060800
v0.18.0
```

The `.dep-fetch/` directory should be in `.gitignore`. If the file is missing, malformed, or older than 24 hours, the tag is re-resolved from the GitHub API and the file is rewritten.

### Receipt file

Location: `.dep-fetch/{name}.receipt` (relative to working directory, alongside the version cache).

Two-line text file — version tag on line 1, SHA-256 hex of the installed binary on line 2:

```
v0.3.1
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

The checksum recorded in the receipt is the SHA-256 of the **extracted binary** placed in `bin_dir/`, not the downloaded asset. For archive assets, these differ intentionally: the asset checksum is verified at download time, and the binary checksum is stored for future receipt checks.

Written atomically on successful install: a temp file is written in `.dep-fetch/`, then renamed into place. The binary itself is installed the same way — downloaded into a temp file, verified, extracted if needed, then renamed into `bin_dir/`. A failed or interrupted install never leaves a partial binary or a receipt that disagrees with the binary on disk.

A tool is considered up-to-date when both the receipt version matches the configured version **and** the binary currently on disk hashes to the recorded checksum. This detects corruption or replacement of the binary independently of the original download verification.

---

## Concurrency Model

`dep-fetch` does not use a lock file. This is safe in practice:

- **CI**: runs in ephemeral containers — one job per container, so `bin_dir` is never shared between concurrent invocations.
- **Local**: single-developer workstation use.

The atomic install flow (temp file → rename) is the safety net: if two invocations somehow ran simultaneously, the worst case is a redundant download. A partial binary landing in `bin_dir` is not possible.

Concurrent invocations against a *shared* `bin_dir` (e.g. a mounted cache volume with multiple jobs writing to the same path) are not supported and could produce inconsistent receipt state.

---

## Allowlist Implementation

The allowlist lives in `allowlist.go` as a package-level slice. Config validation calls into it at parse time — not at download time — so allowlist violations are caught before any network activity. The error message should tell the user exactly what to do: either open a PR to add the source to the allowlist, or switch to `mode: pinned`.

Allowlist membership check is a simple string equality scan over `owner/repo` values. No glob matching, no prefix matching.
