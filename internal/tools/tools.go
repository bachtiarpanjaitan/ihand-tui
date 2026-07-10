package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
)

// ---------------------------------------------------------------------------
// CreateDirTool — membuat direktori/folder dalam direktori yang diizinkan
// ---------------------------------------------------------------------------

type CreateDirTool struct {
	allowedDir string
}

func NewCreateDirTool(allowedDir string) *CreateDirTool {
	return &CreateDirTool{allowedDir: allowedDir}
}

func (t *CreateDirTool) Name() string { return "create_directory" }

func (t *CreateDirTool) Description() string {
	return "Membuat direktori/folder (termasuk parent directories jika belum ada). " +
		"Gunakan untuk membuat struktur folder project SEBELUM menulis file. " +
		"Input: {\"path\": \"cmd/api/handler\"}" +
		"Catatan: write_file juga otomatis membuat parent directory, " +
		"jadi Anda bisa langsung write_file tanpa create_directory terlebih dahulu."
}

func (t *CreateDirTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"path": {Type: "string", Description: "Path folder yang akan dibuat (relatif dari project root)"},
		},
		Required: []string{"path"},
	}
}

func (t *CreateDirTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}

	if params.Path == "" {
		return json.RawMessage(`{"error": "path tidak boleh kosong"}`), nil
	}

	fullPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	// Cek apakah sudah ada sebagai file
	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s sudah ada sebagai file, bukan direktori"}`, params.Path)), nil
	}

	// Buat direktori
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membuat direktori: %s"}`, err.Error())), nil
	}

	result, _ := json.Marshal(map[string]any{
		"success": true,
		"path":    params.Path,
		"message": fmt.Sprintf("Direktori berhasil dibuat: %s", params.Path),
	})
	return json.RawMessage(result), nil
}

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
		"Gunakan untuk membuat file baru atau menimpa file yang sudah ada. " +
		"OTOMATIS membuat folder parent jika belum ada — Anda TIDAK perlu " +
		"memanggil create_directory terlebih dahulu. " +
		"Contoh: write_file({\"path\": \"cmd/api/main.go\", \"content\": \"...\"}) " +
		"akan otomatis membuat folder cmd/api/ jika belum ada." +
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
// EditFileTool — mengedit baris tertentu dalam file dengan search & replace
// ---------------------------------------------------------------------------

type EditFileTool struct {
	allowedDir string
}

func NewEditFileTool(allowedDir string) *EditFileTool {
	return &EditFileTool{allowedDir: allowedDir}
}

func (t *EditFileTool) Name() string { return "edit_file" }

func (t *EditFileTool) Description() string {
	return "Mengedit baris tertentu dalam file yang sudah ada menggunakan search & replace. " +
		"Tidak perlu mengirim seluruh konten file — cukup kirim bagian yang ingin diubah. " +
		"Cari blok teks yang UNIK dalam file, lalu ganti dengan teks baru. " +
		"search HARUS cocok persis dengan teks yang ada di file (termasuk spasi/indentasi). " +
		"Gunakan read_file dulu untuk melihat konten file, lalu copy-paste bagian yang ingin diubah. " +
		"Jauh lebih hemat token daripada write_file untuk file besar. " +
		"Contoh: edit_file({\"path\": \"main.go\", \"search\": \"fmt.Println(\\\"hello\\\")\", \"replace\": \"fmt.Println(\\\"hi\\\")\"})"
}

func (t *EditFileTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"path":    {Type: "string", Description: "Path relatif ke file yang akan diedit"},
			"search":  {Type: "string", Description: "Teks persis yang akan dicari (case-sensitive, termasuk spasi/indentasi). Harus UNIK dalam file."},
			"replace": {Type: "string", Description: "Teks pengganti untuk menggantikan search text"},
		},
		Required: []string{"path", "search", "replace"},
	}
}

