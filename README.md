# dep-fetch

Fetch versioned binary dependencies from GitHub Releases with checksum verification — replace ad-hoc curl scripts with a single declarative config.

## GitHub Actions (primary usage)

Add the reusable action to any workflow. After it runs, all declared tools are available on `PATH`.

```yaml
steps:
  - uses: actions/checkout@v4

  - uses: rancher/dep-fetch/actions/sync-deps@v0.1.0
    with:
      version: v0.1.0

  - name: Run golangci-lint
    run: golangci-lint run ./...
```

Pin `version` to a specific release for production workflows. Omit it to always pull the latest.

## Developer workstations

Download the binary for your platform from [Releases](https://github.com/mallardduck/dep-fetch/releases), or:

```sh
go install github.com/mallardduck/dep-fetch@latest
```

Then run the same commands locally:

```sh
dep-fetch sync     # fetch and verify all tools into ./bin
dep-fetch verify   # verify checksums without re-fetching
dep-fetch list     # show installed vs declared versions
```

## Config

Declare tools in `.bin-deps.yaml` at the root of your repo.

**`pinned` mode** — you supply per-platform checksums. Renovate keeps version and checksums in sync.

```yaml
tools:
  - name: golangci-lint
    mode: pinned
    source: golangci/golangci-lint
    # renovate: datasource=github-releases depName=golangci/golangci-lint
    version: v2.11.4
    checksums:
      linux/amd64:  "abc123..."
      linux/arm64:  "def456..."
      darwin/amd64: "789abc..."
      darwin/arm64: "fed321..."
    release:
      download_template: "golangci-lint-{version|trimprefix:v}-{os}-{arch}.tar.gz"
      extract: "golangci-lint-{version|trimprefix:v}-{os}-{arch}/golangci-lint"
```

**`release-checksums` mode** — dep-fetch fetches and parses the tool's own `checksums.txt` at runtime. No per-platform checksums needed.

```yaml
tools:
  - name: charts-build-scripts-tool
    mode: release-checksums
    source: rancher/charts-build-scripts
    version: latest
```

Use `release.extensions` with the `{ext}` template variable when different platforms ship different archive formats:

```yaml
tools:
  - name: mytool
    mode: pinned
    source: owner/mytool
    version: v1.0.0
    checksums:
      linux/amd64:  "abc123..."
      darwin/amd64: "def456..."
    release:
      download_template: "mytool_{version}_{os}_{arch}.{ext|default:tar.gz}"
      extract: "mytool"
      extensions:
        linux: "tar.gz"
        darwin: "zip"
```

`{ext}` resolves to the value for the current OS. The `default:` modifier supplies a fallback for any OS not listed in `extensions`.

### Template variables

Asset name templates support the following variables and modifiers:

| Token | Description | Example output |
|---|---|---|
| `{name}` | Tool name | `golangci-lint` |
| `{os}` | Operating system | `linux`, `darwin` |
| `{arch}` | Architecture | `amd64`, `arm64` |
| `{version}` | Full version tag | `v2.11.4` |
| `{ext}` | Per-OS file extension from `release.extensions` | `tar.gz`, `zip` |

Modifiers can be applied and chained with additional `|` separators (applied left to right):

| Modifier | Description | Example |
|---|---|---|
| `upper` | Uppercase | `{os\|upper}` → `LINUX` |
| `lower` | Lowercase | `{os\|lower}` → `linux` |
| `title` | Capitalise first character | `{os\|title}` → `Linux` |
| `trimprefix:X` | Remove leading string X | `{version\|trimprefix:v}` → `2.11.4` |
| `trimsuffix:X` | Remove trailing string X | `{name\|trimsuffix:-tool}` → `charts-build-scripts` |
| `replace:FROM=TO` | Replace exact value | `{arch\|replace:amd64=x86_64}` → `x86_64` |
| `default:X` | Use X if the value is empty | `{ext\|default:tar.gz}` → `tar.gz` |

Chain example: `{version\|trimprefix:v\|trimsuffix:.0}` strips the `v` prefix then the `.0` patch suffix (e.g. `v1.2.0` → `1.2`).

