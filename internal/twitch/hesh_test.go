package twitch

import (
	"fmt"
	"path/filepath"
	"testing"

	"subotto/internal/ai"
	"subotto/internal/db"
)

func TestMemoryTurnsSkipsCurrent(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	if err := store.SetAIMemoryEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAIMemoryWindow(ctx, 8); err != nil {
		t.Fatal(err)
	}
	c := New(Options{Store: store, AI: ai.New("")})
	c.remember("alice", "hi", false)
	c.remember("bob", "@subotto yo", false)
	turns := c.memoryTurns(ctx)
	if len(turns) != 1 {
		t.Fatalf("turns=%#v", turns)
	}
	if turns[0].Role != "user" || turns[0].Content != "alice: hi" {
		t.Fatalf("got %+v", turns[0])
	}
}

func TestAddressedMentionAndReply(t *testing.T) {
	mentioned := hasAtName("yo @heshbot", "heshbot")
	replied := "heshbot" == "heshbot"
	if !mentioned || !replied {
		t.Fatal("gate")
	}
	if hasAtName("yo heshbot", "heshbot") {
		t.Fatal("bare name is not a mention")
	}
}

func TestChatLogKeepsLast10(t *testing.T) {
	c := New(Options{})
	for i := 0; i < 12; i++ {
		c.rememberChat("Foo", ChatMessage{Content: fmt.Sprintf("%d", i)})
	}
	got := c.ListRecentMessages("foo", 10)
	if len(got) != 10 || got[0].Content != "2" || got[9].Content != "11" {
		t.Fatalf("%+v", got)
	}
	raw := `@display-name=Cool;id=abc;tmi-sent-ts=1700000000000 :viewer!viewer@viewer.tmi.twitch.tv PRIVMSG #foo :hello`
	msg, ok := parseIRCLine(raw)
	if !ok {
		t.Fatal("parse")
	}
	c.handlePRIVMSG(msg, "bot")
	got = c.ListRecentMessages("foo", 10)
	last := got[len(got)-1]
	if last.Author != "Cool" || last.Content != "hello" || last.Self {
		t.Fatalf("%+v", last)
	}
}
