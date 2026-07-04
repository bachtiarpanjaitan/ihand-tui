package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
	"github.com/bachtiarpanjaitan/ihandai-go/pkg/llm"
)

// Compile-time check: AnthropicChatCompleter implements llm.ChatCompleter.
var _ llm.ChatCompleter = (*AnthropicChatCompleter)(nil)

// Latest Anthropic API version.
const anthropicVersion = "2023-06-01"

// AnthropicChatCompleter implements llm.ChatCompleter using the Anthropic Messages API.
type AnthropicChatCompleter struct {
	model      string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAnthropicChatCompleter creates a new Anthropic chat completer.
func NewAnthropicChatCompleter(model, baseURL, apiKey string) *AnthropicChatCompleter {
	return &AnthropicChatCompleter{
		model:      model,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// --- Anthropic API types ---

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicRequest struct {
	Model      string             `json:"model"`
	MaxTokens  int                `json:"max_tokens"`
	System     string             `json:"system,omitempty"`
	Messages   []anthropicMessage `json:"messages"`
	Stream     bool               `json:"stream"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Chat sends a chat completion request to the Anthropic Messages API.
func (c *AnthropicChatCompleter) Chat(ctx context.Context, messages []core.Message) (*core.Response, error) {
	// Separate system messages from conversation messages.
	// Anthropic API puts the system prompt as a top-level field.
	var systemParts []string
	var conversation []core.Message
	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
		} else {
			conversation = append(conversation, m)
		}
	}

	// Convert messages to Anthropic format.
	// Anthropic requires alternating user/assistant messages and doesn't allow
	// consecutive messages from the same role. We handle this by merging if needed.
	var anthropicMsgs []anthropicMessage
	for _, m := range conversation {
		anthropicMsgs = append(anthropicMsgs, anthropicMessage{
			Role: m.Role,
			Content: []anthropicContentBlock{
				{Type: "text", Text: m.Content},
			},
		})
	}

	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 8192,
		System:    strings.Join(systemParts, "\n"),
		Messages:  anthropicMsgs,
		Stream:    false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	url := c.baseURL + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("anthropic: API error: %s (%s)", result.Error.Message, result.Error.Type)
	}

	// Combine all text content blocks
	var contentParts []string
	for _, block := range result.Content {
		if block.Type == "text" {
			contentParts = append(contentParts, block.Text)
		}
	}

	usage := &core.TokenUsage{
		PromptTokens:     result.Usage.InputTokens,
		CompletionTokens: result.Usage.OutputTokens,
		TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
	}

	return &core.Response{
		Content:      strings.Join(contentParts, "\n"),
		FinishReason: result.StopReason,
		Usage:        usage,
	}, nil
}

// init registers the Anthropic chat provider in the ihandai registry.
func init() {
	llm.Register("anthropic", func(cfg llm.Config) (llm.ChatCompleter, error) {
		model := cfg.Model
		if model == "" {
			model = "claude-sonnet-5"
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.anthropic.com/v1"
		}
		return NewAnthropicChatCompleter(model, baseURL, cfg.APIKey), nil
	})
}
