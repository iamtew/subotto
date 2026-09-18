package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	c := New("test-key", "")
	c.baseURL = srv.URL
	got, err := c.Chat(context.Background(), "you are hesh", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ollie it" {
		t.Fatalf("got %q", got)
	}
}

func TestChatUnconfigured(t *testing.T) {
	c := New("", DefaultModel)
	if c.Configured() {
		t.Fatal("empty key should not be configured")
	}
	if _, err := c.Chat(context.Background(), "sys", "hi"); err == nil {
		t.Fatal("expected error")
	}
}

func TestChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
	}))
	t.Cleanup(srv.Close)
	c := New("test-key", DefaultModel)
	c.baseURL = srv.URL
	_, err := c.Chat(context.Background(), "sys", "hi")
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
	c := New("test-key", DefaultModel)
	c.baseURL = srv.URL
	_, err := c.Chat(context.Background(), "sys", "hi")
	if err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("err=%v", err)
	}
	if HTTPStatus(err) != http.StatusUnauthorized {
		t.Fatalf("status=%d", HTTPStatus(err))
	}
}
