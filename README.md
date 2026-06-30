# uran-api

The Go control plane for **Uran**, a Render-style platform-as-a-service: push to
Git → automatic build → running, routed service. This repo holds the API, build
worker, Kubernetes controller, and CLI. The dashboard lives in the sibling
`uran-web`.

## Features

- **Git-driven deploys** — connect a repo; pushes build and ship automatically.
- **Deploy from an image** — ship a prebuilt container image directly (CI push),
  skipping the build.
- **Multiple service types** — HTTP web services, static sites, background
  workers (no inbound routing), and scheduled cron jobs.
- **Managed databases** — provision Postgres (CloudNativePG) or Redis per
  project; apps connect via an in-namespace connection URI.
- **Zero-config builds** — [Nixpacks](https://nixpacks.com) detects the stack;
  images are cached and pushed to a registry.
- **Kubernetes runtime** — each deploy is reconciled into a Deployment, Service,
  and Traefik route via Server-Side Apply, with rollout health checks.
- **Tenant isolation** — each org gets its own namespace with a NetworkPolicy
  (deny cross-tenant ingress), a ResourceQuota, and default resource limits.
- **Preview environments** — every pull request gets an isolated environment on
  its own hostname, torn down when the PR closes.
- **Custom domains & automatic TLS** — attach your own hostnames; certificates
  are provisioned by cert-manager and served over HTTPS.
- **Env vars & secrets** — injected into workloads via per-service Secrets.
- **Scaling & autoscaling** — set replica count and instance size, or autoscale
  on CPU (HPA), with readiness/liveness health checks gating rollouts.
- **Persistent disks** — attach a volume to a service for stateful workloads;
  data survives restarts and redeploys.
- **Team roles (RBAC)** — owner / admin / member / viewer, with viewer
  read-only and admin/owner-gated member management.
- **Audit log** — every mutating action is recorded (who, what, when, result).
- **API tokens** — issue personal access tokens for CI and programmatic access.
- **Observability** — stream live runtime logs and read per-pod CPU/memory.
- **Instant rollback** — redeploy any previous image without rebuilding.
- **CLI** — drive the whole flow from the terminal with `uran`.

## Architecture

Four processes share one Postgres and coordinate over `LISTEN/NOTIFY`:

```
cmd/api         REST API + GitHub webhooks → writes deploys, NOTIFY uran_deploys
cmd/builder     LISTEN uran_deploys      → clone + Nixpacks + push image
cmd/controller  LISTEN uran_deployments  → k8s apply + rollout → live
                LISTEN uran_teardowns    → remove preview environments
cmd/uran        CLI client for the API
```

Webhook events: `push` → production deploy; `pull_request` opened/synchronize →
preview deploy; `pull_request` closed → preview teardown.

## Getting started

Prerequisites: Go 1.26+, Postgres, a Docker daemon with the `buildx` plugin,
`nixpacks` + `git` on PATH, an image registry, and a Kubernetes cluster with
Traefik (e.g. [k3d](https://k3d.io)).

Configuration is read strictly from the environment with **no defaults** — copy
[`.env.example`](./.env.example) and set every variable. Each process only needs
its own subset (documented in the example file).

```sh
go run ./cmd/api          # control-plane API (migrates on boot)
go run ./cmd/builder      # build worker
go run ./cmd/controller   # kubernetes reconciler
```

## CLI

```sh
go build -o uran ./cmd/uran

uran login    --api http://localhost:8080 --email you@example.com --password ****
uran login    --api http://localhost:8080 --token uran_pat_…    # CI / token auth
uran token create --name ci               # issue a personal access token
uran member add  --org 1 --email dev@x.io --role member
uran member list --org 1
uran deploy   --service 3                 # build + deploy from the service's repo
uran deploy   --service 3 --image registry/app:1.2.3   # deploy a prebuilt image
uran logs     --deploy 6                  # stream build logs
uran status   --deploy 6
uran env set  --service 3 --secret API_KEY=xyz
uran env list --service 3
uran rollback --deploy 5                  # redeploy a prior image (no rebuild)
uran domain add  --service 3 app.example.com
uran domain list --service 3
uran db create     --project 1 maindb            # or --engine redis
uran db connection --database 1
uran scale  --service 3 --replicas 3 --size medium    # or --min 1 --max 4
uran health --service 3 --path /healthz
uran disk attach --service 3 --size 1Gi --path /data
uran logs    --service 3                  # live runtime logs
uran metrics --service 3                  # per-pod CPU/memory
```

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| GET  | `/healthz` | – | Liveness |
| POST | `/v1/auth/register` | – | Create an account → `{token,user}` |
| POST | `/v1/auth/login` | – | Authenticate → `{token,user}` |
| POST | `/v1/auth/logout` | bearer | Revoke session |
| GET  | `/v1/me` | bearer | Current user |
| GET  | `/v1/audit` | bearer | Recent audited actions |
| GET/POST | `/v1/tokens` | bearer | List / create API tokens |
| DELETE | `/v1/tokens/{tokenID}` | bearer | Revoke an API token |
| GET/POST | `/v1/orgs` | bearer | List / create orgs |
| GET/POST | `/v1/orgs/{orgID}/members` | bearer | List / add members (add: admin+) |
| PATCH/DELETE | `/v1/orgs/{orgID}/members/{userID}` | bearer | Set role / remove (admin+) |
| GET/POST | `/v1/orgs/{orgID}/projects` | bearer | List / create projects |
| GET/POST | `/v1/projects/{projectID}/services` | bearer | List / create services |
| GET/POST | `/v1/services/{serviceID}/deploys` | bearer | List / trigger Git build deploys |
| POST | `/v1/services/{serviceID}/image-deploys` | bearer | Deploy a prebuilt image |
| GET  | `/v1/deploys/{deployID}` | bearer | Get a deploy |
| GET  | `/v1/deploys/{deployID}/logs` | bearer | Stream build logs (SSE) |
| GET  | `/v1/services/{serviceID}/runtime-logs` | bearer | Stream live runtime logs |
| GET  | `/v1/services/{serviceID}/metrics` | bearer | Per-pod CPU/memory usage |
| POST | `/v1/deploys/{deployID}/rollback` | bearer | Redeploy a prior image |
| GET/POST | `/v1/services/{serviceID}/env` | bearer | List / upsert env vars |
| DELETE | `/v1/services/{serviceID}/env/{key}` | bearer | Remove an env var |
| GET/POST | `/v1/services/{serviceID}/domains` | bearer | List / add custom domains |
| DELETE | `/v1/services/{serviceID}/domains/{domain}` | bearer | Remove a custom domain |
| GET/POST | `/v1/projects/{projectID}/databases` | bearer | List / create managed databases |
| GET/DELETE | `/v1/databases/{databaseID}` | bearer | Get / delete a database |
| GET | `/v1/databases/{databaseID}/connection` | bearer | Connection URI (when ready) |
| POST | `/v1/services/{serviceID}/scale` | bearer | Replicas, instance size, autoscaling |
| POST | `/v1/services/{serviceID}/health` | bearer | Set health-check path |
| POST/DELETE | `/v1/services/{serviceID}/disk` | bearer | Attach / detach a persistent disk |
| POST | `/v1/webhooks/github` | HMAC | GitHub push / pull_request events |

Authenticated requests send `Authorization: Bearer <token>` — either a session
token or a personal access token (`uran_pat_…`). The webhook is
verified via `X-Hub-Signature-256` against `URAN_GITHUB_WEBHOOK_SECRET`.

## Development

```sh
go build ./...
go test ./...
```
