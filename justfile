# ============================================
# Subotto justfile
# DEV  = Windows (Meat Bag's machine)
# PROD = Linux VPS (EU datacenter)
# ============================================
# No Docker. Native Go binaries only.
# Meat Bag: install Just from https://github.com/casey/just

# Default: list all recipes so you can see what's available
default:
    @just --list

# ---------- DEV (Windows) ----------

# Run Subotto in development mode (Windows)
run:
    go run ./cmd/subotto

# One-time YouTube OAuth (opens browser; needs YOUTUBE_CLIENT_ID/SECRET in .env)
auth-youtube:
    go run ./cmd/subotto -youtube-auth

# Open a listening post (CLI; prefer Admin UI START LISTEN to create playlists).
# Usage: just add-mapping DISCORD_CHANNEL_ID YOUTUBE_PLAYLIST_ID
add-mapping channel playlist name="":
    go run ./cmd/subotto -add-mapping-channel {{channel}} -add-mapping-playlist {{playlist}} -add-mapping-name "{{name}}"

start-listen channel playlist name="":
    just add-mapping {{channel}} {{playlist}} "{{name}}"

# List live listening posts.
list-mappings:
    go run ./cmd/subotto -list-mappings

list-listens:
    just list-mappings

# Resume / pause a listen.
enable-mapping channel:
    go run ./cmd/subotto -enable-mapping {{channel}}

enable-listen channel:
    just enable-mapping {{channel}}

disable-mapping channel:
    go run ./cmd/subotto -disable-mapping {{channel}}

pause-listen channel:
    just disable-mapping {{channel}}

# Cease listen (closes epoch; processed_videos stay for dedup).
delete-mapping channel:
    go run ./cmd/subotto -delete-mapping {{channel}}

cease-listen channel:
    just delete-mapping {{channel}}

# Rescan recent Discord messages for YouTube links (REST only, no reactions).
# Usage: just resync DISCORD_CHANNEL_ID
# Optional 2nd arg = message limit (default 100, max 500).
resync channel limit="100":
    go run ./cmd/subotto -resync-channel {{channel}} -resync-limit {{limit}}

# Build native Windows binary (DEV)
build:
    go build -o bin/subotto.exe ./cmd/subotto

# ---------- PROD (Linux) ----------

# Cross-compile a Linux binary from Windows (copy this to the VPS).
# Just sets GOOS/GOARCH for this recipe — works on PowerShell and cmd.
build-linux:
    GOOS=linux GOARCH=amd64 go build -o bin/subotto-linux ./cmd/subotto

# ---------- Utility ----------

# Clean build artifacts (cmd.exe syntax; Just's default shell on Windows)
clean:
    if exist bin rmdir /s /q bin

# Run all Go tests
test:
    go test ./...
