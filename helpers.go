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
