package main

import (
	lipgloss "charm.land/lipgloss/v2"
)

func (m *model) recalcLayout() {
	fixedOverhead := 7
	sugHeight := 0
	if len(m.suggestions) > 0 {
		sugHeight = 1
	}

	// Calculate prompt/selector height dynamically to avoid cutting off prompt UIs
	extraOverhead := 0
	if m.state == stateSelectingEffort {
		extraOverhead = lipgloss.Height(renderEffortSelector(m)) - 3 // relative to standard 3-line textarea
	} else if m.state == stateConfirming {
		extraOverhead = lipgloss.Height(renderConfirmPrompt(m)) - 3
	} else if m.state == stateTrustPrompt {
		extraOverhead = lipgloss.Height(renderTrustPrompt(m)) - 3
	} else if m.state == stateSettings {
		extraOverhead = lipgloss.Height(renderSettings(m)) - 3
	}
	if extraOverhead < 0 {
		extraOverhead = 0
	}

	// Task panel overhead
	taskPanelHeight := 0
	if len(m.taskList) > 0 {
		taskPanelHeight = len(m.taskList) + 2 // tasks + border lines
		if taskPanelHeight > 10 {
			taskPanelHeight = 10
		}
	}

	vpHeight := m.height - fixedOverhead - sugHeight - extraOverhead - taskPanelHeight
	if vpHeight < 5 {
		vpHeight = 5
	}

	vpWidth := m.width
	if vpWidth < 40 {
		vpWidth = 40
	}

	m.viewport.SetWidth(vpWidth)
	m.viewport.SetHeight(vpHeight)

	taWidth := m.width - 4
	if taWidth < 20 {
		taWidth = 20
	}
	m.textarea.SetWidth(taWidth)
}
