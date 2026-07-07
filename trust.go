package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// ---------------------------------------------------------------------------
// Trust persistence
// ---------------------------------------------------------------------------
// Trusted directories are stored in ~/.config/ihand/trusted_dirs.json
// (or OS equivalent). Each entry maps an absolute directory path to the
// timestamp when it was trusted.

var (
	trustMu sync.RWMutex
)

type trustStore struct {
	Version     int               `json:"version"`
	TrustedDirs map[string]string `json:"trusted_dirs"` // absPath → ISO8601 timestamp
}

func trustFilePath() string {
	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("APPDATA")
		if dir == "" {
			dir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	case "darwin":
		dir = filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
	default:
		dir = os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(os.Getenv("HOME"), ".config")
		}
	}
	return filepath.Join(dir, "ihand", "trusted_dirs.json")
}

func loadTrustStore() *trustStore {
	path := trustFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return &trustStore{
			Version:     1,
			TrustedDirs: make(map[string]string),
		}
	}

	var store trustStore
	if err := json.Unmarshal(data, &store); err != nil {
		return &trustStore{
			Version:     1,
			TrustedDirs: make(map[string]string),
		}
	}

	if store.TrustedDirs == nil {
		store.TrustedDirs = make(map[string]string)
	}
	if store.Version == 0 {
		store.Version = 1
	}

	return &store
}

func saveTrustStore(store *trustStore) error {
	path := trustFilePath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("membuat direktori trust: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trust store: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("menulis trust store: %w", err)
	}

	return nil
}

// isDirTrusted checks if the given absolute directory path is in the trust store.
func isDirTrusted(absPath string) bool {
	trustMu.RLock()
	defer trustMu.RUnlock()

	store := loadTrustStore()
	_, ok := store.TrustedDirs[absPath]
	return ok
}

// trustDir marks an absolute directory path as trusted (persisted).
func trustDir(absPath string) error {
	trustMu.Lock()
	defer trustMu.Unlock()

	store := loadTrustStore()
	store.TrustedDirs[absPath] = "permanent"
	return saveTrustStore(store)
}

// resolveAllowedDir resolves the allowed directory to an absolute path.
func resolveAllowedDir(allowedDir string) string {
	if filepath.IsAbs(allowedDir) {
		return filepath.Clean(allowedDir)
	}
	abs, err := filepath.Abs(allowedDir)
	if err != nil {
		return allowedDir
	}
	return abs
}
