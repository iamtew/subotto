// Package scheduler runs optional background channel re-scans.
//
// Meat Bag: set RESYNC_INTERVAL_HOURS in .env to a positive number (e.g. 6)
// and Subotto will periodically walk recent messages in every *enabled*
// mapping — same path as Admin "Resync" / `just resync`, no emoji spam.
// Leave it at 0 and this package does nothing (live Discord posts still work).
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"subotto/internal/db"
	"subotto/internal/discord"
	"subotto/internal/youtube"
)

// Default message cap per scheduled channel scan (same as CLI default).
const defaultResyncLimit = 100

// resyncFn is the history-scan function. Production uses discord.ResyncChannel;
// tests inject a fake so we do not talk to Discord.
type resyncFn func(
	ctx context.Context,
	token string,
	store *db.DB,
	yt *youtube.Client,
	channelID string,
	limit int,
) (*discord.ResyncSummary, error)

// Scheduler ticks every N hours and re-scans enabled mappings.
type Scheduler struct {
	store        *db.DB
	yt           *youtube.Client
	discordToken string
	interval     time.Duration
	hours        int // original config value for /api/status
	limit        int
	resync       resyncFn

	mu        sync.Mutex
	lastRunAt time.Time
	lastErr   string
}

// Options configures a Scheduler.
type Options struct {
	Store               *db.DB
	YouTube             *youtube.Client
	DiscordToken        string
	ResyncIntervalHours int
	// ResyncLimit caps messages per channel per tick (0 = default 100).
	ResyncLimit int
}

// Info is a snapshot for the Admin /api/status endpoint.
type Info struct {
	Enabled       bool   `json:"scheduler_enabled"`
	IntervalHours int    `json:"resync_interval_hours"`
	LastRunAt     string `json:"scheduler_last_run_at,omitempty"`
	LastError     string `json:"scheduler_last_error,omitempty"`
}

// New builds a scheduler. IntervalHours <= 0 means disabled (Run is a no-op).
func New(opts Options) *Scheduler {
	limit := opts.ResyncLimit
	if limit <= 0 {
		limit = defaultResyncLimit
	}
	hours := opts.ResyncIntervalHours
	var interval time.Duration
	if hours > 0 {
		interval = time.Duration(hours) * time.Hour
	}
	return &Scheduler{
		store:        opts.Store,
		yt:           opts.YouTube,
		discordToken: opts.DiscordToken,
		interval:     interval,
		hours:        hours,
		limit:        limit,
		resync:       discord.ResyncChannel,
	}
}

// Enabled reports whether background resync is configured.
func (s *Scheduler) Enabled() bool {
	return s != nil && s.hours > 0
}

// Info returns a thread-safe status snapshot for the Admin UI.
func (s *Scheduler) Info() Info {
	if s == nil {
		return Info{Enabled: false, IntervalHours: 0}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info := Info{
		Enabled:       s.hours > 0,
		IntervalHours: s.hours,
		LastError:     s.lastErr,
	}
	if !s.lastRunAt.IsZero() {
		info.LastRunAt = s.lastRunAt.UTC().Format(time.RFC3339)
	}
	return info
}

// Run blocks until ctx is cancelled. When disabled, returns immediately.
func (s *Scheduler) Run(ctx context.Context) {
	if s == nil || s.hours <= 0 {
		slog.Info("scheduler disabled (RESYNC_INTERVAL_HOURS is 0 or unset)")
		return
	}

	slog.Info("scheduler started",
		"interval_hours", s.hours,
		"resync_limit", s.limit,
	)

	// First tick waits a full interval so boot does not hammer YouTube quota.
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

// runOnce scans every enabled mapping. Exported-ish for tests via runOnce.
func (s *Scheduler) runOnce(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}

	_ = s.store.LogActivity(ctx, "resync_scheduled_started", map[string]any{
		"source":         "scheduler",
		"interval_hours": s.hours,
		"limit":          s.limit,
	}, true)

	list, err := s.store.ListMappings(ctx)
	if err != nil {
		s.record(err)
		slog.Error("scheduler could not list mappings", "err", err)
		_ = s.store.LogActivity(ctx, "resync_scheduled_finished", map[string]any{
			"source": "scheduler",
			"error":  err.Error(),
		}, false)
		return
	}

	var ran, failed int
	var firstErr error
	for _, m := range list {
		if !m.Enabled {
			continue
		}
		if err := ctx.Err(); err != nil {
			s.record(err)
			return
		}
		ran++
		slog.Info("scheduler resync starting",
			"channel", m.DiscordChannelID,
			"playlist", m.YouTubePlaylistID,
			"name", m.Name,
		)
		_, err := s.resync(ctx, s.discordToken, s.store, s.yt, m.DiscordChannelID, s.limit)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("scheduler resync failed",
				"channel", m.DiscordChannelID,
				"err", err,
			)
		}
	}

	if firstErr != nil {
		s.record(firstErr)
	} else {
		s.record(nil)
	}

	_ = s.store.LogActivity(ctx, "resync_scheduled_finished", map[string]any{
		"source":           "scheduler",
		"channels_scanned": ran,
		"channels_failed":  failed,
	}, firstErr == nil)

	slog.Info("scheduler tick finished",
		"channels_scanned", ran,
		"channels_failed", failed,
	)
}

func (s *Scheduler) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRunAt = time.Now().UTC()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
}
