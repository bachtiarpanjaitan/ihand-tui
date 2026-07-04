package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bachtiarpanjaitan/ihandai-go"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/llm"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/memory"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/tools"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"

	_ "github.com/bachtiarpanjaitan/ihandai-go/plugins/ollama"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type chatState int

const (
	stateReady chatState = iota
	stateThinking
)

type chatMode int

const (
	modeChat chatMode = iota
	modePlan
	modeEdit
	modeAuto
)

func (m chatMode) String() string {
	switch m {
	case modeChat:
		return "Chat"
	case modePlan:
		return "Plan"
	case modeEdit:
		return "Edit"
	case modeAuto:
		return "Auto"
	default:
		return "Chat"
	}
}

func (m chatMode) Color() string {
	switch m {
	case modeChat:
		return "39"  // blue
	case modePlan:
		return "214" // amber
	case modeEdit:
		return "76"  // green
	case modeAuto:
		return "196" // red
	default:
		return "39"
	}
}

func (m chatMode) Placeholder() string {
	switch m {
	case modeChat:
		return "Ketik pesan..."
	case modePlan:
		return "Apa yang ingin direncanakan?..."
	case modeEdit:
		return "Apa yang ingin diubah?..."
	case modeAuto:
		return "Apa yang ingin dikerjakan?..."
	default:
		return "Ketik pesan..."
	}
}

// chatMessage holds a single message in the conversation.
type chatMessage struct {
	role    string        // "user", "assistant", "system", "error"
	content string        // raw text (unrendered) — re-rendered on resize
	tokens  int           // token count for this message
	timing  time.Duration // LLM call duration (assistant only)
}

// --- Async LLM messages (sent from goroutine → Update) ---

type llmResponseMsg struct {
	content   string
	tokens    int
	timing    time.Duration
	usage     *ihandai.TokenUsage // real token counts from the LLM
	toolCalls []toolCallRecord    // tool executions during this turn
}

type toolCallRecord struct {
	toolName string
	input    string
	output   string
	isError  bool
}

type llmErrorMsg struct{ err error }

// toolCallMsg shows a tool invocation result in the conversation.
type toolCallMsg struct {
	toolName string
	input    string
	output   string
	isError  bool
}

// ---------------------------------------------------------------------------
// Styles (adaptive, works in light + dark terminals)
// ---------------------------------------------------------------------------

var (
	borderColor = lipgloss.Color("240") // #585858
	promptColor = lipgloss.Color("39")  // #00AFFF
	checkColor  = lipgloss.Color("76")  // #5FD700
	dimColor    = lipgloss.Color("243") // #767676
	errColor    = lipgloss.Color("196") // #FF0000
	titleColor  = lipgloss.Color("252") // #D0D0D0
	statusBg    = lipgloss.Color("236") // #303030
	userColor   = lipgloss.Color("39")  // same as prompt

	headerStyle = lipgloss.NewStyle().
			Foreground(titleColor).
			Bold(true).
			Padding(0, 1)

	headerBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("234")).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(borderColor)

	dimStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	userPromptStyle = lipgloss.NewStyle().
			Foreground(userColor).
			Bold(true)

	checkStyle = lipgloss.NewStyle().
			Foreground(checkColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errColor).
			Bold(true)

	statusStyle = lipgloss.NewStyle().
			Background(statusBg).
			Foreground(dimColor).
			Padding(0, 1)
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
}

type slashCommand struct {
	name string
	desc string
}

// computeSuggestions filters available commands based on the current input.
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

type model struct {
	// Dimensions
	width  int
	height int
	ready  bool // true after first WindowSizeMsg

	// Components
	viewport viewport.Model
	textarea textarea.Model

	// Chat
	messages    []chatMessage
	totalTokens int
	session     string
	modelName   string
	provider    string

	// LLM
	ai     *ihandai.Client
	ctx    context.Context
	memory memory.ConversationStore

	// Tools
	allowedDir string
	toolList   []tools.Tool

	// State
	state       chatState
	mode        chatMode
	err         error
	suggestions []string
	selSugg     int // selected suggestion index, -1 = none

	// Markdown
	mdRenderer *glamour.TermRenderer
	mdWidth    int
}

