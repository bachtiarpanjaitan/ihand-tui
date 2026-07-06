package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bachtiarpanjaitan/ihandai-go"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/memory"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/tools"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// ReAct regex patterns
// ---------------------------------------------------------------------------

var (
	actionRe   = regexp.MustCompile(`Action:\s*(.+)`)
	inputRe    = regexp.MustCompile(`Action Input:\s*(.+)`)
	finalRe    = regexp.MustCompile(`Final Answer:\s*(.+)`)
	toolCallRe = regexp.MustCompile(`(\w+)\((\{.*?\})\)`)
)

// ---------------------------------------------------------------------------
// Step-based chat loop (replaces makeToolChatCall)
// ---------------------------------------------------------------------------

// startChatLoop initializes the ReAct loop and fires the first LLM call.
func startChatLoop(ai *ihandai.Client, ctx context.Context, session, input string, store memory.ConversationStore, toolList []tools.Tool, mode chatMode, effort effortLevel) tea.Cmd {
	return func() tea.Msg {
		llmProvider := ai.LLM()
		if llmProvider == nil {
			return llmErrorMsg{err: fmt.Errorf("LLM provider tidak tersedia")}
		}

		// Save user message to memory for conversation context
		store.Append(ctx, session, core.Message{Role: "user", Content: input})

		history, _ := store.History(ctx, session)

		activeTools := toolList
		if mode == modePlan || mode == modeChat {
			activeTools = nil
			for _, t := range toolList {
				if t.Name() == "read_file" || t.Name() == "list_files" || t.Name() == "browse" {
					activeTools = append(activeTools, t)
				}
			}
		}

		systemPrompt := buildToolSystemPrompt(activeTools, mode, effort)

		messages := []core.Message{
			{Role: "system", Content: systemPrompt},
		}
		messages = append(messages, history...)

		resp, err := llmProvider.Chat(ctx, messages)

		return chatStepResultMsg{
			state: chatLoopState{
				session:     session,
				messages:    messages,
				activeTools: activeTools,
				iteration:   0,
				toolCalls:   nil,
				totalTokens: countTokens(input),
				startTime:   time.Now(),
			},
			response: resp,
			err:      err,
		}
	}
}

