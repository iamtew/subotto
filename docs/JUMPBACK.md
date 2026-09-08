# Jump-Back Point — Subotto (2026-09-08)

**Phases 0–8** — content + picture listeners, Admin tabs, public OBS slideshow, Linux package / deploy guide.  
**PROD guide:** [DEPLOY.md](DEPLOY.md)  
**Prior Phase 7 brief:** [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md)  
**Phase 6 context:** [HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md)

---

## 1. Operator notes

### Mental model
1. **Content listener** = Discord channel → YouTube playlist (collection window / epoch)
2. **Picture listener** = Discord channel → images on disk under `data/pictures/{slug}/` + public slideshow
3. **START** content listener creates a **public** playlist; **CEASE** closes the epoch
4. One live content listener **and** one live picture listener may share the same channel
5. Content resync stops at the previous content-listener epoch boundary
6. ONLINE/OFFLINE Discord **notices** are global templates (content listeners) in Admin
7. Public OBS overlay: `/slideshow/latest` or `/slideshow/{slug}` (no Basic Auth). Credit shows `Author: NAME` on frosted glass; reactions float nearby (Twemoji + Discord CDN custom emotes). **Reaction render** 1x–10x multiplies how many emote sprites appear. Configure via picture listener **Settings**.
8. Port default **50770**; scheduler optional via `RESYNC_INTERVAL_HOURS` (usually `0`)
9. PROD is headless: copy `.env` + `data/` (DB + `pictures/`). Re-auth without a desktop = SSH tunnel cookbook in DEPLOY.md

### Commands
```text
just run
just auth-youtube
just start-listen / list-listens / cease-listen
just start-picture-listen / list-picture-listens / cease-picture-listen / resync-pictures
just test / just build / just build-linux / just package-linux
```

Admin: `http://localhost:50770` · `admin` / `ADMIN_PASSWORD`  
Slideshow: `http://localhost:50770/slideshow/latest`

Content reacts: **💾** · **♻️ DUPE** · **🛑 OLD** · **❌**  
Picture reacts: **🖼️** · **♻️ DUPE** · **❌**

PROD: `just package-linux` → `dist/subotto-linux.zip` → [DEPLOY.md](DEPLOY.md)

### Parked (later)
- **Shows** — bi-weekly / scheduled listeners, show-runner Discord announce, richer automation

---

## 2. Agent notes

- Keep docs and comments plain and beginner-friendly
- Product terms: **content listener**, **picture listener**, start/cease, **notices**, public **slideshow**
- Prefer voluntary / operator-owned language in copy — never name opposing ideologies
- No Docker; commit only on request
- Prefer `/api/listens` and `/api/picture-listens`; keep legacy content aliases working
- Public routes (no auth): `/slideshow/...`, `/api/slideshow/{slug}`, `/media/pictures/...`

### Architecture
```
just run / just build-linux / just package-linux
  → Discord gateway
       content:  links → ingest → 💾 / ♻️DUPE / 🛑OLD / ❌
       picture:  attachments → data/pictures/{slug}/ → 🖼️
  → Admin (webroot + /api/* Basic Auth, tabs)
  → Public slideshow (OBS Browser Source)
```
