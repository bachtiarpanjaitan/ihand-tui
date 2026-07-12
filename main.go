package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/bachtiarpanjaitan/ihandai-go"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/llm"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/memory"

	tea "charm.land/bubbletea/v2"

	_ "github.com/bachtiarpanjaitan/ihand-tui/internal/providers"
	_ "github.com/bachtiarpanjaitan/ihandai-go/plugins/ollama"
)

var version = "dev" // set via ldflags: -X main.version=1.0.0

// defaultConfigPath returns the OS-specific config directory.
func defaultConfigPath() string {
	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("APPDATA")
		if dir == "" {
			dir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	case "darwin":
		dir = filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
	default: // linux & others
		dir = os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(os.Getenv("HOME"), ".config")
		}
	}
	return filepath.Join(dir, "ihand", "settings.json")
}

// ensureConfig ensures a config file exists at the given path.
// If it doesn't exist, creates the directory and writes a default template.
func ensureConfig(path string) {
	if _, err := os.Stat(path); err == nil {
		return // already exists
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "! Gagal membuat direktori config: %v\n", err)
		return
	}
	defaultJSON := `{
  "profiles": [
    {
      "name": "Ollama Local",
      "schema": "ollama",
      "model": "llama3.2",
      "api_key": "",
      "base_url": "http://localhost:11434"
    }
  ],
  "active_profile": 0,
  "app": {
    "allowed_dir": ".",
    "session": "default"
  }
}
`
	if err := os.WriteFile(path, []byte(defaultJSON), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "! Gagal menulis config default: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "✓ Config dibuat di %s\n", path)
}

func main() {
	defaultCfg := defaultConfigPath()
	configPath := flag.String("config", defaultCfg, "path ke file konfigurasi JSON")
	showVersion := flag.Bool("version", false, "tampilkan versi")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ihand version %s\n", version)
		os.Exit(0)
	}

	ensureConfig(*configPath)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "! Gagal membaca konfigurasi: %v\n", err)
		fmt.Fprintf(os.Stderr, "Menggunakan konfigurasi default.\n\n")
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "! Konfigurasi tidak valid: %v\n\n", err)
		os.Exit(1)
	}

	// Selalu gunakan directory terminal saat ini (.) sebagai default agar
	// file operations terikat pada folder tempat terminal dibuka.
	allowedDir := "."
	if flag.NArg() > 0 {
		allowedDir = flag.Arg(0)
	}

	var llmOpts []llm.Option
	activeCfg := cfg.ActiveConfig()
	llmOpts = append(llmOpts, llm.WithModel(activeCfg.Model))
	if activeCfg.APIKey != "" {
		llmOpts = append(llmOpts, llm.WithAPIKey(activeCfg.APIKey))
	}
	if activeCfg.BaseURL != "" {
		llmOpts = append(llmOpts, llm.WithBaseURL(activeCfg.BaseURL))
	}

	store := memory.NewInMemoryStore()

	ai, err := ihandai.New(
		ihandai.WithLLM(activeCfg.Schema, llmOpts...),
		ihandai.WithMemory(store),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Gagal konek ke LLM %q: %v\n", activeCfg.Schema, err)
		os.Exit(1)
	}
	defer ai.Close()

	fmt.Fprintf(os.Stderr, "Terhubung ke %s/%s\n", activeCfg.Schema, activeCfg.Model)

	m := initModel(ai, store, activeCfg.Schema, activeCfg.Model, cfg.App.Session, allowedDir, *configPath)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
