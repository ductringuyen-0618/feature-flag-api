# Production proof — DigitalOcean App Platform

Captured: 2026-09-15T20:06:43Z

| Field | Value |
|-------|--------|
| Ingress | `https://feature-flag-api-r2hi6.ondigitalocean.app` |
| App | `feature-flag-api` |
| App ID | `7f32125f-1f35-4747-b6ac-f464e4538e34` |
| Demo flag | `demo_checkout_1789502803` |
| Demo user | `demo_alice` |

## Automated smoke

```text
Verifying https://feature-flag-api-r2hi6.ondigitalocean.app
PASS healthz
PASS readyz
PASS create_flag
PASS evaluate_on
PASS kill_switch
PASS evaluate_off
PASS override
PASS evaluate_override
PASS bulk
PASS bad_name

Passed=10 Failed=0
```

## Manual request / response transcript

Each block below is a live call against production. Bodies are pretty-printed JSON.

### 1. Liveness — GET /healthz

**Request**

```http
GET /healthz HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app

```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 16

{
  "status": "ok"
}
```

### 2. Readiness — GET /readyz

**Request**

```http
GET /readyz HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app

```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 19

{
  "status": "ready"
}
```

### 3. Create flag — POST /v1/flags

**Request**

```http
POST /v1/flags HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app
Content-Type: application/json

{"name":"demo_checkout_1789502803","description":"production demo","enabled":true,"rollout_percent":100}
```

**Response**

```http
HTTP/2 201
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 191

{
  "name": "demo_checkout_1789502803",
  "description": "production demo",
  "enabled": true,
  "rollout_percent": 100,
  "created_at": "2026-09-15T20:06:44.356785Z",
  "updated_at": "2026-09-15T20:06:44.356785Z"
}
```

### 4. Evaluate on — GET /v1/evaluate/{name}

**Request**

```http
GET /v1/evaluate/demo_checkout_1789502803?user_id=demo_alice HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app

```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 82

{
  "flag_name": "demo_checkout_1789502803",
  "enabled": true,
  "reason": "BOOLEAN_TOGGLE"
}
```

### 5. Kill switch off — PATCH /v1/flags/{name}

**Request**

```http
PATCH /v1/flags/demo_checkout_1789502803 HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app
Content-Type: application/json

{"enabled":false}
```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 192

{
  "name": "demo_checkout_1789502803",
  "description": "production demo",
  "enabled": false,
  "rollout_percent": 100,
  "created_at": "2026-09-15T20:06:44.356785Z",
  "updated_at": "2026-09-15T20:06:44.530181Z"
}
```

### 6. Evaluate off — GET /v1/evaluate/{name}

**Request**

```http
GET /v1/evaluate/demo_checkout_1789502803?user_id=demo_alice HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app

```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 82

{
  "flag_name": "demo_checkout_1789502803",
  "enabled": false,
  "reason": "FLAG_DISABLED"
}
```

### 7. Per-user override on — PUT /v1/flags/{name}/users/{userID}

**Request**

```http
PUT /v1/flags/demo_checkout_1789502803/users/demo_alice HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app
Content-Type: application/json

{"enabled":true}
```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 79

{
  "enabled": true,
  "flag_name": "demo_checkout_1789502803",
  "user_id": "demo_alice"
}
```

### 8. Evaluate with override — GET /v1/evaluate/{name}

**Request**

```http
GET /v1/evaluate/demo_checkout_1789502803?user_id=demo_alice HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app

```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 76

{
  "flag_name": "demo_checkout_1789502803",
  "enabled": true,
  "reason": "OVERRIDE"
}
```

### 9. Bulk evaluate — POST /v1/evaluate

**Request**

```http
POST /v1/evaluate HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app
Content-Type: application/json

{"user_id":"demo_alice","flags":["demo_checkout_1789502803"]}
```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:44 GMT
content-type: application/json
content-length: 140

{
  "results": {
    "demo_checkout_1789502803": {
      "flag_name": "demo_checkout_1789502803",
      "enabled": true,
      "reason": "OVERRIDE"
    }
  },
  "user_id": "demo_alice"
}
```

### 10. Validation error — POST /v1/flags (bad name)

**Request**

```http
POST /v1/flags HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app
Content-Type: application/json

{"name":"bad:name","enabled":true}
```

**Response**

```http
HTTP/2 400
date: Tue, 15 Sep 2026 20:06:45 GMT
content-type: application/json
content-length: 25

{
  "error": "invalid name"
}
```

### 11. Get flag — GET /v1/flags/{name}

**Request**

```http
GET /v1/flags/demo_checkout_1789502803 HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app

```

**Response**

```http
HTTP/2 200
date: Tue, 15 Sep 2026 20:06:45 GMT
content-type: application/json
content-length: 192

{
  "name": "demo_checkout_1789502803",
  "description": "production demo",
  "enabled": false,
  "rollout_percent": 100,
  "created_at": "2026-09-15T20:06:44.356785Z",
  "updated_at": "2026-09-15T20:06:44.530181Z"
}
```

### 12. Cleanup — DELETE /v1/flags/{name}

**Request**

```http
DELETE /v1/flags/demo_checkout_1789502803 HTTP/1.1
Host: feature-flag-api-r2hi6.ondigitalocean.app

```

**Response**

```http
HTTP/2 204
date: Tue, 15 Sep 2026 20:06:45 GMT

```

## Verdict

Production at `https://feature-flag-api-r2hi6.ondigitalocean.app` responded successfully for health, readiness, create, evaluate, kill switch, override, bulk evaluate, and validation. Automated `scripts/verify.sh` passed 10/10.

Architecture claim script (same session): `./scripts/verify-architecture.sh` → **Passed=61 Failed=0 Skipped=6** (skips are failure-mode / multi-replica checks not safe on shared prod).

## Architecture verify summary

```text
PASS validation_empty_name [measured]
PASS validation_rollout_101 [measured]
PASS validation_patch_empty [measured]
PASS validation_evaluate_missing_user [measured]
PASS validation_bulk_empty_flags [measured]
PASS validation_bulk_over_100 [measured]
PASS validation_body_over_1mib [measured] status=400
PASS override_unknown_flag [measured]
PASS create_bulk [measured]
PASS bulk_ok [measured]
PASS bulk_fail_closed_404 [measured]
PASS bulk_dedupe_repeats [measured]
PASS evaluate_missing_404 [measured]
PASS create_miss [measured]
PASS zero_percent_excludes [measured]
PASS delete_bool [measured]
PASS evaluate_after_delete_404 [measured]

Passed=61 Failed=0 Skipped=6
Labels: PASS lines are measured HTTP evidence. SKIP lines are not safely observable in this prod topology.
```
