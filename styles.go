package main

import (
	lipgloss "charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------------
// Styles (adaptive, works in light + dark terminals)
// ---------------------------------------------------------------------------

var (
	borderColor = lipgloss.Color("240")
	promptColor = lipgloss.Color("39")
	checkColor  = lipgloss.Color("76")
	dimColor    = lipgloss.Color("243")
	errColor    = lipgloss.Color("196")
	titleColor  = lipgloss.Color("252")
	statusBg    = lipgloss.Color("236")
	userColor   = lipgloss.Color("39")

	headerStyle = lipgloss.NewStyle().
			Foreground(titleColor).
			Bold(true).
			Padding(0, 1)

	headerBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("234")).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(borderColor)

	dimStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	userPromptStyle = lipgloss.NewStyle().
			Foreground(userColor).
			Bold(true)

	checkStyle = lipgloss.NewStyle().
			Foreground(checkColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errColor).
			Bold(true)

	statusStyle = lipgloss.NewStyle().
			Background(statusBg).
			Foreground(dimColor).
			Padding(0, 1)

	suggestionBoxStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)

	suggestionSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("240")).
				Foreground(lipgloss.Color("255")).
				Bold(true).
				Padding(0, 1)

	suggestionDimStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Padding(0, 1)
)

func titleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(promptColor).
		Bold(true)
}

func toolStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Padding(0, 1)
}

func toolErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		Padding(0, 1)
}
