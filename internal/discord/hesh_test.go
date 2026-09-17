package discord

import "testing"

func TestHeshUserText(t *testing.T) {
	const botID = "99"
	if got := heshUserText("<@99> kickflip?", botID); got != "kickflip?" {
		t.Fatalf("got %q", got)
	}
	if got := heshUserText("<@!99>", botID); got != "hey" {
		t.Fatalf("empty mention should fallback, got %q", got)
	}
	if got := heshUserText("no mention", botID); got != "no mention" {
		t.Fatalf("got %q", got)
	}
}

func TestHeshMentioned(t *testing.T) {
	if heshMentioned("99", nil) {
		t.Fatal("empty mentions")
	}
}
