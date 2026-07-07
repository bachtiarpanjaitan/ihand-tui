package main

func (m *model) recalcLayout() {
	fixedOverhead := 7
	sugHeight := 0
	if len(m.suggestions) > 0 {
		sugHeight = 1
	}

	// Default textarea is around 3 lines tall, but our selector UI is 5 lines tall.
	// Adjust overhead when rendering those custom UIs.
	extraOverhead := 0
	if m.state == stateSelectingEffort {
		extraOverhead = 2 // 5 lines instead of 3
	} else if m.state == stateConfirming {
		extraOverhead = 3 // 6 lines instead of 3
	} else if m.state == stateTrustPrompt {
		extraOverhead = 8 // ~11 lines instead of 3
	} else if m.state == stateSettings {
		extraOverhead = 14 // ~17 lines for settings form
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