// initialModel creates the starting Bubble Tea model.
func initialModel(ai *ihandai.Client, store memory.ConversationStore, provider, modelName, session, allowedDir string) model {
	// --- Textarea ---
	ta := textarea.New()
	ta.Placeholder = "Ketik pesan..."
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.CharLimit = 8192

	// Styles via SetStyles (v2 API)
	s := ta.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().
		Foreground(promptColor).Bold(true)
	s.Focused.Text = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255"))
	s.Focused.Placeholder = lipgloss.NewStyle().
		Foreground(dimColor)
	s.Blurred.Prompt = lipgloss.NewStyle().
		Foreground(dimColor)
	s.Blurred.Text = lipgloss.NewStyle().
		Foreground(dimColor)
	ta.SetStyles(s)

	ta.Focus()

	// --- Tools ---
	writeTool := NewWriteFileTool(allowedDir)
	readTool := NewReadFileTool(allowedDir)
	listTool := NewListFilesTool(allowedDir)
	toolList := []tools.Tool{writeTool, readTool, listTool}
	ai.SetTools(writeTool, readTool, listTool)

	// --- Viewport ---
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))
	vp.SetContent(welcomeMessage(provider, modelName))
	vp.GotoTop()

	return model{
		viewport:   vp,
		textarea:   ta,
		session:    session,
		provider:   provider,
		modelName:  modelName,
		ai:         ai,
		ctx:        context.Background(),
		memory:     store,
		state:      stateReady,
		mode:       modeChat,
		allowedDir: allowedDir,
		toolList:   toolList,
		selSugg:    -1,
	}
}

func welcomeMessage(provider, modelName string) string {
	var sb strings.Builder
	sb.WriteString(titleStyle().Render("╭────────────────────────────────────────────╮"))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render(fmt.Sprintf("│        Selamat datang di Ihand TUI        │")))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render(fmt.Sprintf("│       %s / %s       │", provider, modelName)))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render("╰────────────────────────────────────────────╯"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Mode:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /chat  — percakapan normal"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /plan  — analisis & rencana (read-only)"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /edit  — implementasi & edit file"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /auto  — otonom (multi-step otomatis)"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Lainnya:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /exit   — keluar"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /clear  — reset percakapan"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /stats  — statistik session"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /help   — bantuan"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Tools: write_file, read_file, list_files"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Enter untuk kirim  ·  Ctrl+J untuk baris baru  ·  Ctrl+C untuk keluar"))
	return sb.String()
}

func titleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(promptColor).
		Bold(true)
}

func toolStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")). // amber
		Padding(0, 1)
}

func toolErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")). // red
		Bold(true).
		Padding(0, 1)
}

var (
	suggestionBoxStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)

	suggestionSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("240")).
				Foreground(lipgloss.Color("255")).
				Bold(true).
				Padding(0, 1)

	suggestionDimStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Padding(0, 1)
)

