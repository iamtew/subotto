package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestClassifyAILevel(t *testing.T) {
	cases := []struct {
		status  int
		apiErr  error
		sendErr error
		want    string
	}{
		{0, nil, nil, AILevelInfo},
		{0, nil, errors.New("discord 403"), AILevelWarning},
		{429, errors.New("rate"), nil, AILevelWarning},
		{200, errors.New("openrouter returned an empty reply"), nil, AILevelWarning},
		{401, errors.New("unauthorized"), nil, AILevelCritical},
		{403, errors.New("forbidden"), nil, AILevelCritical},
		{402, errors.New("payment"), nil, AILevelCritical},
		{0, errors.New("OPENROUTER_API_KEY is not set"), nil, AILevelCritical},
		{500, errors.New("oops"), nil, AILevelError},
		{0, errors.New("openrouter: timeout"), nil, AILevelError},
	}
	for _, tc := range cases {
		got := ClassifyAILevel(tc.status, tc.apiErr, tc.sendErr)
		if got != tc.want {
			t.Fatalf("status=%d api=%v send=%v: got %q want %q", tc.status, tc.apiErr, tc.sendErr, got, tc.want)
		}
	}
}

func TestLogAndListAIRequestsFilterAndKeep(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai-log.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	if err := store.LogAIRequest(ctx, AIRequestEntry{Level: AILevelInfo, Trigger: "admin_chat", UserMessage: "hi", Reply: "yo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.LogAIRequest(ctx, AIRequestEntry{Level: AILevelWarning, Trigger: "discord_mention", Error: "429"}); err != nil {
		t.Fatal(err)
	}
	if err := store.LogAIRequest(ctx, AIRequestEntry{Level: AILevelError, Trigger: "admin_chat", Error: "500"}); err != nil {
		t.Fatal(err)
	}
	if err := store.LogAIRequest(ctx, AIRequestEntry{Level: AILevelCritical, Trigger: "admin_chat", Error: "no key"}); err != nil {
		t.Fatal(err)
	}

	all, err := store.ListAIRequests(ctx, "all", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("all=%d", len(all))
	}
	warn, err := store.ListAIRequests(ctx, "warning", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(warn) != 3 {
		t.Fatalf("warning+=%d", len(warn))
	}
	crit, err := store.ListAIRequests(ctx, "critical", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(crit) != 1 || crit[0].Error != "no key" {
		t.Fatalf("critical=%+v", crit)
	}

	for i := 0; i < aiRequestLogKeep+5; i++ {
		if err := store.LogAIRequest(ctx, AIRequestEntry{Level: AILevelInfo, Trigger: "admin_chat", UserMessage: fmt.Sprintf("n%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	kept, err := store.ListAIRequests(ctx, "all", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != aiRequestLogKeep {
		t.Fatalf("kept %d want %d", len(kept), aiRequestLogKeep)
	}
}
