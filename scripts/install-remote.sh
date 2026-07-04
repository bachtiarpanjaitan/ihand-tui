#!/usr/bin/env bash
# ==============================================================================
# Ihand TUI — Remote Installer (via curl | bash)
# ==============================================================================
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bachtiarpanjaitan/ihandtui/main/scripts/install-remote.sh | bash
#
#   Options:
#     --yes / -y     skip prompts
#     --branch <b>   specify git branch (default: main)
# ==============================================================================

set -euo pipefail

REPO_URL="${IHAND_REPO_URL:-https://github.com/bachtiarpanjaitan/ihandtui.git}"
BRANCH="${IHAND_BRANCH:-main}"
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
        --branch) BRANCH="${2:-main}"; shift ;;
    esac
    shift 2>/dev/null || true
done

echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${RESET}"
echo -e "${GREEN}║  Ihand TUI — Remote Installer        ║${RESET}"
echo -e "${GREEN}╚══════════════════════════════════════╝${RESET}"
echo ""

# --- Prerequisites -----------------------------------------------------------
echo -e "${YELLOW}[1/5]${RESET} Checking prerequisites..."

if ! command -v go &>/dev/null; then
    echo -e "${RED}✗ Go is not installed.${RESET}"
    echo "  macOS:  brew install go"
    echo "  Linux:  sudo apt install golang-go  (or equivalent)"
    echo "  Manual: https://go.dev/dl/"
    exit 1
fi
echo -e "  ✓ Go: $(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1)"

if ! command -v git &>/dev/null; then
    echo -e "${RED}✗ Git is not installed.${RESET}"
    exit 1
fi
echo -e "  ✓ Git: $(git --version | awk '{print $3}')"

# --- Clone -------------------------------------------------------------------
echo -e "${YELLOW}[2/5]${RESET} Cloning repository..."

TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t 'ihand-install')
trap 'rm -rf "$TMP_DIR"' EXIT

echo -e "  → $REPO_URL ($BRANCH)"
git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$TMP_DIR/ihandtui" 2>&1 | sed 's/^/     /'
echo -e "  ✓ Done"

# --- Build -------------------------------------------------------------------
echo -e "${YELLOW}[3/5]${RESET} Building ${BINARY}..."

cd "$TMP_DIR/ihandtui"

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")

if go build -ldflags="-X main.version=$VERSION" -o "$BINARY" . 2>&1 | tail -5 | sed 's/^/     /'; then
    echo -e "  ✓ Build successful (v${VERSION})"
else
    echo -e "${RED}✗ Build failed${RESET}"
    echo "  Try building manually:"
    echo "    git clone $REPO_URL && cd ihandtui && go build -o ihand ."
    exit 1
fi

# --- Install -----------------------------------------------------------------
echo -e "${YELLOW}[4/5]${RESET} Installing to ${INSTALL_DIR}/${BINARY}..."

INSTALL_PATH="${INSTALL_DIR}/${BINARY}"

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
echo -e "  ✓ Installed"

# --- Verify ------------------------------------------------------------------
echo -e "${YELLOW}[5/5]${RESET} Verifying..."

if V=$("$INSTALL_PATH" --version 2>/dev/null); then
    echo -e "  ✓ Version: ${V}"
    echo -e "  ✓ Path: $(which "$BINARY" 2>/dev/null || echo "$INSTALL_PATH")"
else
    echo -e "  ${YELLOW}! Verify manually: ihand --version${RESET}"
fi

# --- Done -------------------------------------------------------------------
echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${RESET}"
echo -e "${GREEN}║       Installation Complete! 🎉       ║${RESET}"
echo -e "${GREEN}╚══════════════════════════════════════╝${RESET}"
echo ""
echo -e "  Run:       ${GREEN}${BINARY}${RESET}"
echo -e "  Uninstall: sudo rm ${INSTALL_PATH}"
echo ""
