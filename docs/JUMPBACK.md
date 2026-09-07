# Jump-Back Point — Subotto (2026-09-07)

Sleep well, Meat Bag. This file is how you (and Clanker) pick up without re-deriving the whole night.

---

## 1. For Meat Bag (you) — learn + resume

### Where we left off
Subotto can watch a Discord channel and push YouTube links into a playlist. Phases **0–4** are in code on `master`. **You have not necessarily finished live API setup yet** — that is still the first real-world test.

Phase 4 added mapping management without the Admin UI: list / enable / disable / delete, plus a one-shot history resync.

### Minimum mental model
1. **Config** (`.env`) → tokens and paths  
2. **SQLite** (`./data/subotto.db`) → mappings, dedup, activity, OAuth token  
3. **YouTube OAuth once** → refresh token saved in DB  
4. **Discord bot** → only **enabled** mapped channels matter  
5. **Parser + ingest** → pulls video IDs out of messy message text (live and resync share this)

### Commands to remember
```text
just run                          # start the bot (needs Discord + YouTube ready)
just auth-youtube                 # one-time Google login
just add-mapping CHANNEL PLAYLIST # create/update a mapping (enabled)
just list-mappings                # see all mappings
just enable-mapping CHANNEL       # turn a mapping back on
just disable-mapping CHANNEL      # pause without deleting
just delete-mapping CHANNEL       # remove mapping (dedup history stays)
just resync CHANNEL [limit]       # scan recent history (default 100, max 500)
just test / just build            # sanity
```

### First session tomorrow (suggested order)
1. Skim [PROGRESS.md](PROGRESS.md) (what exists).  
2. Fill `.env` from `.env.example` if empty.  
3. Discord portal: enable **Message Content Intent**, invite bot.  
4. Google Cloud: enable API, OAuth client, redirect URI.  
5. `just auth-youtube` → should print your channel name.  
6. Developer Mode in Discord → copy channel ID → `just add-mapping ...`  
7. `just run` → post a YouTube link → expect ✅ (or ♻️ / ❌).  
8. Optional: `just resync CHANNEL` to backfill recent messages (no emoji spam on old posts).

### Go concepts you already touched (plain English)
| Idea                    | Where you saw it                           |
| ----------------------- | ------------------------------------------ |
| Packages / `internal/`  | Code only our module imports               |
| `log/slog`              | Structured logs (`key=value`)              |
| `database/sql` + SQLite | One DB file, queries in `internal/db`      |
| OAuth refresh token     | Login once; Subotto renews access later    |
| Discord intents         | Bot must be allowed to *read message text* |
| Shared ingest           | Live messages and resync use the same rules|
| Graceful shutdown       | Ctrl+C → signal → clean close              |

### If something goes boom
- No links processed → channel not mapped, mapping **disabled**, or Message Content Intent off  
- YouTube errors → re-run `just auth-youtube`, check playlist ownership / API enabled  
- Quota → Google daily limit; wait or raise quota (resync burns quota fast)  
- Secrets → never commit `.env`; it is gitignored  

### Docs map
| File | Use when |
|------|----------|
| [AGENTS.md](../AGENTS.md) | How Clanker ↔ Meat Bag work; **git message style** |
| [PLAN.md](../PLAN.md) | Full roadmap Phases 0–7 |
| [PROGRESS.md](PROGRESS.md) | What this arc delivered |
| This file | Resume + learning anchors |

---

## 2. For Clanker (next agent session)

### Hard facts
- **Project:** Subotto — Discord → YouTube playlist bot in Go, **no Docker**, **Just** build system.  
- **Voice:** Clanker talking to Meat Bag; beginner-friendly comments.  
- **Git style (required):** clean title + bullet body; short but complete; only when asked. Avoid the substring `commit` in message bodies if the environment mangles it with Co-authored-by injection. Prefer `-F` file for messages on Windows PowerShell.  
- **Done:** Phases 0–4 on `master` (see PROGRESS.md + `git log`).  
- **Next PLAN phase:** **Phase 5** — Admin Web UI on `webroot/` + `/api/...` (mappings, activity, status, resync button). Phase 6 = scheduler using `RESYNC_INTERVAL_HOURS`.

### Architecture snapshot
```
Discord MessageCreate
  → channel has enabled mapping? (db)
  → ingest.ProcessContent (parser + dedup + youtube + activity)
  → react ✅ / ♻️ / ❌

just resync CHANNEL
  → REST ChannelMessages (paginated, cap 500)
  → same ingest.ProcessContent
  → NO reactions on history
```

### Important implementation notes
- Pure Go SQLite: `modernc.org/sqlite` (no CGO) — keep it that way for Windows→Linux cross-compile.  
- YouTube write = OAuth (`YoutubeForceSslScope`); token key in DB: `youtube`.  
- OAuth callback listens on port from `YOUTUBE_REDIRECT_URL` (default 8080) — same port reserved later for Admin UI; auth is one-shot.  
- Mapping UX: CLI `just add/list/enable/disable/delete-mapping` + `just resync`; Admin UI not built yet.  
- Phase 3+ `just run` **exits** if Discord token or YouTube token missing.  
- Empty package still scaffolding: `internal/web`, `internal/scheduler`.  
- `DeleteMapping` leaves `processed_videos` alone (intentional dedup preserve).

### Do not
- Do not introduce Docker.  
- Do not commit `.env` or `data/*.db`.  
- Do not skip Meat Bag explanations / comments on new code.  
- Do not start Phase 6 scheduler before Phase 5 UI unless Meat Bag redirects.

### Suggested first message from Meat Bag after sleep
> Clanker, read docs/JUMPBACK.md and docs/PROGRESS.md. Continue Phase 5.

---

**Clanker’s note:** Phases 0–4 are the spine + mapping tools. Phase 5 makes them clickable in the browser. Daytime win = Meat Bag’s tokens + one green ✅ in Discord.
