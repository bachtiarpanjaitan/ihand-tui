package main

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m model) View() tea.View {
	if !m.ready {
		return tea.NewView("Initializing...")
	}

	rendered := m.renderFull()
	v := tea.NewView(rendered)
	v.AltScreen = true
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	} else {
		v.MouseMode = tea.MouseModeNone
	}
	return v
}

// ---------------------------------------------------------------------------
// Full-screen rendering
// ---------------------------------------------------------------------------

func (m *model) renderFull() string {
	modeTag := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Foreground(lipgloss.Color(m.mode.Color())).
		Bold(true).
		Padding(0, 1).
		Render(m.mode.String())
	effortTag := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Foreground(lipgloss.Color(m.effort.Color())).
		Bold(true).
		Padding(0, 1).
		Render(m.effort.Tag())

	headerLeft := headerStyle.Render(fmt.Sprintf("Ihand TUI %s · %s/%s", version, m.provider, m.modelName))
	headerLeft = lipgloss.JoinHorizontal(lipgloss.Top, modeTag, effortTag, headerLeft)
	sessionInfo := lipgloss.NewStyle().
		Background(lipgloss.Color("234")).
		Foreground(dimColor).
		Render(fmt.Sprintf("Session: %s", m.session))
	headerGap := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(sessionInfo) - 2
	if headerGap < 1 {
		headerGap = 1
	}
	headerContent := lipgloss.JoinHorizontal(lipgloss.Top,
		headerLeft,
		strings.Repeat(" ", headerGap),
		sessionInfo,
	)
	padRight := m.width - lipgloss.Width(headerContent)
	if padRight < 0 {
		padRight = 0
	}
	header := headerBarStyle.Render(headerContent + strings.Repeat(" ", padRight))

	sep := separatorStyle.Render(strings.Repeat("━", m.width))

	vp := m.viewport.View()

	var status string
	if m.state == stateThinking {
		if len(m.messages) > 0 {
			status = fmt.Sprintf(" ~%d token (%d pesan)",
				m.totalTokens, len(m.messages))
		} else {
			status = ""
		}
		statusW := lipgloss.Width(status)
		if m.width > statusW {
			status = status + strings.Repeat(" ", m.width-statusW)
		}
		status = statusStyle.Render(status)
	} else {
		if m.toolActivity != "" {
			status = " " + m.toolActivity
		} else {
			if len(m.messages) > 0 {
				status = fmt.Sprintf(" Ready  |  ~%d total tokens  |  %d messages",
					m.totalTokens, len(m.messages))
			} else {
				status = " Ready — ketik pesan untuk memulai"
			}
		}
		if m.mouseEnabled {
			status += dimStyle.Render(" mouse (Ctrl+E mati, Shift+select teks)")
		}
		statusW := lipgloss.Width(status)
		if m.width > statusW {
			status = status + strings.Repeat(" ", m.width-statusW)
		}
		status = statusStyle.Render(status)
	}

	var sug string
	if len(m.suggestions) > 0 {
		sug = renderSuggestions(m)
	}

	var bottom string
	if m.state == stateSelectingEffort {
		bottom = renderEffortSelector(m)
	} else if m.state == stateConfirming {
		bottom = renderConfirmPrompt(m)
	} else if m.state == stateThinking {
		// Thinking indicator is now shown in the viewport via streaming message.
		// Bottom area just shows the textarea (blurred).
		bottom = m.textarea.View()
	} else if m.state == stateTrustPrompt {
		bottom = renderTrustPrompt(m)
	} else if m.state == stateSettings {
		bottom = renderSettings(m)
	} else {
		input := m.textarea.View()
		bottom = input
		if sug != "" {
			bottom = lipgloss.JoinVertical(lipgloss.Left, sug, bottom)
		}
	}

	// Prepend task panel to bottom area when tasks exist
	if len(m.taskList) > 0 {
		if bottom != "" {
			taskPanel := renderTaskPanel(m)
			bottom = lipgloss.JoinVertical(lipgloss.Left, taskPanel, bottom)
		} else {
			bottom = renderTaskPanel(m)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		sep,
		vp,
		sep,
		status,
		bottom,
	)
}

// ---------------------------------------------------------------------------
// Stub renderers needed for compilation
// These will be properly implemented later.
// ---------------------------------------------------------------------------

func renderSuggestions(m *model) string {
	if len(m.suggestions) == 0 {
		return ""
	}
	var b strings.Builder
	for i, sug := range m.suggestions {
		if i == m.selSugg {
			b.WriteString(suggestionSelectedStyle.Render("▸ " + sug))
		} else {
			b.WriteString(suggestionDimStyle.Render("  " + sug))
		}
		b.WriteString("\n")
	}
	return suggestionBoxStyle.Render(strings.TrimRight(b.String(), "\n"))
}

func renderEffortSelector(m *model) string {
	levels := []effortLevel{effortLow, effortMedium, effortHigh}
	var b strings.Builder
	b.WriteString("Pilih effort level:\n\n")
	for _, l := range levels {
		if l == m.tempEffort {
			b.WriteString("   > ")
		} else {
			b.WriteString("     ")
		}
		b.WriteString(l.String())
		b.WriteString("\n")
	}
	b.WriteString("\n ↑↓ navigasi  •  Enter pilih  •  Esc batal")
	return b.String()
}

func renderConfirmPrompt(m *model) string {
	var b strings.Builder

	// Header
	b.WriteString(titleStyle().Render(" Konfirmasi Tindakan Keamanan"))
	b.WriteString("\n\n")

	// Detail tindakan
	switch m.pendingTool.name {
	case "write_file":
		var p struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal([]byte(m.pendingTool.input), &p)

		b.WriteString(fmt.Sprintf("  Tindakan: %s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("76")).Bold(true).Render("Menulis file"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Underline(true).Render(p.Path),
		))

	case "edit_file":
		var p struct {
			Path    string `json:"path"`
			Search  string `json:"search"`
			Replace string `json:"replace"`
		}
		_ = json.Unmarshal([]byte(m.pendingTool.input), &p)

		b.WriteString(fmt.Sprintf("  Tindakan: %s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render("Mengedit file"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Underline(true).Render(p.Path),
		))

		// Tampilkan bagian search & replace
		b.WriteString("  " + treeDiffDelStyle.Render("- Cari:") + "\n")
		searchLines := strings.Split(p.Search, "\n")
		for _, line := range searchLines {
			b.WriteString("    " + treeConnectorStyle.Render("│ ") + treeDiffDelStyle.Render(line) + "\n")
		}

		b.WriteString("  " + treeDiffAddStyle.Render("+ Ganti:") + "\n")
		replaceLines := strings.Split(p.Replace, "\n")
		for _, line := range replaceLines {
			b.WriteString("    " + treeConnectorStyle.Render("│ ") + treeDiffAddStyle.Render(line) + "\n")
		}

	case "exec":
		var p struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal([]byte(m.pendingTool.input), &p)

		b.WriteString(fmt.Sprintf("  Tindakan: %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("Menjalankan Perintah"),
		))
		b.WriteString(fmt.Sprintf("  $ %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true).Render(p.Command),
		))

	default:
		// Fallback
		b.WriteString(fmt.Sprintf("  Tindakan: Menjalankan tool %s\n", toolStyle().Render(m.pendingTool.name)))
		b.WriteString(fmt.Sprintf("  Input: %s\n", m.pendingTool.input))
	}

	b.WriteString("\n  Apakah Anda mengizinkan tindakan ini?\n\n")

	options := []string{"Izinkan", "Tolak"}
	for i, opt := range options {
		if i == m.confirmChoice {
			b.WriteString(fmt.Sprintf("    ▸ %s\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(opt)))
		} else {
			b.WriteString(fmt.Sprintf("      %s\n", dimStyle.Render(opt)))
		}
	}
	b.WriteString(dimStyle.Render("\n  ↑↓ navigasi  •  Enter pilih  •  Y/N"))
	return b.String()
}

func renderThinkingIndicator(m *model) string {
	// Tampilkan teks yang sedang di-stream (jika ada) atau spinner
	spinnerFrames := []string{"\u280b", "\u2819", "\u2839", "\u2838", "\u283c", "\u2834", "\u2826", "\u2827", "\u2823", "\u280f"}
	spinner := spinnerFrames[m.tickCount%len(spinnerFrames)]

	// Cari assistant message terakhir untuk ditampilkan
	var content string
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "assistant" {
			content = m.messages[i].content
			break
		}
	}

	if content == "" {
		return fmt.Sprintf(" %s  Sedang Berpikir...", spinner)
	}
	// Hapus spinner prefix untuk display
	for _, f := range spinnerFrames {
		content = strings.TrimPrefix(content, f+" ")
	}
	content = strings.TrimSpace(content)
	if len(content) > 60 {
		content = content[:60] + "..."
	}
	return fmt.Sprintf(" %s  %s", spinner, content)
}

func renderTrustPrompt(m *model) string {
	var b strings.Builder
	b.WriteString(titleStyle().Render("Trust direktori ini?\n\n"))
	b.WriteString(fmt.Sprintf("Izinkan akses file di:\n"))
	b.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(m.allowedDirAbs)))
	options := []string{"Trust — izinkan akses langsung", "Deny — tanya setiap kali"}
	for i, opt := range options {
		if i == m.confirmChoice {
			b.WriteString(fmt.Sprintf("  ▸ %s\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(opt)))
		} else {
			b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(opt)))
		}
	}
	b.WriteString(dimStyle.Render("\n ↑↓ navigasi  •  Enter pilih  •  Y/N  •  Esc nanti"))
	return b.String()
}

