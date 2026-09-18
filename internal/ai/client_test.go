package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var req chatRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatal(err)
		}
		if req.Model != DefaultModel || len(req.Messages) != 2 {
			t.Fatalf("request: %+v", req)
		}
		if req.Messages[0].Role != "system" || req.Messages[1].Content != "hello" {
			t.Fatalf("messages: %+v", req.Messages)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  ollie it  "}}]}`))
	}))
	t.Cleanup(srv.Close)

	c := New("test-key")
	c.baseURL = srv.URL
	got, err := c.Chat(context.Background(), "you are hesh", "hello", DefaultModel, Sampling{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ollie it" {
		t.Fatalf("got %q", got)
	}
}

func TestChatContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := c.Chat(ctx, "sys", "hi", DefaultModel, Sampling{})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
}

func TestChatUnconfigured(t *testing.T) {
	c := New("")
	if c.Configured() {
		t.Fatal("empty key should not be configured")
	}
	if _, err := c.Chat(context.Background(), "sys", "hi", DefaultModel, Sampling{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL
	_, err := c.Chat(context.Background(), "sys", "hi", DefaultModel, Sampling{})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err=%v", err)
	}
	if HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("status=%d", HTTPStatus(err))
	}
}

func TestChatHTTPUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL
	_, err := c.Chat(context.Background(), "sys", "hi", DefaultModel, Sampling{})
	if err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("err=%v", err)
	}
	if HTTPStatus(err) != http.StatusUnauthorized {
		t.Fatalf("status=%d", HTTPStatus(err))
	}
}

func TestChatSamplingOmitsZeros(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL
	if _, err := c.Chat(context.Background(), "sys", "hi", DefaultModel, DefaultSampling()); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"max_tokens", "top_k", "min_p", "top_a"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("expected %s omitted, got %#v", key, payload)
		}
	}
	if payload["temperature"] != 0.7 || payload["top_p"] != 1.0 || payload["repetition_penalty"] != 1.0 {
		t.Fatalf("defaults: %#v", payload)
	}

	if _, err := c.Chat(context.Background(), "sys", "hi", DefaultFlashModel, Sampling{
		MaxTokens:         128,
		Temperature:       0.2,
		TopP:              0.8,
		TopK:              12,
		FrequencyPenalty:  0.3,
		PresencePenalty:   0.4,
		RepetitionPenalty: 1.2,
		MinP:              0.05,
		TopA:              0.1,
	}); err != nil {
		t.Fatal(err)
	}
	if payload["max_tokens"] != float64(128) || payload["top_k"] != float64(12) {
		t.Fatalf("ints: %#v", payload)
	}
	if payload["min_p"] != 0.05 || payload["top_a"] != 0.1 || payload["temperature"] != 0.2 {
		t.Fatalf("floats: %#v", payload)
	}
	if payload["model"] != DefaultFlashModel {
		t.Fatalf("model: %#v", payload["model"])
	}
}
