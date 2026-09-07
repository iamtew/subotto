# Subotto

**Discord channels → YouTube playlists, automatically.**

Written in Go. Designed for a human (Meat Bag) by an AI (Clanker).

See **PLAN.md** for the full architecture and phased implementation plan.  
See **AGENTS.md** for how Clanker and Meat Bag are supposed to talk to each other.

---

## Quick Status

**Phase 3 done:** Discord bot listens for YouTube links in mapped channels and adds them to playlists.  
Prereqs: `.env` tokens, `just auth-youtube`, Message Content Intent on, then:

```
just add-mapping DISCORD_CHANNEL_ID YOUTUBE_PLAYLIST_ID
just run
```

Reactions: ✅ added, ♻️ already on playlist, ❌ failed.  
Next up: **Phase 4** (full mapping CRUD + resync). See PLAN.md.

**No Docker.** We use **Just** + native Go binaries.

---

## What Subotto Does

- Watches Discord text channels you configure.
- Detects YouTube video links.
- Adds those videos to the YouTube playlist mapped to that channel.
- Different channels can point to different playlists.
- Integrated Admin Web UI (plain HTML/CSS/JS served from `webroot/`).
- Optional re-scan / update of a channel’s history.

---

## Prerequisites (Meat Bag Checklist)

- [ ] Go 1.22+ installed
- [ ] Just installed (https://github.com/casey/just)
- [ ] Discord Application + Bot Token (Message Content Intent enabled)
- [ ] Google Cloud project with YouTube Data API v3 enabled
- [ ] OAuth 2.0 Client ID + Secret
- [ ] A Discord server where you can invite the bot
- [ ] YouTube playlists that the Google account you authorize owns

---

## High-Level Setup Flow

1. Copy `.env.example` → `.env` and fill in tokens.
2. Google Cloud: enable YouTube Data API v3, create OAuth client, add redirect `http://localhost:8080/oauth/callback`.
3. `just auth-youtube` once (browser login; refresh token saved in SQLite).
4. `just run` (or `just build` then run the binary).
5. Open the Admin UI in your browser, add channel → playlist mappings (Phase 5).
6. Drop a YouTube link in a mapped channel and watch the magic (Phase 3+).

---

## Development vs Production

|                  | DEV – Windows 10                  | PROD – Linux VPS (EU)                  |
|------------------|-----------------------------------|----------------------------------------|
| Build            | `just build` → `bin/subotto.exe`  | `just build-linux` → `bin/subotto-linux` |
| Run              | `just run`                        | Run the Linux binary (or systemd)      |
| Admin UI         | http://localhost:8080             | Same binary, optionally behind reverse proxy |
| Database         | SQLite in `./data`                | Same, keep the `data/` folder          |
| Web UI files     | `webroot/`                        | Copy `webroot/` next to the binary     |
| Secrets          | `.env` file                       | `.env` or environment variables        |

---

## Philosophy

- No Docker. Native binaries only.
- **Just** as the classic build system.
- Plain HTML/CSS/JS in `webroot/` for the Admin UI.
- Prefer simple, readable Go over clever abstractions.
- Heavy comments written for a beginner.
- Configuration over code changes.

Clanker is ready. Let’s build Subotto the classic way, Meat Bag.
