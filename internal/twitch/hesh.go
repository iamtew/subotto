package twitch

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"subotto/internal/ai"
	"subotto/internal/db"
)

const twitchPromptExtra = "You are in Twitch chat. Keep replies short (one or two sentences), no markdown, stay under 400 characters."

func (c *Client) handlePRIVMSG(msg ircLine, selfLogin string) {
	if c == nil {
		return
	}
	channel := ircChannel(msg)
	if channel == "" {
		return
	}
	nick := strings.ToLower(strings.TrimSpace(msg.Nick))
	text := msg.text()
	who := msg.tag("display-name")
	if who == "" {
		who = nick
	}
	self := nick != "" && nick == selfLogin
	ts := time.Now().UTC().Format(time.RFC3339)
	if ms := msg.tag("tmi-sent-ts"); ms != "" {
		if n, err := strconv.ParseInt(ms, 10, 64); err == nil {
			ts = time.UnixMilli(n).UTC().Format(time.RFC3339)
		}
	}
	c.rememberChat(channel, ChatMessage{
		ID:        msg.tag("id"),
		Author:    who,
		AuthorID:  nick,
		Content:   text,
		Timestamp: ts,
		Self:      self,
	})
	if nick == "" || self {
		return
	}
	if c.store == nil {
		return
	}
	display, _ := c.store.TwitchDisplay(context.Background())
	if display == "" {
		display = selfLogin
	}
	parentLogin := strings.ToLower(msg.tag("reply-parent-user-login"))
	mentioned := hasAtName(text, selfLogin) || hasAtName(text, display)
	replied := parentLogin != "" && parentLogin == selfLogin
	c.remember(nick, text, false)
	if !mentioned && !replied {
		return
	}
	trigger := "twitch_reply"
	if mentioned {
		trigger = "twitch_mention"
	}
	user := userText(text, selfLogin, display)
	parentID := msg.tag("id")
	c.replyHesh(trigger, nick, channel, user, parentID)
}

func (c *Client) replyHesh(trigger, author, channel, userText, parentID string) {
	on, err := c.store.AITwitchEnabled(context.Background())
	if err != nil {
		slog.Error("twitch hesh enabled check failed", "err", err)
		return
	}
	if !on {
		slog.Debug("twitch hesh skipped — disabled in Admin")
		return
	}
	if c.ai == nil || !c.ai.Configured() {
		slog.Debug("twitch hesh skipped — OPENROUTER_API_KEY not set")
		c.logHesh(trigger, author, channel, userText, "", 0, fmtAIKeyMissing(), nil)
		return
	}

	go func() {
		aiCtx, cancel := context.WithTimeout(context.Background(), ai.ChatTimeout)
		defer cancel()

		prompt, err := c.store.AISystemPrompt(aiCtx)
		if err != nil {
			slog.Error("twitch hesh prompt load failed", "err", err)
			c.logHesh(trigger, author, channel, userText, "", 0, err, nil)
			return
		}
		prompt = strings.TrimSpace(prompt) + "\n\n" + twitchPromptExtra
		sampling, err := c.store.LoadAISampling(aiCtx)
		if err != nil {
			slog.Error("twitch hesh sampling load failed", "err", err)
			c.logHesh(trigger, author, channel, userText, "", 0, err, nil)
			return
		}
		catalog, err := c.store.LoadAIModels(aiCtx, c.envAIModel)
		if err != nil {
			slog.Error("twitch hesh model load failed", "err", err)
			c.logHesh(trigger, author, channel, userText, "", 0, err, nil)
			return
		}
		history := c.memoryTurns(aiCtx)
		reply, err := c.ai.Chat(aiCtx, prompt, history, userText, catalog.Model, sampling)
		if err != nil {
			slog.Error("twitch hesh chat failed", "err", err)
			c.logHesh(trigger, author, channel, userText, "", ai.HTTPStatus(err), err, nil)
			fail := heshFailReply()
			if sendErr := c.sendPRIVMSG(channel, fail, parentID); sendErr != nil {
				slog.Error("twitch hesh fail reply failed", "channel", channel, "err", sendErr)
			} else {
				c.remember(c.selfLogin(), fail, true)
			}
			return
		}
		reply = truncateRunes(strings.TrimSpace(reply), heshMaxReplyRunes)
		if reply == "" {
			empty := errors.New("openrouter returned an empty reply")
			c.logHesh(trigger, author, channel, userText, "", 0, empty, nil)
			return
		}
		if err := c.sendPRIVMSG(channel, reply, parentID); err != nil {
			slog.Error("twitch hesh reply failed", "channel", channel, "err", err)
			c.logHesh(trigger, author, channel, userText, reply, 0, nil, err)
			return
		}
		c.remember(c.selfLogin(), reply, true)
		c.logHesh(trigger, author, channel, userText, reply, 0, nil, nil)
	}()
}

