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
func startChatLoop(ai *ihandai.Client, ctx context.Context, session, input string, store memory.ConversationStore, toolList []tools.Tool, mode chatMode) tea.Cmd {
	return func() tea.Msg {
		llmProvider := ai.LLM()
		if llmProvider == nil {
			return llmErrorMsg{err: fmt.Errorf("LLM provider tidak tersedia")}
		}

		// Save user message to memory for conversation context
		store.Append(ctx, session, core.Message{Role: "user", Content: input})

		history, _ := store.History(ctx, session)

		activeTools := toolList
		if mode == modePlan {
			activeTools = nil
			for _, t := range toolList {
				if t.Name() == "read_file" || t.Name() == "list_files" {
					activeTools = append(activeTools, t)
				}
			}
		}

		systemPrompt := buildToolSystemPrompt(activeTools, mode)

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
			content: msg.err.Error(),
		})
		m.rebuildViewport()
		return m.textarea.Focus(), true
	}

	state := msg.state
	resp := msg.response
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
		m.rebuildViewport()
		m.statusMsg = ""
return m.textarea.Focus(), true
	}

	maxIterations := 8
	if m.mode == modeAuto {
		maxIterations = 16
	}

	// --- Tool call ---
	if toolCall.name != "" {
		m.statusMsg = fmt.Sprintf("Menjalankan %s...", toolCall.name)
		toolOutput := executeToolCall(state.activeTools, toolCall)
		isToolError := strings.HasPrefix(toolOutput, "Error")

		// Show tool call immediately in UI with friendly formatting
		display := formatToolDisplay(toolCall.name, toolCall.input, toolOutput)
		role := "tool"
		if isToolError {
			role = "tool-error"
		}
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
			finalContent := "⚠ Agent mencapai batas maksimum iterasi."
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

	m.rebuildViewport()
	m.statusMsg = ""
return m.textarea.Focus(), true
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
// For read_file: shows only the file path and size, not the full content.
// For other tools: shows a clean summary or truncated raw output as fallback.
func formatToolDisplay(toolName, input, output string) string {
	// Try to parse as JSON for clean display
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		// Not JSON — show truncated raw output
		display := output
		if len(display) > 300 {
			display = display[:300] + "..."
		}
		return fmt.Sprintf("%s(%s) → %s", toolName, input, display)
	}

	switch toolName {
	case "read_file":
		path, _ := parsed["path"].(string)
		size := 0
		if s, ok := parsed["size"].(float64); ok {
			size = int(s)
		}
		if errMsg, ok := parsed["error"].(string); ok {
			return fmt.Sprintf("%s(%s) → ✗ %s", toolName, input, errMsg)
		}
		return fmt.Sprintf("%s(%s) → ✓ Dibaca: %s (%d bytes)", toolName, input, path, size)

	case "write_file":
		path, _ := parsed["path"].(string)
		if errMsg, ok := parsed["error"].(string); ok {
			return fmt.Sprintf("%s(%s) → ✗ %s", toolName, input, errMsg)
		}
		msg, _ := parsed["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("File berhasil ditulis: %s", path)
		}
		return fmt.Sprintf("%s(%s) → ✓ %s", toolName, input, msg)

	case "list_files":
		path, _ := parsed["path"].(string)
		count := 0
		if c, ok := parsed["count"].(float64); ok {
			count = int(c)
		}
		if errMsg, ok := parsed["error"].(string); ok {
			return fmt.Sprintf("%s(%s) → ✗ %s", toolName, input, errMsg)
		}
		return fmt.Sprintf("%s(%s) → ✓ %d file/direktori di %s", toolName, input, count, path)

	default:
		// Unknown tool — show truncated raw output
		display := output
		if len(display) > 300 {
			display = display[:300] + "..."
		}
		return fmt.Sprintf("%s(%s) → %s", toolName, input, display)
	}
}

// ---------------------------------------------------------------------------
// Tool execution helpers
// ---------------------------------------------------------------------------

func parseReActResponse(text string) (reActTool, bool) {
	if m := finalRe.FindStringSubmatch(text); len(m) > 1 {
		return reActTool{output: strings.TrimSpace(m[1])}, true
	}

	if m := toolCallRe.FindStringSubmatch(text); len(m) > 2 {
		return reActTool{
			name:  strings.TrimSpace(m[1]),
			input: strings.TrimSpace(m[2]),
		}, false
	}

	if m := actionRe.FindStringSubmatch(text); len(m) > 1 {
		actionStr := strings.TrimSpace(m[1])

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
		return reActTool{name: actionStr, input: "{}"}, false
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
		input = json.RawMessage(fmt.Sprintf("%q", call.input))
	}

	output, err := tool.Execute(context.Background(), input)
	if err != nil {
		return fmt.Sprintf("Error eksekusi %s: %v", call.name, err)
	}

	return string(output)
}

func buildToolSystemPrompt(toolList []tools.Tool, mode chatMode) string {
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

	default:
		b.WriteString("Kamu adalah AI asisten yang membantu menjawab pertanyaan ")
		b.WriteString("dan menyelesaikan tugas.\n\n")
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
