#!/usr/bin/env bash
set -euo pipefail

# Gumroad CLI installer
# Usage: curl -fsSL https://raw.githubusercontent.com/antiwork/gumroad-cli/main/install.sh | bash

REPO="antiwork/gumroad-cli"
BIN_NAME="gumroad"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)   OS_KEY="linux" ;;
  Darwin)  OS_KEY="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) OS_KEY="windows" ;;
  *)
    echo "Unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# Detect arch
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH_KEY="amd64" ;;
  arm64|aarch64) ARCH_KEY="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Resolve latest release tag
echo "Fetching latest Gumroad CLI release..."
TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | head -1 \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"

if [ -z "$TAG" ]; then
  echo "Could not determine latest release. Check https://github.com/${REPO}/releases" >&2
  exit 1
fi
echo "Latest version: $TAG"

# Build download URL
if [ "$OS_KEY" = "windows" ]; then
  EXT="zip"
else
  EXT="tar.gz"
fi

FILENAME="${BIN_NAME}-cli_${OS_KEY}_${ARCH_KEY}.${EXT}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${FILENAME}"

# Download
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading $FILENAME..."
curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$FILENAME"

# Extract
if [ "$EXT" = "tar.gz" ]; then
  tar -xzf "$TMP_DIR/$FILENAME" -C "$TMP_DIR"
else
  unzip -q "$TMP_DIR/$FILENAME" -d "$TMP_DIR"
fi

# Install
BINARY="$TMP_DIR/$BIN_NAME"
if [ ! -f "$BINARY" ]; then
  BINARY="$(find "$TMP_DIR" -name "$BIN_NAME" -type f | head -1)"
fi

if [ -z "$BINARY" ] || [ ! -f "$BINARY" ]; then
  echo "Could not find binary after extraction." >&2
  exit 1
fi

chmod +x "$BINARY"

if [ -w "$INSTALL_DIR" ]; then
  mv "$BINARY" "$INSTALL_DIR/$BIN_NAME"
else
  echo "Installing to $INSTALL_DIR (requires sudo)..."
  sudo mv "$BINARY" "$INSTALL_DIR/$BIN_NAME"
fi

# Verify
if command -v "$BIN_NAME" >/dev/null 2>&1; then
  echo ""
  echo "Gumroad CLI installed successfully!"
  echo "Run: gumroad --help"
else
  echo ""
  echo "Installed to $INSTALL_DIR/$BIN_NAME"
  echo "Make sure $INSTALL_DIR is in your PATH."
fi
