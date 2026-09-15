# Feature Flag REST API

Production-oriented Go service for creating feature flags, managing global and per-user state, and evaluating availability with an in-memory snapshot hot path (Postgres durable store, Redis pub/sub + override cache).

## Architecture

See [docs/architecture.md](docs/architecture.md).

**Evaluation order (rule A):**

1. Per-user override if present  
2. `enabled == false` → kill switch off  
3. `fnv32a(flagName + ":" + userID) % 100 < rollout_percent`  
4. Else boolean on  

Warm evaluate reads flag config from RAM. If Redis is down after boot, `/healthz` returns `degraded`. A failed Redis connect with `REDIS_URL` set does the same. Evaluations still succeed from the last snapshot. `/readyz` pings Postgres. App Platform and Compose probe `/readyz`.

## Quick start

```bash
cp .env.example .env
docker compose up --build
```

API listens on `http://localhost:8080`.

### Examples

```bash
# Create
curl -s -X POST localhost:8080/v1/flags \
  -H 'content-type: application/json' \
  -d '{"name":"checkout","description":"new checkout","enabled":true,"rollout_percent":25}'

# Global kill switch
curl -s -X PATCH localhost:8080/v1/flags/checkout \
  -H 'content-type: application/json' \
  -d '{"enabled":false}'

# Per-user override
curl -s -X PUT localhost:8080/v1/flags/checkout/users/alice \
  -H 'content-type: application/json' \
  -d '{"enabled":true}'

# Evaluate
curl -s 'localhost:8080/v1/evaluate/checkout?user_id=alice'

# Bulk evaluate
curl -s -X POST localhost:8080/v1/evaluate \
  -H 'content-type: application/json' \
  -d '{"user_id":"alice","flags":["checkout"]}'
```

## Local Go run

```bash
docker compose up -d postgres redis
export DATABASE_URL='postgres://flags:flags@127.0.0.1:5432/flags?sslmode=disable'
export REDIS_URL='redis://127.0.0.1:6379/0'
go run ./cmd/server
```

## Tests

```bash
go test ./...
```

## API

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/v1/flags` | Create |
| `GET` | `/v1/flags` | List |
| `GET` | `/v1/flags/{name}` | Get |
| `PATCH` | `/v1/flags/{name}` | Update `enabled`, `description`, `rollout_percent` |
| `DELETE` | `/v1/flags/{name}` | Delete |
| `PUT` | `/v1/flags/{name}/users/{userID}` | Set override |
| `DELETE` | `/v1/flags/{name}/users/{userID}` | Clear override |
| `GET` | `/v1/evaluate/{name}?user_id=` | Single evaluate |
| `POST` | `/v1/evaluate` | Bulk evaluate |
| `GET` | `/healthz` | Process liveness. Body `status` is `ok` or `degraded`. HTTP 200 either way. |
| `GET` | `/readyz` | Postgres ping. HTTP 200 when reachable. |

**Auth:** none (intentional interview gap). Put a gateway or API key in front for production.

## Deploy (DigitalOcean App Platform)

See [docs/digitalocean.md](docs/digitalocean.md) and [deployments/app-platform.yaml](deployments/app-platform.yaml).
