# Jump-Back Point — Subotto (2026-09-07)

Pause note: **Phases 0–5 are complete.** We are **not starting Phase 6/7 yet**. Meat Bag is verifying DEV with live Discord + YouTube first. See [DEV-VERIFY.md](DEV-VERIFY.md).

---

## 1. For Meat Bag (you) — learn + resume

### Where we left off
Code for Phases **0–5** is on `master` (bot, mappings CLI, Admin UI, resync).  
**Your job now:** follow [DEV-VERIFY.md](DEV-VERIFY.md) end-to-end until you get one green ✅ in Discord and the video on the playlist.

Do **not** ask Clanker for Phase 6 until that verify pass feels good (or you explicitly redirect).

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

### If something goes boom
Full table lives in [DEV-VERIFY.md](DEV-VERIFY.md#troubleshooting). Short version:

- No links processed → not mapped / disabled / Message Content Intent off  
- Browser 401 → username must be `admin`; check password  
- YouTube errors → re-run `just auth-youtube`, check playlist ownership  
- Port busy → do not run auth and `just run` on 8080 at the same time  

### Docs map
| File | Use when |
|------|----------|
| **[DEV-VERIFY.md](DEV-VERIFY.md)** | **How to start Subotto in DEV and verify Phases 0–5** |
| [PROGRESS.md](PROGRESS.md) | What this arc delivered |
| [AGENTS.md](../AGENTS.md) | Clanker ↔ Meat Bag; git message style |
| [PLAN.md](../PLAN.md) | Full roadmap (6–7 still future) |
| This file | Resume + learning anchors |

---

## 2. For Clanker (next agent session)

### Hard facts
- **Project:** Subotto — Discord → YouTube playlist bot in Go, **no Docker**, **Just**.  
- **Voice:** Clanker talking to Meat Bag; beginner-friendly comments.  
- **Git style:** clean title + bullet body; only when asked. Prefer `-F` file on Windows PowerShell.  
- **Done in code:** Phases 0–5 on `master`.  
- **Current mode:** **verification pause** — Meat Bag runs [DEV-VERIFY.md](DEV-VERIFY.md).  
- **Do not auto-start Phase 6** unless Meat Bag says verification is done (or explicitly asks to continue).

### Architecture snapshot
```
just run
  → Discord gateway (MessageCreate → ingest → react)
  → HTTP Admin (webroot + /api/*) with Basic Auth
```

### Important implementation notes
- Pure Go SQLite: `modernc.org/sqlite` (no CGO).  
- YouTube OAuth token key in DB: `youtube`.  
- Admin: `ADMIN_HOST`:`ADMIN_PORT` (default `0.0.0.0:8080`), webroot `./webroot`.  
- Empty scaffold left: `internal/scheduler` (Phase 6 — later).

### Do not
- Do not introduce Docker.  
- Do not commit `.env` or `data/*.db`.  
- Do not skip Meat Bag comments on new code.  
- Do not begin Phase 6/7 during the verify pause unless asked.

### Suggested messages from Meat Bag
- While verifying: questions / “it went boom” reports against DEV-VERIFY.  
- When ready: `Verification done. Continue Phase 6.`

---

**Clanker’s note:** Code spine is ready. The win for this break is Meat Bag’s tokens + one ✅ — not more phases.
