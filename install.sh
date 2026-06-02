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
if { [ "$os" = "linux" ] && [ "$arch" = "arm64" ]; } || { [ "$os" = "darwin" ] && [ "$arch" = "amd64" ]; }; then
  echo "${os}/${arch} is not supported. Supported targets: darwin/arm64, linux/amd64." >&2
  exit 1
fi

ASSET="${BINARY}-${os}-${arch}"

if ! command -v gh &>/dev/null; then
  echo "GitHub CLI (gh) is required so release metadata can be verified." >&2
  exit 1
fi

if ! gh auth status &>/dev/null 2>&1; then
  echo "GitHub CLI (gh) must be authenticated to install from ${REPO}." >&2
  exit 1
fi

sha256_file() {
  if command -v shasum &>/dev/null; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  if command -v sha256sum &>/dev/null; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  echo "No SHA-256 checksum tool found (need shasum or sha256sum)." >&2
  return 1
}

# Fetch latest release tag and expected asset digest via gh (works for private repos)
echo "Fetching latest release..."
TAG=$(gh release view --repo "${REPO}" --json tagName -q .tagName 2>/dev/null || true)

if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release tag." >&2
  exit 1
fi

echo "Installing ${BINARY} ${TAG} (${os}/${arch})..."

EXPECTED_DIGEST=$(gh release view "${TAG}" --repo "${REPO}" --json assets -q ".assets[] | select(.name == \"${ASSET}\") | .digest" 2>/dev/null || true)
if [ -z "$EXPECTED_DIGEST" ]; then
  echo "Failed to fetch release asset digest for ${ASSET}." >&2
  exit 1
fi
case "$EXPECTED_DIGEST" in
  sha256:*) EXPECTED_SHA=${EXPECTED_DIGEST#sha256:} ;;
  *)
    echo "Unsupported release asset digest: ${EXPECTED_DIGEST}" >&2
    exit 1
    ;;
esac

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

echo "Downloading via gh..."
gh release download "${TAG}" --repo "${REPO}" --pattern "${ASSET}" --output "$TMP" --clobber

# Validate: non-empty and not an HTML error page
if [ ! -s "$TMP" ]; then
  echo "Download failed: empty file." >&2
  exit 1
fi
if grep -q "<!DOCTYPE" "$TMP" 2>/dev/null; then
  echo "Download failed: received HTML instead of binary." >&2
  exit 1
fi
echo "Verifying release asset digest..."
ACTUAL_SHA=$(sha256_file "$TMP")
if [ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]; then
  echo "Download failed: release asset digest mismatch." >&2
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
