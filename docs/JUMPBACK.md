# Jump-Back Point — Subotto (2026-09-07)

**Meat Bag verified DEV: GOOD.** Phases 0–5 accepted live.  
**Next:** Phase 6 — see [HANDOFF-PHASE6.md](HANDOFF-PHASE6.md). Phase 7 still later.

---

## 1. For Meat Bag (you) — learn + resume

### Where we left off
Subotto works on your Windows DEV machine: Discord links land on YouTube playlists, Admin UI manages mappings, live posts get **💾** / **♻️** / **❌**.

Phases **0–5** are done and verified. Optional next work is **Phase 6** (scheduler + polish). You do not have to start it tonight.

### Minimum mental model
1. **Config** (`.env`) → tokens, `ADMIN_PASSWORD`, host/port  
2. **SQLite** (`./data/subotto.db`) → mappings, dedup, activity, OAuth token  
3. **YouTube OAuth once** → refresh token saved in DB  
4. **Discord bot** → only **enabled** mapped channels matter  
5. **Admin UI** → same process as the bot; Basic Auth (`admin` / `ADMIN_PASSWORD`)  
6. **Parser + ingest** → shared by live messages and resync  

### Commands to remember
```text
just run                          # Discord bot + Admin UI
just auth-youtube                 # one-time Google login
just add-mapping CHANNEL PLAYLIST # or use the Admin UI
just list-mappings / enable / disable / delete / resync
just test / just build
```

Open Admin UI: `http://localhost:8080`  
Username: `admin` · Password: `ADMIN_PASSWORD` from `.env`.

Setup recipe: [DEV-VERIFY.md](DEV-VERIFY.md).

### If something goes boom
Full table: [DEV-VERIFY.md](DEV-VERIFY.md#troubleshooting).

- No links processed → not mapped / disabled / Message Content Intent off  
- Browser 401 → username must be `admin`; check password  
- YouTube errors → re-run `just auth-youtube`, check playlist ownership  
- Reactions missing → bot needs **Add Reactions**; watch for `could not add reaction` Warn logs  
- Port busy → do not run auth and `just run` on 8080 at the same time  

### Docs map
| File | Use when |
|------|----------|
| [DEV-VERIFY.md](DEV-VERIFY.md) | How to run / re-check DEV |
| **[HANDOFF-PHASE6.md](HANDOFF-PHASE6.md)** | **Next agent: implement Phase 6** |
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
- **Done + verified:** Phases 0–5 on `master` (Meat Bag live DEV sign-off 2026-09-07).  
- **Next:** **Phase 6** — follow [HANDOFF-PHASE6.md](HANDOFF-PHASE6.md). Do not start Phase 7 unless asked.

### Architecture snapshot
```
just run
  → Discord gateway (MessageCreate → ingest → react 💾/♻️/❌)
  → HTTP Admin (webroot + /api/*) with Basic Auth
```

### Important implementation notes
- Pure Go SQLite: `modernc.org/sqlite` (no CGO).  
- YouTube OAuth token key in DB: `youtube`.  
- Admin: `ADMIN_HOST`:`ADMIN_PORT` (default `0.0.0.0:8080`), webroot `./webroot`.  
- `RESYNC_INTERVAL_HOURS` is loaded but unused — Phase 6 owns `internal/scheduler`.  
- Success reaction is **💾** (not ✅).

### Do not
- Do not introduce Docker.  
- Do not commit `.env` or `data/*.db`.  
- Do not skip Meat Bag comments on new code.  
- Do not make Meat Bag re-prove Phases 0–5 unless a regression is suspected.

### Suggested first message from Meat Bag
> Clanker, read docs/HANDOFF-PHASE6.md and continue Phase 6.

---

**Clanker’s note:** Verification win locked in. Next Clanker builds the schedule brain — carefully, with quota in mind.
