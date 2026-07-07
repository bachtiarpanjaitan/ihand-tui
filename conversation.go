package main

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
)

// buildConversation assembles all chat messages into a single display string
// for the main viewport.
func (m *model) buildConversation() string {
	if len(m.messages) == 0 {
		return m.welcomeString()
	}

	var b strings.Builder

	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString(userPromptStyle.Render(">> "))
			b.WriteString(msg.content)
			b.WriteString("\n\n")

		case "assistant":
			if msg.timing > 0 {
				header := lipgloss.NewStyle().
					Foreground(lipgloss.Color("39")).
					Bold(true).
					Render(fmt.Sprintf("%s/%s [%s]",
						m.provider, m.modelName, formatDuration(msg.timing)))
				b.WriteString(header)
				b.WriteString("\n")
			}
			b.WriteString(msg.content)
			b.WriteString("\n\n")

		case "tool":
			b.WriteString(dimStyle.Render("  "))
			b.WriteString(msg.content)
			b.WriteString("\n")

		case "tool-error":
			b.WriteString(errorStyle.Render("  ✗ "))
			b.WriteString(msg.content)
			b.WriteString("\n")

		case "error":
			b.WriteString(errorStyle.Render("  ✗ "))
			b.WriteString(msg.content)
			b.WriteString("\n")

		case "system":
			b.WriteString(dimStyle.Render(msg.content))
			b.WriteString("\n\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// welcomeString returns the welcome message when there are no messages yet.
func (m *model) welcomeString() string {
	return welcomeMessage(m.provider, m.modelName, m.width)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
