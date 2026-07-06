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
	if m.state == stateSelectingEffort || m.state == stateConfirming {
		extraOverhead = 2 // 5 lines instead of 3
	}
	
	vpHeight := m.height - fixedOverhead - sugHeight - extraOverhead
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
