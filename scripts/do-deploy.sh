#!/usr/bin/env bash
# Deploy feature-flag-api to DigitalOcean App Platform via doctl.
# Requires: doctl >= 1.168, DIGITALOCEAN_ACCESS_TOKEN, ko (for image publish).
# Publishes the Go server to DOCR and creates/updates the App Platform app
# (Postgres bindable + existing Valkey cluster_name feature-flag-valkey).
#
# Env:
#   IMAGE_TAG              Image tag to publish and pin in the rendered app spec (default: latest).
#   DIGITALOCEAN_APP_ID    Existing App Platform app ID. When set, the script updates that app
#                          and never creates a new app.
#   UPDATE_ONLY=1          Skip Valkey/registry/app create. Fail if the app or Valkey is missing.
#                          Intended for CI against a live stack.
#   DRY_RUN=1              Validate the rendered spec and print the plan. Skip mutate steps.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SPEC="${ROOT}/deployments/app-platform.yaml"
NAME="feature-flag-api"
VALKEY_NAME="feature-flag-valkey"
VALKEY_REGION="nyc1"
VALKEY_SIZE="db-s-1vcpu-1gb"
REGISTRY_NAME="feature-flag-api"
IMAGE_REPO="feature-flag-api"
IMAGE_TAG="${IMAGE_TAG:-latest}"
KO_BIN="${KO_BIN:-$(command -v ko || true)}"
DRY_RUN="${DRY_RUN:-0}"
UPDATE_ONLY="${UPDATE_ONLY:-0}"
APP_ID="${DIGITALOCEAN_APP_ID:-${APP_ID:-}}"

if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi

# Re-read after .env so explicit env and CI secrets win over a stale local .env value.
APP_ID="${DIGITALOCEAN_APP_ID:-${APP_ID:-}}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
UPDATE_ONLY="${UPDATE_ONLY:-0}"
DRY_RUN="${DRY_RUN:-0}"

if [[ -z "${DIGITALOCEAN_ACCESS_TOKEN:-}" ]]; then
  echo "DIGITALOCEAN_ACCESS_TOKEN is missing" >&2
  exit 1
fi

if [[ "${DRY_RUN}" != "1" && -z "${KO_BIN}" ]]; then
  echo "ko is missing; install with: go install github.com/google/ko@latest" >&2
  exit 1
fi

RENDERED="$(mktemp)"
trap 'rm -f "${RENDERED}"' EXIT
# Pin the image tag in a rendered copy so CI can deploy a commit SHA without editing the repo file.
sed "s|^\\([[:space:]]*tag:\\)[[:space:]].*|\\1 ${IMAGE_TAG}|" "${SPEC}" > "${RENDERED}"

echo "Validating rendered app spec (tag=${IMAGE_TAG})..."
doctl apps spec validate "${RENDERED}"

resolve_app_id() {
  if [[ -n "${APP_ID}" ]]; then
    echo "${APP_ID}"
    return
  fi
  doctl apps list --format ID,Spec.Name --no-header 2>/dev/null | awk -v n="$NAME" '$2==n {print $1; exit}' || true
}

if [[ "${DRY_RUN}" == "1" ]]; then
  echo "DRY_RUN=1: skipping Valkey ensure, image publish, and app create/update."
  echo "Rendered service image tag:"
  awk '/^services:/{s=1} s && /tag:/{print; exit}' "${RENDERED}"
  RESOLVED="$(resolve_app_id)"
  if [[ -n "${RESOLVED}" ]]; then
    echo "Would update existing app ${RESOLVED} (UPDATE_ONLY=${UPDATE_ONLY})."
  elif [[ "${UPDATE_ONLY}" == "1" ]]; then
    echo "UPDATE_ONLY=1 and no app ID found; a real run would fail." >&2
    exit 1
  else
    echo "Would create app ${NAME}."
  fi
  exit 0
fi

echo "Checking account..."
doctl account get --format Email,Status,UUID

if [[ "${UPDATE_ONLY}" == "1" ]]; then
  echo "UPDATE_ONLY=1: requiring existing Valkey ${VALKEY_NAME} (no create)..."
  if ! doctl databases list --format Name --no-header | grep -qx "${VALKEY_NAME}"; then
    echo "Valkey cluster ${VALKEY_NAME} is missing; refuse to create under UPDATE_ONLY=1." >&2
    exit 1
  fi
else
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
fi

VALKEY_ID="$(doctl databases list --format ID,Name --no-header | awk -v n="${VALKEY_NAME}" '$2==n {print $1; exit}')"
echo "Valkey ID: ${VALKEY_ID}"

if [[ "${UPDATE_ONLY}" == "1" ]]; then
  echo "UPDATE_ONLY=1: requiring existing DOCR registry (no create)..."
  if ! doctl registry get >/dev/null 2>&1; then
    echo "DOCR registry is missing; refuse to create under UPDATE_ONLY=1." >&2
    exit 1
  fi
else
  echo "Ensuring DOCR registry (${REGISTRY_NAME})..."
  if doctl registry get >/dev/null 2>&1; then
    echo "Registry already exists."
  else
    doctl registry create "${REGISTRY_NAME}"
  fi
fi
doctl registry login

echo "Publishing image to DOCR with ko (tag=${IMAGE_TAG})..."
export KO_DOCKER_REPO="registry.digitalocean.com/${REGISTRY_NAME}/${IMAGE_REPO}"
# App Platform runs linux/amd64; this host may be arm64.
export KO_DEFAULT_PLATFORMS="${KO_DEFAULT_PLATFORMS:-linux/amd64}"
PUBLISHED="$("${KO_BIN}" publish --bare --tags "${IMAGE_TAG}" "${ROOT}/cmd/server")"
echo "Published: ${PUBLISHED}"

RESOLVED="$(resolve_app_id)"
if [[ -n "${RESOLVED}" ]]; then
  APP_ID="${RESOLVED}"
  echo "Updating existing app ${APP_ID}..."
  doctl apps update "${APP_ID}" --spec "${RENDERED}" --wait
  # Force a new deployment so a retagged image is pulled even when the spec looks unchanged.
  echo "Creating deployment for ${APP_ID}..."
  doctl apps create-deployment "${APP_ID}" --wait
elif [[ "${UPDATE_ONLY}" == "1" ]]; then
  echo "UPDATE_ONLY=1 requires DIGITALOCEAN_APP_ID or an existing app named ${NAME}." >&2
  exit 1
else
  echo "Creating app from rendered ${SPEC}..."
  APP_ID="$(doctl apps create --spec "${RENDERED}" --wait --format ID --no-header)"
fi

echo "App ID: ${APP_ID}"
doctl apps get "${APP_ID}" --format ID,DefaultIngress

if [[ -n "${VALKEY_ID}" && -n "${APP_ID}" ]]; then
  echo "Adding app firewall rule on Valkey (idempotent best-effort)..."
  doctl databases firewalls append "${VALKEY_ID}" --rule "app:${APP_ID}" 2>/dev/null || true
fi

INGRESS="$(doctl apps get "${APP_ID}" --format DefaultIngress --no-header)"
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