func fmtAIKeyMissing() error {
	return &ai.APIError{Msg: "OPENROUTER_API_KEY is not set"}
}

func (c *Client) selfLogin() string {
	login, _ := c.store.TwitchLogin(context.Background())
	return strings.ToLower(strings.TrimSpace(login))
}

func (c *Client) remember(login, text string, fromSelf bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c.memMu.Lock()
	defer c.memMu.Unlock()
	c.mem = append(c.mem, memLine{login: login, text: text, fromSelf: fromSelf})
	if len(c.mem) > db.MaxAIMemoryWindow {
		c.mem = c.mem[len(c.mem)-db.MaxAIMemoryWindow:]
	}
}

func (c *Client) memoryTurns(ctx context.Context) []ai.Turn {
	if c == nil || c.store == nil {
		return nil
	}
	on, err := c.store.AIMemoryEnabled(ctx)
	if err != nil || !on {
		return nil
	}
	n, err := c.store.AIMemoryWindow(ctx)
	if err != nil {
		return nil
	}
	c.memMu.Lock()
	lines := append([]memLine(nil), c.mem...)
	c.memMu.Unlock()
	if len(lines) == 0 {
		return nil
	}
	// Skip the current inbound line (last user message we just remembered).
	lines = lines[:len(lines)-1]
	if n > len(lines) {
		n = len(lines)
	}
	lines = lines[len(lines)-n:]
	out := make([]ai.Turn, 0, len(lines))
	for _, l := range lines {
		if l.fromSelf {
			out = append(out, ai.Turn{Role: "assistant", Content: l.text})
			continue
		}
		name := l.login
		if name == "" {
			name = "user"
		}
		out = append(out, ai.Turn{Role: "user", Content: name + ": " + l.text})
	}
	return out
}

func (c *Client) logHesh(trigger, author, channelID, userText, reply string, httpStatus int, apiErr, sendErr error) {
	if c == nil || c.store == nil {
		return
	}
	errText := ""
	if apiErr != nil {
		errText = apiErr.Error()
	} else if sendErr != nil {
		errText = sendErr.Error()
	}
	if err := c.store.LogAIRequest(context.Background(), db.AIRequestEntry{
		Level:       db.ClassifyAILevel(httpStatus, apiErr, sendErr),
		Trigger:     trigger,
		Author:      author,
		ChannelID:   channelID,
		UserMessage: userText,
		Reply:       reply,
		Error:       errText,
		HTTPStatus:  httpStatus,
	}); err != nil {
		slog.Error("twitch hesh ai log failed", "err", err)
	}
}

var heshFailReplies = []string{
	"Brain blanked. Poke me again in a sec.",
	"The model went for a smoke. Try again shortly.",
	"OpenRouter hung up on me. Give it another go.",
	"That's a no from the cloud. Retry in a moment.",
	"The neurons are buffering. Hit me in a bit.",
}

func heshFailReply() string {
	return heshFailReplies[rand.IntN(len(heshFailReplies))]
}
