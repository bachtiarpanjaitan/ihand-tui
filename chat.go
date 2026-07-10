package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	toolspkg "github.com/bachtiarpanjaitan/ihand-tui/internal/tools"
	"github.com/bachtiarpanjaitan/ihandai-go"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/llm"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/memory"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/tools"

	tea "charm.land/bubbletea/v2"
)

const maxRetries = 3

// ---------------------------------------------------------------------------
// ReAct regex patterns
// ---------------------------------------------------------------------------

var (
	actionRe   = regexp.MustCompile(`Action:\s*(.+)`)
	inputRe    = regexp.MustCompile(`Action Input:\s*(.+)`)
	finalRe    = regexp.MustCompile(`Final Answer:\s*([\s\S]*)`)
	toolCallRe = regexp.MustCompile(`(\w+)\((\{.*?\})\)`)
)

// ---------------------------------------------------------------------------
// Step-based chat loop (replaces makeToolChatCall)
// ---------------------------------------------------------------------------

// streamChunkMsg carries a single text token from the LLM stream.
type streamChunkMsg struct {
	state   chatLoopState
	content string
	done    bool // true when the stream is finished
	ch      <-chan llm.Chunk
}

// waitForStreamChunk reads the next chunk from a stream channel and returns it
// as a Bubble Tea message. When the channel closes, it returns done=true.
func waitForStreamChunk(ch <-chan llm.Chunk, state chatLoopState) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return streamChunkMsg{state: state, done: true, ch: ch}
		}
		return streamChunkMsg{
			state:   state,
			content: chunk.Content,
			done:    chunk.FinishReason != "",
			ch:      ch,
		}
	}
}

// startChatLoop initializes the ReAct loop and fires the first LLM call.
func startChatLoop(ai *ihandai.Client, ctx context.Context, session, input string, store memory.ConversationStore, toolList []tools.Tool, mode chatMode, effort effortLevel, allowedDir string) tea.Cmd {
	return func() tea.Msg {
		llmProvider := ai.LLM()
		streamProvider := ai.StreamLLM()

		if llmProvider == nil && streamProvider == nil {
			return llmErrorMsg{err: fmt.Errorf("LLM provider tidak tersedia")}
		}

		// Save user message to memory for conversation context
		store.Append(ctx, session, core.Message{Role: "user", Content: input})

		history, _ := store.History(ctx, session)

		activeTools := toolList
		if mode == modePlan || mode == modeChat {
			activeTools = nil
			for _, t := range toolList {
				switch t.Name() {
				case "read_file", "list_files", "browse",
					"find_files", "search_text", "read_file_lines":
					activeTools = append(activeTools, t)
				}
			}
		}

		var systemPrompt string
		systemPrompt = buildToolSystemPrompt(activeTools, mode, effort)

		// Auto-context: jika ini pesan pertama dalam sesi, sertakan struktur folder + info ekstensi
		if len(history) <= 1 {
			for _, t := range toolList {
				if t.Name() == "list_files" {
					out, err := t.Execute(ctx, []byte(`{"path": "."}`))
					if err == nil {
						systemPrompt += "\n\n--- KONTEKS OTOMATIS (Struktur Root Direktori) ---\n"
						systemPrompt += "Berikut adalah struktur file dan folder di direktori saat ini:\n"
						systemPrompt += string(out)
						systemPrompt += "\n------------------------------------------------\n"
					}
					break
				}
			}
			// Auto-detect extensions yang ada di project
			for _, t := range toolList {
				if t.Name() == "find_files" {
					exts := toolspkg.DiscoverExtensions(allowedDir)
					if len(exts) > 0 {
						systemPrompt += "\n--- EKSTENSI FILE YANG TERDETEKSI ---\n"
						systemPrompt += "Project ini memiliki file dengan ekstensi berikut:\n"
						extStr := toolspkg.FormatExtensions(exts)
						systemPrompt += extStr + "\n"
						systemPrompt += "Gunakan ekstensi ini sebagai file_pattern di find_files/search_text.\n"
						ext := exts[0].Extension
						if len(ext) > 1 {
							firstExt := ext[1:] // remove dot
							systemPrompt += "Contoh: find_files({\"pattern\": \"*." + firstExt + "\"})\n"
						}
						systemPrompt += "------------------------------------------------\n"
					}
					break
				}
			}
		}

		// Auto-baca file konfigurasi project
		if configContent := readProjectConfigs(allowedDir); configContent != "" {
			systemPrompt += "\n--- KONFIGURASI PROJECT ---\n"
			systemPrompt += configContent
			systemPrompt += "------------------------------------------------\n"
		}

		messages := []core.Message{
			{Role: "system", Content: systemPrompt},
		}
		messages = append(messages, history...)

		initialState := chatLoopState{
			session:     session,
			messages:    messages,
			activeTools: activeTools,
			iteration:   0,
			toolCalls:   nil,
			totalTokens: countTokens(input),
			startTime:   time.Now(),
		}

		if streamProvider != nil {
			ch, err := streamProvider.ChatStream(ctx, messages)
			if err != nil {
				return llmErrorMsg{err: err}
			}
			return waitForStreamChunk(ch, initialState)()
		}

		resp, err := llmProvider.Chat(ctx, messages)
		return chatStepResultMsg{
			state:    initialState,
			response: resp,
			err:      err,
		}
	}
}

// continueChatLoop calls the LLM with updated messages and returns the result.
func continueChatLoop(ai *ihandai.Client, ctx context.Context, state chatLoopState) tea.Cmd {
	return func() tea.Msg {
		llmProvider := ai.LLM()
		streamProvider := ai.StreamLLM()

		if llmProvider == nil && streamProvider == nil {
			return llmErrorMsg{err: fmt.Errorf("LLM provider tidak tersedia")}
		}

		if streamProvider != nil {
			ch, err := streamProvider.ChatStream(ctx, state.messages)
			if err != nil {
				return llmErrorMsg{err: err}
			}
			return waitForStreamChunk(ch, state)()
		}

		resp, err := llmProvider.Chat(ctx, state.messages)
		return chatStepResultMsg{
			state:    state,
			response: resp,
			err:      err,
		}
	}
}