func (t *EditFileTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path    string `json:"path"`
		Search  string `json:"search"`
		Replace string `json:"replace"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}

	if params.Path == "" {
		return json.RawMessage(`{"error": "path tidak boleh kosong"}`), nil
	}
	if params.Search == "" {
		return json.RawMessage(`{"error": "search text tidak boleh kosong"}`), nil
	}

	fullPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	// Baca konten file yang ada
	oldContent, err := os.ReadFile(fullPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membaca file: %s — %s"}`, params.Path, err.Error())), nil
	}
	oldStr := string(oldContent)

	// Hitung jumlah occurences
	count := strings.Count(oldStr, params.Search)
	if count == 0 {
		return json.RawMessage(fmt.Sprintf(`{"error": "search text tidak ditemukan di file %s. Pastikan spasi dan indentasi persis sama. Gunakan read_file untuk melihat konten asli file."}`, params.Path)), nil
	}
	if count > 1 {
		return json.RawMessage(fmt.Sprintf(`{"error": "search text ditemukan %d kali di file %s. Sertakan konteks lebih banyak (beberapa baris sebelum/sesudah) agar pencarian UNIK."}`, count, params.Path)), nil
	}

	// Lakukan replace (hanya 1 kali karena sudah dipastikan unik)
	newContent := strings.Replace(oldStr, params.Search, params.Replace, 1)

	// Tulis file
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal menulis file: %s"}`, err.Error())), nil
	}

	info, _ := os.Stat(fullPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	// Hitung diff
	diff := computeDiff(oldStr, newContent)

	// Hitung statistik perubahan
	oldLines := strings.Count(params.Search, "\n") + 1
	newLines := strings.Count(params.Replace, "\n") + 1

	result, _ := json.Marshal(map[string]any{
		"success":       true,
		"path":          params.Path,
		"size":          size,
		"lines_changed": fmt.Sprintf("%d baris diganti dengan %d baris", oldLines, newLines),
		"diff":          diff,
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

	// Jika path adalah direktori, otomatis kembalikan listing isinya
	if info.IsDir() {
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return json.RawMessage(fmt.Sprintf(`{"error": "gagal membaca direktori: %s"}`, err.Error())), nil
		}

		var files []map[string]any
		for _, e := range entries {
			fi, _ := e.Info()
			size := int64(0)
			isDir := e.IsDir()
			if fi != nil && !isDir {
				size = fi.Size()
			}
			files = append(files, map[string]any{
				"name":  e.Name(),
				"isDir": isDir,
				"size":  size,
			})
		}

		result, _ := json.Marshal(map[string]any{
			"path":    params.Path,
			"is_dir":  true,
			"count":   len(files),
			"files":   files,
			"message": fmt.Sprintf("%s adalah direktori dengan %d item", params.Path, len(files)),
		})
		return json.RawMessage(result), nil
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
// FindFilesTool — mencari file berdasarkan nama/pattern
// ---------------------------------------------------------------------------

type FindFilesTool struct {
	allowedDir string
}

func NewFindFilesTool(allowedDir string) *FindFilesTool {
	return &FindFilesTool{allowedDir: allowedDir}
}

func (t *FindFilesTool) Name() string { return "find_files" }

func (t *FindFilesTool) Description() string {
	return "Mencari file berdasarkan nama/pattern di SELURUH project. " +
		"Gunakan glob pattern sesuai bahasa project: *.go (Go), *.py (Python), " +
		"*.js/*.ts (JavaScript/TypeScript), *.rs (Rust), *.java (Java), " +
		"*.rb (Ruby), *.php (PHP), *.c/*.h (C), *.cpp/*.hpp (C++), " +
		"*.cs (C#), *.swift (Swift), *.kt (Kotlin), *.md (dokumentasi), " +
		"*_test.go, test_*.py, *.spec.ts (test files), atau *config* (konfigurasi). " +
		"Jauh lebih cepat daripada list_files manual. " +
		"Input: {\"pattern\": \"*.py\", \"path\": \".\", \"max_results\": 50}"
}

func (t *FindFilesTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"pattern":     {Type: "string", Description: "Glob pattern untuk filter nama file. Contoh: *.go, *.py, *.js, *.ts, *.rs, *.java, main*, *test*, *config*. Kosongkan untuk semua file."},
			"path":        {Type: "string", Description: "Path direktori untuk memulai pencarian (default: \".\")"},
			"max_results": {Type: "string", Description: "Maksimal jumlah hasil yang ditampilkan (default: 50)"},
		},
	}
}

func (t *FindFilesTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		MaxResults string `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}
	if params.Path == "" {
		params.Path = "."
	}

	rootPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	// Cek apakah path adalah direktori
	info, err := os.Stat(rootPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "path tidak ditemukan: %s"}`, params.Path)), nil
	}
	if !info.IsDir() {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s adalah file, bukan direktori"}`, params.Path)), nil
	}

	maxResults := 50
	if params.MaxResults != "" {
		fmt.Sscanf(params.MaxResults, "%d", &maxResults)
		if maxResults < 1 {
			maxResults = 1
		}
	}

	pattern := params.Pattern
	hasPattern := pattern != ""

	// Skip directories
	skipDirs := map[string]bool{".git": true, "node_modules": true, ".claude": true, "vendor": true, "dist": true}

	type fileEntry struct {
		path  string
		isDir bool
		size  int64
	}

	var matches []fileEntry
	maxDepth := 8

	filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip ignored directories
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			// Limit depth
			rel, _ := filepath.Rel(rootPath, path)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= maxDepth {
				return filepath.SkipDir
			}
		}

		// Skip hidden files/dirs (starting with .)
		if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files > 1MB
		if !d.IsDir() {
			fi, _ := d.Info()
			if fi != nil && fi.Size() > 1_000_000 {
				return nil
			}
		}

		// Get relative path for matching
		rel, _ := filepath.Rel(rootPath, path)
		if rel == "." {
			return nil // skip root
		}

		// Match pattern (support comma-separated multi-pattern)
		if hasPattern {
			patterns := strings.Split(pattern, ",")
			matched := false
			for _, p := range patterns {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				m, err := filepath.Match(p, d.Name())
				if err == nil && m {
					matched = true
					break
				}
				// Case-insensitive fallback
				if ciMatch, _ := filepath.Match(strings.ToLower(p), strings.ToLower(d.Name())); ciMatch {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		fi, _ := d.Info()
		size := int64(0)
		if fi != nil && !d.IsDir() {
			size = fi.Size()
		}

		matches = append(matches, fileEntry{
			path:  rel,
			isDir: d.IsDir(),
			size:  size,
		})

		return nil
	})

	// Sort: directories first, then alphabetically
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			swap := false
			if matches[i].isDir != matches[j].isDir {
				swap = matches[j].isDir // directories first
			} else {
				swap = matches[i].path > matches[j].path
			}
			if swap {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Limit results
	total := len(matches)
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	// Build result
	type fileInfo struct {
		Name  string `json:"name"`
		IsDir bool   `json:"isDir"`
		Size  int64  `json:"size,omitempty"`
	}

	resultFiles := make([]fileInfo, len(matches))
	for i, m := range matches {
		resultFiles[i] = fileInfo{Name: m.path, IsDir: m.isDir, Size: m.size}
	}

	exts := DiscoverExtensions(rootPath)
	truncated := total > maxResults
	result, _ := json.Marshal(map[string]any{
		"path":          params.Path,
		"pattern":       pattern,
		"count":         len(resultFiles),
		"total":         total,
		"truncated":     truncated,
		"files":         resultFiles,
		"extensions":    exts,
		"extensions_str": FormatExtensions(exts),
	})
	return json.RawMessage(result), nil
}

// ---------------------------------------------------------------------------
// SearchTextTool — mencari teks dalam file
// ---------------------------------------------------------------------------

type SearchTextTool struct {
	allowedDir string
}

func NewSearchTextTool(allowedDir string) *SearchTextTool {
	return &SearchTextTool{allowedDir: allowedDir}
}

func (t *SearchTextTool) Name() string { return "search_text" }

func (t *SearchTextTool) Description() string {
	return "Mencari teks dalam file-file di direktori (untuk SEMUA jenis project). " +
		"Cocok untuk mencari kode, konfigurasi, atau konten dalam bahasa apapun: " +
		"Go, Python, JavaScript, TypeScript, Rust, Java, Ruby, PHP, C/C++, dll. " +
		"Pattern bisa plain text atau regex (otomatis dideteksi). " +
		"Secara default mencari di SEMUA file non-biner. " +
		"Gunakan file_pattern untuk filter ekstensi. " +
		"Input: {\"pattern\": \"def main\", \"path\": \".\", \"file_pattern\": \"*.py\", \"max_results\": 30}"
}

func (t *SearchTextTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"pattern":      {Type: "string", Description: "Teks atau regex pattern untuk dicari (wajib diisi)"},
			"path":         {Type: "string", Description: "Path direktori untuk memulai pencarian (default: \".\")"},
			"file_pattern": {Type: "string", Description: "Glob pattern filter file berdasarkan ekstensi. Contoh: *.py, *.js, *.ts, *.go, *.rs, *.java, *.md. Bisa multi-pattern dengan koma: *.go,*.yaml,*.md. Kosongkan untuk SEMUA file non-biner."},
			"max_results":  {Type: "string", Description: "Maksimal jumlah hasil yang ditampilkan (default: 30)"},
		},
		Required: []string{"pattern"},
	}
}

