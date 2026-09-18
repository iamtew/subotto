package discord

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/ai"
	"subotto/internal/db"
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
	mentioned := heshMentioned(botID, m.Mentions)
	replied := b.heshReplyToSelf(s, m)
	if !mentioned && !replied {
		return
	}
	trigger := "discord_reply"
	if mentioned {
		trigger = "discord_mention"
	}
	userText := heshUserText(m.Content, botID)
	author := ""
	if m.Author != nil {
		author = m.Author.Username
	}
	channelID := m.ChannelID
	messageID := m.ID

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
		b.logHesh(context.Background(), trigger, author, channelID, userText, "", 0, fmtAIKeyMissing(), nil)
		return
	}

	// OpenRouter can take a few seconds — don't stall YouTube/picture chrome.
	go func() {
		aiCtx, cancel := context.WithTimeout(context.Background(), heshChatTimeout)
		defer cancel()

		prompt, err := b.store.AISystemPrompt(aiCtx)
		if err != nil {
			slog.Error("hesh helper prompt load failed", "err", err)
			b.logHesh(aiCtx, trigger, author, channelID, userText, "", 0, err, nil)
			return
		}
		reply, err := b.ai.Chat(aiCtx, prompt, userText)
		if err != nil {
			slog.Error("hesh helper chat failed", "err", err)
			b.logHesh(aiCtx, trigger, author, channelID, userText, "", ai.HTTPStatus(err), err, nil)
			return
		}
		reply = truncateRunes(strings.TrimSpace(reply), heshMaxReplyRunes)
		if reply == "" {
			empty := errors.New("openrouter returned an empty reply")
			b.logHesh(aiCtx, trigger, author, channelID, userText, "", 0, empty, nil)
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
			b.logHesh(aiCtx, trigger, author, channelID, userText, reply, 0, nil, err)
			return
		}
		b.logHesh(aiCtx, trigger, author, channelID, userText, reply, 0, nil, nil)
	}()
}

func fmtAIKeyMissing() error {
	return &ai.APIError{Msg: "OPENROUTER_API_KEY is not set"}
}

func (b *Bot) logHesh(ctx context.Context, trigger, author, channelID, userText, reply string, httpStatus int, apiErr, sendErr error) {
	if b == nil || b.store == nil {
		return
	}
	errText := ""
	if apiErr != nil {
		errText = apiErr.Error()
	} else if sendErr != nil {
		errText = sendErr.Error()
	}
	_ = b.store.LogAIRequest(ctx, db.AIRequestEntry{
		Level:       db.ClassifyAILevel(httpStatus, apiErr, sendErr),
		Trigger:     trigger,
		Author:      author,
		ChannelID:   channelID,
		UserMessage: userText,
		Reply:       reply,
		Error:       errText,
		HTTPStatus:  httpStatus,
	})
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
