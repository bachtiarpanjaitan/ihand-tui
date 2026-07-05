package main

import (
	"context"
	"time"

	"github.com/bachtiarpanjaitan/ihandai-go"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/memory"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/tools"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	glamour "charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"

	toolspkg "test-ihandai/internal/tools"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type chatState int

const (
	stateReady chatState = iota
	stateThinking
	stateConfirming
)

type chatMode int

const (
	modeChat chatMode = iota
	modePlan
	modeEdit
	modeAuto
)

func (m chatMode) String() string {
	switch m {
	case modeChat:
		return "Chat"
	case modePlan:
		return "Plan"
	case modeEdit:
		return "Edit"
	case modeAuto:
		return "Auto"
	default:
		return "Chat"
	}
}

func (m chatMode) Color() string {
	switch m {
	case modeChat:
		return "39"
	case modePlan:
		return "214"
	case modeEdit:
		return "76"
	case modeAuto:
		return "196"
	default:
		return "39"
	}
}

func (m chatMode) Placeholder() string {
	switch m {
	case modeChat:
		return "Ketik pesan..."
	case modePlan:
		return "Apa yang ingin direncanakan?..."
	case modeEdit:
		return "Apa yang ingin diubah?..."
	case modeAuto:
		return "Apa yang ingin dikerjakan?..."
	default:
		return "Ketik pesan..."
	}
}

type chatMessage struct {
	role    string
	content string
	tokens  int
	timing  time.Duration
}

type llmResponseMsg struct {
	content   string
	tokens    int
	timing    time.Duration
	usage     *ihandai.TokenUsage
	toolCalls []toolCallRecord
}

type toolCallRecord struct {
	toolName string
	input    string
	output   string
	isError  bool
}

type llmErrorMsg struct{ err error }

type toolCallMsg struct {
	toolName string
	input    string
	output   string
	isError  bool
}

// chatLoopState carries the ReAct loop state across async LLM calls.
type chatLoopState struct {
	session     string
	messages    []core.Message
	activeTools []tools.Tool
	iteration   int
	toolCalls   []toolCallRecord
	totalTokens int
	startTime   time.Time
}

// chatStepResultMsg is returned by each async LLM call step.
type chatStepResultMsg struct {
	state    chatLoopState
	response *core.Response
	err      error
}

type slashCommand struct {
	name string
	desc string
}

// reActTool represents a parsed tool call from LLM text output.
type reActTool struct {
	name   string
	input  string
	output string
}

type model struct {
	width  int
	height int
	ready  bool

	viewport viewport.Model
	textarea textarea.Model

	messages    []chatMessage
	totalTokens int
	session     string
	modelName   string
	provider    string

	ai     *ihandai.Client
	ctx    context.Context
	memory memory.ConversationStore

	allowedDir string
	toolList   []tools.Tool

	state       chatState
	mode        chatMode
	statusMsg   string
	toolActivity string // aktivitas tool terakhir (ditampilkan di atas input)
	pendingTool      reActTool
	pendingState     chatLoopState
	pendingToolResp  string
	err         error
	suggestions     []string
	suggestionType  string // "command" atau "file"
	fileQueryStart  int    // posisi karakter '@' di input untuk replace
	selSugg         int

	mouseEnabled bool // toggle mouse capture (for text selection)
	tickCount    int  // animation counter for status dots

	mdRenderer *glamour.TermRenderer
	mdWidth    int
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func initialModel(ai *ihandai.Client, store memory.ConversationStore, provider, modelName, session, allowedDir string) model {
	ta := textarea.New()
	ta.Placeholder = "Ketik pesan..."
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.CharLimit = 8192

	// Tambahkan "ctrl+j" ke InsertNewline karena textarea hanya bind "enter" dan "ctrl+m"
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("enter", "ctrl+m", "ctrl+j", "shift+enter"),
		key.WithHelp("enter", "insert newline"),
	)

	s := ta.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(promptColor).Bold(true)
	s.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(dimColor)
	s.Blurred.Prompt = lipgloss.NewStyle().Foreground(dimColor)
	s.Blurred.Text = lipgloss.NewStyle().Foreground(dimColor)
	ta.SetStyles(s)

	ta.Focus()

	writeTool := toolspkg.NewWriteFileTool(allowedDir)
	readTool := toolspkg.NewReadFileTool(allowedDir)
	listTool := toolspkg.NewListFilesTool(allowedDir)
	browseTool := toolspkg.NewBrowseTool()
	toolList := []tools.Tool{writeTool, readTool, listTool, browseTool}
	ai.SetTools(writeTool, readTool, listTool, browseTool)

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))
	vp.SoftWrap = true
	vp.SetContent(welcomeMessage(provider, modelName, 50))
	vp.GotoTop()

	return model{
		viewport:   vp,
		textarea:   ta,
		session:    session,
		provider:   provider,
		modelName:  modelName,
		ai:         ai,
		ctx:        context.Background(),
		memory:     store,
		state:        stateReady,
		mode:         modeAuto,
		allowedDir:   allowedDir,
		toolList:     toolList,
		mouseEnabled: true,
		selSugg:      -1,
	}
}
