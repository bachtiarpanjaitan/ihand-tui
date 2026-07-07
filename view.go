package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m model) View() tea.View {
	if !m.ready {
		return tea.NewView("Initializing...")
	}

	rendered := m.renderFull()
	v := tea.NewView(rendered)
	v.AltScreen = true
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	} else {
		v.MouseMode = tea.MouseModeNone
	}
	return v
}

// ---------------------------------------------------------------------------
// Full-screen rendering
// ---------------------------------------------------------------------------

func (m *model) renderFull() string {
	modeTag := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Foreground(lipgloss.Color(m.mode.Color())).
		Bold(true).
		Padding(0, 1).
		Render(m.mode.String())
	effortTag := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Foreground(lipgloss.Color(m.effort.Color())).
		Bold(true).
		Padding(0, 1).
		Render(m.effort.Tag())

	// Team role tag (only shown when in modeTeam with an active agent)
	var teamTag string
	if m.mode == modeTeam && m.currentTeamRole != roleNone {
		teamTag = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color(m.currentTeamRole.Color())).
			Bold(true).
			Padding(0, 1).
			Render(m.currentTeamRole.String())
	}

	headerLeft := headerStyle.Render(fmt.Sprintf("Ihand TUI %s · %s/%s", version, m.provider, m.modelName))
	if teamTag != "" {
		headerLeft = lipgloss.JoinHorizontal(lipgloss.Top, modeTag, effortTag, teamTag, headerLeft)
	} else {
		headerLeft = lipgloss.JoinHorizontal(lipgloss.Top, modeTag, effortTag, headerLeft)
	}
	sessionInfo := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Foreground(dimColor).
		Render(fmt.Sprintf("Session: %s", m.session))
	headerGap := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(sessionInfo) - 2
	if headerGap < 1 {
		headerGap = 1
	}
	headerContent := lipgloss.JoinHorizontal(lipgloss.Top,
		headerLeft,
		strings.Repeat(" ", headerGap),
		sessionInfo,
	)
	padRight := m.width - lipgloss.Width(headerContent)
	if padRight < 0 {
		padRight = 0
	}
	header := headerBarStyle.Render(headerContent + strings.Repeat(" ", padRight))

	sep := separatorStyle.Render(strings.Repeat("━", m.width))

	vp := m.viewport.View()

	var status string
	if m.state == stateThinking {
		// Minimal status during thinking — main indicator is in the chatbox area
		if len(m.messages) > 0 {
			status = fmt.Sprintf(" ~%d total tokens  |  %d messages",
				m.totalTokens, len(m.messages))
		} else {
			status = ""
		}
		statusW := lipgloss.Width(status)
		if m.width > statusW {
			status = status + strings.Repeat(" ", m.width-statusW)
		}
		status = statusStyle.Render(status)
	} else {
		if m.toolActivity != "" {
			status = " " + m.toolActivity
		} else {
			if len(m.messages) > 0 {
				status = fmt.Sprintf(" Ready  |  ~%d total tokens  |  %d messages",
					m.totalTokens, len(m.messages))
			} else {
				status = " Ready — ketik pesan untuk memulai"
			}
		}
		// Mouse mode indicator
		if m.mouseEnabled {
			status += dimStyle.Render(" mouse on (Ctrl+E)")
		}
		statusW := lipgloss.Width(status)
		if m.width > statusW {
			status = status + strings.Repeat(" ", m.width-statusW)
		}
		status = statusStyle.Render(status)
	}

	var sug string
	if len(m.suggestions) > 0 {
		sug = m.renderSuggestions()
	}

	var bottom string
	if m.state == stateSelectingEffort {
		bottom = m.renderEffortSelector()
	} else if m.state == stateConfirming {
		bottom = m.renderConfirmPrompt()
	} else if m.state == stateThinking {
		bottom = m.renderThinkingIndicator()
	} else if m.state == stateTrustPrompt {
		bottom = m.renderTrustPrompt()
	} else {
		input := m.textarea.View()
		bottom = input
		if sug != "" {
			bottom = lipgloss.JoinVertical(lipgloss.Left, sug, bottom)
		}
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

// ---------------------------------------------------------------------------
// Thinking indicator (replaces chatbox while AI is working)
// ---------------------------------------------------------------------------

func (m *model) renderThinkingIndicator() string {
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := spinnerFrames[m.tickCount%len(spinnerFrames)]

	accentColor := lipgloss.Color("214")
	if m.mode == modeTeam && m.currentTeamRole != roleNone {
		accentColor = lipgloss.Color(m.currentTeamRole.Color())
	}

	spinnerStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)

	var label string
	if m.mode == modeTeam && m.currentTeamRole != roleNone {
		label = fmt.Sprintf("[%s] ", m.currentTeamRole.String())
	}

	var detail string
	if m.toolActivity != "" {
		detail = m.toolActivity
	} else {
		detail = "Memproses permintaan..."
	}

	line := fmt.Sprintf("  %s %s%s",
		spinnerStyle.Render(spinner),
		lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(label),
		lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(detail),
	)

	// Pad to full width and add empty lines to match textarea height
	w := lipgloss.Width(line)
	if m.width > w {
		line = line + strings.Repeat(" ", m.width-w)
	}
	return line + "\n\n"
}

