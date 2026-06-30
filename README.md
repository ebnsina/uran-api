# uran-api

Go control plane for **Uran**, a Render.com-style Git-driven PaaS. See [`plan.md`](./plan.md) for the roadmap.

## Status
- **M1 — done:** API skeleton, Postgres store + migrations, email/password auth + sessions, CRUD for orgs/projects/services.
- **M2 — done:** deploy/build records + state machine, Postgres LISTEN/NOTIFY event bus, HMAC-verified GitHub push webhooks, manual deploy trigger.
- **M3 — done:** real builder — clone → Nixpacks build (w/ cache) → push image to registry; deploy status advances `queued→building→deploying`; build logs persisted and streamable over SSE.
- **M4 — done:** `cmd/controller` reconciles a built deploy → k8s Namespace + Deployment + Service + Traefik IngressRoute (Server-Side Apply), waits for rollout, and advances `deploying→live`. **Push-to-live verified end-to-end** on a local k3d cluster.
- **M5 — done:** env vars + secrets (injected via a per-service k8s Secret/`envFrom`), instant rollback (reuse a prior image, no rebuild), and the **`uran` CLI** (login, deploy, logs, status, env, rollback).
- **M6 — done:** **preview environments per PR** — a `pull_request` webhook builds the PR head into an isolated `slug-pr-N` workload on its own host; closing the PR tears it down. Verified end-to-end (preview and production served different code simultaneously).

## Architecture (processes)

Four processes share one Postgres and coordinate via LISTEN/NOTIFY:

```
cmd/api         REST API + GitHub webhook → writes deploys, NOTIFY uran_deploys
cmd/builder     LISTEN uran_deploys   → clone+nixpacks+push → NOTIFY uran_deployments
cmd/controller  LISTEN uran_deployments → k8s apply + rollout → status=live
                LISTEN uran_teardowns   → delete preview workload (PR closed)
cmd/uran        CLI client for the API (login/deploy/logs/env/rollback)
```

Webhook events: `push` → production deploy; `pull_request` (opened/synchronize/
reopened) → preview deploy; `pull_request` closed → preview teardown.

## CLI

```sh
uran login    --api http://localhost:8080 --email you@example.com --password ****
uran deploy   --service 3                 # build + deploy from the service's repo
uran logs     --deploy 6                  # stream build logs (SSE)
uran status   --deploy 6
uran env set  --service 3 --secret API_KEY=xyz
uran env list --service 3
uran rollback --deploy 5                  # re-deploy a prior image (also applies env changes)
```

## Configuration

All config comes from the environment with **no defaults** (fail-fast). See
[`.env.example`](./.env.example) for the full required set. Missing/invalid
variables abort startup with a list of every problem.

## Prerequisites for builds (M3)

- A Docker daemon (e.g. OrbStack) with the **buildx** plugin.
- `nixpacks` and `git` on PATH.
- An image registry reachable at `URAN_REGISTRY` (run one locally with
  `docker run -d -p 5005:5000 registry:2`).

## Run locally

Requires Go 1.26+ and a Postgres database.

```sh
export URAN_DATABASE_URL="postgres://user@127.0.0.1:5432/uran?sslmode=disable"
go run ./cmd/api      # migrates on boot, listens on :8080
```

Config (env vars): `URAN_ADDR` (default `:8080`), `URAN_DATABASE_URL`,
`URAN_SESSION_TTL` (default `720h`), `URAN_SHUTDOWN_TIMEOUT`, `URAN_ENV`,
`URAN_GITHUB_WEBHOOK_SECRET`.

The deploy consumer (M2 stub, M3 real builder) runs as a separate process:

```sh
go run ./cmd/builder   # LISTENs on the deploy event bus
```

## API (M1)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET  | `/healthz` | – | Liveness |
| POST | `/v1/auth/register` | – | `{email,password,name}` → `{token,user}` |
| POST | `/v1/auth/login` | – | `{email,password}` → `{token,user}` |
| POST | `/v1/auth/logout` | bearer | Revoke session |
| GET  | `/v1/me` | bearer | Current user |
| GET/POST | `/v1/orgs` | bearer | List / create orgs |
| GET/POST | `/v1/orgs/{orgID}/projects` | bearer | List / create projects |
| GET/POST | `/v1/projects/{projectID}/services` | bearer | List / create services |
| GET/POST | `/v1/services/{serviceID}/deploys` | bearer | List / trigger deploys |
| GET  | `/v1/deploys/{deployID}` | bearer | Get a deploy |
| GET  | `/v1/deploys/{deployID}/logs` | bearer | Stream build logs (SSE) |
| POST | `/v1/deploys/{deployID}/rollback` | bearer | Re-deploy a prior image (no rebuild) |
| GET/POST | `/v1/services/{serviceID}/env` | bearer | List / upsert env vars |
| DELETE | `/v1/services/{serviceID}/env/{key}` | bearer | Remove an env var |
| POST | `/v1/webhooks/github` | HMAC | GitHub push → enqueue deploys for matching services |

Authenticated requests send `Authorization: Bearer <token>`. The webhook is
verified via `X-Hub-Signature-256` against `URAN_GITHUB_WEBHOOK_SECRET`
(empty secret disables verification in development).

## Test

```sh
go test ./...
```
