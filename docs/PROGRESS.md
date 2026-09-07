# Subotto Progress Report

**Date:** 2026-09-07 (night session)  
**Branch:** `master`  
**Status:** Phases **0–3 complete**. Next: Phase 4.

---

## What we built

| Phase | Title | Result |
|-------|--------|--------|
| 0 | Project bootstrap | Go module, folders, justfile (DEV/PROD), `.env.example`, webroot placeholder |
| 1 | Config + database | Env/`.env` loading, SQLite schema, `log/slog`, startup activity row |
| 2 | YouTube OAuth + client | Browser OAuth, token in SQLite, `AddVideoToPlaylist`, quota/auth errors |
| 3 | Discord bot core | Link parser, mapped-channel listener, dedup, reactions, `just add-mapping` |

## Commits (this arc)

1. `Bootstrap Phase 0 Subotto scaffold.` (+ AGENTS.md git message convention)
2. `Wire Phase 1 config, SQLite, and slog.`
3. `Add Phase 2 YouTube OAuth and playlist client.`
4. `Add Phase 3 Discord bot and YouTube link handling.`

## Key paths

```
cmd/subotto/main.go          Entry: run / auth-youtube / add-mapping
internal/config/             Env loading
internal/db/                 SQLite + mappings helpers
internal/youtube/            OAuth + playlist insert
internal/parser/             YouTube URL → video ID
internal/discord/            Bot MessageCreate handler
webroot/                     Admin UI placeholder (Phase 5)
justfile                     run, build, build-linux, auth-youtube, add-mapping
```

## Verified locally

- `just test` — parser + youtube error tests pass
- `just build` — Windows binary builds
- Phase 1/2 boots without live Discord/YouTube secrets (warn / skip)
- Phase 3 normal boot **requires** Discord token + completed `just auth-youtube`

## Not done yet (PLAN.md)

- **Phase 4** — full mapping CRUD, enable/disable, resync history
- **Phase 5** — Admin Web UI + JSON API
- **Phase 6** — scheduler, rate limits, polish
- **Phase 7** — Linux VPS / systemd deploy guide

## Meat Bag checklist still open

- [ ] Discord bot token + **Message Content Intent**
- [ ] Google Cloud: YouTube Data API v3 + OAuth client + redirect `http://localhost:8080/oauth/callback`
- [ ] `just auth-youtube` once
- [ ] `just add-mapping CHANNEL_ID PLAYLIST_ID`
- [ ] `just run` and paste a YouTube link in the mapped channel
