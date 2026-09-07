# Subotto Progress Report

**Date:** 2026-09-07  
**Branch:** `master`  
**Status:** Phases **0–5 complete**. Next: Phase 6 (scheduler & polish).

---

## What we built

| Phase | Title | Result |
|--------|--------|--------|
| 0 | Project bootstrap | Go module, folders, justfile (DEV/PROD), `.env.example`, webroot placeholder |
| 1 | Config + database | Env/`.env` loading, SQLite schema, `log/slog`, startup activity row |
| 2 | YouTube OAuth + client | Browser OAuth, token in SQLite, `AddVideoToPlaylist`, quota/auth errors |
| 3 | Discord bot core | Link parser, mapped-channel listener, dedup, reactions, `just add-mapping` |
| 4 | Mapping CRUD + resync | List/enable/disable/delete CLI, shared ingest, history resync |
| 5 | Admin Web UI | stdlib HTTP, Basic Auth, `/api/*`, plain HTML/CSS/JS dashboard |

## Key paths

```
cmd/subotto/main.go          Entry: run (bot + admin) / auth / mapping CLI / resync
internal/web/                HTTP server, Basic Auth, JSON API
webroot/                     Admin UI (index.html, css/, js/)
internal/db/                 SQLite + mappings + activity list helpers
internal/ingest/             Shared live + resync pipeline
internal/discord/            Bot + REST resync
justfile                     run, build, auth-youtube, mapping CRUD, resync
```

## Verified locally

- `just test` — includes web API auth + mapping CRUD tests
- `just build` — Windows binary builds
- Admin UI served from `webroot/` when `just run` (needs live Discord/YouTube tokens)

## Not done yet (PLAN.md)

- **Phase 6** — scheduler (`RESYNC_INTERVAL_HOURS`), rate-limit handling, polish
- **Phase 7** — Linux VPS / systemd deploy guide

## Meat Bag checklist still open

- [ ] Discord bot token + **Message Content Intent**
- [ ] Google Cloud: YouTube Data API v3 + OAuth client + redirect
- [ ] Set a real `ADMIN_PASSWORD` in `.env`
- [ ] `just auth-youtube` once
- [ ] `just run` → open Admin UI → add mapping → paste a YouTube link
