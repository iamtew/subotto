package discord

import (
	"errors"
	"net/http"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestIsDiscordRateLimit(t *testing.T) {
	if !isDiscordRateLimit(&discordgo.RESTError{
		Response: &http.Response{StatusCode: http.StatusTooManyRequests},
	}) {
		t.Fatal("429 RESTError should count")
	}
	if !isDiscordRateLimit(errors.New("rate limit exceeded")) {
		t.Fatal("message containing rate limit should count")
	}
	if isDiscordRateLimit(errors.New("channel not found")) {
		t.Fatal("unrelated error should not count")
	}
}
