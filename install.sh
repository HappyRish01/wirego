#!/bin/bash

# WireGo Installer Script
set -e

REPO="HappyRish01/wirego"
INSTALL_DIR="/usr/local/bin"

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    i386|i686) ARCH="386" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo ""
echo "WireGo Installer"
echo "────────────────────────────────────────"
echo "OS: $OS"
echo "Arch: $ARCH"
echo "────────────────────────────────────────"

# Get latest release
echo "Fetching latest release..."
LATEST_TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_TAG" ]; then
    echo "error: Could not fetch latest release"
    exit 1
fi

echo "Version: $LATEST_TAG"

# Download and install
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

echo "Downloading..."
curl -sL "https://github.com/$REPO/releases/download/$LATEST_TAG/wirego_${OS}_${ARCH}.tar.gz" | tar -xz -C "$TMP_DIR"

echo "Installing to $INSTALL_DIR..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/wirego" "$INSTALL_DIR/wirego"
    chmod +x "$INSTALL_DIR/wirego"
else
    sudo mv "$TMP_DIR/wirego" "$INSTALL_DIR/wirego"
    sudo chmod +x "$INSTALL_DIR/wirego"
fi

echo ""
echo "────────────────────────────────────────"
echo "WireGo installed successfully!"
echo ""
echo "Usage:"
echo "  wirego send <file-or-directory>"
echo "  wirego receive <code> <folder-name>"
echo "────────────────────────────────────────"