func (t *SearchTextTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Pattern     string `json:"pattern"`
		Path        string `json:"path"`
		FilePattern string `json:"file_pattern"`
		MaxResults  string `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}
	if params.Pattern == "" {
		return json.RawMessage(`{"error": "pattern tidak boleh kosong"}`), nil
	}
	if params.Path == "" {
		params.Path = "."
	}

	rootPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	info, err := os.Stat(rootPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "path tidak ditemukan: %s"}`, params.Path)), nil
	}
	if !info.IsDir() {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s adalah file, bukan direktori"}`, params.Path)), nil
	}

	maxResults := 30
	if params.MaxResults != "" {
		fmt.Sscanf(params.MaxResults, "%d", &maxResults)
		if maxResults < 1 {
			maxResults = 1
		}
	}

	filePattern := params.FilePattern
	hasFilePattern := filePattern != ""
	pattern := params.Pattern

	// Detect if pattern is a regex (contains special regex chars)
	isRegex := strings.ContainsAny(pattern, ".*+?[](){}^$\\|")
	var re *regexp.Regexp
	if isRegex {
		re, err = regexp.Compile(pattern)
		if err != nil {
			return json.RawMessage(fmt.Sprintf(`{"error": "pattern regex tidak valid: %s"}`, err.Error())), nil
		}
	}

	// Skip directories
	skipDirs := map[string]bool{".git": true, "node_modules": true, ".claude": true, "vendor": true, "dist": true}

	type matchEntry struct {
		File     string `json:"file"`
		Line     int    `json:"line"`
		Content  string `json:"content"`
	}

	var results []matchEntry
	maxDepth := 8
	maxLineLen := 200

	filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Only process files, not directories
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(rootPath, path)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		// Filter by file pattern (support comma-separated multiple patterns)
		if hasFilePattern {
			patterns := strings.Split(filePattern, ",")
			matched := false
			for _, p := range patterns {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				m, err := filepath.Match(p, d.Name())
				if err == nil && m {
					matched = true
					break
				}
				// Case-insensitive fallback
				if ciMatch, _ := filepath.Match(strings.ToLower(p), strings.ToLower(d.Name())); ciMatch {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		// Skip binary files (check first 512 bytes for null byte)
		fullPath := path
		fi, _ := d.Info()
		if fi != nil && fi.Size() > 1_000_000 {
			return nil // skip large files
		}
		if fi != nil && fi.Size() > 0 {
			// Check for binary content
			f, err := os.Open(fullPath)
			if err != nil {
				return nil
			}
			defer f.Close()

			buf := make([]byte, 512)
			n, _ := f.Read(buf)
			if n > 0 {
				for i := 0; i < n; i++ {
					if buf[i] == 0 {
						return nil // binary file
					}
				}
			}
		}

		rel, _ := filepath.Rel(rootPath, path)

		// Read file line by line
		f, err := os.Open(fullPath)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			var matched bool
			if isRegex {
				matched = re.MatchString(line)
			} else {
				matched = strings.Contains(strings.ToLower(line), strings.ToLower(pattern))
			}

			if matched {
				// Truncate line content
				content := line
				if len(content) > maxLineLen {
					content = content[:maxLineLen] + "..."
				}

				results = append(results, matchEntry{
					File:    rel,
					Line:    lineNum,
					Content: content,
				})

				if len(results) >= maxResults {
					return filepath.SkipDir // stop walking
				}
			}
		}

		return nil
	})

	exts := DiscoverExtensions(rootPath)
	truncated := len(results) >= maxResults
	result, _ := json.Marshal(map[string]any{
		"path":          params.Path,
		"pattern":       pattern,
		"count":         len(results),
		"truncated":     truncated,
		"matches":       results,
		"extensions":    exts,
		"extensions_str": FormatExtensions(exts),
	})
	return json.RawMessage(result), nil
}

