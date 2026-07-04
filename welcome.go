package main

import (
	"fmt"
	"strings"
)

func welcomeMessage(provider, modelName string) string {
	var sb strings.Builder
	sb.WriteString(titleStyle().Render("╭────────────────────────────────────────────╮"))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render(fmt.Sprintf("      Selamat datang di Ihand TUI v%s      ", version)))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render(fmt.Sprintf("       %s / %s       ", provider, modelName)))
	sb.WriteString("\n")
	sb.WriteString(titleStyle().Render("╰────────────────────────────────────────────╯"))
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
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Lainnya:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /exit   — keluar"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /clear  — reset percakapan"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /stats  — statistik session"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /self-update  — update ke versi terbaru"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  /help   — bantuan"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Tools: write_file, read_file, list_files"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("Enter untuk kirim  ·  Ctrl+J untuk baris baru  ·  Ctrl+C untuk keluar"))
	return sb.String()
}
