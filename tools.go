package main

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

	// Buat direktori jika belum ada
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membuat direktori: %s"}`, err.Error())), nil
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

	return json.RawMessage(fmt.Sprintf(`{"success": true, "path": "%s", "size": %d, "message": "File berhasil ditulis (%d bytes)"}`,
		params.Path, size, size)), nil
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

	return json.RawMessage(fmt.Sprintf(`{"path": "%s", "size": %d, "content": %s}`,
		params.Path, len(data), jsonEscape(string(data)))), nil
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

// jsonEscape meng-escape string untuk dimasukkan ke JSON.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	// Hapus kutip pembuka dan penutup
	return string(b[1 : len(b)-1])
}