// ---------------------------------------------------------------------------
// ReadFileLinesTool — membaca baris tertentu dari file
// ---------------------------------------------------------------------------

type ReadFileLinesTool struct {
	allowedDir string
}

func NewReadFileLinesTool(allowedDir string) *ReadFileLinesTool {
	return &ReadFileLinesTool{allowedDir: allowedDir}
}

func (t *ReadFileLinesTool) Name() string { return "read_file_lines" }

func (t *ReadFileLinesTool) Description() string {
	return "Membaca baris tertentu dari file. " +
		"Lebih cepat daripada read_file jika hanya perlu melihat bagian tertentu dari file. " +
		"Input: {\"path\": \"main.go\", \"start_line\": 10, \"end_line\": 50}"
}

func (t *ReadFileLinesTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"path":       {Type: "string", Description: "Path ke file yang akan dibaca"},
			"start_line": {Type: "string", Description: "Nomor baris awal (default: 1)"},
			"end_line":   {Type: "string", Description: "Nomor baris akhir. Jika kosong, baca 50 baris dari start_line."},
		},
		Required: []string{"path"},
	}
}

func (t *ReadFileLinesTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Path      string `json:"path"`
		StartLine string `json:"start_line"`
		EndLine   string `json:"end_line"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}
	if params.Path == "" {
		return json.RawMessage(`{"error": "path tidak boleh kosong"}`), nil
	}

	fullPath, err := t.resolvePath(params.Path)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s"}`, err.Error())), nil
	}

	// Cek file
	info, err := os.Stat(fullPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "file tidak ditemukan: %s"}`, params.Path)), nil
	}
	if info.IsDir() {
		return json.RawMessage(fmt.Sprintf(`{"error": "%s adalah direktori, bukan file"}`, params.Path)), nil
	}
	if info.Size() > 1_000_000 {
		return json.RawMessage(fmt.Sprintf(`{"error": "file terlalu besar (max 1MB): %d bytes"}`, info.Size())), nil
	}

	startLine := 1
	endLine := 0 // 0 means "read 50 lines from start"
	if params.StartLine != "" {
		fmt.Sscanf(params.StartLine, "%d", &startLine)
		if startLine < 1 {
			startLine = 1
		}
	}
	if params.EndLine != "" {
		fmt.Sscanf(params.EndLine, "%d", &endLine)
	}

	maxLines := 200
	if endLine == 0 {
		endLine = startLine + 50 - 1
	}
	if endLine-startLine+1 > maxLines {
		endLine = startLine + maxLines - 1
	}

	// Read file line by line
	f, err := os.Open(fullPath)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membuka file: %s"}`, err.Error())), nil
	}
	defer f.Close()

	type lineEntry struct {
		Number  int    `json:"number"`
		Content string `json:"content"`
	}

	var lines []lineEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1_000_000)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if lineNum > endLine {
			break
		}
		lines = append(lines, lineEntry{
			Number:  lineNum,
			Content: scanner.Text(),
		})
	}

	totalLines := lineNum

	result, _ := json.Marshal(map[string]any{
		"path":       params.Path,
		"start_line": startLine,
		"end_line":   endLine,
		"total_lines": totalLines,
		"count":      len(lines),
		"lines":      lines,
	})
	return json.RawMessage(result), nil
}

