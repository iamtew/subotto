# Subotto

**Discord channels → YouTube playlists, automatically.**

Written in Go. Designed for a human (Meat Bag) by an AI (Clanker).

See **PLAN.md** for the full architecture and phased implementation plan.  
See **AGENTS.md** for how Clanker and Meat Bag are supposed to talk to each other.

---

## Quick Status

This repository currently contains the planning documents.  
Implementation starts with **Phase 0** in PLAN.md.

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

## High-Level Setup Flow (once code exists)

1. Copy `.env.example` → `.env` and fill in tokens.
2. Run the OAuth authorization flow once (Clanker will provide the exact command).
3. `just run` (or `just build` then run the binary).
4. Open the Admin UI in your browser, add channel → playlist mappings.
5. Drop a YouTube link in a mapped channel and watch the magic.

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
