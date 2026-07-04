package main

func (m *model) recalcLayout() {
	fixedOverhead := 7
	sugHeight := 0
	if len(m.suggestions) > 0 {
		sugHeight = 1
	}
	vpHeight := m.height - fixedOverhead - sugHeight
	if vpHeight < 5 {
		vpHeight = 5
	}

	vpWidth := m.width - 2
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
