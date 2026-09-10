# AGENTS.md – Contributor & Cursor agent conventions

**Project:** Subotto  
**Bot name:** Subotto

## Maintainer (human)

- Owns vision, API keys, Discord server access, testing feedback, and final approval.
- Develops on Windows 10; deploys a native binary to a Linux VPS (EU).
- Creates the Discord application/bot and YouTube OAuth credentials.
- Prefers classic local tooling: Go + Just (no Docker).
- Subotto is operator-owned tooling — keys, epochs, and deploy choices stay with the operator.

## Cursor agent

- Write code, structure the project, add heavy comments aimed at beginners, and explain decisions in plain English.
- Keep docs and comments plain and beginner-friendly. No unexplained magic.
- Product language:
  - **Content listener** — Discord channel → YouTube playlist (**start** / **cease**). One live content listener per channel.
  - **Picture listener** — Discord channel → on-disk images + public OBS slideshow. One live picture listener per channel.
  - Both types may be live on the **same channel at once**.
  - Collection-window length is the operator’s choice — SIGINT vibe, not broadcast.
- **Show episodes** (medium): templates + start/cease + absorb existing live listeners (link only, after deploy/restart) + public `/api/get/episode/{show}`. Still parked: bi-weekly schedules, richer show-runner announce.
- Priorities:
  1. Working, simple, maintainable code over cleverness
  2. Clear configuration and flexibility (content + picture listeners)
  3. Classic local development with **Just** (justfile) as the build system
  4. A simple integrated web server that serves a plain HTML/CSS/JS Admin UI from a `webroot/` folder (plus public `/slideshow/...`)
  5. Good logging so operators do not have to dig too hard
  6. Individual agency in copy and docs — the operator owns the wire
- When in doubt, ask clarifying questions instead of assuming.

## How work proceeds

1. Feed this file into Cursor when starting work. Operator runbooks: [README.md](README.md) (DEV), [docs/DEPLOY.md](docs/DEPLOY.md) (PROD).
2. Build against the product language and parked list above — not against a phase roadmap.
3. Agents generate code with lots of comments.
4. The maintainer tests, provides keys, and reports results.
5. Iterate until Subotto is solid on the Linux VPS (native binary, no containers).

## Git commit style

When the maintainer asks for a commit, use this format:

1. **Clean title** — one short line that says why the commit exists (not a dump of file names).
2. **Body** — a bullet list of changes.
3. **Tone** — short and concise, but do not skip information. Every meaningful change gets a bullet.

Example:

```
Bootstrap Phase 0 Subotto scaffold.

- Add Go module and commented main stub
- Scaffold internal packages, data/, and webroot placeholder
- Add justfile with DEV and PROD recipes
```

Commit only when the maintainer asks. No force-push, no rewriting history that has already been pushed, unless the maintainer explicitly says so.

## Ship it

When the maintainer says **ship it**, that means:

1. Commit the current work (using the commit style above).
2. Push the branch to the configured remote (`git push` / `git push -u` if needed).

Do not force-push unless the maintainer explicitly says so.