// ---------------------------------------------------------------------------
// Conversation rendering
// ---------------------------------------------------------------------------

func (m *model) buildConversation() string {
	// Lebar untuk pesan biasa (dengan indentasi)
	contentWidth := m.width - 6
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Lebar untuk welcome box = lebar viewport (tanpa indentasi)
	boxWidth := m.width
	if boxWidth < 40 {
		boxWidth = 40
	}

	if len(m.messages) == 0 {
		return welcomeMessage(m.provider, m.modelName, boxWidth)
	}

	var sb strings.Builder

	for i, msg := range m.messages {
		switch msg.role {
		case "user":
			sb.WriteString(userPromptStyle.Render("▸ "))
			sb.WriteString(lipgloss.NewStyle().
				MaxWidth(contentWidth).
				Render(msg.content))

		case "assistant":
			info := fmt.Sprintf("✓ [%v · ~%d token]",
				msg.timing.Round(time.Millisecond), msg.tokens)
			sb.WriteString(checkStyle.Render(info))
			sb.WriteString("\n")
			rendered := m.renderMarkdown(msg.content, contentWidth)
			sb.WriteString(rendered)

		case "tool":
			sb.WriteString(toolStyle().Render("" + msg.content))

		case "tool-error":
			sb.WriteString(toolErrorStyle().Render("✗ " + msg.content))

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

func (m *model) renderConfirmPrompt() string {
	toolName := m.pendingTool.name
	path := extractField(m.pendingTool.input, "\"path\"")

	boxWidth := m.width
	if boxWidth < 40 {
		boxWidth = 40
	}
	innerWidth := boxWidth - 2

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	// Option button styles
	btnActiveAllow := lipgloss.NewStyle().
		Background(lipgloss.Color("34")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 2)
	btnInactiveAllow := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("76")).
		Padding(0, 2)
	btnActiveDeny := lipgloss.NewStyle().
		Background(lipgloss.Color("196")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 2)
	btnInactiveDeny := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("196")).
		Padding(0, 2)

	var icon string
	switch toolName {
	case "write_file":
		icon = "\u270f\ufe0f"
	case "read_file":
		icon = "\U0001f4d6"
	default:
		icon = "\U0001f527"
	}

	var sb strings.Builder

	// Top border
	sb.WriteString(borderStyle.Render("\u250c" + strings.Repeat("\u2500", innerWidth) + "\u2510"))
	sb.WriteString("\n")

	// Title: icon + tool name
	title := fmt.Sprintf(" %s  %s  ", icon, titleStyle.Render(toolName))
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString(title)
	padding := innerWidth - lipgloss.Width(title)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString("\n")

	// Path
	pathText := fmt.Sprintf(" %s %s", labelStyle.Render("Path:"), valueStyle.Render(path))
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString(pathText)
	padding = innerWidth - lipgloss.Width(pathText)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString("\n")

	// Empty line separator
	sb.WriteString(borderStyle.Render("\u2502" + strings.Repeat(" ", innerWidth) + "\u2502"))
	sb.WriteString("\n")

	// Option buttons instead of y/n hint
	var allowBtn, denyBtn string
	if m.confirmChoice == 0 {
		allowBtn = btnActiveAllow.Render("✓ Allow")
		denyBtn = btnInactiveDeny.Render("✗ Deny")
	} else {
		allowBtn = btnInactiveAllow.Render("✓ Allow")
		denyBtn = btnActiveDeny.Render("✗ Deny")
	}
	buttons := fmt.Sprintf(" %s  %s  %s", allowBtn, denyBtn, hintStyle.Render("Tab/←→ pilih · Enter konfirmasi"))
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString(buttons)
	padding = innerWidth - lipgloss.Width(buttons)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString("\n")

	// Bottom border
	sb.WriteString(borderStyle.Render("\u2514" + strings.Repeat("\u2500", innerWidth) + "\u2518"))

	return sb.String()
}

// ---------------------------------------------------------------------------
// Trust Prompt rendering
// ---------------------------------------------------------------------------

func (m *model) renderTrustPrompt() string {
	boxWidth := m.width
	if boxWidth < 40 {
		boxWidth = 40
	}
	innerWidth := boxWidth - 2

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	btnActive := lipgloss.NewStyle().
		Background(lipgloss.Color("34")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 2)
	btnInactive := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("76")).
		Padding(0, 2)
	btnActiveDeny := lipgloss.NewStyle().
		Background(lipgloss.Color("196")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 2)
	btnInactiveDeny := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("196")).
		Padding(0, 2)

	var sb strings.Builder

	// Top border
	sb.WriteString(borderStyle.Render("┌" + strings.Repeat("─", innerWidth) + "┐"))
	sb.WriteString("\n")

	// Title
	title := fmt.Sprintf(" \U0001f512  %s  ", titleStyle.Render("Trust Directory?"))
	sb.WriteString(borderStyle.Render("│"))
	sb.WriteString(title)
	padding := innerWidth - lipgloss.Width(title)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("│"))
	sb.WriteString("\n")

	// Directory path
	pathText := fmt.Sprintf(" %s %s", labelStyle.Render("Dir:"), valueStyle.Render(m.allowedDirAbs))
	sb.WriteString(borderStyle.Render("│"))
	sb.WriteString(pathText)
	padding = innerWidth - lipgloss.Width(pathText)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("│"))
	sb.WriteString("\n")

	// Empty line
	sb.WriteString(borderStyle.Render("│" + strings.Repeat(" ", innerWidth) + "│"))
	sb.WriteString("\n")

	// Description lines
	descLines := []string{
		"Percayakan direktori ini?",
		"Dengan percaya, mode auto/edit/team dapat langsung",
		"menulis dan membuat file tanpa konfirmasi setiap kali.",
		"Kamu bisa mengubahnya nanti dengan /trust.",
	}
	for _, line := range descLines {
		lineRendered := "  " + line + strings.Repeat(" ", innerWidth-lipgloss.Width(line)-2)
		sb.WriteString(borderStyle.Render("│"))
		sb.WriteString(lineRendered)
		sb.WriteString(borderStyle.Render("│"))
		sb.WriteString("\n")
	}

	// Empty line
	sb.WriteString(borderStyle.Render("│" + strings.Repeat(" ", innerWidth) + "│"))
	sb.WriteString("\n")

	// Buttons
	var trustBtn, skipBtn string
	if m.confirmChoice == 0 {
		trustBtn = btnActive.Render("✓ Trust")
		skipBtn = btnInactiveDeny.Render("✗ Skip")
	} else {
		trustBtn = btnInactive.Render("✓ Trust")
		skipBtn = btnActiveDeny.Render("✗ Skip")
	}
	buttons := fmt.Sprintf(" %s  %s  %s", trustBtn, skipBtn, hintStyle.Render("Tab/←→ pilih · Enter konfirmasi"))
	sb.WriteString(borderStyle.Render("│"))
	sb.WriteString(buttons)
	padding = innerWidth - lipgloss.Width(buttons)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("│"))
	sb.WriteString("\n")

	// Bottom border
	sb.WriteString(borderStyle.Render("└" + strings.Repeat("─", innerWidth) + "┘"))

	return sb.String()
}

