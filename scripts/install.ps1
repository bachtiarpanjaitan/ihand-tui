# ==============================================================================
# Ihand TUI — Installer for Windows
# ==============================================================================
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/install.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/install.ps1 -AutoYes
# ==============================================================================

param(
    [switch]$AutoYes = $false
)

$ErrorActionPreference = "Stop"
$Binary = "ihand.exe"
$InstallDir = "$env:USERPROFILE\AppData\Local\ihand"

# Colors for PowerShell
function Write-Green  { Write-Host $args -ForegroundColor Green }
function Write-Yellow { Write-Host $args -ForegroundColor Yellow }
function Write-Red    { Write-Host $args -ForegroundColor Red }

Write-Host ""
Write-Green "=============================================="
Write-Green "  Ihand TUI — Installer (Windows)"
Write-Green "=============================================="
Write-Host ""

# --- Prerequisites -----------------------------------------------------------
Write-Yellow "[1/6] Checking prerequisites..."

$GoInstalled = Get-Command go -ErrorAction SilentlyContinue
if (-not $GoInstalled) {
    Write-Red "✗ Go is not installed."
    Write-Host "  Install Go from: https://go.dev/dl/"
    Write-Host "  Or with winget: winget install GoLang.Go"
    exit 1
}

$GoVersion = (go version | Select-String -Pattern 'go[\d.]+').Matches.Value
Write-Host "  ✓ Go found: $GoVersion"

# --- Build -------------------------------------------------------------------
Write-Yellow "[2/6] Building $Binary..."

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ScriptDir
Push-Location $ProjectDir

try {
    $Version = (git describe --tags --always --dirty 2>$null) -replace "`n|`r", ""
    if (-not $Version) { $Version = "dev" }
    $BuildCmd = "go build -ldflags=""-X main.version=$Version"" -o $Binary ."
    Invoke-Expression $BuildCmd
    if ($LASTEXITCODE -ne 0) { throw "Build failed" }
    Write-Host "  ✓ Build successful"
} catch {
    Write-Red "✗ Build failed: $_"
    Pop-Location
    exit 1
}

# --- Create install directory ------------------------------------------------
Write-Yellow "[3/6] Creating install directory..."

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}
Write-Host "  ✓ $InstallDir"

# --- Install -----------------------------------------------------------------
Write-Yellow "[4/6] Installing $Binary..."

$InstallPath = Join-Path $InstallDir $Binary

if (Test-Path $InstallPath) {
    try {
        $ExistingVersion = (& $InstallPath --version 2>$null) -replace "`n|`r", ""
    } catch {
        $ExistingVersion = "unknown"
    }
    Write-Host "  Existing version: $ExistingVersion"

    if (-not $AutoYes) {
        $Response = Read-Host "  Overwrite? [Y/n]"
        if ($Response -notmatch '^[Yy]?$') {
            Write-Host "  Cancelled."
            Pop-Location
            exit 0
        }
    }
}

Copy-Item -Path $Binary -Destination $InstallPath -Force
Write-Host "  ✓ Installed: $InstallPath"

# --- Add to PATH -------------------------------------------------------------
Write-Yellow "[5/6] Adding to User PATH..."

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable(
        "Path",
        "$UserPath;$InstallDir",
        "User"
    )
    # Refresh current session PATH
    $env:Path = "$env:Path;$InstallDir"
    Write-Host "  ✓ Added to PATH (may need terminal restart)"
} else {
    Write-Host "  ✓ Already in PATH"
}

# --- Verify ------------------------------------------------------------------
Write-Yellow "[6/6] Verifying installation..."

try {
    $InstalledVersion = (& $InstallPath --version 2>$null) -replace "`n|`r", ""
    Write-Host "  ✓ Version: $InstalledVersion"
    $WhichCmd = Get-Command ihand -ErrorAction SilentlyContinue
    if ($WhichCmd) {
        Write-Host "  ✓ Location: $($WhichCmd.Source)"
    }
} catch {
    Write-Yellow "  ! Verification skipped (restart terminal and try 'ihand --version')"
}

# --- Cleanup & Done ----------------------------------------------------------
Remove-Item -Path "$ProjectDir\$Binary" -Force -ErrorAction SilentlyContinue
Pop-Location

Write-Host ""
Write-Green "=============================================="
Write-Green "       Installation Complete! 🎉"
Write-Green "=============================================="
Write-Host ""
Write-Host "  Run: ihand"
Write-Host ""
Write-Host "  Uninstall:"
Write-Host "    rm -r $InstallDir"
Write-Host "    (then remove $InstallDir from User PATH)"
Write-Host ""
