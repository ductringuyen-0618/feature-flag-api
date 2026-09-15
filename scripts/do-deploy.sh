#!/usr/bin/env bash
# Deploy feature-flag-api to DigitalOcean App Platform via doctl.
# Requires: doctl >= 1.168, DIGITALOCEAN_ACCESS_TOKEN, ko (for image publish).
# Creates a paid managed Valkey cluster if missing (Valkey has no App Platform "dev" DB tier).
# Publishes the Go server to DOCR and creates/updates the App Platform app (Postgres bindable + Valkey).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SPEC="${ROOT}/deployments/app-platform.yaml"
NAME="feature-flag-api"
VALKEY_NAME="feature-flag-valkey"
VALKEY_REGION="nyc1"
VALKEY_SIZE="db-s-1vcpu-1gb"
REGISTRY_NAME="feature-flag-api"
IMAGE_REPO="feature-flag-api"
IMAGE_TAG="latest"
KO_BIN="${KO_BIN:-$(command -v ko || true)}"

if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi

if [[ -z "${DIGITALOCEAN_ACCESS_TOKEN:-}" ]]; then
  echo "DIGITALOCEAN_ACCESS_TOKEN is missing" >&2
  exit 1
fi

if [[ -z "${KO_BIN}" ]]; then
  echo "ko is missing; install with: go install github.com/google/ko@latest" >&2
  exit 1
fi

echo "Checking account..."
doctl account get --format Email,Status,UUID

echo "Ensuring managed Valkey cluster (${VALKEY_NAME})..."
if doctl databases list --format Name --no-header | grep -qx "${VALKEY_NAME}"; then
  echo "Valkey cluster already exists."
else
  echo "Creating Valkey (paid). This can take several minutes..."
  doctl databases create "${VALKEY_NAME}" \
    --engine valkey \
    --version 8 \
    --region "${VALKEY_REGION}" \
    --size "${VALKEY_SIZE}" \
    --num-nodes 1 \
    --wait
fi

VALKEY_ID="$(doctl databases list --format ID,Name --no-header | awk -v n="${VALKEY_NAME}" '$2==n {print $1; exit}')"
echo "Valkey ID: ${VALKEY_ID}"

echo "Ensuring DOCR registry (${REGISTRY_NAME})..."
if doctl registry get >/dev/null 2>&1; then
  echo "Registry already exists."
else
  doctl registry create "${REGISTRY_NAME}"
fi
doctl registry login

echo "Publishing image to DOCR with ko..."
export KO_DOCKER_REPO="registry.digitalocean.com/${REGISTRY_NAME}/${IMAGE_REPO}"
# App Platform runs linux/amd64; this host may be arm64.
export KO_DEFAULT_PLATFORMS="${KO_DEFAULT_PLATFORMS:-linux/amd64}"
PUBLISHED="$("${KO_BIN}" publish --bare --tags "${IMAGE_TAG}" "${ROOT}/cmd/server")"
echo "Published: ${PUBLISHED}"

echo "Validating app spec..."
doctl apps spec validate "$SPEC"

EXISTING_ID="$(doctl apps list --format ID,Spec.Name --no-header 2>/dev/null | awk -v n="$NAME" '$2==n {print $1; exit}')"

if [[ -n "${EXISTING_ID}" ]]; then
  echo "Updating existing app ${EXISTING_ID}..."
  doctl apps update "$EXISTING_ID" --spec "$SPEC" --wait
  APP_ID="$EXISTING_ID"
else
  echo "Creating app from ${SPEC}..."
  APP_ID="$(doctl apps create --spec "$SPEC" --wait --format ID --no-header)"
fi

echo "App ID: ${APP_ID}"
doctl apps get "$APP_ID" --format ID,DefaultIngress,ActiveDeployment.ID

# Trusted sources: allow the app to reach Valkey
if [[ -n "${VALKEY_ID}" && -n "${APP_ID}" ]]; then
  echo "Adding app firewall rule on Valkey (idempotent best-effort)..."
  doctl databases firewalls append "${VALKEY_ID}" --rule "app:${APP_ID}" 2>/dev/null || true
fi

INGRESS="$(doctl apps get "$APP_ID" --format DefaultIngress --no-header)"
echo "URL: ${INGRESS}"

if [[ -n "${INGRESS}" ]]; then
  echo "Probing ${INGRESS}/readyz ..."
  curl -fsS "${INGRESS}/readyz" || true
  echo
  echo "Probing ${INGRESS}/healthz ..."
  curl -fsS "${INGRESS}/healthz" || true
  echo
fi

echo "Done."
