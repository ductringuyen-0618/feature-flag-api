# DigitalOcean App Platform

## Resources

1. Managed **Postgres** (smallest node is fine for the interview).
2. Managed **Redis** or **Valkey** (Redis protocol). Prefer Valkey if Redis create is unavailable.
3. App Platform app. The deploy script publishes a DOCR image with `ko` and creates the app (avoids needing DigitalOcean↔GitHub OAuth).

## Environment

| Var | Value |
|-----|--------|
| `DATABASE_URL` | Bindable Postgres URI (`sslmode=require` as provided) |
| `REDIS_URL` | `rediss://...` TLS URI for managed cache |
| `PORT` | `8080` (platform sets this; app reads `PORT`) |
| `RELOAD_INTERVAL` | `10s` |
| `OVERRIDE_CACHE_TTL` | `60s` |

App Platform `health_check.http_path` is `/readyz`. That path pings Postgres. `/healthz` is process liveness and stays HTTP 200 with `ok` or `degraded`.

## Spec

See [../deployments/app-platform.yaml](../deployments/app-platform.yaml). Preferred path:

```bash
./scripts/do-deploy.sh
```

That script ensures Valkey, publishes `registry.digitalocean.com/feature-flag-api/feature-flag-api:latest`, then creates or updates the App Platform app. Postgres is an App Platform-managed `db-s-dev-database` bindable. Valkey attaches by `cluster_name: feature-flag-valkey`.

## Rollback

Redeploy the previous deployment in App Platform, or `doctl apps create-deployment <app-id>`. Flag data lives in Postgres and survives app rollbacks.

## Verify

After the app is `ACTIVE`:

```bash
INGRESS="$(doctl apps get REPLACE_APP_ID --format DefaultIngress --no-header)"
./scripts/verify.sh "$INGRESS"
```

Full local and production steps live in [VERIFY.md](VERIFY.md).
