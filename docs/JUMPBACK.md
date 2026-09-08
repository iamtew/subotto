# Jump-Back Point — Subotto (2026-09-08)

**Phases 0–7 complete enough** — listeners, Admin, Linux package / deploy guide.  
**PROD guide:** [DEPLOY.md](DEPLOY.md)  
**Prior Phase 7 brief:** [HANDOFF-PHASE7.md](HANDOFF-PHASE7.md)  
**Phase 6 context:** [HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md)

---

## 1. For Meat Bag

### Mental model
1. **Listener** = one Discord channel under watch → one YouTube playlist (collection window / epoch)
2. **START LISTENER** creates a **public** playlist; **CEASE** closes the epoch
3. One live listener per channel; videos saved once per channel (won’t re-ingest after a new playlist)
4. Resync stops at the previous epoch boundary
5. ONLINE/OFFLINE Discord **notices** are global templates in Admin
6. Port default **50770**; scheduler optional via `RESYNC_INTERVAL_HOURS` (usually `0`)
7. PROD is headless: copy `.env` + `data/subotto.db` (YouTube refresh token is in the DB). Re-auth without a desktop = SSH tunnel cookbook in DEPLOY.md

### Commands
```text
just run
just auth-youtube
just start-listen / list-listens / cease-listen   # aliases of mapping recipes
just test / just build / just build-linux / just package-linux
```

Admin: `http://localhost:50770` · `admin` / `ADMIN_PASSWORD`

Reactions: **💾** · **♻️ DUPE** · **🛑 OLD** · **❌**

PROD: `just package-linux` → `dist/subotto-linux.zip` → [DEPLOY.md](DEPLOY.md)

---

## 2. For Clanker

- Voice: Clanker ↔ Meat Bag; beginner comments
- Product terms: **listener**, start/cease listener, **notices** — SIGINT / ops desk flavor
- Libertarian tone via voluntary / sovereignty language only — never name opposing ideologies in copy
- No Docker; commit only on request
- Prefer `/api/listens`; keep legacy aliases working
- Phase 7 delivered: `package-linux`, `deploy/subotto.service`, [DEPLOY.md](DEPLOY.md)

### Architecture
```
just run / just build-linux / just package-linux
  → Discord gateway (listen → ingest → 💾 / ♻️DUPE / 🛑OLD / ❌)
  → Admin (webroot + /api/*)
  → optional scheduler
  → notices on start/cease listener
```
