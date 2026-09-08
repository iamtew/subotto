package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"subotto/internal/db"
	"subotto/internal/pictures"
)

// PictureResyncSummary is what the CLI / Admin get when a picture history scan finishes.
type PictureResyncSummary struct {
	MessagesScanned int
	Result          pictures.Result
}

// ResyncPictureChannel walks recent messages in a Discord channel (REST only)
// and saves image attachments through pictures.ProcessAttachments.
//
// Status chrome is stamped the same way as live ingest (missing 💾 / DUPE /
// OLD / ❌ get filled in). Stops at the previous picture-listener epoch
// boundary when one exists.
func ResyncPictureChannel(
	ctx context.Context,
	token string,
	store *db.DB,
	channelID string,
	limit int,
) (*PictureResyncSummary, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("discord channel id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > maxResyncLimit {
		slog.Warn("picture resync limit capped", "requested", limit, "cap", maxResyncLimit)
		limit = maxResyncLimit
	}

	pl, err := store.GetPictureListenerByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if pl == nil {
		return nil, fmt.Errorf("no picture listener found for channel %s — start one in Admin first", channelID)
	}
	if !pl.Enabled {
		return nil, fmt.Errorf("picture listener for channel %s is paused — resume it first", channelID)
	}

	notBefore, err := store.PictureResyncNotBefore(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("picture lookback boundary: %w", err)
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("DISCORD_BOT_TOKEN is empty")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}

	_ = store.LogActivity(ctx, "picture_resync_started", map[string]any{
		"channel_id": channelID,
		"slug":       pl.Slug,
		"name":       pl.Name,
		"limit":      limit,
		"not_before": formatNotBefore(notBefore),
	}, true)

	summary := &PictureResyncSummary{}
	beforeID := ""
	hitEpochFloor := false

	for summary.MessagesScanned < limit && !hitEpochFloor {
		page := discordPageSize
		remaining := limit - summary.MessagesScanned
		if remaining < page {
			page = remaining
		}

		msgs, err := session.ChannelMessages(channelID, page, beforeID, "", "")
		if err != nil && isDiscordRateLimit(err) {
			slog.Warn("discord rate limited during picture resync — waiting then retrying once",
				"channel", channelID,
			)
			select {
			case <-ctx.Done():
				return summary, ctx.Err()
			case <-time.After(2 * time.Second):
			}
			msgs, err = session.ChannelMessages(channelID, page, beforeID, "", "")
		}
		if err != nil {
			_ = store.LogActivity(ctx, "picture_resync_finished", map[string]any{
				"channel_id": channelID,
				"error":      err.Error(),
				"scanned":    summary.MessagesScanned,
				"saved":      summary.Result.Saved,
				"skipped":    summary.Result.Skipped,
				"failed":     summary.Result.Failed,
			}, false)
			return summary, fmt.Errorf("fetch channel messages: %w", err)
		}
		if len(msgs) == 0 {
			break
		}

		for _, m := range msgs {
			if !notBefore.IsZero() && m.Timestamp.Before(notBefore) {
				hitEpochFloor = true
				slog.Info("picture resync reached previous listener boundary",
					"channel", channelID,
					"not_before", notBefore.UTC().Format(time.RFC3339),
					"message_time", m.Timestamp.UTC().Format(time.RFC3339),
				)
				break
			}
			summary.MessagesScanned++
			if m.Author != nil && m.Author.Bot {
				continue
			}
			if len(m.Attachments) == 0 {
				continue
			}

			atts := make([]pictures.Attachment, 0, len(m.Attachments))
			for _, a := range m.Attachments {
				atts = append(atts, pictures.Attachment{
					ID:          a.ID,
					URL:         a.URL,
					Filename:    a.Filename,
					ContentType: a.ContentType,
					Width:       a.Width,
					Height:      a.Height,
				})
			}
			authorID := ""
			if m.Author != nil {
				authorID = m.Author.ID
			}
			r := pictures.ProcessAttachments(
				ctx, store, pl, m.ID, authorID, displayNameFromMessage(m),
				atts, reactionsFromMessage(m),
			)
			summary.Result.Merge(r)
			ensureStatusChrome(ctx, session, channelID, m.ID, m, pictureStatusChrome(r))
		}

		if hitEpochFloor {
			break
		}

		beforeID = msgs[len(msgs)-1].ID
		if len(msgs) < page {
			break
		}
	}

	_ = store.LogActivity(ctx, "picture_resync_finished", map[string]any{
		"channel_id": channelID,
		"slug":       pl.Slug,
		"name":       pl.Name,
		"scanned":    summary.MessagesScanned,
		"saved":      summary.Result.Saved,
		"skipped":    summary.Result.Skipped,
		"failed":     summary.Result.Failed,
	}, true)

	slog.Info("picture resync finished",
		"channel", channelID,
		"slug", pl.Slug,
		"scanned", summary.MessagesScanned,
		"saved", summary.Result.Saved,
		"skipped", summary.Result.Skipped,
		"failed", summary.Result.Failed,
	)
	return summary, nil
}

func displayNameFromMessage(m *discordgo.Message) string {
	if m == nil || m.Author == nil {
		return "unknown"
	}
	if m.Member != nil {
		if nick := strings.TrimSpace(m.Member.Nick); nick != "" {
			return nick
		}
	}
	if gn := strings.TrimSpace(m.Author.GlobalName); gn != "" {
		return gn
	}
	if u := strings.TrimSpace(m.Author.Username); u != "" {
		return u
	}
	return m.Author.ID
}