// ---------------------------------------------------------------------------
// Bubble Tea interface
// ---------------------------------------------------------------------------

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.textarea.Focus(),
		tea.RequestWindowSize,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// --- Terminal resize ---
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.recalcLayout()

		// Re-render all messages at the new width
		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()

	// --- Key presses ---
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)

	// --- Async LLM response ---
	case llmResponseMsg:
		callTokens := msg.tokens
		if msg.usage != nil {
			callTokens = msg.usage.TotalTokens
		}
		m.totalTokens += callTokens

		// Show tool calls if any were made
		for _, tc := range msg.toolCalls {
			role := "tool"
			if tc.isError {
				role = "tool-error"
			}
			// Truncate long output for display
			display := tc.output
			if len(display) > 500 {
				display = display[:500] + "..."
			}
			m.messages = append(m.messages, chatMessage{
				role:    role,
				content: fmt.Sprintf("%s(%s) → %s", tc.toolName, tc.input, display),
				tokens:  0,
			})
		}

		m.messages = append(m.messages, chatMessage{
			role:    "assistant",
			content: msg.content,
			tokens:  callTokens,
			timing:  msg.timing,
		})
		m.state = stateReady
		m.err = nil

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		cmds = append(cmds, m.textarea.Focus())

	// --- Async LLM error ---
	case llmErrorMsg:
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: msg.err.Error(),
		})
		m.state = stateReady
		m.err = msg.err

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		cmds = append(cmds, m.textarea.Focus())

	// --- Tool execution result ---
	case toolCallMsg:
		// Append tool message to conversation
		m.messages = append(m.messages, chatMessage{
			role:    "tool",
			content: fmt.Sprintf("%s(%s) → %s", msg.toolName, msg.input, msg.output),
			tokens:  0,
		})
		if msg.isError {
			// Also add the error specifically
			m.messages[len(m.messages)-1].role = "tool-error"
		}

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()

	// --- Mouse: scroll viewport ---
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Update sub-components
	var taCmd tea.Cmd
	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() tea.View {
	if !m.ready {
		return tea.NewView("Initializing...")
	}

	rendered := m.renderFull()
	v := tea.NewView(rendered)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (m model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

	case "ctrl+c", "ctrl+d":
		return m, tea.Quit

	case "ctrl+l":
		m.viewport.GotoTop()
		return m, nil

	case "enter":
		// Enter = kirim pesan
		if m.state == stateThinking {
			return m, nil
		}

		m.suggestions = nil
		m.selSugg = -1

		input := strings.TrimSpace(m.textarea.Value())
		if input == "" {
			return m, nil
		}

		// Check for slash commands
		if strings.HasPrefix(input, "/") {
			return m.handleCommand(input)
		}

		// Submit user message → LLM
		inputTokens := countTokens(input)
		m.messages = append(m.messages, chatMessage{
			role:    "user",
			content: input,
			tokens:  inputTokens,
		})
		m.state = stateThinking
		m.textarea.Reset()
		m.textarea.Blur()

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()

		return m, tea.Batch(
			makeToolChatCall(m.ai, m.ctx, m.session, input, m.memory, m.toolList, m.mode),
		)

	case "ctrl+j":
		// Ctrl+J = baris baru (multiline), forward ke textarea
		if m.state == stateThinking {
			return m, nil
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd

	case "up", "down", "pgup", "pgdown", "home", "end":
		// Scroll viewport with navigation keys
		// Clear suggestions first
		m.suggestions = nil
		m.selSugg = -1
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case "shift+tab":
		// Cycle through modes: Chat → Plan → Edit → Auto → Chat
		switch m.mode {
		case modeChat:
			return m.switchMode(modePlan)
		case modePlan:
			return m.switchMode(modeEdit)
		case modeEdit:
			return m.switchMode(modeAuto)
		case modeAuto:
			return m.switchMode(modeChat)
		}
		return m, nil

	case "tab":
		// Cycle through suggestions if they're visible
		if len(m.suggestions) > 0 {
			m.selSugg = (m.selSugg + 1) % len(m.suggestions)
			// Auto-complete the textarea with the selected suggestion
			m.textarea.SetValue(m.suggestions[m.selSugg] + " ")
			// Move cursor to end
			m.textarea.CursorEnd()
			return m, nil
		}
		return m, nil

	default:
		// Don't forward keys while thinking (textarea is blurred)
		if m.state == stateThinking {
			return m, nil
		}
		// Forward other keys to textarea (typing, backspace, etc.)
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)

		// Compute slash-command suggestions after textarea updates
		m.suggestions = computeSuggestions(m.textarea.Value())
		if len(m.suggestions) > 0 {
			m.selSugg = 0
		} else {
			m.selSugg = -1
		}

		return m, cmd
	}
}

// ---------------------------------------------------------------------------
// Commands
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
			content: "🧹 Percakapan direset.",
		})

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoTop()
		return m, nil

	case "/stats":
		history, _ := m.memory.History(m.ctx, m.session)
		statText := fmt.Sprintf(
			"📊 Session: %s\n   Pesan di memori: %d\n   Total token: ~%d\n   Terminal: %dx%d",
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
			content: fmt.Sprintf("⚠ Perintah tidak dikenal: %s. Ketik /help untuk bantuan.", input),
		})
		m.textarea.Reset()

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		return m, nil
	}
}

