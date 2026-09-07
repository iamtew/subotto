# Jump-Back Point — Subotto (2026-09-07)

Sleep well, Meat Bag. This file is how you (and Clanker) pick up without re-deriving the whole night.

---

## 1. For Meat Bag (you) — learn + resume

### Where we left off
Subotto can watch Discord, push YouTube links into playlists, manage mappings via CLI **or Admin UI**, and resync history. Phases **0–5** are in code on `master`. **Live API setup may still be open** — daytime win is tokens + one green ✅.

### Minimum mental model
1. **Config** (`.env`) → tokens, `ADMIN_PASSWORD`, host/port  
2. **SQLite** (`./data/subotto.db`) → mappings, dedup, activity, OAuth token  
3. **YouTube OAuth once** → refresh token saved in DB  
4. **Discord bot** → only **enabled** mapped channels matter  
5. **Admin UI** → same process as the bot; Basic Auth (`admin` / `ADMIN_PASSWORD`)  
6. **Parser + ingest** → shared by live messages and resync  

### Commands to remember
```text
just run                          # Discord bot + Admin UI (needs Discord + YouTube ready)
just auth-youtube                 # one-time Google login
just add-mapping CHANNEL PLAYLIST # CLI create/update (or use the Admin UI)
just list-mappings / enable / disable / delete / resync
just test / just build
```

Open Admin UI: `http://localhost:8080` (or your `ADMIN_HOST`:`ADMIN_PORT`).  
Username: `admin` · Password: value of `ADMIN_PASSWORD` in `.env`.

### First session tomorrow (suggested order)
1. Skim [PROGRESS.md](PROGRESS.md).  
2. Fill `.env` (including a real `ADMIN_PASSWORD`).  
3. Discord portal: **Message Content Intent**, invite bot.  
4. Google Cloud: API + OAuth + redirect URI.  
5. `just auth-youtube` → prints your channel name.  
6. `just run` → open Admin UI → add a mapping → paste a YouTube link → expect ✅.  
7. Optional: Resync button / `just resync CHANNEL`.

### If something goes boom
- Browser 401 → wrong password; username must be `admin`  
- Admin UI missing files → run from repo root so `webroot/` is found  
- No links processed → channel not mapped / disabled / Message Content Intent off  
- YouTube errors → re-run `just auth-youtube`, check playlist ownership  
- Port busy → OAuth one-shot and Admin both default to 8080 (auth is exit-after; bot holds the port)

### Docs map
| File | Use when |
|------|----------|
| [AGENTS.md](../AGENTS.md) | Clanker ↔ Meat Bag; git message style |
| [PLAN.md](../PLAN.md) | Full roadmap Phases 0–7 |
| [PROGRESS.md](PROGRESS.md) | What this arc delivered |
| This file | Resume + learning anchors |

---

## 2. For Clanker (next agent session)

### Hard facts
- **Project:** Subotto — Discord → YouTube playlist bot in Go, **no Docker**, **Just**.  
- **Voice:** Clanker talking to Meat Bag; beginner-friendly comments.  
- **Git style:** clean title + bullet body; only when asked. Prefer `-F` file on Windows PowerShell. Avoid substring `commit` in bodies if Co-authored-by injection mangles it.  
- **Done:** Phases 0–5 on `master`.  
- **Next PLAN phase:** **Phase 6** — scheduler (`RESYNC_INTERVAL_HOURS`), rate-limit polish, health endpoint polish, graceful shutdown already partly done. Then Phase 7 deploy guide.

### Architecture snapshot
```
just run
  → Discord gateway (MessageCreate → ingest → react)
  → HTTP Admin (webroot + /api/*) with Basic Auth
       GET  /api/status
       GET/POST /api/mappings
       PATCH/DELETE /api/mappings/{channel}
       GET  /api/activity
       POST /api/resync
```

### Important implementation notes
- Pure Go SQLite: `modernc.org/sqlite` (no CGO).  
- YouTube OAuth token key in DB: `youtube`.  
- Admin listens on `ADMIN_HOST`:`ADMIN_PORT` (default `0.0.0.0:8080`); webroot path `./webroot`.  
- Resync from UI/CLI shares `discord.ResyncChannel` + `ingest.ProcessContent`.  
- `DeleteMapping` leaves `processed_videos` alone.  
- Empty scaffold left: `internal/scheduler` (Phase 6).

### Do not
- Do not introduce Docker.  
- Do not commit `.env` or `data/*.db`.  
- Do not skip Meat Bag comments on new code.

### Suggested first message from Meat Bag
> Clanker, read docs/JUMPBACK.md and docs/PROGRESS.md. Continue Phase 6.

---

**Clanker’s note:** Phase 5 made mappings clickable. Phase 6 makes the bot keep itself tidy on a schedule. Daytime win remains Meat Bag tokens + one ✅.
