# Distribution Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship pre-compiled `slackline` binaries for macOS (arm64) and Linux (amd64) via GitHub Releases, with a curl install script and automated CI/release pipelines.

**Architecture:** GitHub Actions builds cross-compiled binaries on `v*` tags and attaches them to a GitHub Release. A shell install script detects platform, downloads the right binary, and installs to `~/.local/bin`. Version is embedded at build time via `-ldflags`.

**Tech Stack:** Go 1.25.0 (go.mod minimum; CI installs this via `go-version-file`; local is 1.26.0 — backward compatible), GitHub Actions, `softprops/action-gh-release@v2`, `golangci/golangci-lint-action@v6` pinned to v2.10.1, bash

**Spec:** `docs/specs/2026-03-17-distribution-design.md`

**Dependency note:** Tasks 1–5 can be done in any order. Task 6 (push to remote) must happen before Task 8 (first release). Task 1 (version embedding) must be complete before Task 8.

---

## Chunk 1: Version embedding

### Task 1: Add `--version` flag with build-time injection

**Files:**
- Modify: `main.go`
- Modify: `cmd/root.go`
- Create: `cmd/version_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/version_test.go`:

```go
package cmd

import (
    "testing"
)

func TestSetVersion(t *testing.T) {
    t.Cleanup(func() { SetVersion("dev") })

    SetVersion("v1.2.3")
    if rootCmd.Version != "v1.2.3" {
        t.Errorf("rootCmd.Version = %q, want %q", rootCmd.Version, "v1.2.3")
    }
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./cmd/... -run TestSetVersion -v
```

Expected: `undefined: SetVersion`

- [ ] **Step 3: Add `SetVersion` to `cmd/root.go`**

Add after the `init()` function in `cmd/root.go`:

```go
// SetVersion stores the build-time version string on the root command.
func SetVersion(v string) {
    rootCmd.Version = v
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./cmd/... -run TestSetVersion -v
```

Expected: `PASS`

- [ ] **Step 5: Add `var version` and call `SetVersion` in `main.go`**

Replace the entire contents of `main.go` with:

```go
package main

import (
    "os"

    "github.com/prime-radiant-inc/slackline/cmd"
)

var version = "dev"

func main() {
    cmd.SetVersion(version)
    os.Exit(cmd.Execute())
}
```

- [ ] **Step 6: Run full test suite**

```bash
go test ./...
```

Expected: all packages pass

- [ ] **Step 7: Verify `--version` flag works**

```bash
go build -o slackline . && ./slackline --version
```

Expected: `slackline version dev`

- [ ] **Step 8: Verify ldflags injection works**

```bash
go build -ldflags "-X main.version=v0.1.0" -o slackline . && ./slackline --version
```

Expected: `slackline version v0.1.0`

- [ ] **Step 9: Commit**

```bash
git add main.go cmd/root.go cmd/version_test.go
git commit -m "feat: add --version flag with build-time injection via ldflags"
```

---

## Chunk 2: GitHub Actions workflows

### Task 2: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create the workflows directory**

```bash
mkdir -p .github/workflows
```

- [ ] **Step 2: Create `.github/workflows/ci.yml`**

```yaml
name: CI

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
      - run: go vet ./...
      - run: go test ./...
      - uses: golangci/golangci-lint-action@v6
        with:
          version: v2.10.1
```

- [ ] **Step 3: Validate YAML is well-formed**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML OK"
```

Expected: `YAML OK`

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add CI workflow (vet, test, golangci-lint on push/PR)"
```

---

### Task 3: Release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create `.github/workflows/release.yml`**

```yaml
name: Release

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
      - name: Build
        run: GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} go build -ldflags "-X main.version=${{ github.ref_name }}" -o ${{ matrix.binary }} .
      - name: Sanity check
        run: file ${{ matrix.binary }}
      - name: Upload to release
        uses: softprops/action-gh-release@v2
        with:
          files: ${{ matrix.binary }}
```

- [ ] **Step 2: Validate YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "YAML OK"
```

Expected: `YAML OK`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow (cross-compile darwin/arm64 + linux/amd64 on v* tags)"
```

---

## Chunk 3: Install script and Makefile

### Task 4: Install script

**Files:**
- Create: `install.sh`

