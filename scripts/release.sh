#!/usr/bin/env bash
# ==============================================================================
# Ihand TUI — Release script
# ==============================================================================
# Usage:
#   bash scripts/release.sh patch    # v0.1.0 → v0.1.1
#   bash scripts/release.sh minor    # v0.1.0 → v0.2.0
#   bash scripts/release.sh major    # v0.1.0 → v1.0.0
#   bash scripts/release.sh 1.2.3    # explicit version → v1.2.3
# ==============================================================================

set -euo pipefail

GREEN="\033[32m"
YELLOW="\033[33m"
RED="\033[31m"
RESET="\033[0m"

# --- Get current version -----------------------------------------------------
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
LATEST_TAG=${LATEST_TAG#v}  # strip leading v
MAJOR=$(echo "$LATEST_TAG" | cut -d. -f1)
MINOR=$(echo "$LATEST_TAG" | cut -d. -f2)
PATCH=$(echo "$LATEST_TAG" | cut -d. -f3)

echo -e "${YELLOW}Current version: v${MAJOR}.${MINOR}.${PATCH}${RESET}"

# --- Determine new version ---------------------------------------------------
BUMP="${1:-patch}"

case "$BUMP" in
    major)
        MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
    minor)
        MINOR=$((MINOR + 1)); PATCH=0 ;;
    patch)
        PATCH=$((PATCH + 1)) ;;
    *)
        # Assume it's an explicit version like "1.2.3"
        NEW_TAG="v${BUMP#v}"
        ;;
esac

if [ -z "${NEW_TAG:-}" ]; then
    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
fi

echo -e "${YELLOW}New version:     ${NEW_TAG}${RESET}"
echo ""

# --- Confirm -----------------------------------------------------------------
if [ "${2:-}" != "--yes" ] && [ "${2:-}" != "-y" ]; then
    read -p "Create and push tag ${NEW_TAG}? [Y/n] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]?$ ]]; then
        echo "Cancelled."
        exit 0
    fi
fi

# --- Ensure on main/master and clean ----------------------------------------
BRANCH=$(git branch --show-current)
if [ "$BRANCH" != "main" ] && [ "$BRANCH" != "master" ]; then
    echo -e "${YELLOW}⚠ You are on branch '${BRANCH}', not main/master.${RESET}"
    read -p "Continue anyway? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Cancelled."
        exit 0
    fi
fi

if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
    echo -e "${RED}✗ Working directory is not clean. Commit or stash changes first.${RESET}"
    exit 1
fi

# --- Create & push tag -------------------------------------------------------
echo ""
echo -e "${GREEN}Creating tag ${NEW_TAG}...${RESET}"
git tag -a "$NEW_TAG" -m "Release ${NEW_TAG}"

echo -e "${GREEN}Pushing tag to origin...${RESET}"
git push origin "$NEW_TAG"

echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${RESET}"
echo -e "${GREEN}║      Release ${NEW_TAG} triggered! 🚀     ║${RESET}"
echo -e "${GREEN}╚══════════════════════════════════════╝${RESET}"
echo ""
echo "  View progress:"
echo "    https://github.com/bachtiarpanjaitan/ihandtui/actions"
echo ""
echo "  Release will appear at:"
echo "    https://github.com/bachtiarpanjaitan/ihandtui/releases/tag/${NEW_TAG}"
echo ""
