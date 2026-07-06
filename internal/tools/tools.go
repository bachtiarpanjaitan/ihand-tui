package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
)

// ---------------------------------------------------------------------------
// WriteFileTool — menulis konten ke file dalam direktori yang diizinkan
// ---------------------------------------------------------------------------

type WriteFileTool struct {
	allowedDir string
}

func NewWriteFileTool(allowedDir string) *WriteFileTool {
	return &WriteFileTool{allowedDir: allowedDir}
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Menulis konten ke file dalam direktori yang diizinkan. " +
		"Gunakan untuk membuat atau menimpa file. " +
		"Input: {\"path\": \"nama/file.txt\", \"content\": \"isi file\"}"
}

func (t *WriteFileTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"path":    {Type: "string", Description: "Path relatif ke file yang akan ditulis"},
			"content": {Type: "string", Description: "Konten yang akan ditulis ke file"},
		},
		Required: []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}

	// Validasi path
	fullPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	// Tolak jika path adalah direktori yang sudah ada
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s adalah direktori, bukan file. Gunakan path file lengkap, contoh: %s/namafile.go"}`, params.Path, params.Path)), nil
	}

	// Buat direktori jika belum ada
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membuat direktori: %s"}`, err.Error())), nil
	}

	// Baca konten lama sebelum ditimpa (untuk diff)
	oldContent := ""
	if _, err := os.Stat(fullPath); err == nil {
		if data, err := os.ReadFile(fullPath); err == nil {
			oldContent = string(data)
		}
	}

	// Tulis file
	if err := os.WriteFile(fullPath, []byte(params.Content), 0644); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal menulis file: %s"}`, err.Error())), nil
	}

	info, _ := os.Stat(fullPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	// Hitung diff
	diff := computeDiff(oldContent, params.Content)

	result, _ := json.Marshal(map[string]any{
		"success": true,
		"path":    params.Path,
		"size":    size,
		"message": fmt.Sprintf("File berhasil ditulis (%d bytes)", size),
		"diff":    diff,
	})
	return json.RawMessage(result), nil
}

// ---------------------------------------------------------------------------
// ReadFileTool — membaca konten file dalam direktori yang diizinkan
// ---------------------------------------------------------------------------

type ReadFileTool struct {
	allowedDir string
}

func NewReadFileTool(allowedDir string) *ReadFileTool {
	return &ReadFileTool{allowedDir: allowedDir}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Membaca konten file dalam direktori yang diizinkan. " +
		"Gunakan untuk melihat isi file. " +
		"Input: {\"path\": \"nama/file.txt\"}"
}

func (t *ReadFileTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"path": {Type: "string", Description: "Path relatif ke file yang akan dibaca"},
		},
		Required: []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}

	fullPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	// Batasi ukuran file yang dibaca (max 1MB)
	info, err := os.Stat(fullPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "file tidak ditemukan: %s"}`, params.Path)), nil
	}
	if info.Size() > 1_000_000 {
		return json.RawMessage(fmt.Sprintf(`{"error": "file terlalu besar (max 1MB): %d bytes"}`, info.Size())), nil
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membaca file: %s"}`, err.Error())), nil
	}

	result, _ := json.Marshal(map[string]any{
		"path":    params.Path,
		"size":    len(data),
		"content": string(data),
	})
	return json.RawMessage(result), nil
}

// ---------------------------------------------------------------------------
// ListFilesTool — list file dalam direktori yang diizinkan
// ---------------------------------------------------------------------------

type ListFilesTool struct {
	allowedDir string
}

func NewListFilesTool(allowedDir string) *ListFilesTool {
	return &ListFilesTool{allowedDir: allowedDir}
}

func (t *ListFilesTool) Name() string { return "list_files" }

func (t *ListFilesTool) Description() string {
	return "Menampilkan daftar file dan folder dalam direktori yang diizinkan. " +
		"Gunakan untuk melihat struktur file. " +
		"Input: {\"path\": \".\"} (kosongkan atau gunakan \".\" untuk root direktori)"
}

func (t *ListFilesTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"path": {Type: "string", Description: "Path relatif ke folder yang akan di-list (default: \".\")"},
		},
		Required: []string{},
	}
}

func (t *ListFilesTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		params.Path = "."
	}
	if params.Path == "" {
		params.Path = "."
	}

	fullPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	// Cek apakah direktori
	info, err := os.Stat(fullPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "direktori tidak ditemukan: %s"}`, params.Path)), nil
	}
	if !info.IsDir() {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s adalah file, bukan direktori"}`, params.Path)), nil
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membaca direktori: %s"}`, err.Error())), nil
	}

	var files []map[string]any
	for _, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		isDir := e.IsDir()
		if info != nil && !isDir {
			size = info.Size()
		}
		files = append(files, map[string]any{
			"name":  e.Name(),
			"isDir": isDir,
			"size":  size,
		})
	}

	result, _ := json.Marshal(map[string]any{
		"path":  params.Path,
		"count": len(files),
		"files": files,
	})
	return json.RawMessage(result), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolvePath mengamankan path agar hanya bisa mengakses file dalam allowedDir.
func (t *WriteFileTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *ReadFileTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *ListFilesTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func resolveSafePath(allowedDir, relPath string) (string, error) {
	// Bersihkan path
	clean := filepath.Clean(relPath)

	// Tolak path traversal
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("path traversal tidak diizinkan: %s", relPath)
	}

	// Resolve absolute path
	absAllowed, err := filepath.Abs(allowedDir)
	if err != nil {
		return "", fmt.Errorf("gagal resolve allowedDir: %w", err)
	}

	fullPath := filepath.Join(absAllowed, clean)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("gagal resolve path: %w", err)
	}

	// Verifikasi path tetap dalam allowedDir
	if !strings.HasPrefix(absPath, absAllowed+string(filepath.Separator)) && absPath != absAllowed {
		return "", fmt.Errorf("akses di luar direktori yang diizinkan: %s", relPath)
	}

	return absPath, nil
}


// computeDiff menghasilkan diff sederhana (unified-style) sebagai JSON array string.
func computeDiff(oldContent, newContent string) string {
	if oldContent == newContent {
		return "[]"
	}

	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// LCS-based diff: cari longest common subsequence
	m, n := len(oldLines), len(newLines)

	// Build LCS table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else if lcs[i-1][j] > lcs[i][j-1] {
				lcs[i][j] = lcs[i-1][j]
			} else {
				lcs[i][j] = lcs[i][j-1]
			}
		}
	}

	// Backtrack untuk menghasilkan diff
	var diffLines []string
	i, j := m, n
	var stack []string
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			stack = append(stack, " "+oldLines[i-1])
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			stack = append(stack, "+"+newLines[j-1])
			j--
		} else if i > 0 {
			stack = append(stack, "-"+oldLines[i-1])
			i--
		}
	}

	// Balikkan stack (dari belakang ke depan) lalu batasi jumlah baris
	totalLines := len(stack)
	start := 0
	if totalLines > 100 {
		start = totalLines - 100
	}
	for k := len(stack) - 1; k >= start; k-- {
		diffLines = append(diffLines, stack[k])
	}

	if totalLines > 100 {
		diffLines = append(diffLines, "... (+" + fmt.Sprintf("%d", totalLines-100) + " more lines)")
	}

	// Gabung baris diff dengan newline (raw text, bukan JSON)
	return strings.Join(diffLines, "\n")
}
