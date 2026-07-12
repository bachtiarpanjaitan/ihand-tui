package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atotto/clipboard"
	tea "charm.land/bubbletea/v2"
)

func countTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

// computeFileSuggestions finds files/folders matching the last @mention in the input.
// Returns matching relative paths and the position of '@' in the input string.
// Returns (nil, -1) if no '@' is found or no files match.
func computeFileSuggestions(input string, allowedDir string) ([]string, int) {
	// Find the last '@' in the input
	atIdx := strings.LastIndex(input, "@")
	if atIdx < 0 {
		return nil, -1
	}

	// Extract query after '@' until space, newline, or end of string
	afterAt := input[atIdx+1:]
	spaceIdx := strings.IndexAny(afterAt, " \t\n\r")
	query := afterAt
	if spaceIdx >= 0 {
		query = afterAt[:spaceIdx]
	}

	// If query is empty, match all files (show everything)
	// Walk directory and collect matches
	var matches []string
	maxDepth := 4
	maxResults := 20
	skipDirs := map[string]bool{".git": true, "node_modules": true, ".claude": true, "vendor": true}

	absAllowed, err := filepath.Abs(allowedDir)
	if err != nil {
		return nil, -1
	}

	filepath.WalkDir(absAllowed, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip ignored directories
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			// Limit depth
			rel, _ := filepath.Rel(absAllowed, path)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= maxDepth {
				return filepath.SkipDir
			}
		}

		// Skip hidden files (except current dir)
		if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary/large files
		if !d.IsDir() {
			info, _ := d.Info()
			if info != nil && info.Size() > 1_000_000 {
				return nil
			}
		}

		// Get relative path
		rel, _ := filepath.Rel(absAllowed, path)
		if rel == "." {
			return nil // skip root itself
		}

		// Match query against path (case-insensitive)
		queryLower := strings.ToLower(query)
		relLower := strings.ToLower(rel)

		if query == "" || strings.Contains(relLower, queryLower) {
			// Add trailing slash for directories
			if d.IsDir() {
				rel += "/"
			}
			matches = append(matches, rel)
		}

		return nil
	})

	if len(matches) == 0 {
		return nil, -1
	}

	// Sort: prefix matches first, then alphabetically
	sort.Slice(matches, func(i, j int) bool {
		qi := strings.ToLower(query)
		iPrefix := strings.HasPrefix(strings.ToLower(matches[i]), qi)
		jPrefix := strings.HasPrefix(strings.ToLower(matches[j]), qi)
		if iPrefix != jPrefix {
			return iPrefix // prefix matches come first
		}
		return matches[i] < matches[j]
	})

	// Limit results
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	return matches, atIdx
}

// copyConversation copies the entire conversation to the system clipboard.
func copyConversation(m *model) tea.Cmd {
	return func() tea.Msg {
		if len(m.messages) == 0 {
			return nil
		}
		var sb strings.Builder
		for _, msg := range m.messages {
			switch msg.role {
			case "user":
				sb.WriteString(">> " + msg.content + "\n")
			case "assistant":
				sb.WriteString(msg.content + "\n")
			case "tool", "tool-error":
				sb.WriteString("-- " + msg.content + "\n")
			}
		}
		text := sb.String()
		if text == "" {
			return nil
		}
		if err := clipboard.WriteAll(text); err != nil {
			return llmErrorMsg{err: fmt.Errorf("gagal copy: %v", err)}
		}
		m.toolActivity = "Disalin ke clipboard (" + fmt.Sprintf("%d", len(text)) + " karakter)"
		return nil
	}
}

// resolveFileMentions replaces @displayName with the full relative path in the input text.
// mentions maps display name (without @) to full relative path.
func resolveFileMentions(input string, mentions map[string]string) string {
	if len(mentions) == 0 {
		return input
	}
	result := input
	for display, fullPath := range mentions {
		result = strings.ReplaceAll(result, "@"+display, fullPath)
	}
	return result
}

func (m *model) trackActionFromContent(streamContent string) {
	// Count new actions in this stream content that haven't been counted yet
	// Simple approach: count occurrences of each tool name
	m.actionCounts = actionCounters{} // reset — recount all from content
	if strings.Contains(streamContent, "read_file") {
		m.actionCounts.read = strings.Count(streamContent, "Action: read_file")
	}
	if strings.Contains(streamContent, "read_file_lines") {
		m.actionCounts.read += strings.Count(streamContent, "Action: read_file_lines")
	}
	if strings.Contains(streamContent, "write_file") {
		m.actionCounts.write = strings.Count(streamContent, "Action: write_file")
	}
	if strings.Contains(streamContent, "edit_file") {
		m.actionCounts.edit = strings.Count(streamContent, "Action: edit_file")
	}
	if strings.Contains(streamContent, "list_files") {
		m.actionCounts.list = strings.Count(streamContent, "Action: list_files")
	}
	if strings.Contains(streamContent, "find_files") {
		m.actionCounts.find = strings.Count(streamContent, "Action: find_files")
	}
	if strings.Contains(streamContent, "search_text") {
		m.actionCounts.search = strings.Count(streamContent, "Action: search_text")
	}
	if strings.Contains(streamContent, "exec") {
		m.actionCounts.exec = strings.Count(streamContent, "Action: exec")
	}
	if strings.Contains(streamContent, "browse") {
		m.actionCounts.browse = strings.Count(streamContent, "Action: browse")
	}
	if strings.Contains(streamContent, "create_directory") {
		m.actionCounts.created = strings.Count(streamContent, "Action: create_directory")
	}
}