// switchMode mengganti mode operasi AI.
func (m model) switchMode(newMode chatMode) (tea.Model, tea.Cmd) {
	if m.mode == newMode {
		return m, nil
	}
	m.mode = newMode
	m.textarea.Placeholder = newMode.Placeholder()

	msg := fmt.Sprintf("🎯 Mode: %s", newMode.String())
	m.messages = append(m.messages, chatMessage{
		role:    "system",
		content: msg,
	})
	m.textarea.Reset()

	content := m.buildConversation()
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
	return m, nil
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

func (m *model) renderFull() string {
	// --- Header (fixed, full-width dengan background) ---
	modeTag := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.mode.Color())).
		Bold(true).
		Padding(0, 1).
		Render(m.mode.String())
	headerLeft := headerStyle.Render(fmt.Sprintf("Ihand TUI · %s/%s", m.provider, m.modelName))
	headerLeft = lipgloss.JoinHorizontal(lipgloss.Top, modeTag, headerLeft)
	sessionInfo := dimStyle.Render(fmt.Sprintf("Session: %s", m.session))
	headerGap := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(sessionInfo) - 2
	if headerGap < 1 {
		headerGap = 1
	}
	headerContent := lipgloss.JoinHorizontal(lipgloss.Top,
		headerLeft,
		strings.Repeat(" ", headerGap),
		sessionInfo,
	)
	// Full-width header bar dengan background
	padRight := m.width - lipgloss.Width(headerContent)
	if padRight < 0 {
		padRight = 0
	}
	header := headerBarStyle.Render(headerContent + strings.Repeat(" ", padRight))

	// --- Double separator (pemisah tegas) ---
	sep := separatorStyle.Render(strings.Repeat("━", m.width))

	// --- Viewport (scrollable) ---
	vp := m.viewport.View()

	// --- Status bar ---
	var status string
	switch m.state {
	case stateThinking:
		status = " ⏳ Thinking..."
	case stateReady:
		if len(m.messages) > 0 {
			status = fmt.Sprintf(" ✓ Ready  |  ~%d total tokens  |  %d messages",
				m.totalTokens, len(m.messages))
		} else {
			status = " ✓ Ready — ketik pesan untuk memulai"
		}
	}
	// Pad to full width
	statusW := lipgloss.Width(status)
	if m.width > statusW {
		status = status + strings.Repeat(" ", m.width-statusW)
	}
	status = statusStyle.Render(status)

	// --- Suggestions ---
	var sug string
	if len(m.suggestions) > 0 {
		sug = m.renderSuggestions()
	}

	// --- Input area ---
	input := m.textarea.View()

	// Build the bottom section: suggestions + input
	bottom := input
	if sug != "" {
		bottom = lipgloss.JoinVertical(lipgloss.Left, sug, input)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		sep,
		vp,
		sep,
		status,
		bottom,
	)
}

// buildConversation renders all messages into a single string for the viewport.
func (m *model) buildConversation() string {
	if len(m.messages) == 0 {
		return welcomeMessage(m.provider, m.modelName)
	}

	contentWidth := m.width - 6
	if contentWidth < 40 {
		contentWidth = 40
	}

	var sb strings.Builder

	for i, msg := range m.messages {
		switch msg.role {
		case "user":
			sb.WriteString(userPromptStyle.Render("▸ "))
			sb.WriteString(lipgloss.NewStyle().
				Width(contentWidth).
				Render(msg.content))

		case "assistant":
			// Timing + token info line
			info := fmt.Sprintf("✓ [%v · ~%d token]",
				msg.timing.Round(time.Millisecond), msg.tokens)
			sb.WriteString(checkStyle.Render(info))
			sb.WriteString("\n")
			sb.WriteString(separatorStyle.Render(strings.Repeat("─", contentWidth)))
			sb.WriteString("\n\n")
			// Markdown rendering
			rendered := m.renderMarkdown(msg.content, contentWidth)
			sb.WriteString(rendered)
			sb.WriteString("\n")
			sb.WriteString(separatorStyle.Render(strings.Repeat("─", contentWidth)))

		case "tool":
			sb.WriteString(toolStyle().Render("🔧 " + msg.content))

		case "tool-error":
			sb.WriteString(toolErrorStyle().Render("🔧✗ " + msg.content))

		case "system":
			sb.WriteString(dimStyle.Render("ℹ " + msg.content))

		case "error":
			sb.WriteString(errorStyle.Render("✗ " + msg.content))
		}

		sb.WriteString("\n")
		if i < len(m.messages)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderMarkdown converts markdown to styled terminal output using glamour v2.
func (m *model) renderMarkdown(text string, width int) string {
	wrapWidth := width - 4
	if wrapWidth < 20 {
		wrapWidth = 20
	}

	// Recreate glamour renderer if width changed
	if m.mdWidth != wrapWidth || m.mdRenderer == nil {
		m.mdWidth = wrapWidth
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(wrapWidth),
		)
		if err != nil {
			r = nil
		}
		m.mdRenderer = r
	}

	// Gunakan persistent renderer jika tersedia
	if m.mdRenderer != nil {
		rendered, err := m.mdRenderer.Render(text)
		if err == nil {
			return rendered
		}
	}

	// Fallback: one-shot render tanpa width control
	rendered, err := glamour.Render(text, "dark")
	if err != nil {
		return text
	}
	return rendered
}

// renderSuggestions returns a styled suggestion bar.
func (m *model) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return ""
	}

	var items []string
	for i, cmd := range m.suggestions {
		// Find the command description
		desc := ""
		for _, ac := range availableCommands {
			if ac.name == cmd {
				desc = ac.desc
				break
			}
		}

		text := fmt.Sprintf(" %s  %s", cmd, suggestionDimStyle.Render(desc))
		if i == m.selSugg {
			items = append(items, suggestionSelectedStyle.Render(text))
		} else {
			items = append(items, suggestionBoxStyle.Render(text))
		}
	}

	// Join items horizontally with a small gap
	row := lipgloss.JoinHorizontal(lipgloss.Top, items...)
	// Add "Tab ↻" hint
	hint := suggestionDimStyle.Render(" Tab ↻")
	return lipgloss.JoinHorizontal(lipgloss.Top, row, hint)
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

