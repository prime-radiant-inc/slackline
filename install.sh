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
  echo "GitHub CLI (gh) is required so release attestations can be verified." >&2
  exit 1
fi

# Fetch latest release tag: prefer authenticated gh (works for private repos), fall back to curl
echo "Fetching latest release..."
if command -v gh &>/dev/null && gh auth status &>/dev/null 2>&1; then
  TAG=$(gh release view --repo "${REPO}" --json tagName -q .tagName 2>/dev/null)
else
  TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
fi

if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release tag." >&2
  exit 1
fi

echo "Installing ${BINARY} ${TAG} (${os}/${arch})..."

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

# Download: prefer gh (works for private repos), fall back to curl (works for public repos)
if command -v gh &>/dev/null && gh auth status &>/dev/null 2>&1; then
  echo "Downloading via gh..."
  gh release download "${TAG}" --repo "${REPO}" --pattern "${ASSET}" --output "$TMP" --clobber
else
  echo "Downloading via curl..."
  curl -fsSL "https://github.com/${REPO}/releases/download/${TAG}/${ASSET}" -o "$TMP"
fi

# Validate: non-empty and not an HTML error page
if [ ! -s "$TMP" ]; then
  echo "Download failed: empty file." >&2
  exit 1
fi
if grep -q "<!DOCTYPE" "$TMP" 2>/dev/null; then
  echo "Download failed: received HTML instead of binary." >&2
  exit 1
fi
echo "Verifying release attestation..."
if ! gh attestation verify "$TMP" --repo "${REPO}" >/dev/null; then
  echo "Download failed: release attestation verification failed." >&2
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
