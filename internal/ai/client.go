// Package ai talks to an OpenAI-compatible chat API (OpenRouter today).
//
// Meat Bag: only the base URL, API key, and model name are provider-specific.
// Swap those and the rest of Subotto stays the same.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultModel   = "nvidia/nemotron-3-ultra-550b-a55b:free"
	defaultBaseURL = "https://openrouter.ai/api/v1"
	httpTimeout    = 45 * time.Second
)

// Client is a tiny chat-completions caller.
type Client struct {
	key     string
	model   string
	baseURL string
	http    *http.Client
}

// New builds a client. Empty key means Configured() is false (AI stays off).
func New(key, model string) *Client {
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		key:     strings.TrimSpace(key),
		model:   model,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: httpTimeout},
	}
}

// Configured is true when an API key is present.
func (c *Client) Configured() bool {
	return c != nil && c.key != ""
}

// Model is the OpenRouter model id from env (read-only in Admin).
func (c *Client) Model() string {
	if c == nil || c.model == "" {
		return DefaultModel
	}
	return c.model
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
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
func (c *Client) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("OPENROUTER_API_KEY is not set")
	}
	systemPrompt = strings.TrimSpace(systemPrompt)
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", fmt.Errorf("message is empty")
	}

	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
	})
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
		return "", fmt.Errorf("openrouter: %w", err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read openrouter response: %w", err)
	}

	var parsed chatResponse
	_ = json.Unmarshal(raw, &parsed)
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", fmt.Errorf("openrouter: %s", parsed.Error.Message)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = res.Status
		}
		return "", fmt.Errorf("openrouter HTTP %d: %s", res.StatusCode, msg)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openrouter returned no choices")
	}
	reply := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if reply == "" {
		return "", fmt.Errorf("openrouter returned an empty reply")
	}
	return reply, nil
}
