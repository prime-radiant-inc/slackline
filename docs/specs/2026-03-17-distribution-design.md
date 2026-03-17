# Slackline Distribution Design

**Date:** 2026-03-17
**Status:** Approved

## Problem

The `slackline` binary only exists locally. Other team members and agents on fresh machines can't use it without cloning the repo and having Go installed.

## Goal

Make `slackline` installable on any macOS (Apple Silicon) or Linux (amd64) machine without requiring Go.

## Approach

GitHub Actions builds pre-compiled binaries on tagged releases. An install script fetches the right binary for the current platform.

**Note:** `go install` is not documented for agents — it requires Go, GOPRIVATE config, and GitHub credentials. The curl install script is the supported path for all non-developer machines.

## Module Path

`go.mod` module path is `github.com/prime-radiant-inc/slackline`, matching the GitHub repo at `github.com/prime-radiant-inc/slackline`.

## Components

### 1. CI Workflow (`.github/workflows/ci.yml`)

Triggers on push and PRs to `main`.

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go test ./...
      - uses: golangci/golangci-lint-action@v6
        with:
          version: v2.1.2   # must match lefthook local version
```

### 2. Release Workflow (`.github/workflows/release.yml`)

Triggers on `v*` tags. Requires `permissions: contents: write` to create releases and upload assets.

```yaml
on:
  push:
    tags: ['v*']

permissions:
  contents: write

jobs:
  release:
    strategy:
      matrix:
        include:
          - goos: darwin
            goarch: arm64
            binary: slackline-darwin-arm64
          - goos: linux
            goarch: amd64
            binary: slackline-linux-amd64
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} go build -ldflags "-X main.version=${{ github.ref_name }}" -o ${{ matrix.binary }} .
      - uses: softprops/action-gh-release@v2
        with:
          files: ${{ matrix.binary }}
```

### 3. Version Embedding

`main.go` declares `var version = "dev"`. The release workflow injects the tag via `-ldflags "-X main.version=..."`. A `--version` flag on the root command prints it.

### 4. Install Script (`install.sh`)

Behaviour:
- Detects OS via `uname -s` (Darwin → darwin, Linux → linux) and arch via `uname -m` (arm64/aarch64 → arm64, x86_64 → amd64)
- Fails fast with a clear error for unsupported platforms
- Fetches latest release tag from GitHub API (`https://api.github.com/repos/prime-radiant-inc/slackline/releases/latest`) using `grep`/`sed` — no `jq` dependency
- Downloads the matching binary with `curl -fsSL`
- Validates the download succeeded (non-empty file, not an HTML error page)
- Creates `~/.local/bin` if it doesn't exist
- Installs to `~/.local/bin/slackline` with `chmod +x`
- Checks whether `~/.local/bin` is in `$PATH`; if not, prints a warning with the shell snippet to add it
- Verifies installation by running `slackline --version`

One-liner:
```bash
curl -fsSL https://raw.githubusercontent.com/prime-radiant-inc/slackline/main/install.sh | bash
```

### 5. Makefile

Add a `release` target with VERSION guard:

```makefile
release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=0.1.0)
endif
	git tag v$(VERSION) && git push origin v$(VERSION)
```

Usage: `make release VERSION=0.1.0`

### 6. Git Remote

```bash
git remote add origin git@github.com:prime-radiant-inc/slackline.git
git push -u origin main
```

### 7. Ops Skill Update

Add an **Installation** section to `primeradiant-ops/skills/slackline/SKILL.md`:

```bash
# Install (no Go required)
curl -fsSL https://raw.githubusercontent.com/prime-radiant-inc/slackline/main/install.sh | bash
```

Note: `~/.local/bin` must be in `$PATH`. The install script warns if it isn't.

### 8. First Release

Tag `v0.1.0` via `make release VERSION=0.1.0` to verify the full pipeline.

## Out of Scope

- Windows support
- Homebrew tap
- Automatic version bumping / changelog generation
- Code signing / notarization / checksum files
- `go install` documentation (requires Go + GOPRIVATE config + GitHub credentials)
