# Subotto Progress Report

**Date:** 2026-09-07  
**Branch:** `master`  
**Status:** Phases **0–4 complete**. Next: Phase 5 (Admin Web UI).

---

## What we built

| Phase | Title | Result |
|--------|--------|--------|
| 0 | Project bootstrap | Go module, folders, justfile (DEV/PROD), `.env.example`, webroot placeholder |
| 1 | Config + database | Env/`.env` loading, SQLite schema, `log/slog`, startup activity row |
| 2 | YouTube OAuth + client | Browser OAuth, token in SQLite, `AddVideoToPlaylist`, quota/auth errors |
| 3 | Discord bot core | Link parser, mapped-channel listener, dedup, reactions, `just add-mapping` |
| 4 | Mapping CRUD + resync | List/enable/disable/delete CLI, shared ingest, history resync |

## Commits (this arc)

1. `Bootstrap Phase 0 Subotto scaffold.` (+ AGENTS.md git message convention)
2. `Wire Phase 1 config, SQLite, and slog.`
3. `Add Phase 2 YouTube OAuth and playlist client.`
4. `Add Phase 3 Discord bot and YouTube link handling.`
5. *(pending)* Phase 4 mapping CRUD + history resync

## Key paths

```
cmd/subotto/main.go          Entry: run / auth / mapping CRUD / resync
internal/config/             Env loading
internal/db/                 SQLite + full mapping CRUD helpers
internal/youtube/            OAuth + playlist insert
internal/parser/             YouTube URL → video ID
internal/ingest/             Shared live + resync pipeline
internal/discord/            Bot MessageCreate + REST resync
webroot/                     Admin UI placeholder (Phase 5)
justfile                     run, build, auth-youtube, add/list/enable/disable/delete/resync
```

## Verified locally

- `just test` — parser, youtube error, db mapping, ingest tests
- `just build` — Windows binary builds
- Mapping CLI (`list-mappings`) needs only a DB — no Discord/YouTube for list/enable/disable/delete
- Resync / `just run` still require Discord token + completed `just auth-youtube`

## Not done yet (PLAN.md)

- **Phase 5** — Admin Web UI + JSON API (`webroot/` + `/api/...`)
- **Phase 6** — scheduler, rate limits, polish
- **Phase 7** — Linux VPS / systemd deploy guide

## Meat Bag checklist still open

- [ ] Discord bot token + **Message Content Intent**
- [ ] Google Cloud: YouTube Data API v3 + OAuth client + redirect `http://localhost:8080/oauth/callback`
- [ ] `just auth-youtube` once
- [ ] `just add-mapping CHANNEL_ID PLAYLIST_ID`
- [ ] `just run` and paste a YouTube link in the mapped channel
- [ ] Optional: `just resync CHANNEL_ID` to backfill recent history