func (m *model) recalcLayout() {
	// Height budget:
	//   header     : 1
	//   header sep : 1
	//   bottom sep : 1
	//   status     : 1
	//   textarea   : 3 (SetHeight)
	//   suggestions: 0 or 1
	//   Fixed overhead = 7

	fixedOverhead := 7
	sugHeight := 0
	if len(m.suggestions) > 0 {
		sugHeight = 1
	}
	vpHeight := m.height - fixedOverhead - sugHeight
	if vpHeight < 5 {
		vpHeight = 5
	}

	vpWidth := m.width - 2
	if vpWidth < 40 {
		vpWidth = 40
	}

	m.viewport.SetWidth(vpWidth)
	m.viewport.SetHeight(vpHeight)

	taWidth := m.width - 4
	if taWidth < 20 {
		taWidth = 20
	}
	m.textarea.SetWidth(taWidth)
}

// ---------------------------------------------------------------------------
// Async LLM call
// ---------------------------------------------------------------------------

// makeToolChatCall runs a tool-enabled conversation loop.
// It sends the user's message with tool descriptions, parses the response
// for tool calls (ReAct format), executes tools, and loops until a final answer.
func makeToolChatCall(ai *ihandai.Client, ctx context.Context, session, input string, store memory.ConversationStore, toolList []tools.Tool, mode chatMode) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()

		// Get the LLM provider
		llmProvider := ai.LLM()
		if llmProvider == nil {
			return llmErrorMsg{err: fmt.Errorf("LLM provider tidak tersedia")}
		}

		// Load conversation history from memory
		history, _ := store.History(ctx, session)

		// Filter tools for plan mode (read-only)
		activeTools := toolList
		if mode == modePlan {
			activeTools = nil
			for _, t := range toolList {
				if t.Name() == "read_file" || t.Name() == "list_files" {
					activeTools = append(activeTools, t)
				}
			}
		}

		// Build mode-specific system prompt
		systemPrompt := buildToolSystemPrompt(activeTools, mode)

		// Build message list: system prompt + history + current query
		messages := []core.Message{
			{Role: "system", Content: systemPrompt},
		}
		messages = append(messages, history...)
		messages = append(messages, core.Message{Role: "user", Content: input})

		// Tool execution loop — more iterations for auto mode
		maxIterations := 8
		if mode == modeAuto {
			maxIterations = 16
		}
		var finalContent string
		var toolCalls []toolCallRecord
		totalTokens := countTokens(input)

		for i := 0; i < maxIterations; i++ {
			select {
			case <-ctx.Done():
				return llmErrorMsg{err: ctx.Err()}
			default:
			}

			resp, err := llmProvider.Chat(ctx, messages)
			if err != nil {
				return llmErrorMsg{err: err}
			}

			totalTokens += countTokens(resp.Content)

			// Parse response for tool calls or final answer
			toolCall, isFinal := parseReActResponse(resp.Content)

			if isFinal {
				finalContent = toolCall.output
				// Save assistant response to memory
				store.Append(ctx, session, core.Message{
					Role: "assistant", Content: resp.Content,
				})
				break
			}

			if toolCall.name != "" {
				// Execute the tool
				toolOutput := executeToolCall(activeTools, toolCall)
				isToolError := strings.HasPrefix(toolOutput, "Error")

				// Record tool call for TUI display
				toolCalls = append(toolCalls, toolCallRecord{
					toolName: toolCall.name,
					input:    toolCall.input,
					output:   toolOutput,
					isError:  isToolError,
				})

				// Append assistant response + tool observation to messages
				messages = append(messages,
					core.Message{Role: "assistant", Content: resp.Content},
					core.Message{Role: "user", Content: fmt.Sprintf(
						"Observation (hasil dari tool %s): %s", toolCall.name, toolOutput,
					)},
				)
			} else {
				// No tool call detected — treat the whole response as the answer
				// (model mungkin merespon langsung tanpa format ReAct)
				finalContent = resp.Content
				store.Append(ctx, session, core.Message{
					Role: "assistant", Content: resp.Content,
				})
				break
			}
		}

	// If no final answer after max iterations, extract last assistant response
	if finalContent == "" && len(messages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" {
				finalContent = messages[i].Content
				break
			}
		}
		if finalContent == "" {
			finalContent = "⚠ Agent mencapai batas maksimum iterasi tanpa respons."
		}
	}

		elapsed := time.Since(start)
		return llmResponseMsg{
			content:   finalContent,
			tokens:    totalTokens,
			timing:    elapsed,
			usage:     nil,
			toolCalls: toolCalls,
		}
	}
}

