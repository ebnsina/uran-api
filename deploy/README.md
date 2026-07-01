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

## Builder + controller (push → build → live)

The build worker and k8s controller turn a deploy into a running, routed pod.

```sh
# 1. Extra env for builder + controller (append to the SAME ~/uran-api/.env —
#    each process reads only its own subset, so one file serves all three).
cat >> ~/uran-api/.env <<'ENV'
URAN_REGISTRY=localhost:5000
URAN_BUILD_WORKDIR=/var/lib/uran/builds
URAN_CERT_ISSUER=uran-selfsigned
URAN_BACKUP_ENDPOINT=http://minio.uran-system:9000
URAN_BACKUP_BUCKET=uran-backups
URAN_BACKUP_ACCESS_KEY=change-me
URAN_BACKUP_SECRET_KEY=change-me
ENV
mkdir -p /var/lib/uran/builds

# 2. cert-manager (for TLS on service routes) + a self-signed cluster issuer.
CMVER=$(curl -s https://api.github.com/repos/cert-manager/cert-manager/releases/latest \
  | grep -oP '"tag_name": "\K[^"]+')
kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CMVER}/cert-manager.yaml"
kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=120s
kubectl apply -f ~/uran-api/deploy/uran-selfsigned.yaml

# 3. Install + start the two units.
install -m 644 deploy/uran-builder.service deploy/uran-controller.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now uran-builder uran-controller
systemctl status uran-builder uran-controller --no-pager | grep -E 'Active|Main PID'
```

Then trigger a deploy from the dashboard (or `uran deploy`) and watch it flow:
`journalctl -u uran-builder -f` then `journalctl -u uran-controller -f`.

> Managed databases (CloudNativePG) are not installed here — that's a separate
> step, only needed when you create a database.

## Deploy the latest

```sh
bash ~/uran-api/deploy/deploy.sh
```

Pulls both repos, rebuilds, restarts the units, health-checks.

## Handy

```sh
systemctl status uran-api uran-builder uran-controller uran-web   # state
journalctl -u uran-builder -f           # live build logs
journalctl -u uran-controller -f        # live reconcile logs
systemctl restart uran-api              # restart one
```
