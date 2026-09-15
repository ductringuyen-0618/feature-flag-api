# DigitalOcean App Platform

## Live app

| Field | Value |
|-------|--------|
| App name | `feature-flag-api` |
| App ID | `7f32125f-1f35-4747-b6ac-f464e4538e34` |
| Ingress | `https://feature-flag-api-r2hi6.ondigitalocean.app` |
| Image | `registry.digitalocean.com/feature-flag-api/feature-flag-api` (DOCR) |
| Postgres | App Platform bindable `db` (`db-s-dev-database`) |
| Valkey | Managed cluster `feature-flag-valkey` (attached by `cluster_name`) |

The app uses a DOCR image, not a GitHub source. App create with a GitHub repo failed with "GitHub user not authenticated". Keep the DOCR path.

## Resources

1. Managed **Postgres** (App Platform bindable `db`).
2. Managed **Valkey** (`feature-flag-valkey`, Redis protocol).
3. App Platform app from [../deployments/app-platform.yaml](../deployments/app-platform.yaml).

## Environment

| Var | Value |
|-----|--------|
| `DATABASE_URL` | Bindable Postgres URI (`sslmode=require` as provided) |
| `REDIS_URL` | `rediss://...` TLS URI for managed cache |
| `PORT` | `8080` (platform sets this; app reads `PORT`) |
| `RELOAD_INTERVAL` | `10s` |
| `OVERRIDE_CACHE_TTL` | `60s` |

App Platform `health_check.http_path` is `/readyz`. That path pings Postgres. `/healthz` is process liveness and stays HTTP 200 with `ok` or `degraded`.

## Spec and local deploy

See [../deployments/app-platform.yaml](../deployments/app-platform.yaml). Preferred local path:

```bash
./scripts/do-deploy.sh
```

That script publishes `registry.digitalocean.com/feature-flag-api/feature-flag-api:${IMAGE_TAG:-latest}` with `ko` (`linux/amd64`), then creates or updates the App Platform app. It reuses an existing Valkey cluster and registry when present.

For an already-live stack (no create):

```bash
export DIGITALOCEAN_APP_ID=7f32125f-1f35-4747-b6ac-f464e4538e34
UPDATE_ONLY=1 ./scripts/do-deploy.sh
```

Dry-run (validate the rendered spec only):

```bash
DRY_RUN=1 DIGITALOCEAN_APP_ID=7f32125f-1f35-4747-b6ac-f464e4538e34 UPDATE_ONLY=1 ./scripts/do-deploy.sh
```

## GitHub Actions deploy

Workflow: [../.github/workflows/deploy.yml](../.github/workflows/deploy.yml).

Triggers: push to `main`, and `workflow_dispatch`.

On each run the workflow installs `doctl` and `ko`, publishes a DOCR image tagged with the commit SHA, then runs `UPDATE_ONLY=1 ./scripts/do-deploy.sh` so it updates the existing app and never creates Valkey, Postgres, registry, or a second app.

### Secrets and variables

Set these in the GitHub repo (Settings → Secrets and variables → Actions).

| Name | Where | Required | Purpose |
|------|--------|----------|---------|
| `DIGITALOCEAN_ACCESS_TOKEN` | Secret | Yes | Personal access token with Apps read/write, Registry, and Databases. `doctl registry login` uses this token. No separate registry password. |
| `DIGITALOCEAN_APP_ID` | Variable (preferred) or Secret | Yes | Live app ID `7f32125f-1f35-4747-b6ac-f464e4538e34`. |

Do not commit tokens. Do not put the token in the app spec.

## Rollback

Redeploy the previous deployment in App Platform, or `doctl apps create-deployment 7f32125f-1f35-4747-b6ac-f464e4538e34`. Flag data lives in Postgres and survives app rollbacks.

## Verify

```bash
./scripts/verify.sh https://feature-flag-api-r2hi6.ondigitalocean.app
```

Full local and production steps live in [VERIFY.md](VERIFY.md).
