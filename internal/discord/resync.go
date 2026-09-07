package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
	"subotto/internal/ingest"
	"subotto/internal/youtube"
)

// Discord's ChannelMessages API max page size.
const discordPageSize = 100

// Hard ceiling so one resync cannot burn an entire day's YouTube quota.
// Meat Bag: each playlistItems.insert costs ~50 quota units; 500 messages
// with one new link each would be a lot. Cap keeps us honest.
const maxResyncLimit = 500

// ResyncSummary is what the CLI prints when a history scan finishes.
type ResyncSummary struct {
	MessagesScanned int
	Result          ingest.Result
}

// ResyncChannel walks recent messages in a Discord channel (REST only — no
// gateway) and feeds each one through ingest.ProcessContent.
//
// Meat Bag: this does NOT put emoji reactions on old messages. That would be
// spammy. Live posts still get 💾 / ♻️ / ❌.
func ResyncChannel(
	ctx context.Context,
	token string,
	store *db.DB,
	yt *youtube.Client,
	channelID string,
	limit int,
) (*ResyncSummary, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("discord channel id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > maxResyncLimit {
		slog.Warn("resync limit capped", "requested", limit, "cap", maxResyncLimit)
		limit = maxResyncLimit
	}

	mapping, err := store.GetMappingByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if mapping == nil {
		return nil, fmt.Errorf("no mapping found for channel %s — run just add-mapping first", channelID)
	}
	if !mapping.Enabled {
		return nil, fmt.Errorf("mapping for channel %s is disabled — run just enable-mapping %s first", channelID, channelID)
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("DISCORD_BOT_TOKEN is empty")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	// REST-only: we never call session.Open(), so no gateway websocket.

	_ = store.LogActivity(ctx, "resync_started", map[string]any{
		"channel_id":  channelID,
		"playlist_id": mapping.YouTubePlaylistID,
		"limit":       limit,
		"mapping":     mapping.Name,
	}, true)

	summary := &ResyncSummary{}
	beforeID := "" // empty = start from the newest messages

	for summary.MessagesScanned < limit {
		page := discordPageSize
		remaining := limit - summary.MessagesScanned
		if remaining < page {
			page = remaining
		}

		msgs, err := session.ChannelMessages(channelID, page, beforeID, "", "")
		if err != nil {
			_ = store.LogActivity(ctx, "resync_finished", map[string]any{
				"channel_id": channelID,
				"error":      err.Error(),
				"scanned":    summary.MessagesScanned,
				"added":      summary.Result.Added,
				"skipped":    summary.Result.Skipped,
				"failed":     summary.Result.Failed,
			}, false)
			return summary, fmt.Errorf("fetch channel messages: %w", err)
		}
		if len(msgs) == 0 {
			break // reached the beginning of the channel
		}

		for _, m := range msgs {
			summary.MessagesScanned++
			if m.Author != nil && m.Author.Bot {
				continue
			}
			r := ingest.ProcessContent(ctx, store, yt, mapping, channelID, m.ID, m.Content)
			summary.Result.Merge(r)
		}

		// discordgo returns newest-first; the last item is the oldest in this page.
		beforeID = msgs[len(msgs)-1].ID
		if len(msgs) < page {
			break // fewer than requested = no more history
		}
	}

	_ = store.LogActivity(ctx, "resync_finished", map[string]any{
		"channel_id": channelID,
		"playlist":   mapping.YouTubePlaylistID,
		"mapping":    mapping.Name,
		"scanned":    summary.MessagesScanned,
		"added":      summary.Result.Added,
		"skipped":    summary.Result.Skipped,
		"failed":     summary.Result.Failed,
	}, true)

	slog.Info("resync finished",
		"channel", channelID,
		"scanned", summary.MessagesScanned,
		"added", summary.Result.Added,
		"skipped", summary.Result.Skipped,
		"failed", summary.Result.Failed,
	)
	return summary, nil
}
