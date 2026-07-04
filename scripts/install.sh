#!/usr/bin/env bash
# ==============================================================================
# Ihand TUI — Installer for macOS & Linux
# ==============================================================================
# Usage:
#   bash scripts/install.sh        # interactive install
#   bash scripts/install.sh --yes  # non-interactive (skip prompts)
# ==============================================================================

set -euo pipefail

BINARY="ihand"
INSTALL_DIR="/usr/local/bin"
COLOR_GREEN="\033[32m"
COLOR_YELLOW="\033[33m"
COLOR_RED="\033[31m"
COLOR_RESET="\033[0m"

AUTO_YES=false
if [[ "${1:-}" == "--yes" ]] || [[ "${1:-}" == "-y" ]]; then
    AUTO_YES=true
fi

echo ""
echo -e "${COLOR_GREEN}╔══════════════════════════════════════╗${COLOR_RESET}"
echo -e "${COLOR_GREEN}║   Ihand TUI — Installer (macOS/Linux) ║${COLOR_RESET}"
echo -e "${COLOR_GREEN}╚══════════════════════════════════════╝${COLOR_RESET}"
echo ""

# --- Prerequisites -----------------------------------------------------------
echo -e "${COLOR_YELLOW}[1/5]${COLOR_RESET} Checking prerequisites..."

if ! command -v go &>/dev/null; then
    echo -e "${COLOR_RED}✗ Go is not installed.${COLOR_RESET}"
    echo "  Install Go from: https://go.dev/dl/"
    echo "  Or on macOS: brew install go"
    echo "  Or on Ubuntu/Debian: sudo apt install golang-go"
    echo "  Or on Fedora: sudo dnf install golang"
    exit 1
fi

GO_VERSION=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1)
echo -e "  ✓ Go found: ${GO_VERSION}"

# --- Build -------------------------------------------------------------------
echo -e "${COLOR_YELLOW}[2/5]${COLOR_RESET} Building ${BINARY}..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

if go build -ldflags="-X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" -o "${BINARY}" .; then
    echo -e "  ✓ Build successful"
else
    echo -e "${COLOR_RED}✗ Build failed${COLOR_RESET}"
    exit 1
fi

# --- Install -----------------------------------------------------------------
echo -e "${COLOR_YELLOW}[3/5]${COLOR_RESET} Installing to ${INSTALL_DIR}/..."

INSTALL_PATH="${INSTALL_DIR}/${BINARY}"

if [[ -f "$INSTALL_PATH" ]]; then
    EXISTING_VERSION=$("$INSTALL_PATH" --version 2>/dev/null || echo "unknown")
    echo -e "  Existing version: ${EXISTING_VERSION}"
    if ! $AUTO_YES; then
        read -p "  Overwrite? [Y/n] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]?$ ]]; then
            echo "  Cancelled."
            exit 0
        fi
    fi
fi

if [[ -w "$INSTALL_DIR" ]]; then
    cp "$BINARY" "$INSTALL_PATH"
else
    echo -e "  ${COLOR_YELLOW}(sudo required for ${INSTALL_DIR})${COLOR_RESET}"
    sudo cp "$BINARY" "$INSTALL_PATH"
fi

chmod +x "$INSTALL_PATH" 2>/dev/null || sudo chmod +x "$INSTALL_PATH"
echo -e "  ✓ Installed: ${INSTALL_PATH}"

# --- Verify ------------------------------------------------------------------
echo -e "${COLOR_YELLOW}[4/5]${COLOR_RESET} Verifying installation..."

INSTALLED_VERSION=$("$INSTALL_PATH" --version 2>/dev/null || echo "unknown")
echo -e "  ✓ Version: ${INSTALLED_VERSION}"
echo -e "  ✓ Location: $(which "$BINARY")"

# --- Cleanup & Done ----------------------------------------------------------
echo -e "${COLOR_YELLOW}[5/5]${COLOR_RESET} Cleaning up..."
rm -f "$PROJECT_DIR/$BINARY"
echo -e "  ✓ Done"

echo ""
echo -e "${COLOR_GREEN}╔══════════════════════════════════════╗${COLOR_RESET}"
echo -e "${COLOR_GREEN}║        Installation Complete! 🎉      ║${COLOR_RESET}"
echo -e "${COLOR_GREEN}╚══════════════════════════════════════╝${COLOR_RESET}"
echo ""
echo -e "  Run: ${COLOR_GREEN}ihand${COLOR_RESET}"
echo ""
echo -e "  Uninstall: sudo rm ${INSTALL_PATH}"
echo ""
