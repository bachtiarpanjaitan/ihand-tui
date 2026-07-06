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
var _ llm.ChatCompleter = (*OpenAIChatCompleter)(nil)
var _ llm.StreamCompleter = (*OpenAIChatCompleter)(nil)

// OpenAIChatCompleter implements llm.ChatCompleter and llm.StreamCompleter
// using the OpenAI chat API. It works with any OpenAI-compatible provider
// (OpenAI, Groq, Together AI, etc.).
type OpenAIChatCompleter struct {
	model      string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIChatCompleter creates a new OpenAI chat completer.
func NewOpenAIChatCompleter(model, baseURL, apiKey string) *OpenAIChatCompleter {
	return &OpenAIChatCompleter{
		model:      model,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// --- OpenAI API types ---

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *core.TokenUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Chat sends a chat completion request to the OpenAI-compatible API.
func (c *OpenAIChatCompleter) Chat(ctx context.Context, messages []core.Message) (*core.Response, error) {
	// Convert messages to OpenAI format
	openaiMsgs := make([]openAIMessage, len(messages))
	for i, m := range messages {
		openaiMsgs[i] = openAIMessage{Role: m.Role, Content: m.Content}
	}

	reqBody := openAIChatRequest{
		Model:    c.model,
		Messages: openaiMsgs,
		Stream:   false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result openAIChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("openai: API error: %s (%s)", result.Error.Message, result.Error.Type)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	choice := result.Choices[0]
	return &core.Response{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage:        result.Usage,
	}, nil
}

// ChatStream sends a streaming chat completion request to the OpenAI-compatible API.
// Returns a channel of llm.Chunk that will receive tokens as they arrive.
func (c *OpenAIChatCompleter) ChatStream(ctx context.Context, messages []core.Message) (<-chan llm.Chunk, error) {
	openaiMsgs := make([]openAIMessage, len(messages))
	for i, m := range messages {
		openaiMsgs[i] = openAIMessage{Role: m.Role, Content: m.Content}
	}

	reqBody := openAIChatRequest{
		Model:    c.model,
		Messages: openaiMsgs,
		Stream:   true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("openai: HTTP %d", resp.StatusCode)
	}

	ch := make(chan llm.Chunk)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer for large streamed chunks
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			if line == "" || line == "data: [DONE]" {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.Error != nil {
				return
			}

			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				var fr string
				if choice.FinishReason != nil {
					fr = *choice.FinishReason
				}
				if choice.Delta.Content != "" || fr != "" {
					ch <- llm.Chunk{
						Content:      choice.Delta.Content,
						FinishReason: fr,
					}
				}
			}
		}
	}()

	return ch, nil
}

// init registers the OpenAI chat provider in the ihandai registry.
func init() {
	llm.Register("openai", func(cfg llm.Config) (llm.ChatCompleter, error) {
		model := cfg.Model
		if model == "" {
			model = "gpt-4o"
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return NewOpenAIChatCompleter(model, baseURL, cfg.APIKey), nil
	})
}
