#!/usr/bin/env bash
# One-command deploy: pull both repos, rebuild, restart the systemd units, and
# health-check. Run after `git push` to roll out the latest.
#
#   bash ~/uran-api/deploy/deploy.sh
#
# Override paths/URLs via env if your layout differs:
#   API_DIR=/root/uran-api WEB_DIR=/root/uran-web bash deploy/deploy.sh
set -euo pipefail

API_DIR=${API_DIR:-/root/uran-api}
WEB_DIR=${WEB_DIR:-/root/uran-web}
API_HEALTH=${API_HEALTH:-http://localhost:8080/healthz}
WEB_HEALTH=${WEB_HEALTH:-http://localhost:3000/}
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

say "API — pull + build"
git -C "$API_DIR" pull --ff-only
( cd "$API_DIR" && go build -o /usr/local/bin/uran-api ./cmd/api )

say "API — restart"
systemctl restart uran-api

say "Web — pull + build"
git -C "$WEB_DIR" pull --ff-only
( cd "$WEB_DIR" && npm ci --no-audit --no-fund && npm run build )

say "Web — restart"
systemctl restart uran-web

say "Health"
sleep 3
printf 'api: '; curl -fsS "$API_HEALTH" && echo
curl -fsS -o /dev/null -w 'web: %{http_code}\n' "$WEB_HEALTH"

say "Deployed ✓"