// processChatStep handles the result of an LLM call and either continues the loop or finishes.
// Returns (model, cmd, done) where done=true means the loop is finished.
func processChatStep(m *model, msg chatStepResultMsg) (tea.Cmd, bool) {
	if msg.err != nil {
		m.retryCount++
		if m.retryCount < maxRetries {
			m.statusMsg = fmt.Sprintf("⚠ Retry %d/%d", m.retryCount, maxRetries)
			m.rebuildViewport()
			return tea.Batch(
				m.textarea.Focus(),
				continueChatLoop(m.ai, m.ctx, msg.state),
			), false
		}
		m.state = stateReady
		m.statusMsg = ""
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: "LLM Error: " + msg.err.Error(),
		})
		m.rebuildViewport()
		return m.textarea.Focus(), true
	}

	state := msg.state
	resp := msg.response
	if resp == nil {
		m.retryCount++
		if m.retryCount < maxRetries {
			m.statusMsg = fmt.Sprintf("⚠ Retry %d/%d", m.retryCount, maxRetries)
			m.rebuildViewport()
			return tea.Batch(
				m.textarea.Focus(),
				continueChatLoop(m.ai, m.ctx, msg.state),
			), false
		}
		m.state = stateReady
		m.statusMsg = ""
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: "LLM Error: respons kosong dari API",
		})
		m.rebuildViewport()
		return m.textarea.Focus(), true
	}
	if resp.Content == "" {
		m.retryCount++
		if m.retryCount < maxRetries {
			m.statusMsg = fmt.Sprintf("⚠ Retry %d/%d", m.retryCount, maxRetries)
			m.rebuildViewport()
			return tea.Batch(
				m.textarea.Focus(),
				continueChatLoop(m.ai, m.ctx, msg.state),
			), false
		}
		m.state = stateReady
		m.statusMsg = ""
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: "LLM Error: respons API kosong — cek API key atau koneksi",
		})
		m.rebuildViewport()
		return m.textarea.Focus(), true
	}
	respTokens := countTokens(resp.Content)
	m.retryCount = 0 // reset retry on success
	state.totalTokens += respTokens
	m.totalTokens += respTokens // update UI counter live

	toolCall, isFinal := parseReActResponse(resp.Content)

	// Detect multiple inline Action: calls (LLM concatenating actions on one line).
	// Execute ALL of them so the LLM gets all expected results.
	if !isFinal && toolCall.name != "" {
		allActions := parseReActAll(resp.Content)
		if len(allActions) > 1 {
			return executeAllToolCalls(m, state, resp, allActions)
		}
	}

	// Jika ada Action: tapi konten juga mengandung Final Answer: setelah Action terakhir,
	// ekstrak Final Answer-nya agar ditampilkan. Tool call sudah dieksekusi via early execution.
	if !isFinal && toolCall.name != "" && strings.Contains(resp.Content, "Final Answer:") && m.earlyTool.toolName != "" {
		if m := finalRe.FindStringSubmatch(resp.Content); len(m) > 1 {
			toolCall.output = strings.TrimSpace(m[1])
			isFinal = true
		}
	}

	// --- Final answer ---
	if isFinal {
		// Cek apakah masih ada task yang belum selesai (Edit/Auto mode)
		if m.mode == modeEdit || m.mode == modeAuto {
			var incompleteDescs []string
			for _, t := range m.taskList {
				if t.status != "completed" {
					if len(incompleteDescs) < 5 {
						incompleteDescs = append(incompleteDescs, t.desc)
					}
				}
			}
			if len(incompleteDescs) > 0 && m.retryCount < maxRetries {
				m.retryCount++
				m.statusMsg = fmt.Sprintf("\u23f3 Task tersisa (%d/%d)", m.retryCount, maxRetries)
				taskList := strings.Join(incompleteDescs, "\n  - ")
				reminder := "PERINGATAN: Masih ada task yang BELUM selesai:\n" +
					"  - " + taskList + "\n\n" +
					"Selesaikan SEMUA task menggunakan write_file() atau edit_file() " +
					"sebelum memberikan Final Answer."
				state.messages = append(state.messages,
					core.Message{Role: "assistant", Content: resp.Content},
					core.Message{Role: "user", Content: reminder},
				)
				state.iteration++
				m.rebuildViewport()
				return tea.Batch(
					m.textarea.Focus(),
					continueChatLoop(m.ai, m.ctx, state),
				), false
			}
		}

		// Auto-complete all tasks when the process finishes successfully
		for i := range m.taskList {
			m.taskList[i].status = "completed"
		}
		m.recalcLayout()

		m.memory.Append(m.ctx, state.session, core.Message{
			Role: "assistant", Content: resp.Content,
		})
		// Update last assistant message (streaming placeholder) instead of appending
		updated := false
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" {
				m.messages[i].content = toolCall.output
				m.messages[i].tokens = state.totalTokens
				m.messages[i].timing = time.Since(state.startTime)
				m.messages[i].streaming = false
				updated = true
				break
			}
		}
		if !updated {
			m.messages = append(m.messages, chatMessage{
				role:    "assistant",
				content: toolCall.output,
				tokens:  state.totalTokens,
				timing:  time.Since(state.startTime),
			})
		}
		m.state = stateReady
		m.totalTokens += state.totalTokens
		m.toolActivity = "✓ Selesai"
		m.rebuildViewport()
		m.statusMsg = ""
		return m.textarea.Focus(), true
	}

	// Calculate max iterations based on effort level
	maxIterations := 8 // default (medium)
	if m.mode == modeAuto {
		maxIterations = 16
	}

	switch m.effort {
	case effortLow:
		maxIterations = 4
	case effortHigh:
		maxIterations = 24
	}

	// --- Tool call ---
	if toolCall.name != "" {
		// Cek apakah tool butuh permission
		if needsPermission(toolCall.name) {
			// Jika user sudah trust write, skip konfirmasi
			if isToolAutoTrusted(m.mode, m.trustWrite, toolCall.name) {
				var toolOutput string
				var isToolError bool
				if m.earlyTool.toolName == toolCall.name && m.earlyTool.input == toolCall.input {
					toolOutput = m.earlyTool.output
					isToolError = m.earlyTool.isError
					m.earlyTool = earlyToolExec{} // reset
				} else {
					toolOutput = executeToolCall(state.activeTools, toolCall)
					isToolError = isToolOutputError(toolOutput)
					display := formatToolDisplay(toolCall.name, toolCall.input, toolOutput)
					role := "tool"
					if isToolError {
						role = "tool-error"
					}
					m.toolActivity = fmt.Sprintf("%s", toolCall.name)
					m.messages = append(m.messages, chatMessage{
						role:     role,
						content:  display,
						toolName: toolCall.name,
						tokens:   0,
					})
				}
				state.toolCalls = append(state.toolCalls, toolCallRecord{
					toolName: toolCall.name,
					input:    toolCall.input,
					output:   toolOutput,
					isError:  isToolError,
				})
				state.messages = append(state.messages,
					core.Message{Role: "assistant", Content: resp.Content},
					core.Message{Role: "user", Content: fmt.Sprintf(
						"Observation (hasil dari tool %s): %s", toolCall.name, toolOutput,
					)},
				)
				state.iteration++

				// Max iteration check — prevents fall-through to second execution
				if state.iteration >= maxIterations {
					finalContent := "! Agent mencapai batas maksimum iterasi."
					for i := len(state.messages) - 1; i >= 0; i-- {
						if state.messages[i].Role == "assistant" {
							finalContent = state.messages[i].Content
							break
						}
					}
					_ = state.toolCalls
					// Update last assistant message instead of appending
				updated := false
				for i := len(m.messages) - 1; i >= 0; i-- {
					if m.messages[i].role == "assistant" {
						m.messages[i].content = finalContent
						m.messages[i].tokens = state.totalTokens
						m.messages[i].timing = time.Since(state.startTime)
						m.messages[i].streaming = false
						updated = true
						break
					}
				}
				if !updated {
					m.messages = append(m.messages, chatMessage{
						role:    "assistant",
						content: finalContent,
						tokens:  state.totalTokens,
						timing:  time.Since(state.startTime),
					})
				}
					m.state = stateReady
					m.totalTokens += state.totalTokens
					m.toolActivity = "\u2713 Selesai"
					m.rebuildViewport()
					m.statusMsg = ""
					return m.textarea.Focus(), true
				}

				// Continue the ReAct loop — return early to prevent double execution
				m.rebuildViewport()
				return tea.Batch(
					m.textarea.Focus(),
					continueChatLoop(m.ai, m.ctx, state),
				), false
			} else {
				m.pendingTool = toolCall
				m.pendingState = state
				m.pendingToolResp = resp.Content
				m.state = stateConfirming
				m.confirmChoice = 0 // default to Allow
				m.statusMsg = ""
				m.toolActivity = fmt.Sprintf("🔍 Konfirmasi: %s", toolCall.name)
				m.recalcLayout()
				m.rebuildViewport()
				return m.textarea.Focus(), true
			}
		}

		toolOutput := executeToolCall(state.activeTools, toolCall)
		isToolError := isToolOutputError(toolOutput)

		// Show tool call in both activity bar + conversation
		display := formatToolDisplay(toolCall.name, toolCall.input, toolOutput)
		role := "tool"
		if isToolError {
			role = "tool-error"
		}
		// Activity bar shows latest tool (above input) - concise single line
		m.toolActivity = fmt.Sprintf("%s", toolCall.name)
		// Conversation keeps full history
		m.messages = append(m.messages, chatMessage{
			role:     role,
			content:  display,
			toolName: toolCall.name,
			tokens:   0,
		})

		state.toolCalls = append(state.toolCalls, toolCallRecord{
			toolName: toolCall.name,
			input:    toolCall.input,
			output:   toolOutput,
			isError:  isToolError,
		})

		// Append tool result to LLM messages for next iteration
		state.messages = append(state.messages,
			core.Message{Role: "assistant", Content: resp.Content},
			core.Message{Role: "user", Content: fmt.Sprintf(
				"Observation (hasil dari tool %s): %s", toolCall.name, toolOutput,
			)},
		)
		state.iteration++

		// Check max iterations
		if state.iteration >= maxIterations {
			// Find last assistant message as final content
			finalContent := "! Agent mencapai batas maksimum iterasi."
			for i := len(state.messages) - 1; i >= 0; i-- {
				if state.messages[i].Role == "assistant" {
					finalContent = state.messages[i].Content
					break
				}
			}
			_ = state.toolCalls // already displayed in real-time above
			m.messages = append(m.messages, chatMessage{
				role:    "assistant",
				content: finalContent,
				tokens:  state.totalTokens,
				timing:  time.Since(state.startTime),
			})
			m.state = stateReady
			m.totalTokens += state.totalTokens
			m.toolActivity = "✓ Selesai"
			m.rebuildViewport()
			m.statusMsg = ""
			return m.textarea.Focus(), true
		}

		// Update UI and continue loop
		m.rebuildViewport()
		return tea.Batch(
			m.textarea.Focus(),
			continueChatLoop(m.ai, m.ctx, state),
		), false
	}

	// --- No tool call detected → treat as direct answer ---
	// Tapi di mode Edit/Auto, jangan terima jawaban tanpa tool call — paksa retry
	if (m.mode == modeEdit || m.mode == modeAuto) && m.retryCount < maxRetries {
		m.retryCount++
		m.statusMsg = fmt.Sprintf("⚠ Paksa tool call (%d/%d)", m.retryCount, maxRetries)
		// Tambah pesan sistem ke state yang mengingatkan untuk pakai Action: format
		reminder := "PERINGATAN: Kamu harus menggunakan Action: format untuk memanggil tools. " +
			"Jangan berikan jawaban langsung tanpa memanggil write_file/read_file dll.\n" +
			"Contoh:\n" +
			"  Action: read_file({\"path\": \"file.go\"})\n" +
			"  Action: write_file({\"path\": \"file.go\", \"content\": \"...\"})\n" +
			"Setelah semua tool selesai, berikan Final Answer."
		state.messages = append(state.messages,
			core.Message{Role: "assistant", Content: resp.Content},
			core.Message{Role: "user", Content: reminder},
		)
		state.iteration++
		m.rebuildViewport()
		return tea.Batch(
			m.textarea.Focus(),
			continueChatLoop(m.ai, m.ctx, state),
		), false
	}

	m.memory.Append(m.ctx, state.session, core.Message{
		Role: "assistant", Content: resp.Content,
	})
	m.messages = append(m.messages, chatMessage{
		role:    "assistant",
		content: resp.Content,
		tokens:  state.totalTokens,
		timing:  time.Since(state.startTime),
	})
	for i := range m.taskList {
		m.taskList[i].status = "completed"
	}
	m.recalcLayout()

	m.state = stateReady
	m.totalTokens += state.totalTokens

	m.toolActivity = "✓ Selesai"
	m.rebuildViewport()
	m.statusMsg = ""
	return m.textarea.Focus(), true
}

