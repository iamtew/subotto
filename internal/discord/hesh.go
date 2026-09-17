package discord

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

const heshChatTimeout = 30 * time.Second

// Discord's message cap. Truncate model output rather than fail the send.
const heshMaxReplyRunes = 2000

// handleHeshHelper replies when Subotto is @mentioned or someone replies to it.
// Listeners still run on the same message — this path is independent.
func (b *Bot) handleHeshHelper(_ context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	if s == nil || m == nil || s.State == nil || s.State.User == nil {
		return
	}
	botID := s.State.User.ID
	if !heshMentioned(botID, m.Mentions) && !b.heshReplyToSelf(s, m) {
		return
	}
	on, err := b.store.AIEnabled(context.Background())
	if err != nil {
		slog.Error("hesh helper enabled check failed", "err", err)
		return
	}
	if !on {
		slog.Debug("hesh helper skipped — disabled in Admin")
		return
	}
	if b.ai == nil || !b.ai.Configured() {
		slog.Debug("hesh helper skipped — OPENROUTER_API_KEY not set")
		return
	}

	userText := heshUserText(m.Content, botID)
	channelID := m.ChannelID
	messageID := m.ID

	// OpenRouter can take a few seconds — don't stall YouTube/picture chrome.
	go func() {
		aiCtx, cancel := context.WithTimeout(context.Background(), heshChatTimeout)
		defer cancel()

		prompt, err := b.store.AISystemPrompt(aiCtx)
		if err != nil {
			slog.Error("hesh helper prompt load failed", "err", err)
			return
		}
		reply, err := b.ai.Chat(aiCtx, prompt, userText)
		if err != nil {
			slog.Error("hesh helper chat failed", "err", err)
			return
		}
		reply = truncateRunes(strings.TrimSpace(reply), heshMaxReplyRunes)
		if reply == "" {
			return
		}
		_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content: reply,
			Reference: &discordgo.MessageReference{
				MessageID: messageID,
				ChannelID: channelID,
			},
		})
		if err != nil {
			slog.Error("hesh helper reply failed", "channel", channelID, "err", err)
		}
	}()
}

func heshMentioned(botID string, mentions []*discordgo.User) bool {
	for _, u := range mentions {
		if u != nil && u.ID == botID {
			return true
		}
	}
	return false
}

func (b *Bot) heshReplyToSelf(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if m.MessageReference == nil || m.MessageReference.MessageID == "" {
		return false
	}
	botID := s.State.User.ID
	if m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil {
		return m.ReferencedMessage.Author.ID == botID
	}
	msg, err := s.ChannelMessage(m.ChannelID, m.MessageReference.MessageID)
	if err != nil || msg == nil || msg.Author == nil {
		return false
	}
	return msg.Author.ID == botID
}

// heshUserText strips the bot mention so the model sees the human words.
func heshUserText(content, botID string) string {
	s := content
	if botID != "" {
		s = strings.ReplaceAll(s, "<@"+botID+">", "")
		s = strings.ReplaceAll(s, "<@!"+botID+">", "")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "hey"
	}
	return s
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max < 4 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
