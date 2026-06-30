# uran-api

The Go control plane for **Uran**, a Render-style platform-as-a-service: push to
Git → automatic build → running, routed service. This repo holds the API, build
worker, Kubernetes controller, and CLI. The dashboard lives in the sibling
`uran-web`.

## Features

- **Git-driven deploys** — connect a repo; pushes build and ship automatically.
- **Zero-config builds** — [Nixpacks](https://nixpacks.com) detects the stack;
  images are cached and pushed to a registry.
- **Kubernetes runtime** — each deploy is reconciled into a Deployment, Service,
  and Traefik route via Server-Side Apply, with rollout health checks.
- **Preview environments** — every pull request gets an isolated environment on
  its own hostname, torn down when the PR closes.
- **Env vars & secrets** — injected into workloads via per-service Secrets.
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
uran deploy   --service 3                 # build + deploy from the service's repo
uran logs     --deploy 6                  # stream build logs
uran status   --deploy 6
uran env set  --service 3 --secret API_KEY=xyz
uran env list --service 3
uran rollback --deploy 5                  # redeploy a prior image (no rebuild)
```

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| GET  | `/healthz` | – | Liveness |
| POST | `/v1/auth/register` | – | Create an account → `{token,user}` |
| POST | `/v1/auth/login` | – | Authenticate → `{token,user}` |
| POST | `/v1/auth/logout` | bearer | Revoke session |
| GET  | `/v1/me` | bearer | Current user |
| GET/POST | `/v1/orgs` | bearer | List / create orgs |
| GET/POST | `/v1/orgs/{orgID}/projects` | bearer | List / create projects |
| GET/POST | `/v1/projects/{projectID}/services` | bearer | List / create services |
| GET/POST | `/v1/services/{serviceID}/deploys` | bearer | List / trigger deploys |
| GET  | `/v1/deploys/{deployID}` | bearer | Get a deploy |
| GET  | `/v1/deploys/{deployID}/logs` | bearer | Stream build logs (SSE) |
| POST | `/v1/deploys/{deployID}/rollback` | bearer | Redeploy a prior image |
| GET/POST | `/v1/services/{serviceID}/env` | bearer | List / upsert env vars |
| DELETE | `/v1/services/{serviceID}/env/{key}` | bearer | Remove an env var |
| POST | `/v1/webhooks/github` | HMAC | GitHub push / pull_request events |

Authenticated requests send `Authorization: Bearer <token>`. The webhook is
verified via `X-Hub-Signature-256` against `URAN_GITHUB_WEBHOOK_SECRET`.

## Development

```sh
go build ./...
go test ./...
```