// needsPermission returns true jika tool memerlukan konfirmasi user sebelum dieksekusi.
func needsPermission(name string) bool {
	switch name {
	case "write_file", "edit_file", "read_file", "create_directory", "exec":
		return true
	default:
		return false
	}
}

// rebuildViewport refreshes the viewport content and auto-scrolls to bottom.
// Use for state transitions (new message, response complete, etc.).
func (m *model) rebuildViewport() {
	content := m.buildConversation()
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

// refreshViewport updates the viewport content WITHOUT auto-scrolling.
// Use during streaming so the user can scroll freely without being interrupted.
func (m *model) refreshViewport() {
	content := m.buildConversation()
	m.viewport.SetContent(content)
}

// ---------------------------------------------------------------------------
// Tool display formatting
// ---------------------------------------------------------------------------

// isToolOutputError checks whether tool output indicates an error.
// Checks both string prefix ("Error") and JSON "error" field.
func isToolOutputError(output string) bool {
	if strings.HasPrefix(output, "Error") {
		return true
	}
	// Check for JSON "error" field
	errField := extractField(output, `"error"`)
	if errField != "" {
		return true
	}
	return false
}

// formatToolDisplay returns a user-friendly display string for a tool call result.
// Uses simple string extraction to avoid issues with imperfect JSON from tool output.
func formatToolDisplay(toolName, input, output string) string {
	path := extractField(output, `"path"`)
	if path == "" {
		path = extractField(output, `"path\":`)
	}
	// Fallback: extract path from input params if output lacks it
	if path == "" {
		path = extractField(input, `"path"`)
	}

	// Check for error
	errMsg := extractField(output, `"error"`)
	if errMsg == "" {
		errMsg = extractField(output, `"error\":`)
	}
	if errMsg != "" {
		switch toolName {
		case "read_file":
			return fmt.Sprintf("%s — %s", path, errMsg)
		case "write_file":
			return fmt.Sprintf("%s — %s", path, errMsg)
		case "list_files", "exec":
			return fmt.Sprintf("%s — %s", path, errMsg)
		}
	}

	// Extract size
	sizeStr := extractField(output, `"size"`)
	if sizeStr == "" {
		sizeStr = extractField(output, `"size\":`)
	}
	size := 0
	fmt.Sscanf(sizeStr, "%d", &size)

	// Extract count
	countStr := extractField(output, `"count"`)
	if countStr == "" {
		countStr = extractField(output, `"count\":`)
	}
	count := 0
	fmt.Sscanf(countStr, "%d", &count)

	switch toolName {
	case "create_directory":
		if path != "" {
			return fmt.Sprintf("%s — Direktori dibuat", path)
		}
	case "find_files":
		countStr := extractField(output, `"count"`)
		count := 0
		fmt.Sscanf(countStr, "%d", &count)
		totalStr := extractField(output, `"total"`)
		total := 0
		fmt.Sscanf(totalStr, "%d", &total)
		truncated := strings.Contains(output, `"truncated": true`)
		if truncated {
			return fmt.Sprintf("Ditemukan %d file (total %d, ditampilkan sebagian)", count, total)
		}
		return fmt.Sprintf("Ditemukan %d file", count)
	case "search_text":
		countStr := extractField(output, `"count"`)
		count := 0
		fmt.Sscanf(countStr, "%d", &count)
		return fmt.Sprintf("Pencarian teks — %d hasil", count)
	case "read_file_lines":
		path := extractField(output, `"path"`)
		startLine := extractField(output, `"start_line"`)
		endLine := extractField(output, `"end_line"`)
		if path != "" {
			return fmt.Sprintf("%s: baris %s-%s", path, startLine, endLine)
		}
	case "read_file":
		// Check if this was a directory auto-listing
		isDirStr := extractField(output, `"is_dir"`)
		if isDirStr == "true" {
			countStr := extractField(output, `"count"`)
			dirCount := 0
			fmt.Sscanf(countStr, "%d", &dirCount)
			if path != "" {
				return fmt.Sprintf("%s — Direktori (%d item)", path, dirCount)
			}
		}
		if path != "" {
			// Extract file content from JSON output for tree-view display
			content := extractJSONStringField(output, "content")
			if content != "" {
				const maxContentLen = 2000
				if len(content) > maxContentLen {
					content = content[:maxContentLen] + "\n... (file terpotong)"
				}
				return fmt.Sprintf("%s — Dibaca (%d bytes)\n%s", path, size, content)
			}
			return fmt.Sprintf("%s — Dibaca (%d bytes)", path, size)
		}
	case "write_file":
		success := strings.Contains(output, `"success": true`) || strings.Contains(output, `"success":true`)
		if path != "" {
			// Extract diff from tool output
			diffStr := extractField(output, `"diff"`)
			var previewDiff string
			if diffStr != "" {
				// Decode JSON string, lalu split
				var diffText string
				if err := json.Unmarshal([]byte(`"`+diffStr+`"`), &diffText); err == nil && diffText != "" {
					diffLines := strings.Split(diffText, "\n")
					var b strings.Builder
					// Git-style diff header
					b.WriteString(fmt.Sprintf("--- a/%s\n", path))
					b.WriteString(fmt.Sprintf("+++ b/%s\n", path))
					for _, line := range diffLines {
						if len(line) > 0 {
							switch line[0] {
							case '+':
								b.WriteString(line + "\n")
							case '-':
								b.WriteString(line + "\n")
							// Skip context lines (starting with space) for cleaner diff
							}
						}
					}
					previewDiff = b.String()
				}
			}
			if success || strings.Contains(output, "berhasil") {
			if previewDiff != "" {
				// Count additions and deletions
				var diffText string
				json.Unmarshal([]byte(`"`+diffStr+`"`), &diffText)
				addCount, delCount := 0, 0
				for _, line := range strings.Split(diffText, "\n") {
					if len(line) > 0 {
						if line[0] == '+' {
							addCount++
						} else if line[0] == '-' {
							delCount++
						}
					}
				}
				// Summary + diff content (renderToolTree adds tree connectors)
				var b strings.Builder
				b.WriteString(fmt.Sprintf("%s — +%d/-%d  ✓  Ditulis (%d bytes)", path, addCount, delCount, size))
				const maxPreviewLines = 20
				diffLines := strings.Split(strings.TrimSpace(previewDiff), "\n")
				if len(diffLines) > maxPreviewLines {
					diffLines = diffLines[:maxPreviewLines]
					diffLines = append(diffLines, "...")
				}
				for _, line := range diffLines {
					b.WriteString("\n" + line)
				}
				return b.String()
			}
			return fmt.Sprintf("%s — Ditulis (%d bytes)  ✓", path, size)
		}
		msg := extractField(output, `"message"`)
		if msg != "" {
			return fmt.Sprintf("%s \u2014 %s", path, msg)
		}
		return fmt.Sprintf("%s \u2014 Selesai", path)		}
	case "list_files":
		if path != "" {
			return fmt.Sprintf("%s — %d item", path, count)
		}
	case "exec":
		execStdout := extractField(output, `"stdout"`)
		execStderr := extractField(output, `"stderr"`)
		exitCode := extractField(output, `"exit_code"`)
		cmdDisplay := toolName
		if p := extractField(input, `"command"`); p != "" {
			cmdDisplay = p
		}
		if exitCode == "0" {
			if execStdout != "" {
				if len(execStdout) > 1000 {
					return fmt.Sprintf("$ %s\n%s\n... (output truncated)", cmdDisplay, execStdout[:1000])
				}
				return fmt.Sprintf("$ %s\n%s", cmdDisplay, execStdout)
			}
			return fmt.Sprintf("$ %s — Selesai (exit 0)", cmdDisplay)
		}
		result := fmt.Sprintf("$ %s (exit %s)", cmdDisplay, exitCode)
		if execStdout != "" {
			if len(execStdout) > 500 {
				result += "\n" + execStdout[:500] + "..."
			} else {
				result += "\n" + execStdout
			}
		}
		if execStderr != "" {
			if len(execStderr) > 500 {
				result += "\nstderr: " + execStderr[:500] + "..."
			} else {
				result += "\nstderr: " + execStderr
			}
		}
		return result
	}

	// Fallback: just extract path and show simple message
	if path != "" {
		return fmt.Sprintf("%s: %s", toolName, path)
	}
	display := output
	if len(display) > 200 {
		display = display[:200] + "..."
	}
	return fmt.Sprintf("%s: %s", toolName, display)
}

// ---------------------------------------------------------------------------
// Tool execution helpers
// ---------------------------------------------------------------------------

// extractBalancedJSON mengambil JSON object dari string dengan menghitung
// bracket {} secara seimbang. Ini menangani kasus di mana konten string
// mengandung karakter } (seperti kode Go, JS, dll).
func extractBalancedJSON(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return "" // tidak menemukan penutup yang seimbang
}

// fixJSON mencoba memperbaiki JSON yang tidak valid dengan melakukan escape
// pada karakter khusus di dalam string value.
func fixJSON(raw string) string {
	pathVal := extractField(raw, `"path"`)
	contentVal := extractField(raw, `"content"`)

	if pathVal == "" {
		return raw
	}

	// Bangun ulang JSON yang valid dengan json.Marshal (auto-escape)
	escapedPath, _ := json.Marshal(pathVal)
	escapedContent, _ := json.Marshal(contentVal)
	return fmt.Sprintf(`{"path": %s, "content": %s}`, string(escapedPath), string(escapedContent))
}

// extractField mengekstrak nilai string dari field JSON seperti "path" atau "content".
// Juga handle nilai non-string seperti angka (size, count).
func extractField(raw, field string) string {
	idx := strings.Index(raw, field)
	if idx < 0 {
		return ""
	}

	// Cari ':' setelah field name
	colonIdx := strings.Index(raw[idx:], ":")
	if colonIdx < 0 {
		return ""
	}
	rest := raw[idx+colonIdx+1:]

	// Skip whitespace
	rest = strings.TrimSpace(rest)

	// Handle quoted string values
	if strings.HasPrefix(rest, `"`) {
		rest = rest[1:] // lewati quote pembuka

		// Cari quote penutup (handle escaped quotes dengan \\)
		end := -1
		for i := 0; i < len(rest); i++ {
			if rest[i] == '\\' {
				i++ // skip escaped char
				continue
			}
			if rest[i] == '"' {
				end = i
				break
			}
		}

		if end < 0 {
			return rest
		}
		return rest[:end]
	}

	// Handle numeric values (size, count, dll)
	end := strings.IndexAny(rest, ",}\n\r ")
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// extractJSONStringField extracts a string field value from a JSON object,
// properly handling JSON escape sequences (\\n, \\t, \\\", etc.).
// This is more robust than extractField for multi-line string values like file content.
func extractJSONStringField(raw, field string) string {
	// Parse the raw JSON into a generic map to extract the field
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return ""
	}
	if val, ok := obj[field]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// truncateStr memotong string ke panjang maksimal.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// cleanToolName strips markdown formatting from a tool name (e.g. "**read_file**" → "read_file").
var knownTools = map[string]bool{
	"read_file": true, "read_file_lines": true, "write_file": true,
	"edit_file": true, "create_directory": true, "list_files": true,
	"find_files": true, "search_text": true, "exec": true, "browse": true,
}

func isKnownTool(name string) bool {
	return knownTools[name]
}

func cleanToolName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "**")
	name = strings.TrimSuffix(name, "**")
	name = strings.TrimPrefix(name, "__")
	name = strings.TrimSuffix(name, "__")
	name = strings.TrimPrefix(name, "*")
	name = strings.TrimSuffix(name, "*")
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	name = strings.TrimPrefix(name, "`")
	name = strings.TrimSuffix(name, "`")
	return strings.TrimSpace(name)
}

