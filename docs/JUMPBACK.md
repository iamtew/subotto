# Jump-Back Point — Subotto (2026-09-07)

**Phases 0–6 in code.** Phases 0–5 Meat Bag verified DEV: GOOD.  
**Next:** Phase 7 deploy guide when Meat Bag is ready.

---

## 1. For Meat Bag (you) — learn + resume

### Where we left off
Subotto works on your Windows DEV machine: Discord links land on YouTube playlists, Admin UI manages mappings, live posts get **💾** / **♻️** / **❌**.

Phase **6** adds optional background re-scans via `RESYNC_INTERVAL_HOURS` (default `0` = off, same as before).

Default Admin port is **50770**. If your `.env` still says 8080, either keep it or switch and update the Google OAuth redirect URI.

### Minimum mental model
1. **Config** (`.env`) → tokens, `ADMIN_PASSWORD`, host/port  
2. **SQLite** (`./data/subotto.db`) → mappings, dedup, activity, OAuth token  
3. **YouTube OAuth once** → refresh token saved in DB  
4. **Discord bot** → only **enabled** mapped channels matter  
5. **Admin UI** → same process as the bot; Basic Auth (`admin` / `ADMIN_PASSWORD`)  
6. **Parser + ingest** → shared by live messages, manual resync, and scheduled resync  
7. **Scheduler** → only when `RESYNC_INTERVAL_HOURS > 0`

### Commands to remember
```text
just run                          # Discord bot + Admin UI (+ scheduler if configured)
just auth-youtube                 # one-time Google login
just add-mapping CHANNEL PLAYLIST # or use the Admin UI
just list-mappings / enable / disable / delete / resync
just test / just build
```

Open Admin UI: `http://localhost:50770`  
Username: `admin` · Password: `ADMIN_PASSWORD` from `.env`.

Setup recipe: [DEV-VERIFY.md](DEV-VERIFY.md).

### If something goes boom
Full table: [DEV-VERIFY.md](DEV-VERIFY.md#troubleshooting).

- No links processed → not mapped / disabled / Message Content Intent off  
- Browser 401 → username must be `admin`; check password  
- YouTube errors → re-run `just auth-youtube`, check playlist ownership  
- Reactions missing → bot needs **Add Reactions**; watch for `could not add reaction` Warn logs  
- Port busy → do not run auth and `just run` on 50770 at the same time  

### Docs map
| File | Use when |
|------|----------|
| [DEV-VERIFY.md](DEV-VERIFY.md) | How to run / re-check DEV |
| [PROGRESS.md](PROGRESS.md) | What shipped + verify sign-off |
| [AGENTS.md](../AGENTS.md) | Clanker ↔ Meat Bag; git style |
| [PLAN.md](../PLAN.md) | Full roadmap |
| This file | Resume + learning anchors |

---

## 2. For Clanker (next agent session)

### Hard facts
- **Project:** Subotto — Discord → YouTube playlist bot in Go, **no Docker**, **Just**.  
- **Voice:** Clanker talking to Meat Bag; beginner-friendly comments.  
- **Git style:** clean title + bullet body; only when asked. Prefer `-F` file on Windows PowerShell.  
- **Done:** Phases 0–6 on `master` (0–5 live DEV sign-off 2026-09-07; Phase 6 shipped in code).  
- **Next:** **Phase 7** — Linux VPS / systemd deploy guide. Do not invent Docker.

### Architecture snapshot
```
just run
  → Discord gateway (MessageCreate → ingest → react 💾/♻️/❌)
  → HTTP Admin (webroot + /api/*) with Basic Auth
  → optional scheduler (RESYNC_INTERVAL_HOURS > 0 → ResyncChannel per enabled mapping)
```

### Important implementation notes
- Pure Go SQLite: `modernc.org/sqlite` (no CGO).  
- YouTube OAuth token key in DB: `youtube`.  
- Admin: `ADMIN_HOST`:`ADMIN_PORT` (default `0.0.0.0:50770`), webroot `./webroot`.  
- `RESYNC_INTERVAL_HOURS` drives `internal/scheduler` (0 = disabled).  
- Success reaction is **💾** (not ✅).

### Do not
- Do not introduce Docker.  
- Do not commit `.env` or `data/*.db`.  
- Do not skip Meat Bag comments on new code.  
- Do not make Meat Bag re-prove Phases 0–5 unless a regression is suspected.

### Suggested first message from Meat Bag
> Clanker, continue with Phase 7 deploy guide.

---

**Clanker’s note:** Port default is 50770. Scheduler respects quota — keep intervals conservative.
