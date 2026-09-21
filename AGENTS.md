# AGENTS.md

Operator-owned Go bot. Just, no Docker. Maintainer: Windows 10 DEV → Linux VPS
native binary. Maintainer owns keys, Discord app, YouTube OAuth, Twitch OAuth, deploy, approval.

## Product
- Content listener: Discord channel → YouTube playlist (start/cease). One live per channel.
- Picture listener: Discord channel → disk images + public OBS slideshow. One live per channel.
- Both may be live on the same channel. Collection window = operator choice (SIGINT vibe, not broadcast).
- Broadcasts: named multi-channel Discord sends; standalone or show-template `{{…}}` from live episode; Admin Fire or `GET /api/broadcasts/{slug}/fire` (Basic `api` / `API_PASSWORD`).
- Admin: public `/` landing; Discord OAuth if `SUPERADMIN_DISCORD_ID` + Discord client id/secret; extra Discord IDs in Admin Dashboard (superadmin); optional Basic `admin` / `ADMIN_PASSWORD`. UI at `/admin`. Superadmin **❌** on a bot-stamped listener post hides pictures or skips a playlist video.
- Hesh Helper: OpenRouter on mention or reply-to-bot (Discord) or @login / reply-to-bot (Twitch); model, system prompt, and optional channel memory in Admin **AI**.
- Twitch chat: optional user OAuth; Subotto chats as that account in one Admin-chosen channel (not necessarily that account’s).
- Show episodes: templates + start/cease + absorb existing live listeners (link only, after deploy/restart) + public `/api/get/episode/{show}`. Parked: bi-weekly schedules, richer show-runner announce.

## Agent
- Simple working code; Admin is `webroot/` HTML/CSS/JS, landing at `/`, UI at `/admin`, public `/slideshow/...`. Log enough for operators. Ask if unsure. Do not invent a phase roadmap.
- Plain beginner comments in code and docs; no unexplained magic.
- Runbooks: [README.md](README.md) (DEV), [docs/DEPLOY.md](docs/DEPLOY.md) (PROD).
- Commit only when asked: one-line why-title + change bullets. No force-push / no rewrite of pushed history unless the maintainer says so.
- **ship it** = that commit + `git push` (`-u` if needed). Same force-push rule.