// ---------------------------------------------------------------------------
// Task panel rendering (plan checklist)
// ---------------------------------------------------------------------------

func renderTaskPanel(m *model) string {
	if len(m.taskList) == 0 {
		return ""
	}

	boxW := m.width - 1
	if boxW < 40 {
		boxW = 40
	}

	var b strings.Builder

	// Top border
	b.WriteString(dimStyle.Render("\u250c" + strings.Repeat("\u2500", boxW-2) + "\u2510"))
	b.WriteString("\n")

	// Separate incomplete vs completed
	var incomplete, completed []taskItem
	for _, t := range m.taskList {
		if t.status == "completed" {
			completed = append(completed, t)
		} else {
			incomplete = append(incomplete, t)
		}
	}

	// Show max 5 items, prioritizing incomplete
	const maxVisible = 5
	var visible []taskItem
	visible = append(visible, incomplete...)
	if len(visible) < maxVisible {
		remaining := maxVisible - len(visible)
		if len(completed) > remaining {
			completed = completed[:remaining]
		}
		visible = append(visible, completed...)
	}
	if len(visible) > maxVisible {
		visible = visible[:maxVisible]
	}
	totalHidden := len(m.taskList) - len(visible)

	for _, task := range visible {
		var icon string
		var iconColor string
		switch task.status {
		case "in_progress":
			icon = "\u280b " // spinner + space
			iconColor = "214"
		case "completed":
			icon = "\u2713 " // checkmark + space
			iconColor = "76"
		case "error":
			icon = "\u2717 " // x-mark + space
			iconColor = "196"
		default:
			icon = "[ ] " // pending indicator
			iconColor = "243"
		}
		iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(iconColor))
		if task.status == "in_progress" {
			iconStyle = iconStyle.Bold(true)
		}

		desc := task.desc
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}

		line := fmt.Sprintf("\u2502 %s%s", iconStyle.Render(icon), desc)
		b.WriteString(dimStyle.Render(line))
		b.WriteString("\n")
	}

	// "+N more" jika ada task tersembunyi
	if totalHidden > 0 {
		moreStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		line := fmt.Sprintf("\u2502 %s+%d lainnya", strings.Repeat(" ", 3), totalHidden)
		b.WriteString(moreStyle.Render(line))
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString(dimStyle.Render("\u2514" + strings.Repeat("\u2500", boxW-2) + "\u2518"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Settings view
// ---------------------------------------------------------------------------

func renderSettings(m *model) string {
	cfg := m.settingsConfig
	if cfg == nil {
		return "Loading settings..."
	}

	// Profile list sub-view
	if m.settingsShowProfileList {
		return renderProfileList(m)
	}

	boxW := m.width - 1
	if boxW < 40 {
		boxW = 40
	}

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(promptColor).
		Bold(true).
		Render("  ⚙ Pengaturan")
	b.WriteString(title)
	b.WriteString("\n")

	// Field definitions
	type fieldDef struct {
		label string
		value string
	}

	// Active profile name
	profileName := "(no profile)"
	if cfg.ActiveProfile >= 0 && cfg.ActiveProfile < len(cfg.Profiles) {
		profileName = cfg.Profiles[cfg.ActiveProfile].Name
	}

	activeCfg := cfg.ActiveConfig()
	fields := []fieldDef{
		{label: "Profile", value: profileName},
		{label: "Nama Profil", value: profileName},
		{label: "Skema", value: activeCfg.Schema},
		{label: "Model", value: activeCfg.Model},
		{label: "API Key", value: maskAPIKey(activeCfg.APIKey)},
		{label: "Base URL", value: activeCfg.BaseURL},
		{label: "Allowed Dir", value: cfg.App.AllowedDir},
		{label: "Session", value: cfg.App.Session},
	}

	// Max label width for alignment
	maxLabelW := 0
	for _, f := range fields {
		if len(f.label) > maxLabelW {
			maxLabelW = len(f.label)
		}
	}
	// Extra padding so label doesn't wrap
	maxLabelW += 5

	// Top border
	b.WriteString(dimStyle.Render("\u250c" + strings.Repeat("\u2500", boxW-2) + "\u2510"))
	b.WriteString("\n")

	for i, f := range fields {
		isCurrent := i == int(m.settingsCurrentField)

		if isCurrent {
			// Current field: show with indicator
			labelStyle := lipgloss.NewStyle().
				Width(maxLabelW).
				Align(lipgloss.Left).
				PaddingRight(3).
				Foreground(lipgloss.Color("255")).
				Bold(true)
			indicator := " ▸ "
			label := labelStyle.Render(f.label + ":")

			var displayVal string
			if m.settingsEditMode {
				// Editing: show the buffer with cursor
				cursor := "█"
				bgColor := lipgloss.Color("240") // normal edit
				if m.settingsSelectAll {
					cursor = ""                    // hide cursor when all selected
					bgColor = lipgloss.Color("25") // selection blue
				}
				displayVal = m.settingsEditBuffer + cursor
				valStyle := lipgloss.NewStyle().
					Background(bgColor).
					Foreground(lipgloss.Color("255")).
					Padding(0, 1)
				b.WriteString(fmt.Sprintf("%s%s %s\n", indicator, label, valStyle.Render(displayVal)))
			} else {
				// Not editing: show current value
				valStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("255")).
					Padding(0, 1)
				displayVal := valStyle.Render(f.value)
				// Profile field: append switch hint
				if i == int(settingsProfile) {
					hintStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("243"))
					displayVal += "  " + hintStyle.Render("▶ enter untuk pilih profil")
				}
				b.WriteString(fmt.Sprintf("%s%s %s\n", indicator, label, displayVal))
			}
		} else {
			// Non-current field: dimmed
			labelStyle := lipgloss.NewStyle().
				Width(maxLabelW).
				Align(lipgloss.Left).
				PaddingRight(3).
				Foreground(dimColor)
			label := labelStyle.Render(f.label + ":")
			valStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)
			displayVal := valStyle.Render(f.value)
			// Profile field: append switch hint (dimmed version)
			if i == int(settingsProfile) {
				hintStyle := lipgloss.NewStyle().
					Foreground(dimColor)
				displayVal += "  " + hintStyle.Render("▶ enter untuk pilih profil")
			}
			b.WriteString(fmt.Sprintf("   %s %s\n", label, displayVal))
		}
	}

	b.WriteString(dimStyle.Render("\u2514" + strings.Repeat("\u2500", boxW-2) + "\u2518"))
	b.WriteString("\n")

	// Controls hint
	controls := dimStyle.Render(
		"↑↓ navigasi  |  Enter: Profile → pilih/switch, lainnya → edit  |  Esc batal  |  Ctrl+S simpan",
	)
	b.WriteString(controls)
	b.WriteString("\n")

	return b.String()
}

