package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// Slash commands & autocomplete
// ---------------------------------------------------------------------------

var availableCommands = []slashCommand{
	{name: "/exit", desc: "keluar dari aplikasi"},
	{name: "/clear", desc: "reset percakapan"},
	{name: "/stats", desc: "statistik session"},
	{name: "/help", desc: "tampilkan bantuan"},
	{name: "/chat", desc: "mode percakapan normal"},
	{name: "/plan", desc: "mode analisis & rencana (read-only)"},
	{name: "/edit", desc: "mode implementasi & edit file"},
	{name: "/auto", desc: "mode otonom (multi-step otomatis)"},
	{name: "/self-update", desc: "update ke versi terbaru"},
}

func computeSuggestions(input string) []string {
	if !strings.HasPrefix(input, "/") {
		return nil
	}
	var matches []string
	lower := strings.ToLower(input)
	for _, cmd := range availableCommands {
		if strings.HasPrefix(strings.ToLower(cmd.name), lower) {
			matches = append(matches, cmd.name)
		}
	}
	return matches
}

// ---------------------------------------------------------------------------
// Command handlers
// ---------------------------------------------------------------------------

func (m model) handleCommand(input string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(strings.TrimSpace(input)) {

	case "/exit":
		return m, tea.Quit

	case "/chat":
		return m.switchMode(modeChat)

	case "/plan":
		return m.switchMode(modePlan)

	case "/edit":
		return m.switchMode(modeEdit)

	case "/auto":
		return m.switchMode(modeAuto)

	case "/clear":
		m.memory.Clear(m.ctx, m.session)
		m.messages = nil
		m.totalTokens = 0
		m.state = stateReady
		m.err = nil
		m.textarea.Reset()

		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: "Percakapan direset.",
		})

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoTop()
		return m, nil

	case "/stats":
		history, _ := m.memory.History(m.ctx, m.session)
		statText := fmt.Sprintf(
			"Session: %s\n   Pesan di memori: %d\n   Total token: ~%d\n   Terminal: %dx%d",
			m.session, len(history), m.totalTokens, m.width, m.height,
		)
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: statText,
		})
		m.textarea.Reset()

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, nil

	case "/self-update":
		result := selfUpdate(version)
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: result,
		})
		m.textarea.Reset()
		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, nil

	case "/help":
		helpText := "Mode: /chat (normal), /plan (analisis), /edit (implementasi), /auto (otonom)\n" +
			"Lainnya: /exit (keluar), /clear (reset), /stats (statistik), /help (bantuan)\n" +
			"Keys: Enter (kirim), Ctrl+J (baris baru), ↑↓ (scroll), Shift+Tab (ganti mode), Ctrl+L (scroll ke atas)"
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: helpText,
		})
		m.textarea.Reset()

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, nil

	default:
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: fmt.Sprintf("! Perintah tidak dikenal: %s. Ketik /help untuk bantuan.", input),
		})
		m.textarea.Reset()

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, nil
	}
}

func (m model) switchMode(newMode chatMode) (tea.Model, tea.Cmd) {
	if m.mode == newMode {
		return m, nil
	}
	m.mode = newMode
	m.textarea.Placeholder = newMode.Placeholder()

	m.toolActivity = fmt.Sprintf("Mode: %s", newMode.String())
	m.textarea.Reset()

	content := m.buildConversation()
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
	return m, nil
}