// continueChatLoop calls the LLM with updated messages and returns the result.
func continueChatLoop(ai *ihandai.Client, ctx context.Context, state chatLoopState) tea.Cmd {
	return func() tea.Msg {
		llmProvider := ai.LLM()
		if llmProvider == nil {
			return llmErrorMsg{err: fmt.Errorf("LLM provider tidak tersedia")}
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
		m.state = stateReady
		m.statusMsg = ""
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: "❌ LLM Error: " + msg.err.Error(),
		})
		m.rebuildViewport()
		return m.textarea.Focus(), true
	}

	state := msg.state
	resp := msg.response
	if resp == nil {
		m.state = stateReady
		m.statusMsg = ""
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: "❌ LLM Error: respons kosong dari API",
		})
		m.rebuildViewport()
		return m.textarea.Focus(), true
	}
	if resp.Content == "" {
		m.state = stateReady
		m.statusMsg = ""
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: "❌ LLM Error: respons API kosong — cek API key atau koneksi",
		})
		m.rebuildViewport()
		return m.textarea.Focus(), true
	}
	state.totalTokens += countTokens(resp.Content)

	toolCall, isFinal := parseReActResponse(resp.Content)

	// --- Final answer ---
	if isFinal {
		m.memory.Append(m.ctx, state.session, core.Message{
			Role: "assistant", Content: resp.Content,
		})
		m.messages = append(m.messages, chatMessage{
			role:    "assistant",
			content: toolCall.output,
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
			m.pendingTool = toolCall
			m.pendingState = state
			m.pendingToolResp = resp.Content
			m.state = stateConfirming
			m.confirmChoice = 0 // default to Allow
			m.statusMsg = ""
			m.toolActivity = fmt.Sprintf("🔍 Konfirmasi: %s", toolCall.name)
			m.messages = append(m.messages, chatMessage{
				role:    "confirm",
				content: fmt.Sprintf(
					"%s|%s|%s",
					toolCall.name,
					extractField(toolCall.input, "\"path\""),
					extractField(toolCall.input, "\"content\""),
				),
			})
			m.recalcLayout()
			m.rebuildViewport()
			return m.textarea.Focus(), true
		}

		toolOutput := executeToolCall(state.activeTools, toolCall)
		isToolError := strings.HasPrefix(toolOutput, "Error")

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
			role:    role,
			content: display,
			tokens:  0,
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
	m.memory.Append(m.ctx, state.session, core.Message{
		Role: "assistant", Content: resp.Content,
	})
	m.messages = append(m.messages, chatMessage{
		role:    "assistant",
		content: resp.Content,
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

// needsPermission returns true jika tool memerlukan konfirmasi user sebelum dieksekusi.
func needsPermission(name string) bool {
	switch name {
	case "write_file", "read_file":
		return true
	default:
		return false
	}
}

// rebuildViewport is a helper to re-render the viewport after state changes.
func (m *model) rebuildViewport() {
	content := m.buildConversation()
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

// ---------------------------------------------------------------------------
// Tool display formatting
// ---------------------------------------------------------------------------

// formatToolDisplay returns a user-friendly display string for a tool call result.
// Uses simple string extraction to avoid issues with imperfect JSON from tool output.
func formatToolDisplay(toolName, input, output string) string {
	// Extract path from any JSON-like output
	path := extractField(output, `"path"`)
	if path == "" {
		path = extractField(output, `"path\":`)
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
		case "list_files":
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
	case "read_file":
		if path != "" {
			// Show content preview from output
			content := extractField(output, `"content"`)
			preview := ""
			if content != "" {
				if len(content) > 500 {
					preview = "\n" + content[:500] + "..."
				} else {
					preview = "\n" + content
				}
			}
			return fmt.Sprintf("%s — Dibaca (%d bytes)%s", path, size, preview)
		}
	case "write_file":
		success := strings.Contains(output, `"success": true`) || strings.Contains(output, `"success":true`)
		if path != "" {
			preview := ""
			// Extract diff from tool output
			diffStr := extractField(output, `"diff"`)
			var previewDiff string
			if diffStr != "" {
				// Decode JSON string, lalu split
				var diffText string
				if err := json.Unmarshal([]byte(`"` + diffStr + `"`), &diffText); err == nil && diffText != "" {
					diffLines := strings.Split(diffText, "\n")
					var b strings.Builder
					for _, line := range diffLines {
						if len(line) > 0 {
							switch line[0] {
							case '+':
								b.WriteString("  \033[32m" + line + "\033[0m\n")
							case '-':
								b.WriteString("  \033[31m" + line + "\033[0m\n")
							default:
								b.WriteString("  " + line + "\n")
							}
						}
					}
					previewDiff = b.String()
				}
			}
			if success || strings.Contains(output, "berhasil") {
				if previewDiff != "" {
					return fmt.Sprintf("%s \u2014 %d baris", path, size) + previewDiff
				}
				// Fallback: content preview (jika diff kosong)
				c := extractField(input, `"content"`)
				preview := ""
				if c != "" {
					if len(c) > 500 {
						preview = "\n" + c[:500] + "..."
					} else {
						preview = "\n" + c
					}
				}
				return fmt.Sprintf("%s \u2014 Ditulis (%d bytes)%s", path, size, preview)
			}
			msg := extractField(output, `"message"`)
			if msg != "" {
				return fmt.Sprintf("%s \u2014 %s%s", path, msg, preview)
			}
			return fmt.Sprintf("%s \u2014 Selesai%s", path, preview)
		}
	case "list_files":
		if path != "" {
			return fmt.Sprintf("%s — %d item", path, count)
		}
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

// truncateStr memotong string ke panjang maksimal.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func parseReActResponse(text string) (reActTool, bool) {
	if m := finalRe.FindStringSubmatch(text); len(m) > 1 {
		return reActTool{output: strings.TrimSpace(m[1])}, true
	}

	// Periksa Action: format dulu — lebih robust untuk JSON dengan nested braces
	if m := actionRe.FindStringSubmatch(text); len(m) > 1 {
		actionStr := strings.TrimSpace(m[1])

		parenIdx := strings.Index(actionStr, "(")
		if parenIdx > 0 {
			name := strings.TrimSpace(actionStr[:parenIdx])
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
		return reActTool{name: actionStr, input: "{}"}, false
	}

	// Fallback: toolCallRe — hanya untuk format tanpa "Action:" prefix
	if m := toolCallRe.FindStringSubmatch(text); len(m) > 2 {
		return reActTool{
			name:  strings.TrimSpace(m[1]),
			input: strings.TrimSpace(m[2]),
		}, false
	}

	if m := inputRe.FindStringSubmatch(text); len(m) > 1 {
		return reActTool{input: strings.TrimSpace(m[1])}, false
	}

	return reActTool{}, false
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
		return fmt.Sprintf("Error: tool %q tidak dikenal. Tools tersedia: write_file, read_file, list_files", call.name)
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
		b.WriteString("- BACA file yang relevan dengan read_file atau list_files\n")
		b.WriteString("- ANALISIS kode yang ada\n")
		b.WriteString("- BUAT rencana langkah-demi-langkah yang terstruktur\n")
		b.WriteString("- JANGAN menulis atau mengubah file apapun\n")
		b.WriteString("- JANGAN mengklaim sudah menulis file — kamu tidak bisa write di mode ini\n")
		b.WriteString("- Akhiri dengan Final Answer: berisi rencana yang jelas\n\n")

	case modeEdit:
		b.WriteString("Kamu adalah AI asisten dalam MODE EDIT. ")
		b.WriteString("Tugasmu adalah mengimplementasikan perubahan secara LANGSUNG. ")
		b.WriteString("Jangan bertanya — langsung kerjakan.\n\n")
		b.WriteString("ATURAN KRITIS:\n")
		b.WriteString("- WAJIB pakai write_file untuk SETIAP perubahan file\n")
		b.WriteString("- JANGAN PERNAH mengklaim \"file sudah diubah\" tanpa Action: write_file\n")
		b.WriteString("- BACA file dulu dengan read_file sebelum mengubahnya\n")
		b.WriteString("- Format: Action: write_file({\"path\": \"...\", \"content\": \"...\"})\n")
		b.WriteString("- Akhiri dengan Final Answer: konfirmasi apa yang SUDAH diubah via tool\n\n")

	case modeAuto:
		b.WriteString("Kamu adalah AI asisten dalam MODE OTONOM. ")
		b.WriteString("Kerjakan tugas sampai SELESAI tanpa perlu konfirmasi user. ")
		b.WriteString("Rencanakan sendiri langkah-langkahnya dan eksekusi berurutan.\n\n")
		b.WriteString("ATURAN KRITIS:\n")
		b.WriteString("- WAJIB gunakan tools (read_file, write_file, list_files) untuk setiap aksi\n")
		b.WriteString("- JANGAN PERNAH mengklaim \"file sudah dibuat/diubah\" tanpa Action: write_file\n")
		b.WriteString("- Gunakan tools secara otonom dan berurutan\n")
		b.WriteString("- Eksekusi multi-step tanpa bertanya ke user\n")
		b.WriteString("- Jika gagal di satu langkah, coba alternatif lain\n")
		b.WriteString("- Akhiri dengan Final Answer: ringkasan apa yang SUDAH dikerjakan via tools\n\n")

	default:
		b.WriteString("Kamu adalah AI asisten dalam MODE CHAT (percakapan normal). ")
		b.WriteString("Bantu jawab pertanyaan, analisis kode, dan diskusi.\n\n")
		b.WriteString("ATURAN PENTING:\n")
		b.WriteString("- Kamu HANYA bisa membaca file (read_file, list_files)\n")
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

	b.WriteString("PENTING: Selalu gunakan Bahasa Indonesia untuk Final Answer.")
	return b.String()
}
