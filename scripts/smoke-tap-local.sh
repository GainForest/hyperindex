#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

project_name="${HYPERINDEX_LOCAL_TAP_PROJECT:-hyperindex-smoke-tap-$(date +%Y%m%d%H%M%S)-$$}"
hyperindex_host_port="${HYPERINDEX_LOCAL_TAP_HOST_PORT:-8080}"
tap_host_port="${HYPERINDEX_LOCAL_TAP_TAP_HOST_PORT:-2480}"
smoke_url="${HYPERINDEX_LOCAL_TAP_SMOKE_URL:-http://127.0.0.1:${hyperindex_host_port}}"
ready_timeout_seconds="${HYPERINDEX_LOCAL_TAP_READY_TIMEOUT_SECONDS:-180}"
tap_settle_seconds="${HYPERINDEX_LOCAL_TAP_SETTLE_SECONDS:-20}"
smoke_timeout_seconds="${HYPERINDEX_LOCAL_TAP_SMOKE_TIMEOUT_SECONDS:-900}"
smoke_retry_interval_seconds="${HYPERINDEX_LOCAL_TAP_SMOKE_RETRY_SECONDS:-15}"
keep_stack="${HYPERINDEX_LOCAL_TAP_KEEP:-0}"

export TAP_SIGNAL_COLLECTION="${TAP_SIGNAL_COLLECTION:-app.certified.actor.profile}"
export TAP_COLLECTION_FILTERS="${TAP_COLLECTION_FILTERS:-app.certified.*,org.hypercerts.*}"
export HYPERINDEX_HOST_PORT="${hyperindex_host_port}"
export TAP_HOST_PORT="${tap_host_port}"
export ADMIN_API_KEY="${ADMIN_API_KEY:-}"
export SECRET_KEY_BASE="${SECRET_KEY_BASE:-}"
export TAP_ADMIN_PASSWORD="${TAP_ADMIN_PASSWORD:-}"
export DATABASE_URL="${HYPERINDEX_LOCAL_TAP_DATABASE_URL:-sqlite:/app/data/hyperindex.db}"
export EXTERNAL_BASE_URL="${EXTERNAL_BASE_URL:-${smoke_url}}"

compose=(docker compose -p "${project_name}" -f "${repo_root}/docker-compose.tap.yml" -f "${repo_root}/docker-compose.tap-smoke.yml")

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_command() {
  local command_name="$1"
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    fail "${command_name} is required for local Tap smoke tests"
  fi
}

require_positive_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "${value}" =~ ^[1-9][0-9]*$ ]]; then
    fail "${name} must be a positive integer, got ${value@Q}"
  fi
}

generate_secret() {
  openssl rand -base64 "$1" | tr -d '\n'
}

cleanup() {
  local status=$?
  if [[ "${keep_stack}" == "1" ]]; then
    printf '\nKeeping local Tap smoke stack running. Clean it up with:\n'
    printf '  docker compose -p %q -f %q -f %q down -v --remove-orphans\n' \
      "${project_name}" "${repo_root}/docker-compose.tap.yml" "${repo_root}/docker-compose.tap-smoke.yml"
  else
    printf '\nStopping local Tap smoke stack %s...\n' "${project_name}"
    "${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  return "${status}"
}

wait_for_ready() {
  local deadline=$(( $(date +%s) + ready_timeout_seconds ))
  printf 'Waiting for Hyperindex readiness at %s/ready ...\n' "${smoke_url}"
  until curl -fsS "${smoke_url}/ready" >/dev/null 2>&1; do
    if (( $(date +%s) >= deadline )); then
      return 1
    fi
    sleep 3
  done
}

run_smoke_once() {
  (
    cd "${repo_root}"
    HYPERINDEX_SMOKE_URL="${smoke_url}" \
    HYPERINDEX_SMOKE_ENV_FILE=/dev/null \
    HYPERINDEX_SMOKE_EXPECTATIONS="${repo_root}/tests/api-smoke/expectations/local-tap.json" \
    HYPERINDEX_SMOKE_WRITE_THROUGH=0 \
    HYPERINDEX_SMOKE_EXTERNAL_LABEL_SOURCE_DID= \
    go test -v -tags=api_smoke ./tests/api-smoke -count=1
  )
}

