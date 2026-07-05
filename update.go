package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
			dots := strings.Repeat(".", (m.tickCount%4)+1)
			m.statusMsg = "Memproses" + dots
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

	case "enter":
		if m.state == stateThinking {
			return m, nil
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

		inputTokens := countTokens(input)
		m.messages = append(m.messages, chatMessage{
			role:    "user",
			content: input,
			tokens:  inputTokens,
		})
		m.state = stateThinking
		m.statusMsg = ""
		m.tickCount = 0
		m.toolActivity = ""
		m.textarea.Reset()
		m.textarea.Blur()

		content := m.buildConversation()
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()

		return m, tea.Batch(
			startChatLoop(m.ai, m.ctx, m.session, input, m.memory, m.toolList, m.mode),
			tickCmd(),
		)

	case "up", "down", "pgup", "pgdown", "home", "end":
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
			return m.switchMode(modeChat)
		}
		return m, nil

	case "tab":
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
				m.textarea.SetValue(before + "@" + m.suggestions[m.selSugg] + after)
			} else {
				m.textarea.SetValue(m.suggestions[m.selSugg] + " ")
			}
			m.textarea.CursorEnd()
			return m, nil
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
