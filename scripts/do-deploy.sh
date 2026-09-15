#!/usr/bin/env bash
# Deploy feature-flag-api to DigitalOcean App Platform via doctl.
# Requires: doctl >= 1.168, DIGITALOCEAN_ACCESS_TOKEN, GitHub app installed for the repo.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SPEC="${ROOT}/deployments/app-platform.yaml"
NAME="feature-flag-api"

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

echo "Checking account..."
doctl account get --format Email,Status,UUID

echo "Validating app spec..."
doctl apps spec validate "$SPEC"

EXISTING_ID="$(doctl apps list --format ID,Spec.Name --no-header 2>/dev/null | awk -v n="$NAME" '$2==n {print $1; exit}')"

if [[ -n "${EXISTING_ID}" ]]; then
  echo "Updating existing app ${EXISTING_ID}..."
  doctl apps update "$EXISTING_ID" --spec "$SPEC" --wait
  APP_ID="$EXISTING_ID"
else
  echo "Creating app from ${SPEC}..."
  echo "Ensure GitHub access is granted: https://cloud.digitalocean.com/apps/github/install"
  APP_ID="$(doctl apps create --spec "$SPEC" --wait --format ID --no-header)"
fi

echo "App ID: ${APP_ID}"
doctl apps get "$APP_ID" --format ID,DefaultIngress,ActiveDeployment.Phase

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