wait_for_smoke() {
  local deadline=$(( $(date +%s) + smoke_timeout_seconds ))
  local attempt=1

  while true; do
    printf '\nRunning local Tap API smoke attempt %d...\n' "${attempt}"
    if run_smoke_once; then
      return 0
    fi

    if (( $(date +%s) >= deadline )); then
      return 1
    fi

    printf '\nSmoke attempt %d failed. Tap may still be discovering/backfilling repos; retrying in %s seconds...\n' \
      "${attempt}" "${smoke_retry_interval_seconds}"
    sleep "${smoke_retry_interval_seconds}"
    attempt=$((attempt + 1))
  done
}

dump_logs() {
  printf '\nLast local Tap smoke stack logs:\n'
  "${compose[@]}" logs --tail=200 tap hyperindex || true
}

require_command docker
require_command curl
require_command go
require_command openssl
require_positive_integer HYPERINDEX_LOCAL_TAP_HOST_PORT "${hyperindex_host_port}"
require_positive_integer HYPERINDEX_LOCAL_TAP_TAP_HOST_PORT "${tap_host_port}"
require_positive_integer HYPERINDEX_LOCAL_TAP_READY_TIMEOUT_SECONDS "${ready_timeout_seconds}"
require_positive_integer HYPERINDEX_LOCAL_TAP_SETTLE_SECONDS "${tap_settle_seconds}"
require_positive_integer HYPERINDEX_LOCAL_TAP_SMOKE_TIMEOUT_SECONDS "${smoke_timeout_seconds}"
require_positive_integer HYPERINDEX_LOCAL_TAP_SMOKE_RETRY_SECONDS "${smoke_retry_interval_seconds}"

if ! docker compose version >/dev/null 2>&1; then
  fail "Docker Compose v2 is required. Install Docker with the 'docker compose' plugin and ensure the Docker daemon is running."
fi

if [[ ! -f "${repo_root}/testdata/lexicons/app/certified/graph/follow.json" ]]; then
  fail "missing app.certified.graph.follow lexicon fixture"
fi

if [[ -z "${ADMIN_API_KEY}" ]]; then
  ADMIN_API_KEY="$(generate_secret 32)"
  export ADMIN_API_KEY
fi
if [[ -z "${SECRET_KEY_BASE}" ]]; then
  SECRET_KEY_BASE="$(generate_secret 48)"
  export SECRET_KEY_BASE
fi
if [[ -z "${TAP_ADMIN_PASSWORD}" ]]; then
  TAP_ADMIN_PASSWORD="$(generate_secret 32)"
  export TAP_ADMIN_PASSWORD
fi

trap cleanup EXIT

cat <<EOF
Starting isolated local Tap smoke stack.

Prerequisites: Docker with Compose v2, Go, curl, and openssl.
Compose project: ${project_name}
Smoke URL:       ${smoke_url}
Hyperindex port: ${HYPERINDEX_HOST_PORT}
Tap admin port:  ${TAP_HOST_PORT}
Tap signal:      ${TAP_SIGNAL_COLLECTION}
Tap filters:     ${TAP_COLLECTION_FILTERS}
Lexicons:        ${repo_root}/testdata/lexicons mounted at /app/testdata/lexicons
EOF

"${compose[@]}" up --build -d

if ! wait_for_ready; then
  dump_logs
  fail "Hyperindex did not become ready within ${ready_timeout_seconds} seconds"
fi

printf 'Hyperindex is ready. Waiting %s seconds for Tap discovery/backfill to warm up before the first smoke attempt...\n' "${tap_settle_seconds}"
sleep "${tap_settle_seconds}"
printf 'Running smoke tests; data-bearing checks retry every %s seconds while Tap catches up.\n' "${smoke_retry_interval_seconds}"

if ! wait_for_smoke; then
  dump_logs
  fail "local Tap smoke tests did not pass within ${smoke_timeout_seconds} seconds"
fi

printf '\n✓ Local Tap smoke tests passed against %s\n' "${smoke_url}"
