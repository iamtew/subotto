package twitch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseIRCLinePRIVMSGTags(t *testing.T) {
	raw := `@badge-info=;display-name=CoolName;id=abc-123;reply-parent-user-login=subotto :viewer!viewer@viewer.tmi.twitch.tv PRIVMSG #otherchan :hello @subotto there`
	msg, ok := parseIRCLine(raw)
	if !ok {
		t.Fatal("parse failed")
	}
	if msg.Command != "PRIVMSG" || msg.Nick != "viewer" {
		t.Fatalf("cmd/nick: %+v", msg)
	}
	if msg.text() != "hello @subotto there" {
		t.Fatalf("text %q", msg.text())
	}
	if msg.tag("display-name") != "CoolName" || msg.tag("id") != "abc-123" {
		t.Fatalf("tags %+v", msg.Tags)
	}
	if msg.tag("reply-parent-user-login") != "subotto" {
		t.Fatalf("parent login %q", msg.tag("reply-parent-user-login"))
	}
}

func TestParseIRCLinePING(t *testing.T) {
	msg, ok := parseIRCLine("PING :tmi.twitch.tv\r\n")
	if !ok || msg.Command != "PING" || msg.text() != "tmi.twitch.tv" {
		t.Fatalf("%+v", msg)
	}
}

func TestUnescapeTag(t *testing.T) {
	if got := unescapeTag(`foo\sbar\:baz`); got != "foo bar;baz" {
		t.Fatalf("got %q", got)
	}
}

func TestHasAtName(t *testing.T) {
	if !hasAtName("hey @SubOtto help", "subotto") {
		t.Fatal("expected mention")
	}
	if hasAtName("hey @subottobot", "subotto") {
		t.Fatal("prefix should not match")
	}
	if !hasAtName("@Cool_Name?", "Cool_Name") {
		t.Fatal("display mention")
	}
}

func TestUserTextStripsMention(t *testing.T) {
	if got := userText("@subotto what's up", "subotto", "SubOtto"); got != "what's up" {
		t.Fatalf("got %q", got)
	}
	if got := userText("@subotto", "subotto", "SubOtto"); got != "hey" {
		t.Fatalf("empty leftover: %q", got)
	}
}

func TestNormalizeChannel(t *testing.T) {
	if got := NormalizeChannel(" #Cool_Chan "); got != "cool_chan" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeChannel("bad chan"); got != "" {
		t.Fatalf("invalid should empty, got %q", got)
	}
	out, err := ParseChannel("#Ok_1")
	if err != nil || out != "ok_1" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := ParseChannel("nope!"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPrivmsgOut(t *testing.T) {
	got := privmsgOut("Other", "hi\nthere", "id-1")
	if got != "@reply-parent-msg-id=id-1 PRIVMSG #other :hi there" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("x", 600)
	out := privmsgOut("c", long, "")
	if !strings.HasPrefix(out, "PRIVMSG #c :") {
		t.Fatalf("%q", out)
	}
	body := strings.TrimPrefix(out, "PRIVMSG #c :")
	if len([]rune(body)) != heshMaxReplyRunes {
		t.Fatalf("len %d", len([]rune(body)))
	}
}

func TestFetchUserHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Client-Id") != "cid" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("headers %+v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"login":"MyBot","display_name":"MyBot"}]}`))
	}))
	defer srv.Close()
	u, err := fetchUserHTTP(t.Context(), srv.Client(), srv.URL, "cid", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if u.Login != "mybot" || u.DisplayName != "MyBot" {
		t.Fatalf("%+v", u)
	}
}
