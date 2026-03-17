# Slackline Distribution Design

**Date:** 2026-03-17
**Status:** Approved

## Problem

The `slackline` binary only exists locally. Other team members and agents on fresh machines can't use it without cloning the repo and having Go installed.

## Goal

Make `slackline` installable on any macOS (Apple Silicon) or Linux (amd64) machine without requiring Go.

## Approach

GitHub Actions builds pre-compiled binaries on tagged releases. An install script fetches the right binary for the current platform. Developers with Go can use `go install` instead.

## Components

### CI Workflow (`.github/workflows/ci.yml`)

Triggers on push and PRs to `main`. Runs:
- `go test ./...`
- `golangci-lint run`

Ensures main is always green before tagging a release.

### Release Workflow (`.github/workflows/release.yml`)

Triggers on `v*` tags (e.g. `v0.1.0`). Matrix strategy builds two targets:

| GOOS | GOARCH | Binary name |
|------|--------|-------------|
| darwin | arm64 | `slackline-darwin-arm64` |
| linux | amd64 | `slackline-linux-amd64` |

Both binaries are attached to the GitHub release as assets.

### Install Script (`install.sh`)

Detects the current platform, downloads the matching binary from the latest GitHub release, and installs it to `~/.local/bin/slackline`.

One-liner:
```bash
curl -fsSL https://raw.githubusercontent.com/prime-radiant-inc/slackline/main/install.sh | bash
```

### Makefile

Add a `release` target:
```makefile
release:
	git tag v$(VERSION) && git push origin v$(VERSION)
```

Usage: `make release VERSION=0.1.0`

### Git Remote

Wire the existing local repo to `github.com:prime-radiant-inc/slackline` and push `main`.

### Ops Skill Update

Add an **Installation** section to `primeradiant-ops/skills/slackline/SKILL.md`:

```bash
# Fresh machine (no Go required)
curl -fsSL https://raw.githubusercontent.com/prime-radiant-inc/slackline/main/install.sh | bash

# Developer (Go installed)
go install github.com/prime-radiant/slackline@latest
```

### First Release

Tag `v0.1.0` to verify the full pipeline end-to-end.

## Out of Scope

- Windows support
- Homebrew tap
- Automatic version bumping / changelog generation
- Code signing / notarization
