# ============================================
# Subotto justfile
# DEV  = Windows (Meat Bag's machine)
# PROD = Linux VPS (EU datacenter)
# ============================================
# No Docker. Native Go binaries only.
# Meat Bag: install Just from https://github.com/casey/just
#
# Cursor / Git for Windows run recipes with sh, not cmd.exe.
# Keep recipe bodies POSIX (rm, GOOS=linux go build). powershell.exe is fine to call.
#
# Listeners, resync, shows, broadcasts: Admin UI, not Just.

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

# Build native Windows binary (DEV)
build:
    go build -o bin/subotto.exe ./cmd/subotto

# ---------- PROD (Linux) ----------

# Cross-compile a Linux binary from Windows (copy this to the VPS).
build-linux:
    GOOS=linux GOARCH=amd64 go build -o bin/subotto-linux ./cmd/subotto

# Wipe bin/dist, rebuild Linux binary, zip webroot + binary (no .env / no SQLite).
# Output: dist/subotto-linux.zip — see docs/DEPLOY.md
package-linux: clean build-linux
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-linux.ps1

# ---------- Utility ----------

# Clean build artifacts (sh: Git Bash / Cursor)
clean:
    rm -rf bin dist

# Run all Go tests
test:
    go test ./...
