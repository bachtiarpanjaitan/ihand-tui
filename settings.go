package main

import (
	"fmt"

	"github.com/bachtiarpanjaitan/ihandai-go"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/llm"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/tools"

	toolspkg "github.com/bachtiarpanjaitan/ihand-tui/internal/tools"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// Settings save / load
// ---------------------------------------------------------------------------

// saveSettings writes the current settings config to disk and exits settings mode.
func (m model) saveSettings() (tea.Model, tea.Cmd) {
	cfg := m.settingsConfig
	if cfg == nil {
		return m, nil
	}

	if err := cfg.SaveConfig(m.configPath); err != nil {
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: fmt.Sprintf("Gagal menyimpan pengaturan: %v", err),
		})
		m.state = stateReady
		m.settingsConfig = nil
		m.textarea.Focus()
		m.recalcLayout()
		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, nil
	}

	// Update runtime values from the saved config
	activeCfg := cfg.ActiveConfig()
	m.provider = activeCfg.Provider
	m.modelName = activeCfg.Model
	m.currentProfile = cfg.ActiveProfile
	m.session = cfg.App.Session
	m.allowedDir = cfg.App.AllowedDir

	m.toolActivity = "✓ Pengaturan disimpan"
	m.state = stateReady
	m.settingsConfig = nil
	m.textarea.Focus()
	m.recalcLayout()
	m.rebuildViewport()
	return m, nil
}

// enterSettings loads the config from disk and enters settings mode.
func (m model) enterSettings() (tea.Model, tea.Cmd) {
	cfg, err := LoadConfig(m.configPath)
	if err != nil {
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: fmt.Sprintf("Gagal membaca konfigurasi: %v", err),
		})
		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, nil
	}

	m.state = stateSettings
	m.settingsCurrentField = 0
	m.settingsEditMode = false
	m.settingsEditBuffer = ""
	m.settingsShowProfileList = false
	m.settingsProfileSel = 0
	m.settingsConfig = &cfg
	m.textarea.Reset()
	m.textarea.Blur()
	m.recalcLayout()
	m.rebuildViewport()
	return m, nil
}

// ---------------------------------------------------------------------------
// Profile switching
// ---------------------------------------------------------------------------

// switchProfile recreates the LLM client with the profile at the given index.
func (m *model) switchProfile(profileIdx int) error {
	cfg := m.settingsConfig
	if cfg == nil {
		return fmt.Errorf("settings config not loaded")
	}
	if profileIdx < 0 || profileIdx >= len(cfg.Profiles) {
		return fmt.Errorf("invalid profile index: %d", profileIdx)
	}

	profile := cfg.Profiles[profileIdx]

	var llmOpts []llm.Option
	llmOpts = append(llmOpts, llm.WithModel(profile.Model))
	if profile.APIKey != "" {
		llmOpts = append(llmOpts, llm.WithAPIKey(profile.APIKey))
	}
	if profile.BaseURL != "" {
		llmOpts = append(llmOpts, llm.WithBaseURL(profile.BaseURL))
	}

	newAI, err := ihandai.New(
		ihandai.WithLLM(profile.Provider, llmOpts...),
		ihandai.WithMemory(m.memory),
	)
	if err != nil {
		return fmt.Errorf("gagal konek ke %s/%s: %w", profile.Provider, profile.Model, err)
	}

	// Close old client
	m.ai.Close()
	m.ai = newAI

	// Re-create tools (allowedDir and other settings may differ)
	mkdirTool := toolspkg.NewCreateDirTool(m.allowedDir)
	writeTool := toolspkg.NewWriteFileTool(m.allowedDir)
	editTool := toolspkg.NewEditFileTool(m.allowedDir)
	readTool := toolspkg.NewReadFileTool(m.allowedDir)
	listTool := toolspkg.NewListFilesTool(m.allowedDir)
	browseTool := toolspkg.NewBrowseTool()
	findFilesTool := toolspkg.NewFindFilesTool(m.allowedDir)
	searchTextTool := toolspkg.NewSearchTextTool(m.allowedDir)
	readFileLinesTool := toolspkg.NewReadFileLinesTool(m.allowedDir)
	execTool := toolspkg.NewExecTool(m.allowedDir)
	m.toolList = []tools.Tool{mkdirTool, writeTool, editTool, readTool, listTool, browseTool, findFilesTool, searchTextTool, readFileLinesTool, execTool}
	m.ai.SetTools(mkdirTool, writeTool, editTool, readTool, listTool, browseTool, findFilesTool, searchTextTool, readFileLinesTool, execTool)

	// Update state
	cfg.ActiveProfile = profileIdx
	m.currentProfile = profileIdx
	m.provider = profile.Provider
	m.modelName = profile.Model

	return nil
}

// ---------------------------------------------------------------------------
// Settings key handling (dispatched from handleKeyPress)
// ---------------------------------------------------------------------------

