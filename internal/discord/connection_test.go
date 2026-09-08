package discord

import (
	"testing"
	"time"
)

func TestMarkUpAndMarkDown(t *testing.T) {
	b := &Bot{}
	if b.Connected() {
		t.Fatal("new bot should start disconnected until markUp")
	}

	b.markDown("test")
	if b.Connected() {
		t.Fatal("markDown should clear ready")
	}
	downAt := time.Unix(0, b.lastDownUnix.Load())
	if downAt.IsZero() || time.Since(downAt) > time.Second {
		t.Fatalf("expected recent lastDownUnix, got %v", downAt)
	}

	b.markUp()
	if !b.Connected() {
		t.Fatal("markUp should set ready")
	}

	b.markDown("again")
	if b.Connected() {
		t.Fatal("second markDown should clear ready")
	}
}

func TestConnectedNilBot(t *testing.T) {
	var b *Bot
	if b.Connected() {
		t.Fatal("nil bot must report disconnected")
	}
}
