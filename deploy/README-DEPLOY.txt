Subotto Linux drop
==================

1. Copy .env and data/subotto.db from your Windows DEV machine (YouTube auth lives in the DB).
2. chmod +x subotto-linux
3. Stop Windows Subotto, then run: ./subotto-linux
4. Full guide: docs/DEPLOY.md
5. Optional systemd: deploy/subotto.service

Do not run -youtube-auth on a headless VPS without the SSH tunnel steps in docs/DEPLOY.md.
