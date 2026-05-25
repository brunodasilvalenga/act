#!/bin/sh
set -e

REPO="brunodasilvalenga/act"
BINARY="act"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  msys*|mingw*|cygwin*) echo "For Windows, use: irm https://raw.githubusercontent.com/brunodasilvalenga/act/main/install.ps1 | iex"; exit 1 ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version
VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "Error: could not determine latest version"
  exit 1
fi

# Download
ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/v${VERSION}/checksums.txt"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${BINARY} v${VERSION} for ${OS}/${ARCH}..."
curl -sSfL "$URL" -o "${TMP}/${ARCHIVE}"
curl -sSfL "$CHECKSUM_URL" -o "${TMP}/checksums.txt"

# Verify checksum
EXPECTED=$(grep "$ARCHIVE" "${TMP}/checksums.txt" | awk '{print $1}')
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "${TMP}/${ARCHIVE}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "${TMP}/${ARCHIVE}" | awk '{print $1}')
else
  echo "Warning: no sha256 tool found, skipping checksum verification"
  ACTUAL="$EXPECTED"
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Checksum mismatch!"; exit 1
fi

# Extract and install
tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"
echo "Installed ${BINARY} v${VERSION} to ${INSTALL_DIR}/${BINARY}"
