package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the complete Ihand TUI configuration.
type Config struct {
	LLM LLMConfig `json:"llm"`
	App AppConfig `json:"app"`
}

// LLMConfig holds the LLM provider configuration.
type LLMConfig struct {
	Provider string `json:"provider"` // "openai", "anthropic", or "ollama"
	Model    string `json:"model"`    // model name (e.g., "gpt-4o", "claude-sonnet-5")
	APIKey   string `json:"api_key"`  // API key (required for openai/anthropic)
	BaseURL  string `json:"base_url"` // optional custom endpoint URL
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	AllowedDir string `json:"allowed_dir"` // directory for file operations
	Session    string `json:"session"`     // default session name
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		LLM: LLMConfig{
			Provider: "ollama",
			Model:    "llama3.2",
			APIKey:   "",
			BaseURL:  "",
		},
		App: AppConfig{
			AllowedDir: ".",
			Session:    "default",
		},
	}
}

// LoadConfig reads and parses a JSON config file.
// It merges values from the file onto the defaults, so the file only needs
// to specify the values it wants to override.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("tidak bisa membaca file konfigurasi %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("tidak bisa parsing file konfigurasi %s: %w", path, err)
	}

	// If llm.base_url is empty, set sensible defaults based on provider
	if cfg.LLM.BaseURL == "" {
		switch cfg.LLM.Provider {
		case "openai":
			cfg.LLM.BaseURL = "https://api.openai.com/v1"
		case "anthropic":
			cfg.LLM.BaseURL = "https://api.anthropic.com/v1"
		case "ollama":
			cfg.LLM.BaseURL = "http://localhost:11434"
		}
	}

	// Apply defaults for empty app fields
	if cfg.App.Session == "" {
		cfg.App.Session = "default"
	}
	if cfg.App.AllowedDir == "" {
		cfg.App.AllowedDir = "."
	}

	return cfg, nil
}

// Validate checks that the config is usable.
func (c Config) Validate() error {
	validProviders := map[string]bool{
		"openai":    true,
		"anthropic": true,
		"ollama":    true,
	}
	if !validProviders[c.LLM.Provider] {
		return fmt.Errorf("provider tidak dikenal: %q (gunakan openai, anthropic, atau ollama)", c.LLM.Provider)
	}

	if c.LLM.Provider == "openai" || c.LLM.Provider == "anthropic" {
		if c.LLM.APIKey == "" {
			return fmt.Errorf("api_key diperlukan untuk provider %s", c.LLM.Provider)
		}
	}

	return nil
}
