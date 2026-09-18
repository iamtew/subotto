package discord

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/ai"
	"subotto/internal/db"
)

// Discord's message cap. Truncate model output rather than fail the send.
const heshMaxReplyRunes = 2000

const heshTypingEvery = 8 * time.Second

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
		b.logHesh(trigger, author, channelID, userText, "", 0, fmtAIKeyMissing(), nil)
		return
	}

	// OpenRouter can take a few seconds — don't stall YouTube/picture chrome.
	go func() {
		aiCtx, cancel := context.WithTimeout(context.Background(), ai.ChatTimeout)
		defer cancel()

		stopTyping := make(chan struct{})
		go keepHeshTyping(s, channelID, stopTyping)
		defer close(stopTyping)

		prompt, err := b.store.AISystemPrompt(aiCtx)
		if err != nil {
			slog.Error("hesh helper prompt load failed", "err", err)
			b.logHesh(trigger, author, channelID, userText, "", 0, err, nil)
			return
		}
		sampling, err := b.store.LoadAISampling(aiCtx)
		if err != nil {
			slog.Error("hesh helper sampling load failed", "err", err)
			b.logHesh(trigger, author, channelID, userText, "", 0, err, nil)
			return
		}
		reply, err := b.ai.Chat(aiCtx, prompt, userText, sampling)
		if err != nil {
			slog.Error("hesh helper chat failed", "err", err)
			b.logHesh(trigger, author, channelID, userText, "", ai.HTTPStatus(err), err, nil)
			fail := heshFailReply()
			if sendErr := heshReply(s, channelID, messageID, fail); sendErr != nil {
				slog.Error("hesh helper fail reply failed", "channel", channelID, "err", sendErr)
			}
			return
		}
		reply = truncateRunes(strings.TrimSpace(reply), heshMaxReplyRunes)
		if reply == "" {
			empty := errors.New("openrouter returned an empty reply")
			b.logHesh(trigger, author, channelID, userText, "", 0, empty, nil)
			return
		}
		if err := heshReply(s, channelID, messageID, reply); err != nil {
			slog.Error("hesh helper reply failed", "channel", channelID, "err", err)
			b.logHesh(trigger, author, channelID, userText, reply, 0, nil, err)
			return
		}
		b.logHesh(trigger, author, channelID, userText, reply, 0, nil, nil)
	}()
}

func fmtAIKeyMissing() error {
	return &ai.APIError{Msg: "OPENROUTER_API_KEY is not set"}
}

func keepHeshTyping(s *discordgo.Session, channelID string, stop <-chan struct{}) {
	if s == nil {
		return
	}
	_ = s.ChannelTyping(channelID)
	ticker := time.NewTicker(heshTypingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_ = s.ChannelTyping(channelID)
		}
	}
}

func heshReply(s *discordgo.Session, channelID, messageID, content string) error {
	_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: truncateRunes(strings.TrimSpace(content), heshMaxReplyRunes),
		Reference: &discordgo.MessageReference{
			MessageID: messageID,
			ChannelID: channelID,
		},
	})
	return err
}

var heshFailReplies = []string{
	"Brain blanked. Poke me again in a sec.",
	"The model went for a smoke. Try again shortly.",
	"I had the thought, then it timed out. Classic.",
	"OpenRouter hung up on me. Give it another go.",
	"That's a no from the cloud. Retry in a moment.",
	"I tripped over my own context window. Try again.",
	"The neurons are buffering. Hit me in a bit.",
	"I almost had it. Then the timeout laughed. Retry?",
	"Servers are doing a kickflip into a wall. Try again soon.",
	"My witty comeback expired in transit. One more time.",
	"That's a 408 from the universe. Ping me again.",
	"I stared into the void. The void timed out. Retry.",
	"Couldn't land it. Run it back in a moment.",
	"The AI took a bathroom break. Catch me in a sec.",
	"I dropped the thought like a board on a crack. Try again.",
	"Out of clever. Back in a moment if you ask again.",
	"Model's buffering like 2006 YouTube. Retry shortly.",
	"I started answering and then... nothing. Again?",
	"Temporary brain freeze. Not the fun kind. Try again.",
	"Lost the plot mid-sentence. Poke me again.",
	"The hamster fell off the wheel. Give it a second.",
	"Couldn't fetch a thought. Inventory's empty. Retry.",
	"That's a bail. Roll up and ask again in a bit.",
	"My inner monologue hit a paywall. Try again shortly.",
	"The punchline timed out. I'll get it next attempt.",
	"I was this close. Then the clock said no. Retry.",
	"Cloud said \"brb\". It lied. Ask again in a moment.",
	"Error: charm unavailable. Please insert another mention.",
	"I fumbled the reply. Not my proudest ollie. Try again.",
	"Timed out waiting for my own brain. Hit me soon.",
	"That one got lost between here and the model. Retry.",
	"I plead timeout. Come back in a sec and I'll behave.",
}

func heshFailReply() string {
	return heshFailReplies[rand.IntN(len(heshFailReplies))]
}

func (b *Bot) logHesh(trigger, author, channelID, userText, reply string, httpStatus int, apiErr, sendErr error) {
	if b == nil || b.store == nil {
		return
	}
	errText := ""
	if apiErr != nil {
		errText = apiErr.Error()
	} else if sendErr != nil {
		errText = sendErr.Error()
	}
	if err := b.store.LogAIRequest(context.Background(), db.AIRequestEntry{
		Level:       db.ClassifyAILevel(httpStatus, apiErr, sendErr),
		Trigger:     trigger,
		Author:      author,
		ChannelID:   channelID,
		UserMessage: userText,
		Reply:       reply,
		Error:       errText,
		HTTPStatus:  httpStatus,
	}); err != nil {
		slog.Error("hesh helper ai log failed", "err", err)
	}
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
