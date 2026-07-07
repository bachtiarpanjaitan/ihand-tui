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
	stateSelectingEffort
	stateTrustPrompt
)

type chatMode int

const (
	modeChat chatMode = iota
	modePlan
	modeEdit
	modeAuto
	modeTeam
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
	case modeTeam:
		return "Team"
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
	case modeTeam:
		return "99" // Purple
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
	case modeTeam:
		return "Tugas kompleks apa yang ingin dikerjakan bersama tim?..."
	default:
		return "Ketik pesan..."
	}
}

type teamRole int

const (
	roleNone teamRole = iota
	roleArchitect
	roleDeveloper
	roleReviewer
)

func (r teamRole) String() string {
	switch r {
	case roleArchitect:
		return "Architect"
	case roleDeveloper:
		return "Developer"
	case roleReviewer:
		return "Reviewer"
	default:
		return ""
	}
}

func (r teamRole) Color() string {
	switch r {
	case roleArchitect:
		return "99" // Purple
	case roleDeveloper:
		return "76" // Green
	case roleReviewer:
		return "214" // Yellow
	default:
		return "243" // Gray
	}
}

type effortLevel int

const (
	effortLow effortLevel = iota
	effortMedium
	effortHigh
)

func (e effortLevel) String() string {
	switch e {
	case effortLow:
		return "Low"
	case effortMedium:
		return "Medium"
	case effortHigh:
		return "High"
	default:
		return "Medium"
	}
}

func (e effortLevel) Color() string {
	switch e {
	case effortLow:
		return "39" // cyan
	case effortMedium:
		return "214" // yellow
	case effortHigh:
		return "196" // red
	default:
		return "214"
	}
}

// Tag returns a visual indicator for the effort level in the header.
func (e effortLevel) Tag() string {
	switch e {
	case effortLow:
		return "▸ Min"
	case effortMedium:
		return "▸▸ Med"
	case effortHigh:
		return "▸▸▸ Max"
	default:
		return "▸▸ Med"
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
	session         string
	messages        []core.Message
	activeTools     []tools.Tool
	iteration       int
	toolCalls       []toolCallRecord
	totalTokens     int
	startTime       time.Time
	teamRole        teamRole
	reviewIteration int
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

// earlyToolExec stores the result of a tool call executed during LLM streaming.
type earlyToolExec struct {
	toolName string
	input    string
	output   string
	isError  bool
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

	state           chatState
	mode            chatMode
	effort          effortLevel
	tempEffort      effortLevel
	currentTeamRole teamRole
	statusMsg       string
	toolActivity    string // aktivitas tool terakhir (ditampilkan di atas input)
	pendingTool     reActTool
	pendingState    chatLoopState
	pendingToolResp string
	err             error
	suggestions     []string
	suggestionType  string // "command" atau "file"
	fileQueryStart  int    // posisi karakter '@' di input untuk replace
	selSugg         int

	fileMentions  map[string]string // @display → full relative path
	confirmChoice int               // 0 = Allow, 1 = Deny (option selector)
	trustWrite    bool              // setelah approve 1×, skip konfirmasi write_file & create_directory

	// Trust prompt
	trustConfirmed bool   // apakah user sudah konfirmasi trust untuk folder ini
	allowedDirAbs  string // absolute path dari allowedDir (untuk trust checking)

	mouseEnabled bool // toggle mouse capture (for text selection)
	tickCount    int  // animation counter for status dots

	// Streaming state
	streamingContent string        // accumulated text from stream chunks
	earlyTool        earlyToolExec // tool yang sudah dieksekusi saat streaming
	streamStartTime  time.Time     // when the current stream started
	lastStreamRender time.Time     // when the stream was last rendered to UI

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

	mkdirTool := toolspkg.NewCreateDirTool(allowedDir)
	writeTool := toolspkg.NewWriteFileTool(allowedDir)
	readTool := toolspkg.NewReadFileTool(allowedDir)
	listTool := toolspkg.NewListFilesTool(allowedDir)
	browseTool := toolspkg.NewBrowseTool()
	findFilesTool := toolspkg.NewFindFilesTool(allowedDir)
	searchTextTool := toolspkg.NewSearchTextTool(allowedDir)
	readFileLinesTool := toolspkg.NewReadFileLinesTool(allowedDir)
	toolList := []tools.Tool{mkdirTool, writeTool, readTool, listTool, browseTool, findFilesTool, searchTextTool, readFileLinesTool}
	ai.SetTools(mkdirTool, writeTool, readTool, listTool, browseTool, findFilesTool, searchTextTool, readFileLinesTool)

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))
	vp.SoftWrap = true
	vp.SetContent(welcomeMessage(provider, modelName, 50))
	vp.GotoTop()

	return model{
		viewport:        vp,
		textarea:        ta,
		session:         session,
		provider:        provider,
		modelName:       modelName,
		ai:              ai,
		ctx:             context.Background(),
		memory:          store,
		state:           stateReady,
		mode:            modeAuto,
		effort:          effortMedium,
		currentTeamRole: roleNone,
		allowedDir:      allowedDir,
		toolList:        toolList,
		mouseEnabled:    true,
		selSugg:         -1,
		fileMentions:    make(map[string]string),
		allowedDirAbs:   resolveAllowedDir(allowedDir),
	}
}

func initModel(ai *ihandai.Client, store memory.ConversationStore, provider, modelName, session, allowedDir string) model {
	m := initialModel(ai, store, provider, modelName, session, allowedDir)

	// Check if this directory has been trusted before
	absDir := m.allowedDirAbs
	if isDirTrusted(absDir) {
		m.trustConfirmed = true
		m.trustWrite = true
		m.state = stateReady
	} else {
		m.trustConfirmed = false
		m.state = stateTrustPrompt
	}

	return m
}
