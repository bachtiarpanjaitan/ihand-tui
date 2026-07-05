package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
)

// ---------------------------------------------------------------------------
// BrowseTool — mengambil konten halaman web
// ---------------------------------------------------------------------------

type BrowseTool struct{}

func NewBrowseTool() *BrowseTool {
	return &BrowseTool{}
}

func (t *BrowseTool) Name() string { return "browse" }

func (t *BrowseTool) Description() string {
	return "Mengambil konten halaman web dari URL. " +
		"Gunakan untuk membaca halaman web, dokumentasi, atau artikel. " +
		"Mengembalikan teks dari halaman (max 50KB). " +
		"Input: {\"url\": \"https://example.com\"}"
}

func (t *BrowseTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"url": {Type: "string", Description: "URL halaman web yang akan dibaca (harus diawali http:// atau https://)"},
		},
		Required: []string{"url"},
	}
}

func (t *BrowseTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "input tidak valid: %s"}`, err.Error())), nil
	}

	if params.URL == "" {
		return json.RawMessage(`{"error": "URL tidak boleh kosong"}`), nil
	}

	// Tambahkan https:// jika tidak ada skema
	if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
		params.URL = "https://" + params.URL
	}

	// HTTP client dengan timeout
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membuat request: %s"}`, err.Error())), nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; IhandTUI/1.0; +https://github.com/bachtiarpanjaitan/ihandtui)")
	req.Header.Set("Accept", "text/html,text/plain,*/*")

	resp, err := client.Do(req)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal mengakses URL: %s"}`, err.Error())), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return json.RawMessage(fmt.Sprintf(`{"error": "HTTP %d: %s"}`, resp.StatusCode, resp.Status)), nil
	}

	// Batasi pembacaan (max 50KB konten)
	limitReader := io.LimitReader(resp.Body, 50*1024)
	body, err := io.ReadAll(limitReader)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "gagal membaca konten: %s"}`, err.Error())), nil
	}

	content := string(body)

	// Konversi Content-Type untuk info
	contentType := resp.Header.Get("Content-Type")

	result, _ := json.Marshal(map[string]any{
		"url":         params.URL,
		"status":      resp.StatusCode,
		"content":     content,
		"contentType": contentType,
		"size":        len(content),
	})

	return json.RawMessage(result), nil
}
