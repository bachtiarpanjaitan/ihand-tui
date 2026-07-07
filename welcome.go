package main

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

func welcomeMessage(provider, modelName string, width int) string {
	if width < 20 {
		width = 20
	}

	titleText := fmt.Sprintf("Ihand TUI %s", version)
	modelText := fmt.Sprintf("%s / %s", provider, modelName)

	// Dynamic border mengikuti lebar window
	hLine := strings.Repeat("─", width-2)
	topBorder := "╭" + hLine + "╮"
	bottomBorder := "╰" + hLine + "╯"

	// Teks di tengah
	titleLine := "│" + centerText(titleText, width-2) + "│"
	modelLine := "│" + centerText(modelText, width-2) + "│"

	var sb strings.Builder
	sb.WriteString(titleStyle().Render(topBorder))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render(titleLine))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render(modelLine))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render(bottomBorder))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Mode:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /chat  — percakapan normal"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /plan  — analisis & rencana (read-only)"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /edit  — implementasi & edit file"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /auto  — otonom (multi-step otomatis)"))
	sb.WriteString("\n")
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Lainnya:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /settings  — pengaturan"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /exit   — keluar"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /clear  — reset percakapan"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /stats  — statistik session"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /effort  — atur kedalaman AI"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /self-update  — update ke versi terbaru"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /help   — bantuan"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Tools: write_file, edit_file, read_file, list_files"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Enter kirim  ·  Shift+Enter baris baru  ·  Ctrl+S copy all  ·  Ctrl+E mouse  ·  Ctrl+C keluar"))
	return sb.String()
}

// centerText meratakan teks ke tengah dalam lebar tertentu.
func centerText(text string, width int) string {
	tw := lipgloss.Width(text)
	if tw >= width {
		return text
	}
	left := (width - tw) / 2
	right := width - tw - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}
