# Jump-Back Point — Subotto (2026-09-08)

**Phases 0–8** — content + picture listeners, Admin tabs, public OBS slideshow (+ polish).  
**Phase 8 handoff (new agent):** [HANDOFF-PHASE8.md](HANDOFF-PHASE8.md)  
**PROD guide:** [DEPLOY.md](DEPLOY.md)  
**Prior Phase 7 brief:** [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md)

**Git:** `master` is **2 commits ahead of origin** (`ffec62a` Phase 8, `86a66f3` slideshow polish) — **not pushed**.

---

## 1. Operator notes

### Mental model
1. **Content listener** = Discord channel → YouTube playlist (collection window / epoch)
2. **Picture listener** = Discord channel → images on disk under `data/pictures/{slug}/` + public slideshow
3. **START** content listener creates a **public** playlist; **CEASE** closes the epoch
4. One live content listener **and** one live picture listener may share the same channel
5. Content / picture resync stop at previous epoch boundaries for that listener type
6. ONLINE/OFFLINE Discord **notices** are global templates (content listeners) in Admin
7. Public OBS overlay: `/slideshow/latest` or `/slideshow/{slug}` (no Basic Auth). Credit: `Author: NAME` on frosted glass; reaction **floaters** (Twemoji + Discord CDN). Settings: corner, advance, scales, **reaction render 1x–25x** (floater stage grows with it)
8. Port default **50770**; scheduler optional via `RESYNC_INTERVAL_HOURS` (usually `0`)
9. PROD is headless: copy `.env` + `data/` (DB + `pictures/`). Same Discord bot token → do not run DEV + PROD together

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

Content + picture reacts: **💾** · **♻️ DUPE** · **🛑 OLD** · **❌**  
Slideshow floaters: human reacts only (bot status chrome excluded)

### Parked (later)
- **Shows** — bi-weekly / scheduled listeners, show-runner Discord announce, richer automation

---

## 2. Agent notes

- Read [HANDOFF-PHASE8.md](HANDOFF-PHASE8.md) first when continuing Phase 8 polish
- Product terms: **content listener**, **picture listener**, start/cease, **notices**, public **slideshow**
- Prefer `/api/listens` and `/api/picture-listens`; public `/slideshow/...`, `/api/slideshow/{slug}`, `/media/pictures/...`
- No Docker; commit only on request; no push unless asked

### Architecture
```
just run
  → Discord gateway
       content:  links → ingest → YouTube playlist
       picture:  attachments → data/pictures/{slug}/
  → Admin (Basic Auth, tabs)
  → Public slideshow (OBS Browser Source)
```
