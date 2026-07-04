#!/usr/bin/env bash
# ==============================================================================
# Ihand TUI — Remote Installer (via curl | bash)
# ==============================================================================
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bachtiarpanjaitan/ihand-tui/master/scripts/install-remote.sh | bash
#
# Downloads pre-built binary from GitHub Releases. No Go required.
# Options: --yes / -y (skip prompts)
# ==============================================================================

set -euo pipefail

REPO="bachtiarpanjaitan/ihand-tui"
BINARY="ihand"
INSTALL_DIR="/usr/local/bin"
GREEN="\033[32m"
YELLOW="\033[33m"
RED="\033[31m"
RESET="\033[0m"

AUTO_YES=false

for arg in "$@"; do
    case $arg in
        --yes|-y) AUTO_YES=true ;;
    esac
done

echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${RESET}"
echo -e "${GREEN}║  Ihand TUI — Installer               ║${RESET}"
echo -e "${GREEN}╚══════════════════════════════════════╝${RESET}"
echo ""

# --- Detect platform ---------------------------------------------------------
echo -e "${YELLOW}[1/4]${RESET} Detecting platform..."

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo -e "${RED}✗ Unsupported architecture: $ARCH${RESET}"
        exit 1
        ;;
esac

case "$OS" in
    darwin)  PLATFORM="darwin" ;;
    linux)   PLATFORM="linux" ;;
    mingw*|msys*|cygwin*)
        echo -e "${RED}✗ Windows detected. Use install.ps1 instead.${RESET}"
        echo "  powershell -ExecutionPolicy Bypass -File scripts/install.ps1"
        exit 1
        ;;
    *)
        echo -e "${RED}✗ Unsupported OS: $OS${RESET}"
        exit 1
        ;;
esac

echo -e "  ✓ ${PLATFORM}/${ARCH}"

# --- Fetch latest release ----------------------------------------------------
echo -e "${YELLOW}[2/4]${RESET} Fetching latest release..."

API="https://api.github.com/repos/${REPO}/releases/latest"
RELEASE_JSON=$(curl -fsSL "$API" 2>/dev/null || echo "")

if [ -z "$RELEASE_JSON" ]; then
    echo -e "${RED}✗ Cannot reach GitHub API.${RESET}"
    echo "  Check your internet connection or try again later."
    exit 1
fi

TAG=$(echo "$RELEASE_JSON" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)".*/\1/')
echo -e "  ✓ Latest: ${TAG}"

# --- Download binary ---------------------------------------------------------
echo -e "${YELLOW}[3/4]${RESET} Downloading ${BINARY}..."

PACKAGE="ihand-${TAG}-${PLATFORM}-${ARCH}.tar.gz"

# Map to download URL from release assets
DOWNLOAD_URL=$(echo "$RELEASE_JSON" | grep -o "\"browser_download_url\": *\"[^\"]*${PACKAGE}[^\"]*\"" | head -1 | sed 's/.*"\([^"]*\)".*/\1/')

if [ -z "$DOWNLOAD_URL" ]; then
    # Fallback: try raw binary name
    RAW_BIN="ihand-${PLATFORM}-${ARCH}"
    if [ "$OS" = "windows" ]; then RAW_BIN="${RAW_BIN}.exe"; fi
    DOWNLOAD_URL=$(echo "$RELEASE_JSON" | grep -o "\"browser_download_url\": *\"[^\"]*${RAW_BIN}[^\"]*\"" | head -1 | sed 's/.*"\([^"]*\)".*/\1/')
fi

if [ -z "$DOWNLOAD_URL" ]; then
    echo -e "${RED}✗ No pre-built binary for ${PLATFORM}/${ARCH}.${RESET}"
    echo "  Install from source instead:"
    echo "    git clone https://github.com/${REPO}.git"
    echo "    cd ihand-tui && go build -o ihand . && sudo cp ihand /usr/local/bin/"
    exit 1
fi

TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t 'ihand-install')
trap 'rm -rf "$TMP_DIR"' EXIT

echo -e "  → ${DOWNLOAD_URL}"
curl -fsSL --progress-bar -o "$TMP_DIR/$PACKAGE" "$DOWNLOAD_URL"
echo -e "  ✓ Downloaded"

# --- Extract & install -------------------------------------------------------
cd "$TMP_DIR"

if [[ "$PACKAGE" == *.tar.gz ]]; then
    tar xzf "$PACKAGE"
elif [[ "$PACKAGE" == *.zip ]]; then
    unzip -q "$PACKAGE"
fi

# Find the binary in extracted files
if [ -f "ihand" ]; then
    : # found
elif [ -f "ihand.exe" ]; then
    BINARY="ihand.exe"
elif [ -f "$RAW_BIN" ]; then
    cp "$RAW_BIN" "$BINARY" 2>/dev/null || true
else
    # Search for it
    FOUND=$(find . -name "ihand" -o -name "ihand.exe" 2>/dev/null | head -1)
    if [ -n "$FOUND" ]; then
        cp "$FOUND" "$BINARY"
    else
        echo -e "${RED}✗ Binary not found in package.${RESET}"
        exit 1
    fi
fi

chmod +x "$BINARY" 2>/dev/null || true

INSTALL_PATH="${INSTALL_DIR}/ihand"  # always install as 'ihand' (no .exe on unix)

if [ -f "$INSTALL_PATH" ]; then
    EXISTING=$("$INSTALL_PATH" --version 2>/dev/null || echo "unknown")
    echo -e "  Existing: ${EXISTING}"
    if ! $AUTO_YES; then
        read -p "  Overwrite? [Y/n] " -n 1 -r; echo
        if [[ ! $REPLY =~ ^[Yy]?$ ]]; then
            echo "  Cancelled."; exit 0
        fi
    fi
fi

if [ -w "$INSTALL_DIR" ]; then
    cp "$BINARY" "$INSTALL_PATH"
else
    echo -e "  ${YELLOW}(sudo required for ${INSTALL_DIR})${RESET}"
    sudo cp "$BINARY" "$INSTALL_PATH"
fi
chmod +x "$INSTALL_PATH" 2>/dev/null || sudo chmod +x "$INSTALL_PATH"
echo -e "  ✓ Installed: ${INSTALL_PATH}"

# --- Verify ------------------------------------------------------------------
echo -e "${YELLOW}[4/4]${RESET} Verifying..."

if V=$("$INSTALL_PATH" --version 2>/dev/null); then
    echo -e "  ✓ ${V}"
else
    echo -e "  ${YELLOW}! Verify manually: ihand --version${RESET}"
fi

echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${RESET}"
echo -e "${GREEN}║       Installation Complete! 🎉       ║${RESET}"
echo -e "${GREEN}╚══════════════════════════════════════╝${RESET}"
echo ""
echo -e "  Run:       ${GREEN}ihand${RESET}"
echo -e "  Uninstall: sudo rm ${INSTALL_PATH}"
echo ""