// ---------------------------------------------------------------------------
// Tool execution helpers
// ---------------------------------------------------------------------------

// reActTool represents a parsed tool call from LLM text output.
type reActTool struct {
	name   string
	input  string
	output string // used for final answer
}

// ReAct regex patterns for parsing LLM output.
var (
	actionRe   = regexp.MustCompile(`Action:\s*(.+)`)
	inputRe    = regexp.MustCompile(`Action Input:\s*(.+)`)
	finalRe    = regexp.MustCompile(`Final Answer:\s*(.+)`)
	toolCallRe = regexp.MustCompile(`(\w+)\((\{.*?\})\)`)
)

// parseReActResponse parses LLM output for tool calls or final answers.
// Returns (toolCall, isFinal). If isFinal, toolCall.output holds the final answer.
// If toolCall.name is empty, the LLM didn't produce a parsable action or answer.
func parseReActResponse(text string) (reActTool, bool) {
	// Check for final answer
	if m := finalRe.FindStringSubmatch(text); len(m) > 1 {
		return reActTool{output: strings.TrimSpace(m[1])}, true
	}

	// Try the toolCallRe pattern first: tool_name({"key":"value"})
	if m := toolCallRe.FindStringSubmatch(text); len(m) > 2 {
		return reActTool{
			name:  strings.TrimSpace(m[1]),
			input: strings.TrimSpace(m[2]),
		}, false
	}

	// Try Action: format
	if m := actionRe.FindStringSubmatch(text); len(m) > 1 {
		actionStr := strings.TrimSpace(m[1])

		// Parse "tool_name(args)" format
		parenIdx := strings.Index(actionStr, "(")
		if parenIdx > 0 {
			name := strings.TrimSpace(actionStr[:parenIdx])
			inputStr := actionStr[parenIdx+1:]
			if lastParen := strings.LastIndex(inputStr, ")"); lastParen > 0 {
				inputStr = inputStr[:lastParen]
			}
			return reActTool{
				name:  name,
				input: strings.TrimSpace(inputStr),
			}, false
		}
		// Just a tool name without args
		return reActTool{name: actionStr, input: "{}"}, false
	}

	// Try Action Input: format (alternative)
	if m := inputRe.FindStringSubmatch(text); len(m) > 1 {
		return reActTool{input: strings.TrimSpace(m[1])}, false
	}

	return reActTool{}, false
}

// executeToolCall finds and executes the named tool.
func executeToolCall(toolList []tools.Tool, call reActTool) string {
	var tool tools.Tool
	for _, t := range toolList {
		if strings.EqualFold(t.Name(), call.name) {
			tool = t
			break
		}
	}
	if tool == nil {
		return fmt.Sprintf("Error: tool %q tidak dikenal. Tools tersedia: write_file, read_file, list_files", call.name)
	}

	// Ensure valid JSON input
	input := json.RawMessage(call.input)
	if !json.Valid(input) {
		// Try quoting if not valid JSON
		input = json.RawMessage(fmt.Sprintf("%q", call.input))
	}

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		return fmt.Sprintf("Error eksekusi %s: %v", call.name, err)
	}

	return string(output)
}

