#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

K8S_NAMESPACE="${K8S_NAMESPACE:-github-app}"
HELM_RELEASE="${HELM_RELEASE:-github-app}"
LOCAL_HEALTH_PORT="${LOCAL_HEALTH_PORT:-18080}"

log() {
  printf '[kind-deploy] %s\n' "$*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

cleanup() {
  if [[ -n "${PORT_FORWARD_PID:-}" ]]; then
    kill "$PORT_FORWARD_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${HEALTH_OUTPUT_FILE:-}" && -f "${HEALTH_OUTPUT_FILE}" ]]; then
    rm -f "${HEALTH_OUTPUT_FILE}"
  fi
}

trap cleanup EXIT

require_cmd make
require_cmd kubectl
require_cmd kind
require_cmd helm
require_cmd docker
require_cmd curl

if [[ -z "${GITHUB_WEBHOOK_SECRET:-}" ]]; then
  # Generate a random ephemeral secret for local deploys instead of using a fixed default
  GITHUB_WEBHOOK_SECRET="$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32)"
  export GITHUB_WEBHOOK_SECRET
  log "GITHUB_WEBHOOK_SECRET not set; generated a random ephemeral value for local deploy"
fi

log "Bootstrapping Kind cluster and core dependencies"
make kind-bootstrap

log "Installing cert-manager issuers"
make kind-install-issuers

log "Creating/refreshing app secret"
make kind-create-secrets

log "Building image, loading into Kind, and deploying Helm release"
make kind-deploy-local

log "Resolving deployment and service names for release ${HELM_RELEASE}"
DEPLOYMENT_NAME="$(kubectl -n "$K8S_NAMESPACE" get deploy -l "app.kubernetes.io/instance=${HELM_RELEASE}" -o jsonpath='{.items[0].metadata.name}')"
SERVICE_NAME="$(kubectl -n "$K8S_NAMESPACE" get svc -l "app.kubernetes.io/instance=${HELM_RELEASE}" -o jsonpath='{.items[0].metadata.name}')"

if [[ -z "$DEPLOYMENT_NAME" || -z "$SERVICE_NAME" ]]; then
  printf 'error: failed to find deployment/service for release %s in namespace %s\n' "$HELM_RELEASE" "$K8S_NAMESPACE" >&2
  exit 1
fi

log "Waiting for deployment/${DEPLOYMENT_NAME} rollout"
kubectl -n "$K8S_NAMESPACE" rollout status "deployment/${DEPLOYMENT_NAME}" --timeout=240s

SERVICE_PORT="$(kubectl -n "$K8S_NAMESPACE" get svc "${SERVICE_NAME}" -o jsonpath='{.spec.ports[?(@.name=="http")].port}')"
if [[ -z "${SERVICE_PORT}" ]]; then
  SERVICE_PORT="$(kubectl -n "$K8S_NAMESPACE" get svc "${SERVICE_NAME}" -o jsonpath='{.spec.ports[0].port}')"
fi

log "Port-forwarding service/${SERVICE_NAME} port ${SERVICE_PORT} to localhost:${LOCAL_HEALTH_PORT}"
kubectl -n "$K8S_NAMESPACE" port-forward "service/${SERVICE_NAME}" "${LOCAL_HEALTH_PORT}:${SERVICE_PORT}" >/tmp/github-app-port-forward.log 2>&1 &
PORT_FORWARD_PID=$!

HEALTH_OUTPUT_FILE="$(mktemp -t github-app-healthz.XXXXXX)"
HEALTH_PROBE_SUCCEEDED=false

for _ in {1..20}; do
  if curl --silent --fail "http://127.0.0.1:${LOCAL_HEALTH_PORT}/healthz" >"$HEALTH_OUTPUT_FILE"; then
    HEALTH_PROBE_SUCCEEDED=true
    break
  fi
  sleep 1
done

if [[ "$HEALTH_PROBE_SUCCEEDED" != true ]] || ! grep -q "ok" "$HEALTH_OUTPUT_FILE"; then
  printf 'error: health check failed, expected response containing "ok"\n' >&2
  printf 'hint: see /tmp/github-app-port-forward.log for port-forward output\n' >&2
  exit 1
fi

log "Health check passed: $(tr -d '\n' <"$HEALTH_OUTPUT_FILE")"
log "Deployment verification complete"
