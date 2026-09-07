# PLAN.md – Subotto (Discord → YouTube Playlist Bot in Go)

**Project name:** Subotto  
**Bot name:** Subotto

**Goal:**  
A flexible Discord bot written in **Go** that watches specific Discord text channels for YouTube links and automatically adds those videos to corresponding YouTube playlists. Different channels can map to different playlists. An integrated Admin Web UI (plain HTML/CSS/JS) lets you manage the mappings, view activity, and trigger updates. Designed for a Meat Bag who is new to this stack.

**Clanker ↔ Meat Bag language is required** in docs and comments.

**No Docker.** We build and run native Go binaries using **Just** (justfile) as the classic build system.

---

## 1. Core Requirements (What Subotto Must Do)

### Must Have
- Listen to Discord messages in configured channels.
- Detect YouTube links (youtube.com/watch?v=, youtu.be/, youtube.com/shorts/, etc.).
- Extract clean video ID.
- Look up which YouTube playlist belongs to that Discord channel (or guild + channel).
- Add the video to the playlist using YouTube Data API v3 (`playlistItems.insert`).
- Avoid duplicates (don't add the same video twice to the same playlist if possible).
- Flexible mapping: one channel → one playlist. Support multiple independent mappings.
- Ability to "update / re-sync" a single channel's playlist on demand or on a schedule (process historical messages or just force a check).
- **Integrated Admin Web UI**:
  - Served by the same Go binary
  - Static files live in a `webroot/` folder (plain HTML + CSS + JS)
  - REST/JSON API endpoints for the UI to talk to
  - Manage channel ↔ playlist mappings
  - View recent activity / logs
  - Manually trigger a re-scan of a channel
  - Basic status (bot online, last activity, etc.)
- Heavily commented code aimed at beginners.
- Configuration via environment variables + optional simple config file.
- Cross-platform: develop on Windows 10, deploy native binary on Linux VPS (EU).
- **Just** (justfile) as the build/run system (like a modern Makefile).

### Nice to Have (Phase 2+)
- Slash commands for admins (`/mapchannel`, `/status`, etc.).
- Deduplication across the whole system.
- Support for YouTube Music links / playlists.
- Rate limit handling and graceful retries.
- Simple authentication on the Admin UI (password or basic auth).
- Metrics / how many videos added today.

### Out of Scope (for now)
- Playing music in voice channels.
- Managing Discord roles or moderation.
- Multi-user YouTube accounts (one Google account owns all the target playlists).
- Docker / containers of any kind.

---

## 2. Important Technical Reality Check (Read This, Meat Bag)

**YouTube Data API write operations require OAuth 2.0**, not just an API key.

- An API key is enough for reading public data.
- Adding a video to a playlist (`playlistItems.insert`) requires the authenticated user (the Google account that owns the playlist) to have granted permission via OAuth.
- Therefore Subotto will need:
  1. A Google Cloud project with YouTube Data API v3 enabled.
  2. OAuth 2.0 Client ID (Desktop or Web application type).
  3. A one-time (or occasional) authorization flow so the bot can act on behalf of your Google account.
  4. Storage of the OAuth refresh token so Subotto can keep working without you logging in every day.

Clanker will implement a simple OAuth flow + token storage. You will do the Google Cloud console setup once.

Discord side is simpler: Bot token + Message Content Intent + (optionally) Server Members Intent.

---

## 3. Recommended Tech Stack

| Layer              | Choice                              | Why                                      |
|--------------------|-------------------------------------|------------------------------------------|
| Language           | Go 1.22+                            | Meat Bag requested it. Fast, simple, single binary. |
| Discord            | github.com/bwmarrin/discordgo       | Most popular & maintained Go Discord library. |
| YouTube API        | google.golang.org/api/youtube/v3 + golang.org/x/oauth2 | Official. |
| Web / Admin UI     | net/http (stdlib) or Gin + static files from `webroot/` | Plain HTML/CSS/JS. No frontend build step. |
| Database           | SQLite (modernc.org/sqlite or gorm + sqlite) | Zero-config, perfect for single VPS. Easy backup. |
| Config             | Environment variables + optional YAML/JSON | Flexible. |
| Logging            | log/slog (stdlib)                   | Structured, built-in, easy. |
| Build system       | **Just** (justfile)                 | Classic, simple, cross-platform alternative to Make. |
| Deployment         | Native Go binary + systemd (Linux)  | No containers. Just copy the binary + webroot + data. |

---

## 4. High-Level Architecture

```
┌─────────────────┐     messages      ┌──────────────────────┐
│  Discord Server │ ───────────────►  │  Subotto (Go binary) │
│  (channels)     │                   │  - discordgo         │
└─────────────────┘                   │  - message handler   │
                                      │  - YouTube link parse│
                                      │  - integrated HTTP   │
                                      └──────────┬───────────┘
                                                 │
                         ┌───────────────────────┼───────────────────────┐
                         │                       │                       │
                         ▼                       ▼                       ▼
              ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
              │ YouTube Data API │    │  SQLite DB       │    │  webroot/        │
              │ (OAuth)          │    │  - mappings      │    │  (HTML/CSS/JS)   │
              └──────────────────┘    │  - processed     │    │  Admin UI        │
                                      │  - activity log  │    └──────────────────┘
                                      │  - OAuth tokens  │
                                      └──────────────────┘
```

**Data model (simple):**

- `channel_mappings`: discord_channel_id, guild_id, youtube_playlist_id, name/label, enabled, created_at
- `processed_videos`: video_id, playlist_id, discord_message_id, added_at (for dedup)
- `activity_log`: timestamp, event_type, details (json), success
- OAuth token storage (file or DB)

---

## 5. Project Structure (Suggested)

```
subotto/
├── cmd/
│   └── subotto/
│       └── main.go                 # Entry point
├── internal/
│   ├── config/                     # Load env + files
│   ├── discord/                    # Bot setup, handlers
│   ├── youtube/                    # OAuth, client, add-to-playlist
│   ├── db/                         # SQLite models & queries
│   ├── web/                        # HTTP server, API routes, static file serving
│   ├── parser/                     # YouTube URL extraction
│   └── scheduler/                  # Optional interval jobs
├── webroot/                        # <-- Plain HTML / CSS / JS for Admin UI
│   ├── index.html
│   ├── css/
│   ├── js/
│   └── ...
├── data/                           # Runtime data (gitignored)
│   └── subotto.db
├── justfile                        # Build / run / test commands
├── .env.example
├── AGENTS.md
├── PLAN.md
├── README.md
└── go.mod
```

All code heavily commented for Meat Bag.

---

## 6. Implementation Phases (Feed this into Cursor)

### Phase 0 – Project Bootstrap (Do this first)
- Initialize Go module.
- Create the folder structure above.
- Add AGENTS.md, PLAN.md, README.md.
- Create a basic `main.go` that prints "Subotto is online. Clanker stands ready, Meat Bag."
- Create a starter `justfile` with clear **DEV (Windows)** and **PROD (Linux)** targets (`run`, `build`, `build-linux`, `clean`, etc.).
- Create `.env.example`.
- Create a minimal `webroot/index.html` so the web server has something to serve.

### Phase 1 – Configuration & Database
- Config loading (env vars).
- SQLite setup + schema for the tables above.
- Basic structured logging with `log/slog`.

### Phase 2 – YouTube OAuth & Client
- OAuth2 flow (local callback or desktop-style).
- Save / load refresh token.
- Helper: `AddVideoToPlaylist(playlistID, videoID) error`
- Good error messages when quota or permissions fail.

### Phase 3 – Discord Bot Core
- Connect with discordgo.
- MessageCreate handler.
- Only process messages in mapped channels.
- Extract YouTube video IDs robustly.
- Call YouTube add function.
- Record in DB + activity log.
- Optional reaction or short reply in Discord.

### Phase 4 – Channel ↔ Playlist Mapping
- CRUD operations in the DB layer.
- Support enabling/disabling mappings.
- Optional: "resync" that goes back N messages in a channel.

### Phase 5 – Integrated Admin Web UI
- Go HTTP server (stdlib or Gin) that:
  - Serves static files from `webroot/`
  - Exposes clean JSON API endpoints (`/api/mappings`, `/api/activity`, `/api/status`, `/api/resync`, etc.)
- Plain HTML + CSS + vanilla JS (or light Alpine/HTMX if desired later) in `webroot/`.
- Simple password protection (basic auth or session cookie).
- Dashboard, mappings table, activity log, resync button.

### Phase 6 – Scheduling & Polish
- Optional background job for periodic re-scans.
- Rate-limit handling.
- Graceful shutdown.
- Health / status endpoint.
- Final justfile recipes and README deployment section.

### Phase 7 – Deployment Guide (Native)
- How to cross-compile the Linux PROD binary from Windows DEV (`just build-linux`).
- How to run on the VPS as a systemd service.
- How to keep the binary + webroot + data directory in sync.
- Backup strategy for the SQLite file.
- Optional reverse proxy (Caddy/Nginx) in front of the Admin UI.

---

## 7. Configuration Keys (what Meat Bag needs to provide)

```env
# Discord
DISCORD_BOT_TOKEN=...
DISCORD_GUILD_ID=...          # optional

# YouTube / Google
YOUTUBE_CLIENT_ID=...
YOUTUBE_CLIENT_SECRET=...
YOUTUBE_REDIRECT_URL=http://localhost:8080/oauth/callback

# Admin UI
ADMIN_PASSWORD=change-me-please
ADMIN_PORT=8080
ADMIN_HOST=0.0.0.0            # or 127.0.0.1 if you put a reverse proxy in front

# Database
DATABASE_PATH=./data/subotto.db

# Optional
LOG_LEVEL=info
RESYNC_INTERVAL_HOURS=0       # 0 = disabled
```

---

## 8. Justfile Philosophy (DEV = Windows, PROD = Linux)

We use **Just** (https://github.com/casey/just) as our build system.

**Important:**
- **DEV environment** = Windows 10 (Meat Bag’s machine)
- **PROD environment** = Linux VPS (EU datacenter)

The justfile **must** have clear, separate targets for both.

Clanker will create a justfile with at least these recipes:

```just
# ============================================
# Subotto justfile
# DEV  = Windows
# PROD = Linux
# ============================================

# Default recipe
default:
    @just --list

# ---------- DEV (Windows) ----------

# Run Subotto in development mode (Windows)
run:
    go run ./cmd/subotto

# Build native Windows binary (DEV)
build:
    go build -o bin/subotto.exe ./cmd/subotto

# ---------- PROD (Linux) ----------

# Cross-compile Linux binary from Windows (for PROD)
build-linux:
    set GOOS=linux
    set GOARCH=amd64
    go build -o bin/subotto-linux ./cmd/subotto

# Alternative (works in Git Bash / WSL / PowerShell with env):
# build-linux:
#     GOOS=linux GOARCH=amd64 go build -o bin/subotto-linux ./cmd/subotto

# ---------- Utility ----------

# Clean build artifacts
clean:
    if exist bin rmdir /s /q bin
    # (or rm -rf bin/ when running under Unix-like shell)

# Run tests
test:
    go test ./...
```

Notes for Meat Bag:
- On Windows you will mostly use: `just run` and `just build`
- When you are ready to deploy: `just build-linux` → copy `bin/subotto-linux` + `webroot/` + `data/` to the VPS
- On the Linux VPS you just run the binary (or put it under systemd). No Just required on the server.

Clanker must keep the Windows (DEV) and Linux (PROD) targets clearly separated and working.

---

## 9. How Meat Bag Should Work With Cursor

1. Open the project folder in Cursor.
2. Make sure AGENTS.md and PLAN.md are in the root.
3. Start with Phase 0. Tell Cursor:  
   "Clanker, implement Phase 0 from PLAN.md for Subotto. No Docker. Use Just. Keep comments friendly for Meat Bag."
4. After each phase, test what you can, then move to the next.
5. When you hit the OAuth part, Clanker will give you exact Google Cloud Console steps.
6. When Subotto is running locally, invite the bot to your test server and drop a YouTube link in a mapped channel.

---

## 10. Immediate Next Steps for Meat Bag (Right Now)

1. Create a Discord Application → Bot → copy token. Enable **Message Content Intent**.
2. Create a Google Cloud project → enable YouTube Data API v3 → create OAuth 2.0 Client ID.
3. Drop these updated files into your repo.
4. Install Just if you don’t have it yet (https://github.com/casey/just#installation).
5. Tell Clanker (in Cursor) to start Phase 0 for Subotto.

---

**Clanker’s promise:**  
No Docker. Classic Just + Go. Plain webroot for the Admin UI. Simple, readable code with lots of comments for Meat Bag.

Subotto is ready to be built the old-school way.
