# DigitalOcean App Platform

## Resources

1. Managed **Postgres** (smallest node is fine for the interview).
2. Managed **Redis** or **Valkey** (Redis protocol). Prefer Valkey if Redis create is unavailable.
3. App Platform app from this GitHub repo, Dockerfile build.

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

See [../deployments/app-platform.yaml](../deployments/app-platform.yaml). Replace placeholders, then:

```bash
doctl apps create --spec deployments/app-platform.yaml
```

Or attach databases in the App Platform UI and map connection strings to env vars.

## Rollback

Redeploy the previous deployment in App Platform, or `doctl apps create-deployment <app-id>`. Flag data lives in Postgres and survives app rollbacks.
