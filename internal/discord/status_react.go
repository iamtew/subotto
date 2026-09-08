package discord

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/ingest"
	"subotto/internal/pictures"
)

// Status chrome Subotto stamps on posts so Meat Bag can see the bot working.
// Live ingest and history resync both use these — missing reacts get filled in.
var (
	chromeFailed = []string{"❌"}
	chromeSaved  = []string{"💾"}
	chromeOld    = []string{"🛑", "🇴", "🇱", "🇩"}
	chromeDupe   = []string{"♻️", "🇩", "🇺", "🇵", "🇪"}
)

// contentStatusChrome picks 💾 / DUPE / OLD / ❌ for a content-listener result.
// Empty slice = nothing to stamp (no YouTube links in the message).
func contentStatusChrome(res ingest.Result) []string {
	return statusChromeEmojis(res.Failed, res.Added, res.OriginHits, res.SkippedOld, res.SkippedSame, res.Skipped)
}

// pictureStatusChrome picks the same chrome for a picture-listener result.
func pictureStatusChrome(res pictures.Result) []string {
	return statusChromeEmojis(res.Failed, res.Saved, res.OriginHits, res.SkippedOld, res.SkippedSame, res.Skipped)
}

// statusChromeEmojis is the single stamp policy for every listener.
//
//   - ❌  when every item failed and this post is not already on file
//   - 💾  when we saved now, or this Discord message is the original filing (resync backfill)
//   - OLD when the items were filed under a previous listener epoch
//   - DUPE when the same listener already has them from a *different* message
func statusChromeEmojis(failed, saved, originHits, skippedOld, skippedSame, skipped int) []string {
	if saved == 0 && skipped == 0 && failed == 0 && originHits == 0 {
		return nil
	}
	switch {
	case failed > 0 && saved == 0 && originHits == 0:
		return chromeFailed
	case saved > 0 || originHits > 0:
		return chromeSaved
	case skippedOld > 0:
		return chromeOld
	case skippedSame > 0 || skipped > 0:
		return chromeDupe
	default:
		return nil
	}
}

// ensureStatusChrome adds any missing Subotto status reacts on a message.
// Already-present bot reacts (Discord's Me flag, matched without variation
// selectors) are left alone so resync can backfill without spamming duplicates.
func ensureStatusChrome(ctx context.Context, s *discordgo.Session, channelID, messageID string, existing *discordgo.Message, emojis []string) {
	if s == nil || channelID == "" || messageID == "" {
		return
	}
	wanted := missingStatusEmojis(existing, emojis)
	if len(wanted) == 0 {
		return
	}
	for _, emoji := range wanted {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		if err := addMessageReaction(ctx, s, channelID, messageID, emoji); err != nil {
			slog.Warn("could not add reaction",
				"emoji", emoji,
				"channel", channelID,
				"message", messageID,
				"err", err,
			)
			return
		}
	}
}

// missingStatusEmojis returns the chrome emojis not yet stamped by this bot.
func missingStatusEmojis(msg *discordgo.Message, wanted []string) []string {
	if len(wanted) == 0 {
		return nil
	}
	have := botReactionSet(msg)
	var missing []string
	for _, emoji := range wanted {
		if have[normalizeEmoji(emoji)] {
			continue
		}
		missing = append(missing, emoji)
	}
	return missing
}

func botReactionSet(msg *discordgo.Message) map[string]bool {
	out := map[string]bool{}
	if msg == nil {
		return out
	}
	for _, r := range msg.Reactions {
		if r == nil || r.Emoji == nil || !r.Me || r.Emoji.Name == "" {
			continue
		}
		out[normalizeEmoji(r.Emoji.Name)] = true
	}
	return out
}

// normalizeEmoji strips emoji variation selectors so ♻️ and ♻ compare equal.
func normalizeEmoji(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\uFE0F' || r == '\uFE0E' {
			return -1
		}
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, s)
}

func addMessageReaction(ctx context.Context, s *discordgo.Session, channelID, messageID, emoji string) error {
	err := s.MessageReactionAdd(channelID, messageID, emoji)
	if err == nil || !isDiscordRateLimit(err) {
		return err
	}
	slog.Warn("discord rate limited while adding reaction — waiting then retrying once",
		"channel", channelID,
		"message", messageID,
		"emoji", emoji,
	)
	wait := 2 * time.Second
	if ctx == nil {
		time.Sleep(wait)
		return s.MessageReactionAdd(channelID, messageID, emoji)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
	}
	return s.MessageReactionAdd(channelID, messageID, emoji)
}