// showPendingToolCalls extracts tool calls from streaming content and appends
// pending tool messages BELOW the thinking process so users see what's happening in real-time.
func (m *model) showPendingToolCalls() {
	// Only parse tool calls before "Final Answer" to avoid parsing answer text
	content := m.streamingContent
	if hasFinalAnswer(content) {
		// Find the first Final Answer indicator and truncate before it
		for _, kw := range []string{"Final Answer", "Jawaban Akhir", "Kesimpulan"} {
			if idx := strings.Index(content, kw); idx >= 0 {
				content = content[:idx]
				break
			}
		}
	}
	toolCalls := parseReActAll(content)
	for _, tc := range toolCalls {
		// Validate: skip obviously invalid tool calls
		if !isKnownTool(tc.name) {
			continue
		}
		// For exec: command must not look like Final Answer text or markdown
		if tc.name == "exec" {
			cmd := extractField(tc.input, `"command"`)
			cmd = strings.TrimSpace(cmd)
			if cmd == "" || len(cmd) > 200 ||
				strings.HasPrefix(cmd, "**") || strings.HasPrefix(cmd, "✅") ||
				strings.HasPrefix(cmd, "- ") || strings.HasPrefix(cmd, "* ") ||
				strings.Contains(cmd, "telah menyelesaikan") ||
				strings.Contains(cmd, "task sudah selesai") ||
				strings.Contains(cmd, "tidak ada lagi") {
				continue
			}
		}
		// For read_file/write_file/edit_file: path must look like a real file path
		if tc.name == "read_file" || tc.name == "write_file" || tc.name == "edit_file" {
			path := extractField(tc.input, `"path"`)
			path = strings.TrimSpace(path)
			if path == "" || len(path) > 200 ||
				strings.HasPrefix(path, "✅") || strings.HasPrefix(path, "**") ||
				strings.Contains(path, "task") || strings.Contains(path, "TASK") ||
				strings.Contains(path, "sudah selesai") ||
				strings.Count(path, " ") > 5 { // too many spaces = not a path
				continue
			}
		}
		// Build dedup key from extracted args (robust against JSON formatting differences)
		path := extractField(tc.input, `"path"`)
		if path != "" && !filepath.IsAbs(path) {
			if absPath, err := filepath.Abs(filepath.Join(m.allowedDir, path)); err == nil {
				path = absPath
			}
		}
		command := extractField(tc.input, `"command"`)
		key := tc.name + "|" + path + "|" + command
		if m.shownToolKeys[key] {
			continue
		}
		m.shownToolKeys[key] = true

		// Build pending display: first line must be parseable by extractToolArg
		var pendingDisplay string
		switch tc.name {
		case "read_file", "read_file_lines":
			if path != "" {
				pendingDisplay = path + " — membaca..."
			}
		case "write_file":
			if path != "" {
				pendingDisplay = path + " — menulis..."
				// Show content preview (decode JSON properly to handle \n, \t, etc.)
				content := extractJSONStringField(tc.input, "content")
				if content != "" {
					preview := truncateContentPreview(content, 300, 10)
					pendingDisplay += "\n" + preview
				}
			}
		case "edit_file":
			if path != "" {
				pendingDisplay = path + " — mengedit..."
				// Show search/replace preview (decode JSON properly)
				search := extractJSONStringField(tc.input, "search")
				replace := extractJSONStringField(tc.input, "replace")
				if search != "" || replace != "" {
					preview := "-" + truncateStr(search, 80)
					if replace != "" {
						preview += "\n+" + truncateStr(replace, 80)
					}
					pendingDisplay += "\n" + preview
				}
			}
		case "create_directory":
			if path != "" {
				pendingDisplay = path + " — membuat direktori..."
			}
		case "exec":
			if command != "" {
				if len(command) > 60 {
					command = command[:60] + "..."
				}
				pendingDisplay = "$ " + command + " — menjalankan..."
			}
		case "list_files":
			if path != "" {
				pendingDisplay = path + " — membaca direktori..."
			}
		case "find_files":
			if path != "" {
				pendingDisplay = path + " — mencari file..."
			} else {
				pendingDisplay = "mencari file..."
			}
		case "search_text":
			if path != "" {
				pendingDisplay = path + " — mencari teks..."
			} else {
				pendingDisplay = "mencari teks..."
			}
		default:
			if path != "" {
				pendingDisplay = path + " — memproses..."
			} else if command != "" {
				pendingDisplay = "$ " + command + " — memproses..."
			}
		}

		if pendingDisplay == "" {
			pendingDisplay = tc.name + " — memproses..."
		}

		// Double-check: don't append if the last tool message already has the same content
		lastIdx := len(m.messages) - 1
		if lastIdx >= 0 && m.messages[lastIdx].toolName == tc.name &&
			m.messages[lastIdx].role == "tool" &&
			m.messages[lastIdx].content == pendingDisplay {
			continue
		}

		// APPEND after the last message so tool calls appear BELOW the thinking process
		toolMsg := chatMessage{
			role:     "tool",
			content:  pendingDisplay,
			toolName: tc.name,
			tokens:   0,
		}
		m.messages = append(m.messages, toolMsg)
	}
}

// truncateContentPreview truncates content for preview display.
func truncateContentPreview(content string, maxChars, maxLines int) string {
	// Limit by character count first
	if len(content) > maxChars {
		content = content[:maxChars] + "..."
	}
	// Limit by line count
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "... (terpotong)")
	}
	return strings.Join(lines, "\n")
}
