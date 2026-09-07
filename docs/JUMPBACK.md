# Jump-Back Point — Subotto (2026-09-07)

**Phase 6a committed** (scheduler + port 50770).  
**Now:** stay on Phase 6 — **Admin UI polish** → [HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md).  
**Later:** Phase 7 deploy guide.

---

## 1. For Meat Bag (you) — learn + resume

### Where we left off
Subotto works on Windows DEV: Discord → YouTube playlists, Admin UI, reactions **💾** / **♻️** / **❌**, optional background resync.

You asked to improve the **Admin UI a lot** before moving on. Backend Phase 6 pieces can sit while we rebuild the cockpit.

### Minimum mental model
1. **Config** (`.env`) → tokens, `ADMIN_PASSWORD`, host/port  
2. **SQLite** (`./data/subotto.db`) → mappings, dedup, activity, OAuth token  
3. **YouTube OAuth once** → refresh token saved in DB  
4. **Discord bot** → only **enabled** mapped channels matter  
5. **Admin UI** → same process; Basic Auth (`admin` / `ADMIN_PASSWORD`)  
6. **Parser + ingest** → live, manual resync, scheduled resync  
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

If you still have `8080` in `.env` / Google redirect, switch to **50770** (or keep 8080 explicitly — env wins over defaults).

Setup recipe: [DEV-VERIFY.md](DEV-VERIFY.md).

### Docs map
| File | Use when |
|------|----------|
| **[HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md)** | **Next: Admin UI polish** |
| [DEV-VERIFY.md](DEV-VERIFY.md) | How to run / re-check DEV |
| [PROGRESS.md](PROGRESS.md) | What shipped |
| [HANDOFF-PHASE6.md](HANDOFF-PHASE6.md) | Phase 6a scheduler brief (mostly done) |
| [AGENTS.md](../AGENTS.md) | Clanker ↔ Meat Bag; git style |
| [PLAN.md](../PLAN.md) | Full roadmap |
| This file | Resume + learning anchors |

---

## 2. For Clanker (next agent session)

### Hard facts
- **Project:** Subotto — Discord → YouTube playlist bot in Go, **no Docker**, **Just**.  
- **Voice:** Clanker talking to Meat Bag; beginner-friendly comments.  
- **Git:** commit `1061bae` — Phase 6a scheduler + port 50770.  
- **Next focus:** Admin UI in `webroot/` — follow [HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md). Ask Meat Bag for look/priority before a big redesign.  
- **Do not** jump to Phase 7 unless asked.

### Architecture snapshot
```
just run
  → Discord gateway (MessageCreate → ingest → react 💾/♻️/❌)
  → HTTP Admin (webroot + /api/*) with Basic Auth
  → optional scheduler (RESYNC_INTERVAL_HOURS > 0)
```

### Do not
- Do not introduce Docker or a JS framework build step.  
- Do not commit `.env` or `data/*.db`.  
- Do not break Basic Auth or the existing `/api/*` contracts without need.

### Suggested first message from Meat Bag
> Clanker, read docs/HANDOFF-ADMIN-UI.md — here’s how I want the Admin UI to feel: …

---

**Clanker’s note:** Cockpit next. Keep the engine quiet unless the UI needs a new API field.
