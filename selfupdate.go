package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	updateRepo  = "bachtiarpanjaitan/ihand-tui"
	updateAPI   = "https://api.github.com/repos/" + updateRepo + "/releases/latest"
)

// selfUpdate checks GitHub for the latest release and updates the running binary.
// Returns a user-friendly status message.
func selfUpdate(currentVersion string) string {
	// --- Fetch latest release info -----------------------------------------
	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Sprintf("❌ Gagal cek update: %v", err)
	}

	latest := strings.TrimPrefix(release.Tag, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	if latest == current {
		return fmt.Sprintf("✅ Sudah versi terbaru: v%s", current)
	}

	// --- Find the right asset for this platform ----------------------------
	assetName := platformAssetName(release.Tag)
	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.URL
			break
		}
	}
	if downloadURL == "" {
		// Fallback: try raw binary
		rawName := rawBinaryName()
		for _, a := range release.Assets {
			if strings.Contains(a.Name, rawName) {
				downloadURL = a.URL
				break
			}
		}
	}
	if downloadURL == "" {
		return fmt.Sprintf("❌ Tidak ada binary untuk platform ini (%s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	// --- Download ----------------------------------------------------------
	tmpDir, err := os.MkdirTemp("", "ihand-update")
	if err != nil {
		return fmt.Sprintf("❌ Gagal buat temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, assetName)
	if err := downloadFile(downloadURL, archivePath); err != nil {
		return fmt.Sprintf("❌ Gagal download: %v", err)
	}

	// --- Extract binary ----------------------------------------------------
	newBinary, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return fmt.Sprintf("❌ Gagal ekstrak: %v", err)
	}

	// --- Replace current binary --------------------------------------------
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("❌ Gagal cari binary saat ini: %v", err)
	}

	// On macOS/Linux: rename old, copy new
	// On Windows: rename old (can't delete running exe), copy new
	backup := currentPath + ".old"
	os.Remove(backup)
	if err := os.Rename(currentPath, backup); err != nil {
		return fmt.Sprintf("❌ Gagal backup binary lama: %v", err)
	}

	// Copy new binary
	src, err := os.Open(newBinary)
	if err != nil {
		os.Rename(backup, currentPath) // rollback
		return fmt.Sprintf("❌ Gagal buka binary baru: %v", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(currentPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		os.Rename(backup, currentPath) // rollback
		src.Close()
		return fmt.Sprintf("❌ Gagal tulis binary baru: %v", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Rename(backup, currentPath) // rollback
		return fmt.Sprintf("❌ Gagal copy binary: %v", err)
	}
	dst.Close()
	src.Close()

	// Cleanup backup
	os.Remove(backup)

	return fmt.Sprintf("✅ Diupdate ke v%s!\n\n⚠ Silakan restart ihand untuk menggunakan versi baru.", latest)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type githubRelease struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchLatestRelease() (*githubRelease, error) {
	req, _ := http.NewRequest("GET", updateAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ihand-tui")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("gagal parse response: %w", err)
	}
	return &release, nil
}

func platformAssetName(tag string) string {
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("ihand-%s-windows-amd64.zip", tag)
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return fmt.Sprintf("ihand-%s-darwin-arm64.tar.gz", tag)
		}
		return fmt.Sprintf("ihand-%s-darwin-amd64.tar.gz", tag)
	default: // linux
		if runtime.GOARCH == "arm64" {
			return fmt.Sprintf("ihand-%s-linux-arm64.tar.gz", tag)
		}
		return fmt.Sprintf("ihand-%s-linux-amd64.tar.gz", tag)
	}
}

func rawBinaryName() string {
	switch runtime.GOOS {
	case "windows":
		return "ihand-windows-amd64.exe"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "ihand-darwin-arm64"
		}
		return "ihand-darwin-amd64"
	default:
		if runtime.GOARCH == "arm64" {
			return "ihand-linux-arm64"
		}
		return "ihand-linux-amd64"
	}
}

func downloadFile(url, dst string) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ihand-tui")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractBinary(archivePath, tmpDir string) (string, error) {
	ext := filepath.Ext(archivePath)
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, tmpDir)
	}
	if ext == ".zip" {
		return extractZip(archivePath, tmpDir)
	}
	// Assume it's already a binary
	return archivePath, nil
}

func extractTarGz(tarGzPath, dst string) (string, error) {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Extract only the binary (named "ihand" or "ihand.exe")
		name := filepath.Base(hdr.Name)
		if name == "ihand" || name == "ihand.exe" {
			outPath := filepath.Join(dst, name)
			out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			return outPath, nil
		}
	}
	return "", fmt.Errorf("binary not found in archive")
}

func extractZip(zipPath, dst string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == "ihand" || name == "ihand.exe" {
			src, err := f.Open()
			if err != nil {
				return "", err
			}
			defer src.Close()

			outPath := filepath.Join(dst, name)
			out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, src); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			return outPath, nil
		}
	}
	return "", fmt.Errorf("binary not found in zip")
}
