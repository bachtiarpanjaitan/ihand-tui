package providers

import (
	"bufio"
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

// Compile-time checks
var _ llm.ChatCompleter = (*AnthropicChatCompleter)(nil)
var _ llm.StreamCompleter = (*AnthropicChatCompleter)(nil)

// Latest Anthropic API version.
const anthropicVersion = "2023-06-01"

// AnthropicChatCompleter implements llm.ChatCompleter and llm.StreamCompleter
// using the Anthropic Messages API.
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
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// anthropicStreamEvent represents an SSE event from the Anthropic streaming API.
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta *struct {
		Type       string `json:"type,omitempty"`
		Text       string `json:"text,omitempty"`
		StopReason string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	ContentBlock *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content_block,omitempty"`
	Message *anthropicResponse `json:"message,omitempty"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// prepareAnthropicRequest builds the common request components.
func (c *AnthropicChatCompleter) prepareRequest(messages []core.Message, stream bool) (anthropicRequest, error) {
	var systemParts []string
	var conversation []core.Message
	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
		} else {
			conversation = append(conversation, m)
		}
	}

	var anthropicMsgs []anthropicMessage
	for _, m := range conversation {
		anthropicMsgs = append(anthropicMsgs, anthropicMessage{
			Role: m.Role,
			Content: []anthropicContentBlock{
				{Type: "text", Text: m.Content},
			},
		})
	}

	return anthropicRequest{
		Model:     c.model,
		MaxTokens: 8192,
		System:    strings.Join(systemParts, "\n"),
		Messages:  anthropicMsgs,
		Stream:    stream,
	}, nil
}

// Chat sends a chat completion request to the Anthropic Messages API.
func (c *AnthropicChatCompleter) Chat(ctx context.Context, messages []core.Message) (*core.Response, error) {
	reqBody, err := c.prepareRequest(messages, false)
	if err != nil {
		return nil, err
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

// ChatStream sends a streaming chat completion request to the Anthropic Messages API.
// Returns a channel of llm.Chunk that will receive tokens as they arrive.
func (c *AnthropicChatCompleter) ChatStream(ctx context.Context, messages []core.Message) (<-chan llm.Chunk, error) {
	reqBody, err := c.prepareRequest(messages, true)
	if err != nil {
		return nil, err
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan llm.Chunk)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()

			// Anthropic SSE format: "event: <type>\ndata: <json>"
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_delta":
				if event.Delta != nil && event.Delta.Text != "" {
					ch <- llm.Chunk{
						Content: event.Delta.Text,
					}
				}
			case "message_delta":
				if event.Delta != nil && event.Delta.StopReason != "" {
					ch <- llm.Chunk{
						FinishReason: event.Delta.StopReason,
					}
				}
			case "error":
				// Stream error, stop processing
				return
			case "message_stop":
				return
			}
		}
	}()

	return ch, nil
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
