package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bachtiarpanjaitan/ihandai-go"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/llm"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/memory"

	tea "charm.land/bubbletea/v2"

	_ "github.com/bachtiarpanjaitan/ihandai-go/plugins/ollama"
	_ "test-ihandai/internal/providers"
)

var version = "dev" // set via ldflags: -X main.version=1.0.0

func main() {
	configPath := flag.String("config", "settings.json", "path ke file konfigurasi JSON")
	showVersion := flag.Bool("version", false, "tampilkan versi")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ihand version %s\n", version)
		os.Exit(0)
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Gagal membaca konfigurasi: %v\n", err)
		fmt.Fprintf(os.Stderr, "Menggunakan konfigurasi default.\n\n")
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Konfigurasi tidak valid: %v\n\n", err)
		os.Exit(1)
	}

	allowedDir := cfg.App.AllowedDir
	if flag.NArg() > 0 {
		allowedDir = flag.Arg(0)
	}

	var llmOpts []llm.Option
	llmOpts = append(llmOpts, llm.WithModel(cfg.LLM.Model))
	if cfg.LLM.APIKey != "" {
		llmOpts = append(llmOpts, llm.WithAPIKey(cfg.LLM.APIKey))
	}
	if cfg.LLM.BaseURL != "" {
		llmOpts = append(llmOpts, llm.WithBaseURL(cfg.LLM.BaseURL))
	}

	store := memory.NewInMemoryStore()

	ai, err := ihandai.New(
		ihandai.WithLLM(cfg.LLM.Provider, llmOpts...),
		ihandai.WithMemory(store),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Gagal konek ke LLM provider %q: %v\n", cfg.LLM.Provider, err)
		os.Exit(1)
	}
	defer ai.Close()

	fmt.Fprintf(os.Stderr, "✓ Terhubung ke %s/%s\n", cfg.LLM.Provider, cfg.LLM.Model)

	m := initialModel(ai, store, cfg.LLM.Provider, cfg.LLM.Model, cfg.App.Session, allowedDir)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
