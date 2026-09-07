# Jump-Back Point — Subotto (2026-09-07)

Sleep well, Meat Bag. This file is how you (and Clanker) pick up without re-deriving the whole night.

---

## 1. For Meat Bag (you) — learn + resume

### Where we left off
Subotto can, in theory, watch a Discord channel and push YouTube links into a playlist. Code for Phases 0–3 is on `master`. **You have not necessarily finished live API setup yet** — that is the first real-world test when you wake up.

### Minimum mental model
1. **Config** (`.env`) → tokens and paths  
2. **SQLite** (`./data/subotto.db`) → mappings, dedup, activity, OAuth token  
3. **YouTube OAuth once** → refresh token saved in DB  
4. **Discord bot** → only **mapped** channels matter  
5. **Parser** → pulls video IDs out of messy message text  

### Commands to remember
```text
just run                          # start the bot (needs Discord + YouTube ready)
just auth-youtube                 # one-time Google login
just add-mapping CHANNEL PLAYLIST # map a channel until Admin UI exists
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

### Go concepts you already touched (plain English)
| Idea | Where you saw it |
|------|------------------|
| Packages / `internal/` | Code only our module imports |
| `log/slog` | Structured logs (`key=value`) |
| `database/sql` + SQLite | One DB file, queries in `internal/db` |
| OAuth refresh token | Login once; Subotto renews access later |
| Discord intents | Bot must be allowed to *read message text* |
| Graceful shutdown | Ctrl+C → signal → clean close |

### If something goes boom
- No links processed → channel not mapped, or Message Content Intent off  
- YouTube errors → re-run `just auth-youtube`, check playlist ownership / API enabled  
- Quota → Google daily limit; wait or raise quota  
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
- **Done:** Phases 0–3 on `master` (see PROGRESS.md + `git log`).  
- **Next PLAN phase:** **Phase 4** — mapping CRUD (enable/disable), optional channel history resync. Then Phase 5 Admin UI on `webroot/` + `/api/...`.

### Architecture snapshot
```
Discord MessageCreate
  → channel has enabled mapping? (db)
  → parser.ExtractVideoIDs(content)
  → dedup processed_videos
  → youtube.AddVideoToPlaylist
  → mark processed + activity_log
  → react ✅ / ♻️ / ❌
```

### Important implementation notes
- Pure Go SQLite: `modernc.org/sqlite` (no CGO) — keep it that way for Windows→Linux cross-compile.  
- YouTube write = OAuth (`YoutubeForceSslScope`); token key in DB: `youtube`.  
- OAuth callback listens on port from `YOUTUBE_REDIRECT_URL` (default 8080) — same port reserved later for Admin UI; auth is one-shot.  
- Mapping UX today: CLI `just add-mapping` / flags; Admin UI not built.  
- Phase 3 `just run` **exits** if Discord token or YouTube token missing (stricter than Phase 1/2).  
- Empty packages still scaffolding: `internal/web`, `internal/scheduler`.

### Do not
- Do not introduce Docker.  
- Do not commit `.env` or `data/*.db`.  
- Do not skip Meat Bag explanations / comments on new code.  
- Do not start Phase 5 UI before Phase 4 CRUD unless Meat Bag redirects.

### Suggested first message from Meat Bag after sleep
> Clanker, read docs/JUMPBACK.md and docs/PROGRESS.md. Continue Phase 4.

---

**Clanker’s note:** Phases 0–3 are the spine. Phase 4 makes mappings manageable; Phase 5 makes them clickable. Overnight win = code ready; daytime win = Meat Bag’s tokens + one green ✅ in Discord.
