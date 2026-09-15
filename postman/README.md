# Postman — Feature Flag API

Manual demo collection for local and DigitalOcean production.

## Import

1. Open Postman → **Import**.
2. Add:
   - `Feature-Flag-API.postman_collection.json`
   - `Feature-Flag-API.production.postman_environment.json` (live App Platform)
   - Optional: `Feature-Flag-API.local.postman_environment.json` (`http://127.0.0.1:8080`)
3. Select the **Feature Flag API — Production** environment.

## Demo order

Run folder **1. Demo — happy path** top to bottom:

1. Create flag (`{{flagName}}`, default `demo_checkout`)
2. Evaluate on → `BOOLEAN_TOGGLE`
3. Kill switch off → evaluate → `FLAG_DISABLED`
4. Override on → evaluate → `OVERRIDE`
5. Bulk evaluate

Then **2. Demo — validation & reads**, then **3. Cleanup**.

If create returns **409**, change `flagName` in the environment (for example `demo_checkout_2`) and start again.

## Variables

| Variable | Production default | Notes |
|----------|--------------------|-------|
| `baseUrl` | `https://feature-flag-api-r2hi6.ondigitalocean.app` | No trailing slash |
| `flagName` | `demo_checkout` | Letters, digits, `.`, `_`, `-` only |
| `userId` | `demo_alice` | Same identifier rules |

## Proof

A captured production request/response transcript lives in [`docs/production-proof.md`](../docs/production-proof.md).
