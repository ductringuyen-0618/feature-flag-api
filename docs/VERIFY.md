# How to verify the Feature Flag API

Use this guide when you need to prove the service works on your machine or on DigitalOcean App Platform.

## What you prove

Each check below must pass:

1. `/readyz` returns ready (Postgres is up).
2. `/healthz` returns `ok` or `degraded` with HTTP 200.
3. You can create a flag, evaluate it, flip the kill switch, set a per-user override, and bulk-evaluate.
4. Invalid flag names return HTTP 400.

The script [`scripts/verify.sh`](../scripts/verify.sh) runs those checks against any base URL.

## Local

### Option A. Docker Compose via OrbStack (preferred on macOS)

This Cursor Linux workspace often runs **on** OrbStack but does **not** mount the host Docker socket (`/var/run/docker.sock`). Inside the workspace, `docker` cannot talk to OrbStack’s engine. Run Compose on the **Mac host** where OrbStack provides Docker:

```bash
# On the Mac (OrbStack / Docker CLI), from a checkout of this repo:
docker compose up --build -d
./scripts/verify.sh http://127.0.0.1:8080
docker compose down
```

To use Compose from inside a Cursor/OrbStack Linux machine instead, share the Docker socket into that environment (OrbStack → Docker → “Share Docker socket” / bind-mount `/var/run/docker.sock`), then:

```bash
docker compose up --build -d
./scripts/verify.sh http://127.0.0.1:8080
```

Compose starts Postgres, Redis, and the API on port `8080`. The healthcheck probes `/readyz`.

### Option A2. Docker Engine on Linux (no OrbStack)

```bash
sudo apt-get update
sudo apt-get install -y docker.io docker-compose-v2
sudo service docker start
docker compose up --build -d
./scripts/verify.sh http://127.0.0.1:8080
```
### Option B. Go binary + Compose data stores only

If the API image build is slow, run Postgres and Redis in Compose and the API with Go:

```bash
docker compose up -d postgres redis
export DATABASE_URL='postgres://flags:flags@127.0.0.1:5432/flags?sslmode=disable'
export REDIS_URL='redis://127.0.0.1:6379/0'
go run ./cmd/server
```

In another terminal:

```bash
./scripts/verify.sh http://127.0.0.1:8080
```

### Option C. Unit tests only (no Docker)

This does not prove Postgres, Redis, or HTTP against a live process. It still catches evaluate and handler regressions:

```bash
go test ./...
go test -race ./internal/flag ./internal/httpapi ./internal/store/cached
```

## Production (App Platform)

1. Wait until the app deployment is active:

```bash
doctl apps list --format ID,Spec.Name,DefaultIngress,ActiveDeployment.Phase
APP_ID='…'   # from the list
doctl apps list-deployments "$APP_ID" --format ID,Phase,Cause,Created
```

2. Read the public URL (no trailing slash):

```bash
INGRESS="$(doctl apps get "$APP_ID" --format DefaultIngress --no-header)"
echo "$INGRESS"
```

3. Run the smoke script, then the architecture claim script, against production:

```bash
./scripts/verify.sh "$INGRESS"
./scripts/verify-architecture.sh "$INGRESS"
```

`scripts/verify-architecture.sh` checks rule A, sticky/monotonic rollout, create 409, override vs kill, validation 400s, bulk fail-closed 404, and the create/patch/delete/override lifecycle. It skips Redis-down, Postgres-down, and multi-replica races (not safe on shared production). Each run uses a unique flag-name prefix and deletes those flags on exit.

4. Optional manual probes:

```bash
curl -sS "$INGRESS/readyz"
curl -sS "$INGRESS/healthz"
curl -sS -X POST "$INGRESS/v1/flags" \
  -H 'content-type: application/json' \
  -d '{"name":"prod_smoke","enabled":true,"rollout_percent":100}'
curl -sS "$INGRESS/v1/evaluate/prod_smoke?user_id=alice"
```

### Health meanings

| Path | Pass means |
|------|------------|
| `/readyz` | Postgres accepts connections. App Platform should probe this. |
| `/healthz` body `ok` | Process is up. Redis is fine or was not required. |
| `/healthz` body `degraded` | Process is up, but Redis was wanted and is missing or failing. Evaluate still uses the last flag snapshot. |

### If production `/readyz` fails

1. Confirm the App Platform Postgres component is online.
2. Confirm `DATABASE_URL` is bound to `${db.DATABASE_URL}`.
3. Confirm Valkey firewall allows the app (`doctl databases firewalls list` on `feature-flag-valkey`).
4. Re-check deployment logs: `doctl apps logs "$APP_ID" --type run`.

## While you wait on deploy

```bash
# Poll until Phase is ACTIVE (or failed)
watch -n 20 'doctl apps list-deployments '"$APP_ID"' --format ID,Phase,Created | head'
```

When `DefaultIngress` is set and Phase is `ACTIVE`, run `./scripts/verify.sh` against that URL.
