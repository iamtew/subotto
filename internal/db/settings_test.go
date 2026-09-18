package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"subotto/internal/ai"
)

func TestListenAnnounceMessagesDefaultsAndSave(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	start, stop, err := store.ListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start == "" || stop == "" {
		t.Fatal("expected default start/stop messages")
	}
	if err := store.SetSettings(ctx, map[string]string{
		SettingListenStartMessage: "ONLINE {{name}}",
		SettingListenStopMessage:  "OFFLINE {{channel_id}}",
	}); err != nil {
		t.Fatal(err)
	}
	start, stop, err = store.ListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != "ONLINE {{name}}" || stop != "OFFLINE {{channel_id}}" {
		t.Fatalf("got start=%q stop=%q", start, stop)
	}
}

func TestPictureListenAnnounceMessagesDefaultsAndSave(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "picture-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	start, stop, err := store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != DefaultPictureListenStartMessage || stop != DefaultPictureListenStopMessage {
		t.Fatalf("expected picture defaults, got start=%q stop=%q", start, stop)
	}
	if err := store.SetSettings(ctx, map[string]string{
		SettingPictureListenStartMessage: "PICS ONLINE {{slug}}",
		SettingPictureListenStopMessage:  "PICS OFFLINE {{name}}",
	}); err != nil {
		t.Fatal(err)
	}
	start, stop, err = store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != "PICS ONLINE {{slug}}" || stop != "PICS OFFLINE {{name}}" {
		t.Fatalf("got start=%q stop=%q", start, stop)
	}
}

func TestAnnounceSilenceVsDefault(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "silence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	// Missing keys → defaults.
	start, stop, err := store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start == "" || stop == "" {
		t.Fatal("missing keys should use defaults, not silence")
	}

	// Saved empty → silence.
	if err := store.SetSettings(ctx, map[string]string{
		SettingPictureListenStartMessage: "",
		SettingPictureListenStopMessage:  "",
	}); err != nil {
		t.Fatal(err)
	}
	start, stop, err = store.PictureListenAnnounceMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start != "" || stop != "" {
		t.Fatalf("saved empty should silence, got start=%q stop=%q", start, stop)
	}
}

func TestValidateAnnounceTemplate(t *testing.T) {
	if err := ValidateAnnounceTemplate("ONLINE notice", "ok"); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("a", MaxAnnounceTemplateLength+1)
	if err := ValidateAnnounceTemplate("ONLINE notice", huge); err == nil {
		t.Fatal("expected oversize error")
	}
	if NormalizeAnnounceTemplate("  \n\t  ") != "" {
		t.Fatal("whitespace-only should normalize to empty")
	}
}

func TestResyncSchedulerLoadSaveAndWants(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "sched.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	got, err := store.LoadResyncScheduler(ctx, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got.IntervalHours != 6 || got.Limit != DefaultResyncLimit || got.Scope != ResyncScopeAll {
		t.Fatalf("env fallback: %+v", got)
	}

	saved, err := store.SaveResyncScheduler(ctx, ResyncScheduler{
		IntervalHours: 3,
		Limit:         25,
		Scope:         ResyncScopeSelected,
		Targets: []ResyncTarget{
			{Kind: ResyncKindContent, ChannelID: "111"},
			{Kind: ResyncKindPicture, ChannelID: "111"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Wants(ResyncKindContent, "111") || saved.Wants(ResyncKindContent, "222") {
		t.Fatalf("wants: %+v", saved)
	}

	again, err := store.LoadResyncScheduler(ctx, 99)
	if err != nil {
		t.Fatal(err)
	}
	if again.IntervalHours != 3 || again.Limit != 25 || again.Scope != ResyncScopeSelected {
		t.Fatalf("db wins over env: %+v", again)
	}
	if len(again.Targets) != 2 {
		t.Fatalf("targets: %+v", again.Targets)
	}
}

func TestAISystemPromptDefaultAndSave(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	got, err := store.AISystemPrompt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultAISystemPrompt {
		t.Fatalf("default: %q", got)
	}

	if err := store.SetSetting(ctx, SettingAISystemPrompt, "keep it short"); err != nil {
		t.Fatal(err)
	}
	got, err = store.AISystemPrompt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "keep it short" {
		t.Fatalf("saved: %q", got)
	}

	if err := store.SetSetting(ctx, SettingAISystemPrompt, "   "); err != nil {
		t.Fatal(err)
	}
	got, err = store.AISystemPrompt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultAISystemPrompt {
		t.Fatalf("empty should fall back, got %q", got)
	}
}

func TestAIEnabledDefaultAndSave(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai-enabled.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	on, err := store.AIEnabled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("default should be enabled")
	}
	if err := store.SetAIEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	on, err = store.AIEnabled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("expected disabled")
	}
}

func TestAISamplingDefaultRoundTripAndClamp(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai-sampling.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	got, err := store.LoadAISampling(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := ai.DefaultSampling()
	if got != want {
		t.Fatalf("default: %+v want %+v", got, want)
	}

	saved, err := store.SaveAISampling(ctx, ai.Sampling{
		MaxTokens:         256,
		Temperature:       0.4,
		TopP:              0.9,
		TopK:              40,
		FrequencyPenalty:  0.1,
		PresencePenalty:   -0.2,
		RepetitionPenalty: 1.1,
		MinP:              0.05,
		TopA:              0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.LoadAISampling(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again != saved {
		t.Fatalf("round-trip: %+v vs %+v", again, saved)
	}

	clamped, err := store.SaveAISampling(ctx, ai.Sampling{
		MaxTokens:         99999,
		Temperature:       -1,
		TopP:              2,
		TopK:              999,
		FrequencyPenalty:  9,
		PresencePenalty:   -9,
		RepetitionPenalty: 9,
		MinP:              -1,
		TopA:              4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clamped.MaxTokens != ai.MaxSamplingTokens || clamped.TopK != ai.MaxSamplingTopK {
		t.Fatalf("int clamp: %+v", clamped)
	}
	if clamped.Temperature != 0 || clamped.TopP != 1 || clamped.MinP != 0 || clamped.TopA != 1 {
		t.Fatalf("float clamp: %+v", clamped)
	}
	if clamped.FrequencyPenalty != 2 || clamped.PresencePenalty != -2 || clamped.RepetitionPenalty != 2 {
		t.Fatalf("penalty clamp: %+v", clamped)
	}

	if err := store.SetSetting(ctx, SettingAISampling, `{"temperature":0.2}`); err != nil {
		t.Fatal(err)
	}
	partial, err := store.LoadAISampling(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Temperature != 0.2 || partial.TopP != 1 || partial.RepetitionPenalty != 1 {
		t.Fatalf("partial json should keep defaults: %+v", partial)
	}
}
