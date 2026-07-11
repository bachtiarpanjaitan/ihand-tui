package main

import (
	"context"
	"fmt"
	"strings"
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

	toolspkg "github.com/bachtiarpanjaitan/ihand-tui/internal/tools"
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
	stateSettings
)

// SettingsFieldType identifies which setting field is being edited.
type settingsField int

const (
	settingsProfile settingsField = iota
	settingsProfileName
	settingsSchema
	settingsModel
	settingsAPIKey
	settingsBaseURL
	settingsAllowedDir
	settingsSession
	settingsFieldCount // total number of fields
)

func (s settingsField) String() string {
	switch s {
	case settingsSchema:
		return "Skema"
	case settingsModel:
		return "Model"
	case settingsAPIKey:
		return "API Key"
	case settingsBaseURL:
		return "Base URL"
	case settingsAllowedDir:
		return "Allowed Dir"
	case settingsSession:
		return "Session"
	default:
		return "Unknown"
	}
}

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
	role      string
	content   string
	toolName  string // nama tool (untuk tree-view rendering)
	tokens    int
	timing    time.Duration
	streaming bool // true jika pesan masih dalam proses streaming
}

// taskItem represents one item in the plan/task checklist.
type taskItem struct {
	desc   string
	status string // "pending", "in_progress", "completed", "error"
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
	session          string
	messages         []core.Message
	activeTools      []tools.Tool
	iteration        int
	toolCalls        []toolCallRecord
	totalTokens      int
	startTime        time.Time
	consecutiveFails int // jumlah tool gagal berturut-turut (max 1 retry)
}

// chatStepResultMsg is returned by each async LLM call step.
type chatStepResultMsg struct {
	state        chatLoopState
	response     *core.Response
	err          error
	finishReason string // API stop reason: "end_turn", "tool_use", "stop", etc.
}

type slashCommand struct {
	name string
	desc string
}

// actionCounters tracks tool call counts for Claude Code-style activity display.
type actionCounters struct {
	read    int
	write   int
	edit    int
	list    int
	find    int
	search  int
	exec    int
	browse  int
	created int
}

func (c actionCounters) total() int {
	return c.read + c.write + c.edit + c.list + c.find + c.search + c.exec + c.browse + c.created
}

func (c actionCounters) String() string {
	var parts []string
	if c.read > 0 {
		parts = append(parts, fmt.Sprintf("%d file dibaca", c.read))
	}
	if c.write > 0 {
		parts = append(parts, fmt.Sprintf("%d file ditulis", c.write))
	}
	if c.edit > 0 {
		parts = append(parts, fmt.Sprintf("%d file diedit", c.edit))
	}
	if c.list > 0 {
		parts = append(parts, fmt.Sprintf("%d direktori", c.list))
	}
	if c.find > 0 {
		parts = append(parts, fmt.Sprintf("%d pencarian file", c.find))
	}
	if c.search > 0 {
		parts = append(parts, fmt.Sprintf("%d pencarian teks", c.search))
	}
	if c.exec > 0 {
		parts = append(parts, fmt.Sprintf("%d perintah", c.exec))
	}
	if c.browse > 0 {
		parts = append(parts, fmt.Sprintf("%d URL", c.browse))
	}
	if c.created > 0 {
		parts = append(parts, fmt.Sprintf("%d direktori dibuat", c.created))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
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
	retryCount   int  // hitungan retry untuk error LLM

	// Streaming activity display (Claude Code-style)
	actionCounts    actionCounters
	activityContent string // base content without spinner, managed by streaming handler

	// Task list (plan panel)
	taskList    []taskItem // daftar task dari plan checklist
	taskUpdated bool       // true jika taskList berubah

	// Streaming state
	streamingContent  string        // accumulated text from stream chunks
	earlyTools        []earlyToolExec // tools yang sudah dieksekusi saat streaming (multiple)
		earlyToolKeys     map[string]bool // dedup key → sudah dieksekusi early
	streamStartTime   time.Time     // when the current stream started
	lastStreamRender  time.Time     // when the stream was last rendered to UI
	lastFinishReason  string        // API stop reason from the last chunk
	shownToolKeys     map[string]bool // tool calls already shown during this stream (key = name+input)

	mdRenderer *glamour.TermRenderer
	mdWidth    int

	// Settings state
	configPath          string        // path ke file settings.json
	currentProfile      int           // index profil yang aktif
	settingsCurrentField settingsField // field yang sedang dipilih di settings
	settingsEditMode     bool          // true saat sedang mengedit nilai field
	settingsEditBuffer   string        // buffer untuk input nilai baru
	settingsSelectAll    bool          // true saat seluruh buffer terseleksi (Cmd+A)
	settingsProfileSel   int           // index yang dipilih di daftar profil
	settingsShowProfileList bool      // true saat menampilkan daftar profil

		settingsConfig *Config // pointer ke config saat di settings mode (nil jika tidak di settings)
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func initialModel(ai *ihandai.Client, store memory.ConversationStore, provider, modelName, session, allowedDir, configPath string) model {
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
	editTool := toolspkg.NewEditFileTool(allowedDir)
	readTool := toolspkg.NewReadFileTool(allowedDir)
	listTool := toolspkg.NewListFilesTool(allowedDir)
	browseTool := toolspkg.NewBrowseTool()
	findFilesTool := toolspkg.NewFindFilesTool(allowedDir)
	searchTextTool := toolspkg.NewSearchTextTool(allowedDir)
	readFileLinesTool := toolspkg.NewReadFileLinesTool(allowedDir)
	execTool := toolspkg.NewExecTool(allowedDir)
	toolList := []tools.Tool{mkdirTool, writeTool, editTool, readTool, listTool, browseTool, findFilesTool, searchTextTool, readFileLinesTool, execTool}
	ai.SetTools(mkdirTool, writeTool, editTool, readTool, listTool, browseTool, findFilesTool, searchTextTool, readFileLinesTool, execTool)

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
		allowedDir:      allowedDir,
		toolList:        toolList,
		mouseEnabled:    true,
		selSugg:         -1,
		fileMentions:    make(map[string]string),
		allowedDirAbs:   resolveAllowedDir(allowedDir),
		configPath:      configPath,
	}
}

func initModel(ai *ihandai.Client, store memory.ConversationStore, provider, modelName, session, allowedDir, configPath string) model {
	m := initialModel(ai, store, provider, modelName, session, allowedDir, configPath)

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
