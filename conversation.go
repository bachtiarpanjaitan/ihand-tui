package main

import (
	"fmt"
	"strings"
	"time"

	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// buildConversation assembles all chat messages into a single display string
// for the main viewport.
func (m *model) buildConversation() string {
	if len(m.messages) == 0 {
		return m.welcomeString()
	}

	var b strings.Builder

	for i, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString(userPromptStyle.Render("❯ "))
			b.WriteString(msg.content)
			b.WriteString("\n\n")

		case "assistant":
			if !msg.streaming {
				if msg.timing > 0 {
					header := lipgloss.NewStyle().
						Foreground(lipgloss.Color("39")).
						Bold(true).
						Render(fmt.Sprintf("%s/%s [%s]",
							m.provider, m.modelName, formatDuration(msg.timing)))
					b.WriteString(header)
					b.WriteString("\n")
				}
				// Render markdown untuk final answer
				rendered := m.renderMarkdown(msg.content)
				b.WriteString(rendered)
				b.WriteString("\n")
			} else {
				// Streaming content — plain text (belum final)
				b.WriteString(msg.content)
				b.WriteString("\n\n")
			}

		case "tool", "tool-error":
			b.WriteString(renderToolTree(msg))
			// Only add extra newline if the NEXT message is NOT a tool call
			// This groups consecutive tool calls together without extra spacing
			isNextTool := i+1 < len(m.messages) &&
				(m.messages[i+1].role == "tool" || m.messages[i+1].role == "tool-error")
			if !isNextTool {
				b.WriteString("\n")
			}

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

// ---------------------------------------------------------------------------
// Tree-view rendering for tool calls (Claude Code style)
// ---------------------------------------------------------------------------

// renderToolTree renders a single tool call as a tree node with connector lines.
//
// Output example:
//   ⎿  read_file("main.go")                                  ✓ 1024 bytes
//   │  package main
//   │  func main() { ... }
//   │  ... (+30 lines)
//
func renderToolTree(msg chatMessage) string {
	var b strings.Builder
	isError := msg.role == "tool-error"

	// --- Header line: ⎿  tool_name("path")  status ---
	connector := treeConnectorStyle.Render("  ⎿  ")

	toolDisplay := buildToolHeader(msg.toolName, msg.content, isError)
	b.WriteString(connector)
	b.WriteString(toolDisplay)
	b.WriteString("\n")

	// --- Content lines: │  content ---
	contentLines := extractToolContentLines(msg.content)
	if len(contentLines) > 0 {
		const maxLines = 20
		pipe := treeConnectorStyle.Render("  │  ")

		for i, line := range contentLines {
			if i >= maxLines {
				remaining := len(contentLines) - maxLines
				b.WriteString(pipe)
				b.WriteString(treeMetaStyle.Render(fmt.Sprintf("... (+%d lines)", remaining)))
				b.WriteString("\n")
				break
			}
			b.WriteString(pipe)
			b.WriteString(styleContentLine(line))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// buildToolHeader constructs the header portion of a tool tree node.
// Format: tool_name("path/arg")  ✓ meta   or   tool_name("path")  ✗ Error
func buildToolHeader(toolName, content string, isError bool) string {
	// Determine display argument (path or command)
	arg := extractToolArg(toolName, content)

	var header string
	if arg != "" {
		header = treeToolNameStyle.Render(toolName) +
			treeConnectorStyle.Render("(") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("215")).Render("\""+arg+"\"") +
			treeConnectorStyle.Render(")")
	} else {
		header = treeToolNameStyle.Render(toolName)
	}

	// Status + meta
	if isError {
		header += "  " + treeErrorStyle.Render("✗")
	} else {
		meta := extractToolMeta(toolName, content)
		header += "  " + treeSuccessStyle.Render("✓")
		if meta != "" {
			header += " " + treeMetaStyle.Render(meta)
		}
	}

	return header
}

// extractToolArg extracts the primary argument (path or command) from tool display content.
func extractToolArg(toolName, content string) string {
	// Content from formatToolDisplay already starts with path or "$ command"
	firstLine := content
	if idx := strings.Index(content, "\n"); idx >= 0 {
		firstLine = content[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)

	switch toolName {
	case "exec":
		// Format: "$ command ..." or "$ command — Selesai (exit 0)"
		if strings.HasPrefix(firstLine, "$ ") {
			cmd := strings.TrimPrefix(firstLine, "$ ")
			// Remove status suffix
			if idx := strings.Index(cmd, " — "); idx >= 0 {
				cmd = cmd[:idx]
			}
			if idx := strings.Index(cmd, " (exit"); idx >= 0 {
				cmd = cmd[:idx]
			}
			if len(cmd) > 60 {
				cmd = cmd[:60] + "..."
			}
			return cmd
		}
	case "read_file", "write_file", "edit_file", "create_directory":
		// Format: "path/to/file — info"
		if idx := strings.Index(firstLine, " —"); idx >= 0 {
			return firstLine[:idx]
		}
		if idx := strings.Index(firstLine, " \u2014"); idx >= 0 {
			return firstLine[:idx]
		}
		// Might just be "path — info" with em dash
		return firstLine
	case "list_files":
		if idx := strings.Index(firstLine, " —"); idx >= 0 {
			return firstLine[:idx]
		}
		if idx := strings.Index(firstLine, " \u2014"); idx >= 0 {
			return firstLine[:idx]
		}
	case "read_file_lines":
		// Format: "path: baris X-Y"
		if idx := strings.Index(firstLine, ":"); idx >= 0 {
			return firstLine[:idx]
		}
	case "find_files", "search_text":
		// No path arg, just results
		return ""
	}

	// Fallback: first word of first line if it looks like a path
	if strings.Contains(firstLine, "/") || strings.Contains(firstLine, ".") {
		if idx := strings.IndexAny(firstLine, " —\u2014"); idx >= 0 {
			return firstLine[:idx]
		}
	}

	return ""
}

// extractToolMeta extracts meta info (bytes, line count, etc.) from tool display content.
func extractToolMeta(toolName, content string) string {
	firstLine := content
	if idx := strings.Index(content, "\n"); idx >= 0 {
		firstLine = content[:idx]
	}

	switch toolName {
	case "read_file":
		// Format: "path — Dibaca (1024 bytes)"
		if idx := strings.Index(firstLine, "("); idx >= 0 {
			end := strings.Index(firstLine[idx:], ")")
			if end >= 0 {
				return firstLine[idx+1 : idx+end]
			}
		}
	case "write_file", "edit_file":
		// Format: "path — 1024 byte (N baris diubah)"
		if idx := strings.Index(firstLine, "\u2014 "); idx >= 0 {
			meta := firstLine[idx+len("\u2014 "):]
			return meta
		}
		if idx := strings.Index(firstLine, "— "); idx >= 0 {
			meta := firstLine[idx+3:]
			return meta
		}
	case "exec":
		// Format: "$ cmd — Selesai (exit 0)" or "$ cmd (exit N)"
		if strings.Contains(firstLine, "exit 0") {
			return "exit 0"
		}
		if idx := strings.Index(firstLine, "(exit"); idx >= 0 {
			end := strings.Index(firstLine[idx:], ")")
			if end >= 0 {
				return firstLine[idx+1 : idx+end]
			}
		}
	case "list_files":
		// Format: "path — N item"
		if idx := strings.Index(firstLine, "— "); idx >= 0 {
			return firstLine[idx+3:]
		}
	case "find_files":
		return firstLine
	case "search_text":
		return firstLine
	case "create_directory":
		if idx := strings.Index(firstLine, "— "); idx >= 0 {
			return firstLine[idx+3:]
		}
	case "read_file_lines":
		// Format: "path: baris X-Y"
		if idx := strings.Index(firstLine, ": "); idx >= 0 {
			return firstLine[idx+2:]
		}
	}

	return ""
}

// extractToolContentLines extracts the multi-line content body from a tool display.
// The first line is the header (path/status), subsequent lines are content.
func extractToolContentLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 1 {
		return nil
	}
	// Skip the first line (header) and any trailing empty lines
	body := lines[1:]
	// Trim trailing empty lines
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	return body
}

// styleContentLine applies diff-style coloring to a content line.
func styleContentLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Check for ANSI-colored diff lines (from formatToolDisplay)
	if strings.Contains(line, "\033[32m") {
		// Already green (added line) — strip ANSI and re-apply with our style
		cleaned := stripANSI(line)
		return treeDiffAddStyle.Render(strings.TrimSpace(cleaned))
	}
	if strings.Contains(line, "\033[31m") {
		// Already red (removed line) — strip ANSI and re-apply with our style
		cleaned := stripANSI(line)
		return treeDiffDelStyle.Render(strings.TrimSpace(cleaned))
	}

	// Raw diff lines (from content preview without ANSI)
	if strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++") {
		return treeDiffAddStyle.Render(trimmed)
	}
	if strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---") {
		return treeDiffDelStyle.Render(trimmed)
	}

	// stderr lines
	if strings.HasPrefix(trimmed, "stderr:") {
		return treeErrorStyle.Render(trimmed)
	}

	// "..." truncation indicator
	if strings.HasPrefix(trimmed, "...") {
		return treeMetaStyle.Render(trimmed)
	}

	return treeContentStyle.Render(trimmed)
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip until 'm'
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip 'm'
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// welcomeString returns the welcome message when there are no messages yet.
func (m *model) welcomeString() string {
	return welcomeMessage(m.provider, m.modelName, m.width)
}

// renderMarkdown merender teks Markdown ke format terminal menggunakan glamour.
func (m *model) renderMarkdown(text string) string {
	if text == "" {
		return ""
	}
	// Hitung lebar konten
	contentWidth := m.width - 6
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Buat atau update renderer jika lebar berubah
	if m.mdWidth != contentWidth || m.mdRenderer == nil {
		m.mdWidth = contentWidth
		// Custom style override to hide heading hashes/prefixes (H2-H6)
		customStyle := []byte(`{
			"h2": { "prefix": "" },
			"h3": { "prefix": "" },
			"h4": { "prefix": "" },
			"h5": { "prefix": "" },
			"h6": { "prefix": "" }
		}`)
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithStylesFromJSONBytes(customStyle),
			glamour.WithWordWrap(contentWidth),
		)
		if err != nil {
			m.mdRenderer = nil
		} else {
			m.mdRenderer = r
		}
	}

	if m.mdRenderer != nil {
		rendered, err := m.mdRenderer.Render(text)
		if err == nil {
			return rendered
		}
	}

	// Fallback: fallback rendering manual untuk bold/italic/kode
	result := text
	result = strings.ReplaceAll(result, "**", "")
	result = strings.ReplaceAll(result, "__", "")
	result = strings.ReplaceAll(result, "`", "")
	return result
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