func parseReActResponse(text string) (reActTool, bool) {
	tool, hasTool := parseReActSingle(text)
	hasFinal := strings.Contains(text, "Final Answer:")

	// If both tool call and Final Answer present, prioritize tool call.
	// The Final Answer will be captured after tool execution.
	if hasTool && hasFinal {
		return tool, false
	}

	// Final Answer only — extract content for display
	if hasFinal {
		if m := finalRe.FindStringSubmatch(text); len(m) > 1 {
			tool.output = strings.TrimSpace(m[1])
		}
		return tool, true
	}

	if hasTool {
		return tool, false
	}
	return reActTool{}, false
}

// parseReActSingle extracts the FIRST Action: tool call from the text.
func parseReActSingle(text string) (reActTool, bool) {
	// Periksa Action: format dulu — cari tool call SEBELUM Final Answer
	// agar tool call tidak terlewat jika LLM menyertakan keduanya dalam satu respons.
	if m := actionRe.FindStringSubmatch(text); len(m) > 1 {
		actionStr := strings.TrimSpace(m[1])

		parenIdx := strings.Index(actionStr, "(")
		if parenIdx > 0 {
			name := cleanToolName(actionStr[:parenIdx])
			// Validate tool name — must match a known tool
			if !isKnownTool(name) {
				return reActTool{}, false
			}
			inputStr := actionStr[parenIdx+1:]

			// Gunakan bracket counting untuk handle JSON dengan nested {} di dalam string
			jsonStr := extractBalancedJSON(inputStr)
			if jsonStr != "" {
				return reActTool{
					name:  name,
					input: strings.TrimSpace(jsonStr),
				}, false
			}
		}
		return reActTool{}, false
	}

	// Fallback: toolCallRe — hanya untuk format tanpa "Action:" prefix
	if m := toolCallRe.FindStringSubmatch(text); len(m) > 2 {
		return reActTool{
			name:  cleanToolName(m[1]),
			input: strings.TrimSpace(m[2]),
		}, false
	}

	if m := inputRe.FindStringSubmatch(text); len(m) > 1 {
		return reActTool{input: strings.TrimSpace(m[1])}, false
	}

	return reActTool{}, false
}

