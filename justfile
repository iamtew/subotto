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

# Map a Discord channel to a YouTube playlist (until Admin UI exists).
# Usage: just add-mapping DISCORD_CHANNEL_ID YOUTUBE_PLAYLIST_ID
# Optional 3rd arg = label name.
add-mapping channel playlist name="":
    go run ./cmd/subotto -add-mapping-channel {{channel}} -add-mapping-playlist {{playlist}} -add-mapping-name "{{name}}"

# List all channel ↔ playlist mappings (including disabled).
list-mappings:
    go run ./cmd/subotto -list-mappings

# Enable a previously disabled mapping.
# Usage: just enable-mapping DISCORD_CHANNEL_ID
enable-mapping channel:
    go run ./cmd/subotto -enable-mapping {{channel}}

# Disable a mapping (bot ignores that channel until re-enabled).
# Usage: just disable-mapping DISCORD_CHANNEL_ID
disable-mapping channel:
    go run ./cmd/subotto -disable-mapping {{channel}}

# Delete a mapping permanently (processed_videos stay for dedup).
# Usage: just delete-mapping DISCORD_CHANNEL_ID
delete-mapping channel:
    go run ./cmd/subotto -delete-mapping {{channel}}

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
