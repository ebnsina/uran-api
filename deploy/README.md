# Deploy (single-node)

systemd units + a one-command deploy script for the all-in-one test/box setup
(control plane + dashboard on one machine).

## One-time setup (as root)

```sh
cd ~/uran-api && git pull

# Dashboard env (adapter-node reads these). Match ORIGIN to the URL you open.
cat > ~/uran-web/.env <<'ENV'
URAN_API_URL=http://localhost:8080
ORIGIN=http://YOUR_SERVER_IP:3000
PORT=3000
HOST=0.0.0.0
ENV

# Stop any old nohup processes, then hand over to systemd. Match the exact
# process name (`uran-api`) — a full-path pattern misses a bare-name launch.
pkill -x uran-api 2>/dev/null || true
pkill -f '/usr/local/bin/uran-api' 2>/dev/null || true
pkill -f 'node build' 2>/dev/null || true

install -m 644 deploy/uran-api.service deploy/uran-web.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now uran-api uran-web
```

The API `.env` (`~/uran-api/.env`) must already contain every required variable
(see `.env.example`).

## Deploy the latest

```sh
bash ~/uran-api/deploy/deploy.sh
```

Pulls both repos, rebuilds, restarts the units, health-checks.

## Handy

```sh
systemctl status uran-api uran-web      # state
journalctl -u uran-api -f               # live API logs
journalctl -u uran-web -f               # live web logs
systemctl restart uran-api              # restart one
```