// parseReActAll extracts ALL Action: tool calls from the text.
// Handles LLM outputs that concatenate multiple Action: calls inline.
func parseReActAll(text string) []reActTool {
	var tools []reActTool

	// Cari semua kemunculan "Action:" diikuti tool_name(
	// Gunakan bracket counting untuk extract JSON dengan benar (handle nested {})
	remaining := text
	safety := 0
	for safety < 500 {
		safety++
		idx := strings.Index(remaining, "Action:")
		if idx < 0 {
			break
		}
		remaining = remaining[idx+7:] // skip "Action:"
		remaining = strings.TrimSpace(remaining)

		// Extract tool name (chars before first '(')
		parenIdx := strings.Index(remaining, "(")
		if parenIdx <= 0 {
			// No valid tool call here — skip 1 char to avoid infinite loop
			if len(remaining) > 1 {
				remaining = remaining[1:]
			} else {
				break
			}
			continue
		}
		name := cleanToolName(remaining[:parenIdx])

		// Extract balanced JSON starting from parenIdx
		jsonStr := extractBalancedJSON(remaining[parenIdx+1:])
		if jsonStr == "" {
			// JSON parsing failed — skip past this "(" and continue
			if parenIdx+1 < len(remaining) {
				remaining = remaining[parenIdx+1:]
			} else {
				break
			}
			continue
		}

		tools = append(tools, reActTool{
			name:  name,
			input: strings.TrimSpace(jsonStr),
		})

		// Advance past the parsed tool call: skip name + ( + json + )
		advance := parenIdx + 1 + len(jsonStr) + 1
		if advance < len(remaining) {
			remaining = remaining[advance:]
		} else {
			break
		}
	}

	// If no matches found, fall back to single parse
	if len(tools) == 0 {
		tool, _ := parseReActSingle(text)
		if tool.name != "" {
			tools = append(tools, tool)
		}
	}

	return tools
}

