# Subotto Progress Report

**Date:** 2026-09-07  
**Branch:** `master`  
**Status:** Phases **0–6 complete** (Phase 6 scheduler + polish in code; Meat Bag DEV smoke still welcome).  
**Next:** Phase 7 Linux VPS / systemd deploy guide  

---

## What we built

| Phase | Title | Result |
|--------|--------|--------|
| 0 | Project bootstrap | Go module, folders, justfile (DEV/PROD), `.env.example`, webroot |
| 1 | Config + database | Env/`.env` loading, SQLite schema, `log/slog` |
| 2 | YouTube OAuth + client | Browser OAuth, token in SQLite, `AddVideoToPlaylist` |
| 3 | Discord bot core | Link parser, mapped-channel listener, dedup, reactions |
| 4 | Mapping CRUD + resync | List/enable/disable/delete CLI, shared ingest, history resync |
| 5 | Admin Web UI | stdlib HTTP, Basic Auth, `/api/*`, plain HTML/CSS/JS dashboard |
| 6 | Scheduling & polish | `RESYNC_INTERVAL_HOURS` scheduler, rate-limit retries, status fields |

## Key paths

```
cmd/subotto/main.go          Entry: run (bot + admin + scheduler) / auth / mapping CLI / resync
internal/scheduler/          Optional interval re-scans of enabled mappings
internal/web/                HTTP server, Basic Auth, JSON API
webroot/                     Admin UI (index.html, css/, js/)
internal/db/                 SQLite + mappings + activity list helpers
internal/ingest/             Shared live + resync pipeline
internal/discord/            Bot + REST resync (success react = 💾)
justfile                     run, build, auth-youtube, mapping CRUD, resync
docs/DEV-VERIFY.md           DEV startup guide
```

## Verified — code

- `just test` — parser, youtube errors, db mappings, ingest, web API, scheduler  
- `just build` — Windows binary builds  

## Verified — live DEV (Meat Bag sign-off, 2026-09-07)

Acceptance from [DEV-VERIFY.md](DEV-VERIFY.md) for Phases 0–5:

- [x] Discord bot token + **Message Content Intent** + bot invited  
- [x] Google Cloud: YouTube Data API v3 + OAuth client + redirect + test user  
- [x] Real `ADMIN_PASSWORD` in `.env`  
- [x] `just auth-youtube` once  
- [x] `just run` → Admin UI → mapping → paste YouTube link → **💾** + playlist update  
- [x] Reactions work (Add Reactions permission); failures visible at Warn if they recur  

Phase 6 live check (optional): set a small `RESYNC_INTERVAL_HOURS` in DEV and confirm scheduled activity rows; leave at `0` for normal use.

Default Admin/OAuth port is now **50770** (update Google redirect + `.env` if you still use 8080).

## Not done yet (PLAN.md)

- **Phase 7** — Linux VPS / systemd deploy guide  
