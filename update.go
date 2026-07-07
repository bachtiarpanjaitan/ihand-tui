package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
)

// tickMsg is sent every 500ms to animate the status dots while thinking.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ---------------------------------------------------------------------------
// Bubble Tea interface
// ---------------------------------------------------------------------------

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.textarea.Focus(),
		tea.RequestWindowSize,
	)
}

func formatStreamForDisplay(content string) string {
	// Cari tool call — tampilkan hanya tool yang sedang dipanggil
	if strings.Contains(content, "Final Answer:") {
		// Tampilkan Final Answer saja
		if idx := strings.Index(content, "Final Answer:"); idx >= 0 {
			answer := strings.TrimSpace(content[idx+13:])
			// Batasi panjang preview
			if len(answer) > 2000 {
				answer = answer[:2000] + "\n\n... *(respons terlalu panjang)*"
			}
			return "Jawaban: " + answer
		}
	}

	if strings.Contains(content, "Action:") {
		// Ekstrak nama tool dari Action: line
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Action:") {
				action := strings.TrimSpace(strings.TrimPrefix(line, "Action:"))
				// Ambil nama tool sebelum '('
				if idx := strings.Index(action, "("); idx > 0 {
					action = strings.TrimSpace(action[:idx])
				}
				return fmt.Sprintf("**Memanggil:** %s ...", action)
			}
		}
	}

	// Jika tidak ada tool call, tampilkan indikator berpikir
	return "Sedang Berpikir..."
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.recalcLayout()

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)

	case streamChunkMsg:
		if m.streamStartTime.IsZero() {
			m.streamStartTime = time.Now()
			m.streamingContent = ""
			// Add a placeholder message for the stream
			m.messages = append(m.messages, chatMessage{
				role:    "assistant",
				content: "",
				timing:  0,
			})
		}

		if !msg.done {
			m.streamingContent += msg.content
			m.messages[len(m.messages)-1].content = formatStreamForDisplay(m.streamingContent)

			// Early tool execution: eksekusi tool auto-trusted langsung saat streaming
			if m.earlyTool.toolName == "" {
				if toolCall, isFinal := parseReActResponse(m.streamingContent); toolCall.name != "" && !isFinal && toolCall.input != "{}" {
					if isToolAutoTrustedMode(m.mode, toolCall.name) {
						toolOutput := executeToolCall(msg.state.activeTools, toolCall)
						isToolErr := strings.HasPrefix(toolOutput, "Error")
						m.earlyTool = earlyToolExec{
							toolName: toolCall.name,
							input:    toolCall.input,
							output:   toolOutput,
							isError:  isToolErr,
						}
						display := formatToolDisplay(toolCall.name, toolCall.input, toolOutput)
						role := "tool"
						if isToolErr {
							role = "tool-error"
						}
						m.toolActivity = fmt.Sprintf("%s", toolCall.name)
						m.messages = append(m.messages, chatMessage{
							role:    role,
							content: display,
							tokens:  0,
						})
						m.rebuildViewport()
					}
				}
			}
			m.messages[len(m.messages)-1].timing = time.Since(m.streamStartTime)

			// Throttle UI rendering to max ~20 FPS (50ms) to avoid lag
			now := time.Now()
			if now.Sub(m.lastStreamRender) > 50*time.Millisecond {
				m.rebuildViewport()
				m.lastStreamRender = now
			}
			return m, waitForStreamChunk(msg.ch, msg.state)
		}

		// Stream is done, finalize the response
		finalResp := &core.Response{
			Content: m.streamingContent,
		}

		// Reset stream state
		m.streamStartTime = time.Time{}
		m.streamingContent = ""

		// Remove the placeholder assistant message — it was added at stream start and
		// processChatStep will add the real assistant message from resp.Content.
		// If mid-stream tool execution added messages after the placeholder, keep those.
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" && m.messages[i].content == "" {
				m.messages = append(m.messages[:i], m.messages[i+1:]...)
				break
			}
		}

		// Dispatch to the regular ReAct handler
		return m, func() tea.Msg {
			return chatStepResultMsg{
				state:    msg.state,
				response: finalResp,
				err:      nil,
			}
		}

	case chatStepResultMsg:
		// Process one step of the ReAct loop. Each tool call, thinking step,
		// or final answer is processed individually so the UI updates in real-time.
		cmd, done := processChatStep(&m, msg)
		if done {
			cmds = append(cmds, cmd)
		} else {
			return m, cmd
		}

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

	case tickMsg:
		if m.state == stateThinking {
			m.tickCount++
			spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spinner := spinnerFrames[m.tickCount%len(spinnerFrames)]

			// Status message (bottom bar)
			if m.mode == modeTeam && m.currentTeamRole != roleNone {
				m.statusMsg = fmt.Sprintf("[%s] %s Memproses", m.currentTeamRole.String(), spinner)
			} else {
				m.statusMsg = fmt.Sprintf("%s Memproses", spinner)
			}

			// Update the last message with spinner prefix for animated effect
			for i := len(m.messages) - 1; i >= 0; i-- {
				msg := m.messages[i]
				if msg.role == "tool" || msg.role == "tool-error" || msg.role == "assistant" {
					base := msg.content
					// Remove any existing spinner prefix
					for _, f := range spinnerFrames {
						base = strings.TrimPrefix(base, f+" ")
					}
					base = strings.TrimLeft(base, ".")
					// For tool messages, add animated dot prefix
					if msg.role == "tool" || msg.role == "tool-error" {
						m.messages[i].content = spinner + " " + strings.TrimSpace(base)
					}
					break
				}
			}

			cmds = append(cmds, tickCmd())
		}

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	var taCmd tea.Cmd
	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)

	return m, tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (m model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

	case "esc":
		if m.state == stateSelectingEffort {
			m.state = stateReady
			m.textarea.Focus()
			m.recalcLayout()
			m.rebuildViewport()
			return m, nil
		}
		return m, nil

	case "ctrl+c", "ctrl+d":
		return m, tea.Quit

	case "ctrl+l":
		m.viewport.GotoTop()
		return m, nil

	case "ctrl+e":
		m.mouseEnabled = !m.mouseEnabled
		return m, nil

	case "ctrl+s":
		return m, copyConversation(&m)

	case "shift+enter", "ctrl+j":
		if m.state == stateThinking {
			return m, nil
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd

	case "y":
		if m.state == stateTrustPrompt {
			return m.handleTrustApprove()
		}
		if m.state == stateConfirming {
			return m.handleConfirmApprove()
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		// Recalculate suggestions after typing
		fileSugs, atPos := computeFileSuggestions(m.textarea.Value(), m.allowedDir)
		if len(fileSugs) > 0 {
			m.suggestions = fileSugs
			m.suggestionType = "file"
			m.fileQueryStart = atPos
			m.selSugg = 0
		} else {
			m.suggestions = computeSuggestions(m.textarea.Value())
			m.suggestionType = "command"
			if len(m.suggestions) > 0 {
				m.selSugg = 0
			} else {
				m.selSugg = -1
			}
		}
		return m, cmd

	case "n":
		if m.state == stateTrustPrompt {
			return m.handleTrustDeny()
		}
		if m.state == stateConfirming {
			return m.handleConfirmDeny()
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		fileSugs, atPos := computeFileSuggestions(m.textarea.Value(), m.allowedDir)
		if len(fileSugs) > 0 {
			m.suggestions = fileSugs
			m.suggestionType = "file"
			m.fileQueryStart = atPos
			m.selSugg = 0
		} else {
			m.suggestions = computeSuggestions(m.textarea.Value())
			m.suggestionType = "command"
			if len(m.suggestions) > 0 {
				m.selSugg = 0
			} else {
				m.selSugg = -1
			}
		}
		return m, cmd

	case "enter":
		if m.state == stateThinking {
			return m, nil
		}

		// Handle confirmation via option selector
		if m.state == stateConfirming {
			if m.confirmChoice == 0 {
				return m.handleConfirmApprove()
			}
			return m.handleConfirmDeny()
		}

		if m.state == stateSelectingEffort {
			m.state = stateReady
			m.textarea.Focus()
			return m.switchEffort(m.tempEffort)
		}

		// Handle trust prompt
		if m.state == stateTrustPrompt {
			if m.confirmChoice == 0 {
				return m.handleTrustApprove()
			}
			return m.handleTrustDeny()
		}

		if m.selSugg >= 0 && len(m.suggestions) > 0 {
			if m.suggestionType == "command" {
				cmdStr := m.suggestions[m.selSugg]
				m.suggestions = nil
				m.suggestionType = ""
				m.selSugg = -1
				m.textarea.Reset()
				return m.handleCommand(cmdStr)
			}
		}

		m.suggestions = nil
		m.suggestionType = ""
		m.selSugg = -1

		input := strings.TrimSpace(m.textarea.Value())
		if input == "" {
			return m, nil
		}

		if strings.HasPrefix(input, "/") {
			return m.handleCommand(input)
		}

		// Resolve @mentions: replace display names with full paths
		llmInput := resolveFileMentions(input, m.fileMentions)

		inputTokens := countTokens(llmInput)
		m.messages = append(m.messages, chatMessage{
			role:    "user",
			content: input,
			tokens:  inputTokens,
		})
		m.state = stateThinking
		if m.mode == modeTeam {
			m.currentTeamRole = roleArchitect
		} else {
			m.currentTeamRole = roleNone
		}
		m.statusMsg = ""
		m.tickCount = 0
		m.toolActivity = ""
		m.trustWrite = m.trustConfirmed
		m.textarea.Reset()
		m.textarea.Blur()
		// Clear file mentions after sending
		m.fileMentions = make(map[string]string)

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()

		return m, tea.Batch(
			startChatLoop(m.ai, m.ctx, m.session, llmInput, m.memory, m.toolList, m.mode, m.effort, m.allowedDir),
			tickCmd(),
		)

	case "up", "left":
		if m.state == stateConfirming {
			m.confirmChoice = (m.confirmChoice + 1) % 2
			m.rebuildViewport()
			return m, nil
		}
		if m.state == stateTrustPrompt {
			m.confirmChoice = (m.confirmChoice + 1) % 2
			m.rebuildViewport()
			return m, nil
		}
		if m.state == stateSelectingEffort {
			if m.tempEffort > effortLow {
				m.tempEffort--
			} else {
				m.tempEffort = effortHigh
			}
			m.rebuildViewport()
			return m, nil
		}
		if msg.String() == "up" {
			m.suggestions = nil
			m.suggestionType = ""
			m.selSugg = -1
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd

	case "down", "right", "tab":
		if m.state == stateConfirming {
			m.confirmChoice = (m.confirmChoice + 1) % 2
			m.rebuildViewport()
			return m, nil
		}
		if m.state == stateTrustPrompt {
			m.confirmChoice = (m.confirmChoice + 1) % 2
			m.rebuildViewport()
			return m, nil
		}
		if m.state == stateSelectingEffort {
			if m.tempEffort < effortHigh {
				m.tempEffort++
			} else {
				m.tempEffort = effortLow
			}
			m.rebuildViewport()
			return m, nil
		}
		if msg.String() == "tab" {
			if len(m.suggestions) > 0 {
				m.selSugg = (m.selSugg + 1) % len(m.suggestions)
				if m.suggestionType == "file" {
					// Replace only the @query part, preserving text before and after
					currentValue := m.textarea.Value()
					before := currentValue[:m.fileQueryStart]
					// Find the end of the @query (space, newline, or end of string)
					afterAt := currentValue[m.fileQueryStart+1:]
					spaceIdx := strings.IndexAny(afterAt, " \t\n\r")
					var after string
					if spaceIdx >= 0 {
						after = afterAt[spaceIdx:]
					}
					// Show only file/folder basename in textarea, store full path
					fullPath := m.suggestions[m.selSugg]
					displayName := filepath.Base(strings.TrimSuffix(fullPath, "/"))
					// Preserve trailing slash for directories
					if strings.HasSuffix(fullPath, "/") {
						displayName += "/"
					}
					m.fileMentions[displayName] = fullPath
					m.textarea.SetValue(before + "@" + displayName + after)
					m.textarea.CursorEnd()
				} else {
					// For commands, just cycle the selection, do not fill the textarea
				}
				return m, nil
			}
			return m, nil
		}
		if msg.String() == "down" {
			m.suggestions = nil
			m.suggestionType = ""
			m.selSugg = -1
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd

	case "pgup", "pgdown", "home", "end":
		m.suggestions = nil
		m.suggestionType = ""
		m.selSugg = -1
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case "shift+tab":
		switch m.mode {
		case modeChat:
			return m.switchMode(modePlan)
		case modePlan:
			return m.switchMode(modeEdit)
		case modeEdit:
			return m.switchMode(modeAuto)
		case modeAuto:
			return m.switchMode(modeTeam)
		case modeTeam:
			return m.switchMode(modeChat)
		}
		return m, nil

	default:
		if m.state == stateThinking {
			return m, nil
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)

		// Check for @file mentions first, then fall back to slash commands
		fileSugs, atPos := computeFileSuggestions(m.textarea.Value(), m.allowedDir)
		if len(fileSugs) > 0 {
			m.suggestions = fileSugs
			m.suggestionType = "file"
			m.fileQueryStart = atPos
			m.selSugg = 0
		} else {
			m.suggestions = computeSuggestions(m.textarea.Value())
			m.suggestionType = "command"
			if len(m.suggestions) > 0 {
				m.selSugg = 0
			} else {
				m.selSugg = -1
			}
		}

		return m, cmd
	}
}

// handleTrustApprove menyimpan trust untuk direktori ini dan melanjutkan.
func (m model) handleTrustApprove() (tea.Model, tea.Cmd) {
	// Persist trust ke disk
	if err := trustDir(m.allowedDirAbs); err != nil {
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: fmt.Sprintf("Gagal menyimpan trust: %v", err),
			tokens:  0,
		})
	}
	m.trustConfirmed = true
	m.trustWrite = true
	m.state = stateReady
	m.toolActivity = "✓ Direktori dipercaya — mode auto/edit/team dapat menulis file langsung"
	m.recalcLayout()
	m.rebuildViewport()
	return m, nil
}

// handleTrustDeny melewati trust prompt tanpa menyimpan trust.
func (m model) handleTrustDeny() (tea.Model, tea.Cmd) {
	m.trustConfirmed = false
	m.trustWrite = false
	m.state = stateReady
	m.toolActivity = "✗ Trust dilewati — konfirmasi akan diminta setiap kali"
	m.recalcLayout()
	m.rebuildViewport()
	return m, nil
}

// handleConfirmApprove mengeksekusi tool setelah disetujui user.
func (m model) handleConfirmApprove() (tea.Model, tea.Cmd) {
	toolOutput := executeToolCall(m.pendingState.activeTools, m.pendingTool)
	state := m.pendingState
	state.messages = append(state.messages,
		core.Message{Role: "assistant", Content: m.pendingToolResp},
		core.Message{Role: "user", Content: fmt.Sprintf(
			"Observation (hasil dari tool %s): %s", m.pendingTool.name, toolOutput,
		)},
	)
	state.iteration++
	display := formatToolDisplay(m.pendingTool.name, m.pendingTool.input, toolOutput)
	// Activity bar: concise summary (single line, tanpa diff)
	path := extractField(toolOutput, `"path"`)
	if path == "" {
		path = m.pendingTool.name
	}
	m.toolActivity = fmt.Sprintf("%s \u2014 Selesai", path)
	m.messages = append(m.messages, chatMessage{
		role:    "tool",
		content: display,
		tokens:  0,
	})
	m.state = stateThinking
	m.pendingTool = reActTool{}
	m.pendingToolResp = ""
	m.recalcLayout()
	m.rebuildViewport()
	return m, tea.Batch(
		continueChatLoop(m.ai, m.ctx, state),
		tickCmd(),
	)
}

// handleConfirmDeny memberi tahu LLM bahwa tool ditolak user.
func (m model) handleConfirmDeny() (tea.Model, tea.Cmd) {
	state := m.pendingState
	state.messages = append(state.messages,
		core.Message{Role: "assistant", Content: m.pendingToolResp},
		core.Message{Role: "user", Content: fmt.Sprintf(
			"User DENIED permission to run tool '%s'. "+
				"Do NOT retry the same tool call. Inform the user and suggest alternatives.",
			m.pendingTool.name,
		)},
	)
	state.iteration++
	m.toolActivity = fmt.Sprintf("✗ %s — Ditolak", m.pendingTool.name)
	m.messages = append(m.messages, chatMessage{
		role:    "system",
		content: fmt.Sprintf("✗ Tool %s ditolak user", m.pendingTool.name),
		tokens:  0,
	})
	m.state = stateThinking
	m.pendingTool = reActTool{}
	m.pendingToolResp = ""
	m.recalcLayout()
	m.rebuildViewport()
	return m, tea.Batch(
		continueChatLoop(m.ai, m.ctx, state),
		tickCmd(),
	)
}
