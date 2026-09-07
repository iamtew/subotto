// Package ingest is the shared "find YouTube links → add to playlist" pipeline.
//
// Meat Bag: both the live Discord handler and the history resync call this.
// Keeping the rules in one place means a video found in history will not be
// re-added when someone posts it later (and vice versa).
package ingest

import (
	"context"
	"fmt"
	"log/slog"

	"subotto/internal/db"
	"subotto/internal/parser"
)

// PlaylistAdder is anything that can insert a video into a YouTube playlist.
// *youtube.Client already matches this; tests use a fake.
type PlaylistAdder interface {
	AddVideoToPlaylist(ctx context.Context, playlistID, videoID string) error
}

// Result counts what happened for one Discord message's content.
type Result struct {
	Added   int
	Skipped int
	Failed  int
}

// ProcessContent extracts YouTube video IDs from text and adds each one to the
// mapping's playlist (skipping duplicates, logging activity).
//
// channelID / messageID are stored in the activity log and dedup table so we
// can see which Discord message caused an add.
func ProcessContent(
	ctx context.Context,
	store *db.DB,
	yt PlaylistAdder,
	mapping *db.ChannelMapping,
	channelID, messageID, content string,
) Result {
	var res Result
	if mapping == nil {
		return res
	}
	if yt == nil {
		slog.Warn("youtube client missing; cannot add videos", "channel", channelID)
		res.Failed++
		return res
	}

	ids := parser.ExtractVideoIDs(content)
	if len(ids) == 0 {
		return res
	}

	for _, videoID := range ids {
		already, err := store.WasVideoProcessedOnChannel(ctx, videoID, channelID)
		if err != nil {
			slog.Error("dedup check failed", "video", videoID, "err", err)
			res.Failed++
			continue
		}
		if already {
			slog.Info("skip duplicate video", "video", videoID, "channel", channelID, "playlist", mapping.YouTubePlaylistID)
			res.Skipped++
			_ = store.LogActivity(ctx, "video_skipped", map[string]any{
				"reason":     "duplicate",
				"video_id":   videoID,
				"playlist":   mapping.YouTubePlaylistID,
				"channel_id": channelID,
				"message_id": messageID,
			}, true)
			continue
		}

		if err := yt.AddVideoToPlaylist(ctx, mapping.YouTubePlaylistID, videoID); err != nil {
			slog.Error("add video to playlist failed",
				"video", videoID,
				"playlist", mapping.YouTubePlaylistID,
				"err", err,
			)
			res.Failed++
			_ = store.LogActivity(ctx, "video_add_failed", map[string]any{
				"video_id":   videoID,
				"playlist":   mapping.YouTubePlaylistID,
				"channel_id": channelID,
				"message_id": messageID,
				"error":      err.Error(),
			}, false)
			continue
		}

		if err := store.MarkVideoProcessed(ctx, videoID, mapping.YouTubePlaylistID, channelID, messageID); err != nil {
			slog.Error("mark processed failed", "video", videoID, "err", err)
		}
		_ = store.LogActivity(ctx, "video_added", map[string]any{
			"video_id":   videoID,
			"playlist":   mapping.YouTubePlaylistID,
			"channel_id": channelID,
			"message_id": messageID,
			"mapping":    mapping.Name,
		}, true)
		slog.Info("added video to playlist",
			"video", videoID,
			"playlist", mapping.YouTubePlaylistID,
			"channel", channelID,
			"mapping", mapping.Name,
		)
		res.Added++
	}
	return res
}

// Merge adds another Result into r (used by resync to accumulate totals).
func (r *Result) Merge(other Result) {
	r.Added += other.Added
	r.Skipped += other.Skipped
	r.Failed += other.Failed
}

// String is a short human-readable summary.
func (r Result) String() string {
	return fmt.Sprintf("added=%d skipped=%d failed=%d", r.Added, r.Skipped, r.Failed)
}