// ---------------------------------------------------------------------------
// discoverExtensions — mengumpulkan ekstensi file unik dari project
// ---------------------------------------------------------------------------

type ExtCount struct {
	Extension string `json:"extension"`
	Count     int    `json:"count"`
}

// DiscoverExtensions memindai direktori dan mengembalikan daftar ekstensi
// file unik beserta jumlahnya. Melewati hidden dirs, binary, dan file >1MB.
func DiscoverExtensions(allowedDir string) []ExtCount {
	skipDirs := map[string]bool{".git": true, "node_modules": true, ".claude": true, "vendor": true, "dist": true}
	extMap := make(map[string]int)

	filepath.WalkDir(allowedDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip hidden files
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		// Skip file > 1MB
		fi, _ := d.Info()
		if fi != nil && fi.Size() > 1_000_000 {
			return nil
		}
		// Skip binary (cek null byte di 512 byte pertama)
		if fi != nil && fi.Size() > 0 {
			f, err := os.Open(path)
			if err == nil {
				buf := make([]byte, 512)
				n, _ := f.Read(buf)
				f.Close()
				for i := 0; i < n; i++ {
					if buf[i] == 0 {
						return nil
					}
				}
			}
		}
		// Ambil ekstensi
		ext := filepath.Ext(d.Name())
		if ext != "" {
			extMap[ext]++
		}
		return nil
	})

	var result []ExtCount
	for ext, count := range extMap {
		result = append(result, ExtCount{Extension: ext, Count: count})
	}

	// Sort: yang paling banyak dulu
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Count < result[j].Count ||
				(result[i].Count == result[j].Count && result[i].Extension > result[j].Extension) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// FormatExtensions menghasilkan string deskripsi ekstensi yang ditemukan.
func FormatExtensions(exts []ExtCount) string {
	if len(exts) == 0 {
		return "(tidak ada file non-biner ditemukan)"
	}
	var parts []string
	for _, e := range exts {
		parts = append(parts, fmt.Sprintf("%s (%d file)", e.Extension, e.Count))
	}
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolvePath mengamankan path agar hanya bisa mengakses file dalam allowedDir.
func (t *CreateDirTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *WriteFileTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *EditFileTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *ReadFileTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *ListFilesTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *FindFilesTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *SearchTextTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func (t *ReadFileLinesTool) resolvePath(relPath string) (string, error) {
	return resolveSafePath(t.allowedDir, relPath)
}

func resolveSafePath(allowedDir, relPath string) (string, error) {
	// Bersihkan path
	clean := filepath.Clean(relPath)

	// Tolak path traversal
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("path traversal tidak diizinkan: %s", relPath)
	}

	// Resolve absolute path dari allowedDir
	absAllowed, err := filepath.Abs(allowedDir)
	if err != nil {
		return "", fmt.Errorf("gagal resolve allowedDir: %w", err)
	}

	var fullPath string
	if filepath.IsAbs(clean) {
		fullPath = clean
	} else {
		fullPath = filepath.Join(absAllowed, clean)
	}

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