// executeAllToolCalls executes multiple tool calls from a single LLM response.
// Read-only tools are executed concurrently; write tools are executed sequentially.
func executeAllToolCalls(m *model, state chatLoopState, resp *core.Response, toolCalls []reActTool) (tea.Cmd, bool) {
	// Check if any tool needs permission (non-auto-trusted)
	for _, call := range toolCalls {
		if call.name == "" {
			continue
		}
		if needsPermission(call.name) && !isToolAutoTrustedMode(m.mode, m.trustWrite, call.name) {
			// For non-auto-trusted tools, pause and ask for confirmation (first one only)
			m.pendingTool = call
			m.pendingState = state
			m.pendingToolResp = resp.Content
			m.state = stateConfirming
			m.confirmChoice = 0
			m.statusMsg = ""
			m.toolActivity = fmt.Sprintf("Konfirmasi: %s", call.name)
			m.recalcLayout()
			m.rebuildViewport()
			return m.textarea.Focus(), true
		}
	}

	// All tools are auto-trusted — execute concurrently
	type toolResult struct {
		index   int
		name    string
		input   string
		output  string
		isError bool
	}
	results := make([]toolResult, len(toolCalls))
	var wg sync.WaitGroup

	for i, call := range toolCalls {
		if call.name == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, c reActTool) {
			defer wg.Done()
			output := executeToolCall(state.activeTools, c)
			results[idx] = toolResult{
				index:   idx,
				name:    c.name,
				input:   c.input,
				output:  output,
				isError: isToolOutputError(output),
			}
		}(i, call)
	}
	wg.Wait()

	// Collect results in order, update UI
	var allObservations []string
	for _, r := range results {
		if r.name == "" {
			continue
		}
		display := formatToolDisplay(r.name, r.input, r.output)
		role := "tool"
		if r.isError {
			role = "tool-error"
		}
		m.messages = append(m.messages, chatMessage{
			role:     role,
			content:  display,
			toolName: r.name,
			tokens:   0,
		})
		state.toolCalls = append(state.toolCalls, toolCallRecord{
			toolName: r.name,
			input:    r.input,
			output:   r.output,
			isError:  r.isError,
		})
		allObservations = append(allObservations, fmt.Sprintf(
			"Observation (hasil dari tool %s): %s", r.name, r.output,
		))
	}
	m.toolActivity = fmt.Sprintf("%d tools selesai", len(allObservations))

	// Feed all observations back to LLM
	state.messages = append(state.messages, core.Message{Role: "assistant", Content: resp.Content})
	state.messages = append(state.messages, core.Message{Role: "user", Content: strings.Join(allObservations, "\n")})
	state.iteration++

	// Max iteration check
	maxIterations := 8
	if m.mode == modeAuto {
		maxIterations = 16
	}
	switch m.effort {
	case effortLow:
		maxIterations = 4
	case effortHigh:
		maxIterations = 24
	}
	if state.iteration >= maxIterations {
		m.state = stateReady
		m.totalTokens += state.totalTokens
		m.toolActivity = "✓ Selesai"
		m.rebuildViewport()
		m.statusMsg = ""
		return m.textarea.Focus(), true
	}

	m.rebuildViewport()
	return tea.Batch(
		m.textarea.Focus(),
		continueChatLoop(m.ai, m.ctx, state),
	), false
}

func executeToolCall(toolList []tools.Tool, call reActTool) string {
	var tool tools.Tool
	for _, t := range toolList {
		if strings.EqualFold(t.Name(), call.name) {
			tool = t
			break
		}
	}
	if tool == nil {
		return fmt.Sprintf("Error: tool %q tidak dikenal. Tools tersedia: write_file, read_file, list_files, create_directory, browse, find_files, search_text, read_file_lines", call.name)
	}

	input := json.RawMessage(call.input)
	if !json.Valid(input) {
		// Coba perbaiki JSON dengan melakukan escape pada konten string
		fixed := fixJSON(call.input)
		if json.Valid(json.RawMessage(fixed)) {
			input = json.RawMessage(fixed)
		} else {
			// Kembalikan error yang jelas agar LLM bisa memperbaiki formatnya
			return fmt.Sprintf("Error: Format JSON tidak valid untuk tool %s. Input: %s. Pastikan JSON valid — gunakan escape yang benar untuk newline dan quotes di dalam string.", call.name, truncateStr(call.input, 200))
		}
	}

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		return fmt.Sprintf("Error eksekusi %s: %v", call.name, err)
	}

	return string(output)
}

