# dep-fetch

A lightweight, security-conscious CLI for fetching versioned binary dependencies from GitHub Releases — designed to replace ad-hoc per-tool fetch scripts with a single declarative config that works identically in local dev and CI.

## Why this exists

Binary tooling dependencies (linters, chart tools, release scripts) are fetched today via a scattered mix of per-repo `pull-scripts`, Makefile one-liners, and ad-hoc curl invocations.

Problems with that approach:
- Not auditable
- Most skip checksum verification
- Each repo re-invents the same logic independently

`dep-fetch` replaces all of that with a single declarative config (`.bin-deps.yaml`), checksum verification on every download, local caching, and a Renovate-compatible schema so versions and checksums stay current automatically.

## Quick look

```yaml
# .bin-deps.yaml
tools:
  - name: charts-build-scripts
    version: latest
    source: rancher/charts-build-scripts
    mode: release-checksums

  - name: golangci-lint
    # renovate: datasource=github-releases depName=golangci/golangci-lint
    version: v1.57.2
    source: golangci/golangci-lint
    mode: pinned
    checksums:
      linux/amd64:  "abc123..."
      darwin/arm64: "def456..."
```

```
dep-fetch sync        # fetch/verify all tools
dep-fetch verify      # verify checksums without re-fetching
dep-fetch list        # show current state
dep-fetch update <name> <version>  # update a tool's version and checksums
```

## Design philosophy

This is intentionally a **stop-gap tool** — "stop-gap" meaning *better than nothing*, not *temporary*.

**Use something else if you can.** If your environment already has a mature solution (Homebrew bundles, Nix, devbox), use that. `dep-fetch` is not a migration target when better options exist.

**Use this where nothing is in place.** It's a legitimate answer there: auditable, consistent, and easy to replace if something better eventually fits.

**Scope is deliberately narrow:** GitHub Releases only, two trust modes, no plugin system. If a requirement pushes significantly beyond that, a different tool is probably the right answer.

---

## Docs

- [Tool Specification](./spec.md) — config schema, modes, allowlist, caching, and error handling
- [Renovate Integration](./renovate.md) — how Renovate bumps versions and checksums, custom managers, data generator design
- [Implementation Notes](./implementation.md) — bootstrap strategy, self-hosting, internal storage formats, concurrency model