// maskAPIKey masks an API key for display, showing only the last 4 characters.
// renderProfileList renders the profile selection sub-view.
func renderProfileList(m *model) string {
	profiles := m.settingsConfig.Profiles

	boxW := m.width - 1
	if boxW < 40 {
		boxW = 40
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(promptColor).
		Bold(true).
		Render("Pilih Profil LLM")
	b.WriteString(title)
	b.WriteString("\n\n")

	addNewIdx := len(profiles)

	for i, p := range profiles {
		isSelected := i == m.settingsProfileSel
		isActive := i == m.settingsConfig.ActiveProfile

		prefix := "  "
		if isSelected {
			prefix = " ▸"
		}

		nameStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))
		if isSelected {
			nameStyle = nameStyle.Bold(true)
		}

		var activeMark string
		if isActive {
			activeMark = lipgloss.NewStyle().
				Foreground(lipgloss.Color("76")).
				Render(" \u2713 aktif")
		} else {
			activeMark = dimStyle.Render("   ")
		}

		detailStr := fmt.Sprintf("%s/%s", p.Schema, p.Model)
		detail := dimStyle.Render(detailStr)

		line := fmt.Sprintf("%s %s%s  %s\n", prefix, nameStyle.Render(p.Name), activeMark, detail)
		b.WriteString(line)
	}

	// Add New Profile option
	atEnd := m.settingsProfileSel == addNewIdx
	addPrefix := "  "
	if atEnd {
		addPrefix = " ▸"
	}
	addStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39"))
	if atEnd {
		addStyle = addStyle.Bold(true)
	}
	b.WriteString(fmt.Sprintf("%s %s\n", addPrefix, addStyle.Render("+ Add New Profile")))

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" \u2191\u2193 navigasi  •  Enter pilih/switch  •  + Add New Profile  •  Esc batal"))
	return b.String()
}

func maskAPIKey(key string) string {
	if key == "" {
		return "(kosong)"
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}