func handleSettingsKey(m model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.settingsConfig == nil {
		m.state = stateReady
		return m, nil
	}

	// Profile list view mode
	if m.settingsShowProfileList {
		return handleProfileListKey(m, msg)
	}

	// If in edit mode, handle editing keys
	if m.settingsEditMode {
		switch msg.String() {
		case "enter":
			// Confirm edit — apply buffer value to config
			applySettingsFieldValue(&m, m.settingsCurrentField, m.settingsEditBuffer)
			m.settingsEditMode = false
			m.settingsEditBuffer = ""
			m.rebuildViewport()
			return m, nil

		case "esc":
			// Cancel editing
			m.settingsEditMode = false
			m.settingsEditBuffer = ""
			m.rebuildViewport()
			return m, nil

		case "backspace", "ctrl+h":
			if len(m.settingsEditBuffer) > 0 {
				m.settingsEditBuffer = m.settingsEditBuffer[:len(m.settingsEditBuffer)-1]
				m.rebuildViewport()
			}
			return m, nil

		case "ctrl+u":
			m.settingsEditBuffer = ""
			m.rebuildViewport()
			return m, nil

		default:
			if len(msg.String()) == 1 && msg.String()[0] >= 32 {
				m.settingsEditBuffer += msg.String()
				m.rebuildViewport()
			}
			return m, nil
		}
	}

	// Navigation mode (not editing)
	switch msg.String() {
	case "esc":
		// Exit settings without saving
		m.state = stateReady
		m.settingsConfig = nil
		m.settingsShowProfileList = false
		m.textarea.Focus()
		m.recalcLayout()
		m.rebuildViewport()
		return m, nil

	case "up", "left":
		if m.settingsCurrentField > 0 {
			m.settingsCurrentField--
		} else {
			m.settingsCurrentField = settingsField(settingsFieldCount - 1)
		}
		m.rebuildViewport()
		return m, nil

	case "down", "right", "tab":
		if m.settingsCurrentField < settingsFieldCount-1 {
			m.settingsCurrentField++
		} else {
			m.settingsCurrentField = 0
		}
		m.rebuildViewport()
		return m, nil

	case "enter":
		// If focused on Profile field, show profile list
		if m.settingsCurrentField == settingsProfile {
			m.settingsShowProfileList = true
			m.settingsProfileSel = m.settingsConfig.ActiveProfile
			m.rebuildViewport()
			return m, nil
		}
		// Otherwise enter edit mode
		m.settingsEditBuffer = getSettingsFieldValue(m, m.settingsCurrentField)
		m.settingsEditMode = true
		m.rebuildViewport()
		return m, nil

	case "ctrl+s":
		return m.saveSettings()

	default:
		return m, nil
	}
}

// handleProfileListKey handles key presses in the profile list sub-view.
func handleProfileListKey(m model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	profiles := m.settingsConfig.Profiles
	if len(profiles) == 0 {
		m.settingsShowProfileList = false
		m.rebuildViewport()
		return m, nil
	}

	switch msg.String() {
	case "up", "left":
		if m.settingsProfileSel > 0 {
			m.settingsProfileSel--
		} else {
			m.settingsProfileSel = len(profiles) - 1
		}
		m.rebuildViewport()
		return m, nil

	case "down", "right", "tab":
		if m.settingsProfileSel < len(profiles)-1 {
			m.settingsProfileSel++
		} else {
			m.settingsProfileSel = 0
		}
		m.rebuildViewport()
		return m, nil

	case "enter":
		// Switch to selected profile
		selected := m.settingsProfileSel
		if selected >= 0 && selected < len(profiles) {
			if err := m.switchProfile(selected); err != nil {
				m.messages = append(m.messages, chatMessage{
					role:    "error",
					content: fmt.Sprintf("❌ Gagal switch profil: %v", err),
				})
			} else {
				// Simpan active_profile ke disk
				if err := m.settingsConfig.SaveConfig(m.configPath); err != nil {
					m.messages = append(m.messages, chatMessage{
						role:    "error",
						content: fmt.Sprintf("❌ Gagal menyimpan profil: %v", err),
					})
				}
				m.toolActivity = fmt.Sprintf("✓ Profil: %s (%s/%s)",
					profiles[selected].Name, profiles[selected].Provider, profiles[selected].Model)
			}
		}
		m.state = stateReady
		m.settingsConfig = nil
		m.settingsShowProfileList = false
		m.textarea.Focus()
		m.recalcLayout()
		m.rebuildViewport()
		return m, nil

	case "esc":
		m.settingsShowProfileList = false
		m.rebuildViewport()
		return m, nil

	default:
		return m, nil
	}
}

// ---------------------------------------------------------------------------
// Settings field value accessors
// ---------------------------------------------------------------------------

// getSettingsFieldValue returns the current value of a settings field from the config.
func getSettingsFieldValue(m model, field settingsField) string {
	cfg := m.settingsConfig
	if cfg == nil {
		return ""
	}
	switch field {
	case settingsProfile:
		if cfg.ActiveProfile >= 0 && cfg.ActiveProfile < len(cfg.Profiles) {
			return cfg.Profiles[cfg.ActiveProfile].Name
		}
		return "(no profile)"
	case settingsProvider:
		return cfg.ActiveConfig().Provider
	case settingsModel:
		return cfg.ActiveConfig().Model
	case settingsAPIKey:
		return cfg.ActiveConfig().APIKey
	case settingsBaseURL:
		return cfg.ActiveConfig().BaseURL
	case settingsAllowedDir:
		return cfg.App.AllowedDir
	case settingsSession:
		return cfg.App.Session
	default:
		return ""
	}
}

// applySettingsFieldValue sets a value on the settings config.
func applySettingsFieldValue(m *model, field settingsField, value string) {
	if m.settingsConfig == nil {
		return
	}
	// Get the active profile
	profileIdx := m.settingsConfig.ActiveProfile
	if profileIdx < 0 || profileIdx >= len(m.settingsConfig.Profiles) {
		return
	}

	switch field {
	case settingsProfile:
		// Profile name change
		m.settingsConfig.Profiles[profileIdx].Name = value
	case settingsProvider:
		m.settingsConfig.Profiles[profileIdx].Provider = value
	case settingsModel:
		m.settingsConfig.Profiles[profileIdx].Model = value
	case settingsAPIKey:
		m.settingsConfig.Profiles[profileIdx].APIKey = value
	case settingsBaseURL:
		m.settingsConfig.Profiles[profileIdx].BaseURL = value
	case settingsAllowedDir:
		m.settingsConfig.App.AllowedDir = value
	case settingsSession:
		m.settingsConfig.App.Session = value
	}
}
