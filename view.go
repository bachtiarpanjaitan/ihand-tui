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
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// ---------------------------------------------------------------------------
// Full-screen rendering
// ---------------------------------------------------------------------------

func (m *model) renderFull() string {
	modeTag := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.mode.Color())).
		Bold(true).
		Padding(0, 1).
		Render(m.mode.String())
	headerLeft := headerStyle.Render(fmt.Sprintf("Ihand TUI v%s · %s/%s", version, m.provider, m.modelName))
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
	padRight := m.width - lipgloss.Width(headerContent)
	if padRight < 0 {
		padRight = 0
	}
	header := headerBarStyle.Render(headerContent + strings.Repeat(" ", padRight))

	sep := separatorStyle.Render(strings.Repeat("━", m.width))

	vp := m.viewport.View()

	var status string
	switch m.state {
	case stateThinking:
		if m.statusMsg != "" {
			status = fmt.Sprintf(" ⏳ %s", m.statusMsg)
		} else {
			status = " ⏳ Thinking..."
		}
	case stateReady:
		if len(m.messages) > 0 {
			status = fmt.Sprintf(" ✓ Ready  |  ~%d total tokens  |  %d messages",
				m.totalTokens, len(m.messages))
		} else {
			status = " ✓ Ready — ketik pesan untuk memulai"
		}
	}
	statusW := lipgloss.Width(status)
	if m.width > statusW {
		status = status + strings.Repeat(" ", m.width-statusW)
	}
	status = statusStyle.Render(status)

	var sug string
	if len(m.suggestions) > 0 {
		sug = m.renderSuggestions()
	}

	input := m.textarea.View()
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

// ---------------------------------------------------------------------------
// Conversation rendering
// ---------------------------------------------------------------------------

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
			info := fmt.Sprintf("✓ [%v · ~%d token]",
				msg.timing.Round(time.Millisecond), msg.tokens)
			sb.WriteString(checkStyle.Render(info))
			sb.WriteString("\n")
			sb.WriteString(separatorStyle.Render(strings.Repeat("─", contentWidth)))
			sb.WriteString("\n\n")
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
	hint := suggestionDimStyle.Render(" Tab ↻")
	return lipgloss.JoinHorizontal(lipgloss.Top, row, hint)
}