- [ ] **Step 1: Create `install.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

REPO="prime-radiant-inc/slackline"
INSTALL_DIR="$HOME/.local/bin"
BINARY="slackline"

# Detect OS
OS=$(uname -s)
case "$OS" in
  Darwin) os="darwin" ;;
  Linux)  os="linux" ;;
  *)
    echo "Unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
  arm64|aarch64) arch="arm64" ;;
  x86_64)        arch="amd64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Only darwin/arm64 and linux/amd64 are supported
if [ "$os" = "linux" ] && [ "$arch" = "arm64" ]; then
  echo "linux/arm64 is not supported. Supported targets: darwin/arm64, linux/amd64." >&2
  exit 1
fi

ASSET="${BINARY}-${os}-${arch}"

# Fetch latest release tag (no jq required)
echo "Fetching latest release..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')

if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release tag." >&2
  exit 1
fi

echo "Installing ${BINARY} ${TAG} (${os}/${arch})..."

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

curl -fsSL "https://github.com/${REPO}/releases/download/${TAG}/${ASSET}" -o "$TMP"

# Validate: non-empty and not an HTML error page
if [ ! -s "$TMP" ]; then
  echo "Download failed: empty file." >&2
  exit 1
fi
if grep -q "<!DOCTYPE" "$TMP" 2>/dev/null; then
  echo "Download failed: received HTML instead of binary." >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
cp "$TMP" "${INSTALL_DIR}/${BINARY}"
chmod +x "${INSTALL_DIR}/${BINARY}"

echo "Installed to ${INSTALL_DIR}/${BINARY}"

# Warn if not in PATH
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
  echo ""
  echo "WARNING: $INSTALL_DIR is not in your PATH."
  case "${SHELL:-}" in
    */zsh)  CFG="~/.zshrc" ;;
    */bash) CFG="~/.bashrc" ;;
    *)      CFG="your shell config file" ;;
  esac
  echo "Add it by running:"
  echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ${CFG} && source ${CFG}"
  echo ""
fi

# Verify
if command -v "$BINARY" &>/dev/null; then
  "$BINARY" --version
else
  "${INSTALL_DIR}/${BINARY}" --version
fi

echo "Done."
```

- [ ] **Step 2: Make executable and syntax check**

```bash
chmod +x install.sh
bash -n install.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

- [ ] **Step 3: Commit**

```bash
git add install.sh
git commit -m "feat: add install.sh for curl-based binary installation"
```

---

### Task 5: Makefile `release` target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Replace `.PHONY` line and append `release` target**

Replace the existing `.PHONY` line:
```makefile
.PHONY: build test vet clean
```
with:
```makefile
.PHONY: build test vet clean release
```

Then append to the end of the file:
```makefile

release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=0.1.0)
endif
	git diff --exit-code HEAD && git diff --cached --exit-code || (echo "Uncommitted changes — commit first" && exit 1)
	git tag v$(VERSION) && git push origin v$(VERSION)
```

- [ ] **Step 2: Verify the VERSION guard fires**

```bash
make release 2>&1 | grep "VERSION is required"
```

Expected: output contains `VERSION is required`

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add make release target with VERSION guard and dirty-tree check"
```

---

## Chunk 4: Wire remote, update skill, tag first release

### Task 6: Wire git remote and push

- [ ] **Step 1: Check if remote already exists, add if not**

```bash
git remote get-url origin 2>/dev/null && echo "Remote already set" || git remote add origin git@github.com:prime-radiant-inc/slackline.git
```

- [ ] **Step 2: Push to origin**

```bash
git push -u origin main
```

Expected: all commits pushed, tracking branch set

- [ ] **Step 3: Verify**

```bash
git remote -v
```

Expected:
```
origin  git@github.com:prime-radiant-inc/slackline.git (fetch)
origin  git@github.com:prime-radiant-inc/slackline.git (push)
```

---

### Task 7: Ops skill — add Installation section

**Files:**
- Modify: `/Users/drewritter/prime-rad/cc-plugin-primeradiant-ops/skills/slackline/SKILL.md`

- [ ] **Step 1: Add Installation section**

Insert this section between the opening description paragraph and the `## Prerequisites` section:

```markdown
## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/prime-radiant-inc/slackline/main/install.sh | bash
```

Installs to `~/.local/bin/slackline`. The script warns if that directory isn't in `$PATH`.
```

- [ ] **Step 2: Verify the file looks correct**

```bash
head -35 /Users/drewritter/prime-rad/cc-plugin-primeradiant-ops/skills/slackline/SKILL.md
```

- [ ] **Step 3: Commit and push**

```bash
cd /Users/drewritter/prime-rad/cc-plugin-primeradiant-ops
git add skills/slackline/SKILL.md
git commit -m "feat: add installation section to slackline skill"
git push
cd -
```

---

### Task 8: Tag first release

**Note:** Tasks 1 and 6 must be complete before this task.

- [ ] **Step 1: Confirm clean state and tests pass**

```bash
go test ./... && git status
```

Expected: all tests pass, `nothing to commit, working tree clean`

- [ ] **Step 2: Tag and push**

```bash
make release VERSION=0.1.0
```

Expected: tag `v0.1.0` created and pushed, release workflow triggers

- [ ] **Step 3: Find and watch the release workflow run**

```bash
gh run list --workflow=release.yml --limit 3
```

Then watch the specific run (use the run ID from the list):

```bash
gh run watch <run-id>
```

Expected: both matrix jobs (darwin/arm64, linux/amd64) complete with green checkmarks

- [ ] **Step 4: Verify release assets**

```bash
gh release view v0.1.0
```

Expected: release shows `slackline-darwin-arm64` and `slackline-linux-amd64` as downloadable assets

- [ ] **Step 5: Smoke test the install script end-to-end**

```bash
INSTALL_DIR=/tmp/slackline-test bash -c 'source install.sh'
```

Or test directly:

```bash
mkdir -p /tmp/slackline-test
curl -fsSL "https://github.com/prime-radiant-inc/slackline/releases/download/v0.1.0/slackline-darwin-arm64" -o /tmp/slackline-test/slackline
chmod +x /tmp/slackline-test/slackline
/tmp/slackline-test/slackline --version
```

Expected: `slackline version v0.1.0`
