package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// LLMProfile holds a named LLM provider configuration (one entry in the profiles list).
type LLMProfile struct {
	Name     string `json:"name"`     // display name, e.g. "Ollama Local"
	Provider string `json:"provider"` // "openai", "anthropic", or "ollama"
	Model    string `json:"model"`    // model name (e.g., "gpt-4o", "claude-sonnet-5")
	APIKey   string `json:"api_key"`  // API key (required for openai/anthropic)
	BaseURL  string `json:"base_url"` // optional custom endpoint URL
}

// LLMConfig holds a single LLM provider configuration (kept for backward compat).
type LLMConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	AllowedDir string `json:"allowed_dir"` // directory for file operations
	Session    string `json:"session"`     // default session name
}

// Config holds the complete Ihand TUI configuration.
type Config struct {
	Profiles      []LLMProfile `json:"profiles,omitempty"`
	ActiveProfile int          `json:"active_profile"`
	LLM           LLMConfig    `json:"llm,omitempty"`       // kept for backward compat
	App           AppConfig    `json:"app"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		App: AppConfig{
			AllowedDir: ".",
			Session:    "default",
		},
	}
}

// LoadConfig reads and parses a JSON config file.
// Merges values from the file onto the defaults.
// If the file uses the old flat format (no profiles), migrates to the new format.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("tidak bisa membaca file konfigurasi %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("tidak bisa parsing file konfigurasi %s: %w", path, err)
	}

	// Backward compat: detect old format (has "llm" field from JSON)
	// If profiles wasn't in the JSON (DefaultConfig didn't set it, unmarshal
	// didn't overwrite it), migrate from the old flat llm field.
	if cfg.LLM.Provider != "" {
		cfg.Profiles = []LLMProfile{
			{
				Name:     "Default",
				Provider: cfg.LLM.Provider,
				Model:    cfg.LLM.Model,
				APIKey:   cfg.LLM.APIKey,
				BaseURL:  cfg.LLM.BaseURL,
			},
		}
		cfg.ActiveProfile = 0
	}

	// Ensure at least one profile exists
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = []LLMProfile{
			{
				Name:     "Ollama Local",
				Provider: "ollama",
				Model:    "llama3.2",
				APIKey:   "",
				BaseURL:  "http://localhost:11434",
			},
		}
		cfg.ActiveProfile = 0
	}

	// Clamp active profile index
	if cfg.ActiveProfile < 0 || cfg.ActiveProfile >= len(cfg.Profiles) {
		cfg.ActiveProfile = 0
	}

	// Fill in default base URLs for each profile
	for i := range cfg.Profiles {
		p := &cfg.Profiles[i]
		if p.BaseURL == "" {
			switch p.Provider {
			case "openai":
				p.BaseURL = "https://api.openai.com/v1"
			case "anthropic":
				p.BaseURL = "https://api.anthropic.com/v1"
			case "ollama":
				p.BaseURL = "http://localhost:11434"
			}
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

// ActiveConfig returns the currently active LLM profile as an LLMConfig.
func (c Config) ActiveConfig() LLMConfig {
	if c.ActiveProfile >= 0 && c.ActiveProfile < len(c.Profiles) {
		p := c.Profiles[c.ActiveProfile]
		return LLMConfig{
			Provider: p.Provider,
			Model:    p.Model,
			APIKey:   p.APIKey,
			BaseURL:  p.BaseURL,
		}
	}
	return LLMConfig{}
}

// SaveConfig writes the configuration to a JSON file.
func (c Config) SaveConfig(path string) error {
	// Clear the old flat llm field when saving — profiles is the new format
	c.LLM = LLMConfig{}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal serialize konfigurasi: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("gagal menulis konfigurasi ke %s: %w", path, err)
	}
	return nil
}

// Validate checks that the config is usable.
func (c Config) Validate() error {
	validProviders := map[string]bool{
		"openai":    true,
		"anthropic": true,
		"ollama":    true,
	}

	active := c.ActiveConfig()
	if active.Provider == "" && len(c.Profiles) > 0 {
		active = LLMConfig{
			Provider: c.Profiles[0].Provider,
			Model:    c.Profiles[0].Model,
			APIKey:   c.Profiles[0].APIKey,
			BaseURL:  c.Profiles[0].BaseURL,
		}
	}

	if !validProviders[active.Provider] {
		return fmt.Errorf("provider tidak dikenal: %q (gunakan openai, anthropic, atau ollama)", active.Provider)
	}

	if active.Provider == "openai" || active.Provider == "anthropic" {
		if active.APIKey == "" {
			return fmt.Errorf("api_key diperlukan untuk provider %s", active.Provider)
		}
	}

	return nil
}