// ---------------------------------------------------------------------------
// Effort Selector rendering
// ---------------------------------------------------------------------------

func (m *model) renderEffortSelector() string {
	boxWidth := m.width
	if boxWidth < 40 {
		boxWidth = 40
	}
	innerWidth := boxWidth - 2

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	// Option button styles
	btnActiveLow := lipgloss.NewStyle().
		Background(lipgloss.Color("39")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 2)
	btnInactiveLow := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("39")).
		Padding(0, 2)

	btnActiveMed := lipgloss.NewStyle().
		Background(lipgloss.Color("214")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 2)
	btnInactiveMed := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("214")).
		Padding(0, 2)

	btnActiveHigh := lipgloss.NewStyle().
		Background(lipgloss.Color("196")).
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Padding(0, 2)
	btnInactiveHigh := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("196")).
		Padding(0, 2)

	var sb strings.Builder

	// Top border
	sb.WriteString(borderStyle.Render("\u250c" + strings.Repeat("\u2500", innerWidth) + "\u2510"))
	sb.WriteString("\n")

	// Title
	title := fmt.Sprintf(" \u2699\ufe0f  %s ", titleStyle.Render("Set AI Effort Level"))
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString(title)
	padding := innerWidth - lipgloss.Width(title)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString("\n")

	// Empty line separator
	sb.WriteString(borderStyle.Render("\u2502" + strings.Repeat(" ", innerWidth) + "\u2502"))
	sb.WriteString("\n")

	// Option buttons
	var lowBtn, medBtn, highBtn string
	if m.tempEffort == effortLow {
		lowBtn = btnActiveLow.Render("Low")
	} else {
		lowBtn = btnInactiveLow.Render("Low")
	}

	if m.tempEffort == effortMedium {
		medBtn = btnActiveMed.Render("Medium")
	} else {
		medBtn = btnInactiveMed.Render("Medium")
	}

	if m.tempEffort == effortHigh {
		highBtn = btnActiveHigh.Render("High")
	} else {
		highBtn = btnInactiveHigh.Render("High")
	}

	buttons := fmt.Sprintf(" %s  %s  %s  %s", lowBtn, medBtn, highBtn, hintStyle.Render("←→ pilih · Enter konfirmasi · Esc batal"))
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString(buttons)
	padding = innerWidth - lipgloss.Width(buttons)
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(borderStyle.Render("\u2502"))
	sb.WriteString("\n")

	// Bottom border
	sb.WriteString(borderStyle.Render("\u2514" + strings.Repeat("\u2500", innerWidth) + "\u2518"))

	return sb.String()
}

// ---------------------------------------------------------------------------
// Markdown rendering
// ---------------------------------------------------------------------------

func (m *model) renderMarkdown(text string, width int) string {
	wrapWidth := width - 4
	if wrapWidth < 20 {
		wrapWidth = 20
	}

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

	if m.mdRenderer != nil {
		rendered, err := m.mdRenderer.Render(text)
		if err == nil {
			return rendered
		}
	}

	rendered, err := glamour.Render(text, "dark")
	if err != nil {
		return text
	}
	return rendered
}

// ---------------------------------------------------------------------------
// Suggestion bar rendering
// ---------------------------------------------------------------------------

func (m *model) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return ""
	}

	var items []string
	for i, cmd := range m.suggestions {
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

	row := lipgloss.JoinHorizontal(lipgloss.Top, items...)
	hint := suggestionDimStyle.Render(" Tab \u21bb")
	return lipgloss.JoinHorizontal(lipgloss.Top, row, hint)
}