func buildToolSystemPrompt(toolList []tools.Tool, mode chatMode, effort effortLevel) string {
	var b strings.Builder

	switch mode {
	case modePlan:
		b.WriteString("Kamu adalah AI asisten dalam MODE PERENCANAAN. ")
		b.WriteString("Tugasmu adalah menganalisis, membaca file yang relevan, ")
		b.WriteString("dan membuat rencana detail SEBELUM implementasi.\n\n")
		b.WriteString("ATURAN PENTING:\n")
		b.WriteString("- BACA file yang relevan dengan read_file, list_files, find_files, atau search_text\n")
		b.WriteString("- ANALISIS kode yang ada\n")
		b.WriteString("- BUAT rencana langkah-demi-langkah yang terstruktur\n")
		b.WriteString("- JANGAN menulis atau mengubah file apapun\n")
		b.WriteString("- JANGAN mengklaim sudah menulis file — kamu tidak bisa write di mode ini\n")
		b.WriteString("- Akhiri dengan Final Answer: berisi rencana yang jelas\n\n")

	case modeEdit:
		b.WriteString("Kamu adalah AI asisten dalam MODE EDIT.\n")
		b.WriteString("Tugasmu adalah mengimplementasikan perubahan secara LANGSUNG.\n")
		b.WriteString("Jangan bertanya — langsung kerjakan.\n\n")
		b.WriteString("!!! INSTRUKSI WAJIB – BACA DENGAN SEKSAMA !!!\n")
		b.WriteString("Kamu HARUS menggunakan Action: format untuk SETIAP tool call.\n")
		b.WriteString("JANGAN PERNAH memberikan Final Answer tanpa memanggil write_file.\n")
		b.WriteString("\nWAJIB: Buat PLAN checklist sebelum eksekusi!\n")
		b.WriteString("\nLangkah:\n")
		b.WriteString("1. BUAT PLAN GENERAL: kelompokkan file terkait jadi 1 task besar\n")
		b.WriteString("2. KERJAKAN: SATU task mencakup BANYAK write_file()\n")
		b.WriteString("3. UPDATE: centang [x] task SETELAH semua file di dalamnya selesai\n")
		b.WriteString("4. REVIEW: exec() untuk build/check\n")
		b.WriteString("5. Final Answer hanya jika SEMUA task selesai\n\n")
		b.WriteString("\nContoh task GENERAL (bukan per file):\n")
		b.WriteString("  - [ ] Setup struktur project (package.json, vite.config, index.html)\n")
		b.WriteString("    Action: write_file({\"path\": \"package.json\", \"content\": \"...\"})\n")
		b.WriteString("    Action: write_file({\"path\": \"vite.config.js\", \"content\": \"...\"})\n")
		b.WriteString("    Action: write_file({\"path\": \"index.html\", \"content\": \"...\"})\n")
		b.WriteString("  - [x] Setup struktur project\n")
		b.WriteString("  - [ ] Implementasi komponen UI (Navbar, Hero, Footer)\n")
		b.WriteString("    Action: write_file({\"path\": \"src/Navbar.vue\", \"content\": \"...\"})\n")
		b.WriteString("    Action: write_file({\"path\": \"src/Hero.vue\", \"content\": \"...\"})\n")
		b.WriteString("  - [x] Implementasi komponen UI\n")
		b.WriteString("  Action: exec({\"command\": \"npm run build\"})\n")
		b.WriteString("  Final Answer: Semua task selesai.\n\n")
		b.WriteString("ATURAN:\n")
		b.WriteString("- 1 task GENERAL = banyak file terkait\n")
		b.WriteString("- Centang [x] SETELAH semua file dalam task selesai\n")
		b.WriteString("- Jangan buat task per-file, buat task per-fitur\n")
		b.WriteString("- Final Answer hanya jika SEMUA task tercentang\n\n")
	case modeAuto:
		b.WriteString("Kamu adalah AI asisten dalam MODE OTONOM.\n")
		b.WriteString("Kerjakan tugas sampai SELESAI tanpa perlu konfirmasi user.\n")
		b.WriteString("!!! INSTRUKSI WAJIB – BACA DENGAN SEKSAMA !!!\n")
		b.WriteString("Kamu HARUS menggunakan Action: format untuk SETIAP tool call.\n")
		b.WriteString("JANGAN PERNAH memberikan Final Answer tanpa memanggil write_file.\n")
		b.WriteString("\nWAJIB: Buat PLAN checklist sebelum eksekusi!\n")
		b.WriteString("\nLangkah:\n")
		b.WriteString("1. BUAT PLAN GENERAL: kelompokkan file terkait jadi 1 task\n")
		b.WriteString("2. KERJAKAN: SATU task = BANYAK write_file()\n")
		b.WriteString("3. CENTANG [x] task SETELAH semua file di dalamnya selesai\n")
		b.WriteString("4. REVIEW: exec() untuk build/check\n")
		b.WriteString("5. Final Answer hanya jika SEMUA task selesai\n\n")
		b.WriteString("\nContoh task GENERAL:\n")
		b.WriteString("  - [ ] Setup project (package.json, vite.config, index.html)\n")
		b.WriteString("    Action: write_file({\"path\": \"package.json\", \"content\": \"...\"})\n")
		b.WriteString("    Action: write_file({\"path\": \"vite.config.js\", \"content\": \"...\"})\n")
		b.WriteString("    Action: write_file({\"path\": \"index.html\", \"content\": \"...\"})\n")
		b.WriteString("  - [x] Setup project\n")
		b.WriteString("  - [ ] Buat komponen utama (Navbar.vue, Hero.vue, Footer.vue)\n")
		b.WriteString("    Action: write_file({\"path\": \"src/Navbar.vue\", \"content\": \"...\"})\n")
		b.WriteString("    Action: write_file({\"path\": \"src/Hero.vue\", \"content\": \"...\"})\n")
		b.WriteString("  - [x] Buat komponen utama\n")
		b.WriteString("  Action: exec({\"command\": \"npm run build\"})\n")
		b.WriteString("  Final Answer: Semua task selesai.\n\n")
		b.WriteString("ATURAN:\n")
		b.WriteString("- 1 task GENERAL = banyak file dalam 1 fitur\n")
		b.WriteString("- Centang [x] SETELAH semua file dalam task selesai\n")
		b.WriteString("- Jangan buat task per-file\n")
		b.WriteString("- Final Answer hanya jika SEMUA task tercentang\n\n")
	default:
		b.WriteString("Kamu adalah AI asisten dalam MODE CHAT (percakapan normal). ")
		b.WriteString("Bantu jawab pertanyaan, analisis kode, dan diskusi.\n\n")
		b.WriteString("ATURAN PENTING:\n")
		b.WriteString("- Kamu HANYA bisa membaca file (read_file, list_files, find_files, search_text, read_file_lines)\n")
		b.WriteString("- Kamu TIDAK BISA menulis/mengubah file di mode ini\n")
		b.WriteString("- Jika user minta mengubah/membuat file, SARANKAN mereka switch ke mode /edit atau /auto\n")
		b.WriteString("- Contoh: \"Untuk menulis file, silakan switch ke mode /edit atau /auto dengan Shift+Tab\"\n")
		b.WriteString("- JANGAN coba-coba pakai write_file — itu tidak akan berfungsi\n\n")
	}

	// Inject effort instructions
	switch effort {
	case effortLow:
		b.WriteString("INSTRUKSI EFFORT (LOW):\n")
		b.WriteString("- Jawablah dengan SINGKAT, CEPAT, dan langsung ke inti permasalahan.\n")
		b.WriteString("- Tidak perlu penjelasan panjang lebar atau elaborasi berlebihan.\n\n")
	case effortHigh:
		b.WriteString("INSTRUKSI EFFORT (HIGH):\n")
		b.WriteString("- Berpikirlah secara MENDALAM. Analisis masalah dari berbagai sudut pandang.\n")
		b.WriteString("- Jika menulis kode, pertimbangkan edge cases, performa, dan best practices.\n")
		b.WriteString("- Berikan penjelasan yang sangat komprehensif, detail, dan menyeluruh.\n")
		b.WriteString("- Eksplorasi berbagai alternatif solusi sebelum memutuskan yang terbaik.\n\n")
	}

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

	// Mode Edit/Auto: ingatkan checklist di akhir (paling dekat dengan respon LLM)
	if mode == modeEdit || mode == modeAuto {
		b.WriteString("\nSEBELUM MEMULAI: Buat PLAN checklist dengan format:\n")
		b.WriteString("  - [ ] Deskripsi task general (mencakup banyak file)\n")
		b.WriteString("  - [ ] Task berikutnya\n")
		b.WriteString("Centang [x] SETELAH semua file dalam task selesai ditulis.\n")
		b.WriteString("Jangan memberikan Final Answer sebelum SEMUA task tercentang.\n\n")
	}

	b.WriteString("PENTING: Selalu gunakan Bahasa Indonesia untuk Final Answer.")
	return b.String()
}

