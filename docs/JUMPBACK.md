# Jump-Back Point — Subotto (2026-09-08)

**Phase 6 in progress** — listening posts, dark Admin, epochs, announce copy.  
**Handoff:** [HANDOFF-ADMIN-UI.md](HANDOFF-ADMIN-UI.md)  
**Later:** Phase 7 deploy guide (only when Meat Bag asks).

---

## 1. For Meat Bag

### Mental model
1. **Listening post** = one Discord channel under watch → one YouTube playlist (collection window / epoch)
2. **START LISTEN** creates a private playlist; **CEASE** closes the epoch
3. One live listen per channel; videos saved once per channel (won’t re-ingest after a new playlist)
4. Resync stops at the previous epoch boundary
5. Online/offline Discord messages are **global** templates you edit in Admin
6. Port default **50770**; scheduler optional via `RESYNC_INTERVAL_HOURS`

### Commands
```text
just run
just auth-youtube
just start-listen / list-listens / cease-listen   # aliases of mapping recipes
just test / just build
```

Admin: `http://localhost:50770` · `admin` / `ADMIN_PASSWORD`

---

## 2. For Clanker

- Voice: Clanker ↔ Meat Bag; beginner comments
- Product terms: **listening post**, start/cease listen — SIGINT / ops desk flavor
- Libertarian tone via voluntary / sovereignty language only — never name opposing ideologies in copy
- No Docker; commit only on request
- Prefer `/api/listens`; keep legacy aliases working

### Architecture
```
just run
  → Discord gateway (listen → ingest → 💾/♻️/❌)
  → Admin (webroot + /api/*)
  → optional scheduler
  → announce on start/cease listen
```
