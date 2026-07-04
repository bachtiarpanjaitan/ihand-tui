# Ihand TUI — Build & Install Makefile
# Usage:
#   make build       — compile binary locally
#   make install     — build & install to /usr/local/bin (macOS/Linux)
#   make uninstall   — remove from /usr/local/bin
#   make build-all   — cross-compile for all platforms → dist/

BINARY   := ihand
PKG      := .
DIST_DIR := dist

# Version — override with: make build VERSION=1.2.3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags="-X main.version=$(VERSION)"

.PHONY: build install install-remote uninstall build-all clean release unrelease

build:
	@echo "🔨 Building $(BINARY)..."
	go build $(LDFLAGS) -o $(BINARY) $(PKG)
	@echo "✅ Binary: $(BINARY)"

install: build
	@echo "📦 Installing $(BINARY) to /usr/local/bin..."
	@if [ -w /usr/local/bin ]; then \
		cp $(BINARY) /usr/local/bin/$(BINARY); \
	else \
		sudo cp $(BINARY) /usr/local/bin/$(BINARY); \
	fi
	@echo "✅ Installed! Run 'ihand' to start."
	@echo "   Uninstall: make uninstall"

install-remote:
	@echo "🌐 Running remote installer..."
	bash scripts/install-remote.sh --yes
	@echo ""
	@echo "To use via curl:"
	@echo "  curl -fsSL <raw-url>/scripts/install-remote.sh | bash"

release:
	bash scripts/release.sh $(filter-out $@,$(MAKECMDGOALS)) --yes

unrelease:
	@echo "🗑 Removing tag $(TAG)..."
	git tag -d $(TAG) 2>/dev/null || true
	git push --delete origin $(TAG) 2>/dev/null || true
	@echo "✅ Tag $(TAG) removed locally + remote."
	@echo "⚠  Release still needs manual deletion at:"
	@echo "   https://github.com/bachtiarpanjaitan/ihandtui/releases"

uninstall:
	@echo "🗑 Removing /usr/local/bin/$(BINARY)..."
	@if [ -w /usr/local/bin ]; then \
		rm -f /usr/local/bin/$(BINARY); \
	else \
		sudo rm -f /usr/local/bin/$(BINARY); \
	fi
	@echo "✅ Removed."

build-all:
	@echo "🏗 Cross-compiling for all platforms → $(DIST_DIR)/"
	@mkdir -p $(DIST_DIR)
	@echo "  Windows amd64..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-windows-amd64.exe $(PKG)
	@echo "  macOS amd64..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-darwin-amd64 $(PKG)
	@echo "  macOS arm64..."
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-darwin-arm64 $(PKG)
	@echo "  Linux amd64..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-linux-amd64 $(PKG)
	@echo "  Linux arm64..."
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-linux-arm64 $(PKG)
	@echo ""
	@echo "✅ Build complete:"
	@ls -lh $(DIST_DIR)/
	@echo ""
	@echo "To install: make install"
	@echo "Or use scripts/install.sh (macOS/Linux) or scripts/install.ps1 (Windows)"

clean:
	rm -rf $(DIST_DIR) $(BINARY)
	@echo "✅ Cleaned."
