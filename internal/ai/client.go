// Package ai talks to an OpenAI-compatible chat API (OpenRouter today).
//
// Meat Bag: only the base URL, API key, and model name are provider-specific.
// Swap those and the rest of Subotto stays the same.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIError is an OpenRouter (or local) chat failure with an HTTP status when one exists.
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

func apiErr(status int, format string, args ...any) error {
	return &APIError{Status: status, Msg: fmt.Sprintf(format, args...)}
}

// HTTPStatus returns the OpenRouter status from err, or 0.
func HTTPStatus(err error) int {
	var e *APIError
	if errors.As(err, &e) && e != nil {
		return e.Status
	}
	return 0
}

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	// ChatTimeout is the Discord + Admin + HTTP budget for one OpenRouter turn.
	ChatTimeout = 60 * time.Second
)

// Client is a tiny chat-completions caller.
type Client struct {
	key     string
	baseURL string
	http    *http.Client
}

// New builds a client. Empty key means Configured() is false (AI stays off).
func New(key string) *Client {
	return &Client{
		key:     strings.TrimSpace(key),
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: ChatTimeout},
	}
}

// Configured is true when an API key is present.
func (c *Client) Configured() bool {
	return c != nil && c.key != ""
}

type chatRequest struct {
	Model             string        `json:"model"`
	Messages          []chatMessage `json:"messages"`
	MaxTokens         *int          `json:"max_tokens,omitempty"`
	Temperature       *float64      `json:"temperature,omitempty"`
	TopP              *float64      `json:"top_p,omitempty"`
	TopK              *int          `json:"top_k,omitempty"`
	FrequencyPenalty  *float64      `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64      `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float64      `json:"repetition_penalty,omitempty"`
	MinP              *float64      `json:"min_p,omitempty"`
	TopA              *float64      `json:"top_a,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat sends one user turn plus a system prompt. No conversation memory.
func (c *Client) Chat(ctx context.Context, systemPrompt, userMessage, model string, sampling Sampling) (string, error) {
	if !c.Configured() {
		return "", apiErr(0, "OPENROUTER_API_KEY is not set")
	}
	systemPrompt = strings.TrimSpace(systemPrompt)
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", fmt.Errorf("message is empty")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}

	reqBody := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
	}
	sampling.apply(&reqBody)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://subotto.local")
	req.Header.Set("X-Title", "Subotto")

	res, err := c.http.Do(req)
	if err != nil {
		return "", wrapChatErr(ctx, "openrouter", err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", wrapChatErr(ctx, "read openrouter response", err)
	}

	var parsed chatResponse
	_ = json.Unmarshal(raw, &parsed)
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", apiErr(res.StatusCode, "openrouter: %s", parsed.Error.Message)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = res.Status
		}
		return "", apiErr(res.StatusCode, "openrouter HTTP %d: %s", res.StatusCode, msg)
	}
	if len(parsed.Choices) == 0 {
		return "", apiErr(res.StatusCode, "openrouter returned no choices")
	}
	reply := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if reply == "" {
		return "", apiErr(res.StatusCode, "openrouter returned an empty reply")
	}
	return reply, nil
}

func wrapChatErr(ctx context.Context, prefix string, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return fmt.Errorf("openrouter timed out after %s: %w", ChatTimeout, err)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