// isToolAutoTrusted returns true jika tool bisa langsung dieksekusi tanpa konfirmasi.
func isToolAutoTrusted(mode chatMode, trustWrite bool, toolName string) bool {
	// exec selalu membutuhkan konfirmasi manual dari user
	if toolName == "exec" {
		return false
	}
	// Jika folder sudah dipercaya (trustWrite == true), skip konfirmasi untuk semua operasi file
	if toolName == "write_file" || toolName == "edit_file" || toolName == "create_directory" || toolName == "read_file" {
		return trustWrite
	}
	return false
}

// isToolAutoTrustedMode returns true jika mode saat ini mengizinkan eksekusi tool langsung saat streaming.
func isToolAutoTrustedMode(mode chatMode, trustWrite bool, toolName string) bool {
	// exec selalu membutuhkan konfirmasi manual dari user
	if toolName == "exec" {
		return false
	}
	// Jika folder sudah dipercaya, skip konfirmasi untuk semua operasi file
	if toolName == "write_file" || toolName == "edit_file" || toolName == "create_directory" || toolName == "read_file" {
		return trustWrite
	}
	return false
}

// parseTaskList extracts checklist items from LLM streaming content.
// Parses lines like "- [ ] desc" (pending) and "- [x] desc" (completed).
// Deduplicates: if same desc appears as [ ] then [x], keeps the LATEST status.
// Validates: a task first seen as [x] (never seen as [ ]) stays pending.
func parseTaskList(content string, oldTasks []taskItem) []taskItem {
	if content == "" {
		return oldTasks
	}

	// Scan all lines and collect latest status per description
	type foundItem struct {
		desc   string
		status string
	}
	var foundList []foundItem
	seen := make(map[string]int) // desc → index

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ] ") {
			desc := strings.TrimPrefix(trimmed, "- [ ] ")
			if idx, exists := seen[desc]; exists {
				foundList[idx].status = "pending"
			} else {
				seen[desc] = len(foundList)
				foundList = append(foundList, foundItem{desc: desc, status: "pending"})
			}
		} else if strings.HasPrefix(trimmed, "- [x] ") {
			desc := strings.TrimPrefix(trimmed, "- [x] ")
			if idx, exists := seen[desc]; exists {
				foundList[idx].status = "completed"
			} else {
				seen[desc] = len(foundList)
				foundList = append(foundList, foundItem{desc: desc, status: "completed"})
			}
		}
	}

	if len(foundList) == 0 {
		return oldTasks
	}

	// Build oldMap for status preservation
	oldMap := make(map[string]string)
	for _, t := range oldTasks {
		oldMap[t.desc] = t.status
	}

	// Merge: pertahankan urutan foundList, status dari oldMap
	var result []taskItem
	for _, f := range foundList {
		oldStatus, wasInOld := oldMap[f.desc]

		if wasInOld {
			// Known task: upgrade status but never downgrade
			if f.status == "completed" || oldStatus == "completed" {
				result = append(result, taskItem{desc: f.desc, status: "completed"})
			} else {
				result = append(result, taskItem{desc: f.desc, status: oldStatus})
			}
		} else {
			// New task: if it first appears as [x], treat as pending
			// (LLM might be summarizing, not reporting real progress)
			result = append(result, taskItem{desc: f.desc, status: "pending"})
		}
	}

	// Tandai task pending pertama sebagai "in_progress"
	markedInProgress := false
	for i := range result {
		if result[i].status == "pending" && !markedInProgress {
			result[i].status = "in_progress"
			markedInProgress = true
		}
	}

	return result
}

// readProjectConfigs membaca file konfigurasi project yang umum.
func readProjectConfigs(allowedDir string) string {
	// Daftar config file yang akan diperiksa (urut prioritas)
	configFiles := []struct {
		path  string
		label string
	}{
		{"go.mod", "Go Module"}, {"package.json", "Node.js"}, {"Cargo.toml", "Rust"}, {"pyproject.toml", "Python (pyproject)"},
		{"requirements.txt", "Python (pip)"}, {"Gemfile", "Ruby"}, {"composer.json", "PHP"}, {"pom.xml", "Maven"}, {"build.gradle", "Gradle"},
		{"Makefile", "Makefile"}, {"Dockerfile", "Docker"}, {"docker-compose.yml", "Docker Compose"}, {"tsconfig.json", "TypeScript"},
	}

	absDir, err := filepath.Abs(allowedDir)
	if err != nil {
		return ""
	}

	var result strings.Builder
	for _, cf := range configFiles {
		fullPath := filepath.Join(absDir, cf.path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue // file tidak ada
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		// Batasi panjang konten (max 30 baris)
		lines := strings.Split(content, "\n")
		if len(lines) > 30 {
			lines = lines[:30]
			lines = append(lines, "... (+ lebih banyak)")
		}
		result.WriteString(fmt.Sprintf("=== %s (%s) ===\n", cf.label, cf.path))
		result.WriteString(strings.Join(lines, "\n"))
		result.WriteString("\n\n")
	}

	// Auto-baca SEMUA file markdown (.md) di root project untuk konteks
	matches, _ := filepath.Glob(filepath.Join(absDir, "*.md"))
	if len(matches) > 10 {
		matches = matches[:10] // batasi maks 10 file markdown
	}
	for _, fullPath := range matches {
		name := filepath.Base(fullPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		// Batasi panjang konten (max 50 baris untuk markdown)
		lines := strings.Split(content, "\n")
		if len(lines) > 50 {
			lines = lines[:50]
			lines = append(lines, "... (+ lebih banyak)")
		}
		result.WriteString(fmt.Sprintf("=== PROJECT MARKDOWN (%s) ===\n", name))
		result.WriteString(strings.Join(lines, "\n"))
		result.WriteString("\n\n")
	}

	return strings.TrimRight(result.String(), "\n")
}