// buildToolSystemPrompt creates the ReAct-style system prompt with tool descriptions.
func buildToolSystemPrompt(toolList []tools.Tool, mode chatMode) string {
	var b strings.Builder

	// Mode-specific preamble
	switch mode {
	case modePlan:
		b.WriteString("Kamu adalah AI asisten dalam MODE PERENCANAAN. ")
		b.WriteString("Tugasmu adalah menganalisis, membaca file yang relevan, ")
		b.WriteString("dan membuat rencana detail SEBELUM implementasi.\n\n")
		b.WriteString("ATURAN PENTING:\n")
		b.WriteString("- BACA file yang relevan dengan read_file atau list_files\n")
		b.WriteString("- ANALISIS kode yang ada\n")
		b.WriteString("- BUAT rencana langkah-demi-langkah yang terstruktur\n")
		b.WriteString("- JANGAN menulis atau mengubah file apapun\n")
		b.WriteString("- Akhiri dengan Final Answer: berisi rencana yang jelas\n\n")

	case modeEdit:
		b.WriteString("Kamu adalah AI asisten dalam MODE EDIT. ")
		b.WriteString("Tugasmu adalah mengimplementasikan perubahan secara LANGSUNG. ")
		b.WriteString("Jangan bertanya — langsung kerjakan.\n\n")
		b.WriteString("ATURAN PENTING:\n")
		b.WriteString("- TULIS file dengan write_file untuk membuat/mengubah\n")
		b.WriteString("- BACA file yang perlu diubah dengan read_file\n")
		b.WriteString("- IMPLEMENTASIKAN perubahan yang diminta sekarang juga\n")
		b.WriteString("- Akhiri dengan Final Answer: konfirmasi apa yang sudah diubah\n\n")

	case modeAuto:
		b.WriteString("Kamu adalah AI asisten dalam MODE OTONOM. ")
		b.WriteString("Kerjakan tugas sampai SELESAI tanpa perlu konfirmasi user. ")
		b.WriteString("Rencanakan sendiri langkah-langkahnya dan eksekusi berurutan.\n\n")
		b.WriteString("ATURAN PENTING:\n")
		b.WriteString("- Gunakan tools secara otonom dan berurutan\n")
		b.WriteString("- Eksekusi multi-step tanpa bertanya ke user\n")
		b.WriteString("- Laporkan progress di setiap langkah\n")
		b.WriteString("- Jika gagal di satu langkah, coba alternatif lain\n")
		b.WriteString("- Akhiri dengan Final Answer: ringkasan apa yang sudah dikerjakan\n\n")

	default: // modeChat
		b.WriteString("Kamu adalah AI asisten yang membantu menjawab pertanyaan ")
		b.WriteString("dan menyelesaikan tugas.\n\n")
	}

	// Tool calling format
	if len(toolList) > 0 {
		b.WriteString("FORMAT MEMANGGIL TOOLS:\n")
		b.WriteString("Action: nama_tool({\"key\": \"value\"})\n\n")
		b.WriteString("FORMAT JAWABAN AKHIR:\n")
		b.WriteString("Final Answer: jawaban dalam Bahasa Indonesia\n\n")
		b.WriteString("Jangan gunakan format lain untuk memanggil tools.\n\n")
		b.WriteString("Tools yang tersedia:\n")

		for _, t := range toolList {
			schema, _ := json.Marshal(t.InputSchema())
			b.WriteString(fmt.Sprintf("- %s: %s\n  Schema: %s\n\n", t.Name(), t.Description(), string(schema)))
		}
	} else {
		b.WriteString("Tidak ada tools yang tersedia. Jawab langsung dengan Final Answer.\n\n")
	}

	b.WriteString("PENTING: Selalu gunakan Bahasa Indonesia untuk Final Answer.")
	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// countTokens estimates token count (~4 chars = 1 token for English/Indonesian).
func countTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "settings.json", "path ke file konfigurasi JSON")
	flag.Parse()

	// Load configuration
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

	// Override allowed_dir from command-line argument if provided
	allowedDir := cfg.App.AllowedDir
	if flag.NArg() > 0 {
		allowedDir = flag.Arg(0)
	}

	// Build LLM options
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
