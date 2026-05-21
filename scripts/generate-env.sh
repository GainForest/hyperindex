#!/usr/bin/env bash
set -euo pipefail

output_path="${1:-.env}"

fail() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

prompt_value() {
  local prompt="$1"
  local value
  read -r -p "$prompt" value
  trim "$value"
}

prompt_secret_value() {
  local prompt="$1"
  local value
  read -r -s -p "$prompt" value
  printf '\n' >&2
  trim "$value"
}

confirm_yes() {
  local prompt="$1"
  local answer
  read -r -p "$prompt" answer
  case "$(trim "$answer")" in
    y|Y|yes|YES|Yes) return 0 ;;
    *) return 1 ;;
  esac
}

generate_secret() {
  command -v openssl >/dev/null 2>&1 || fail "openssl is required when generating secrets. Install openssl or enter all secret values manually."
  openssl rand -hex 32
}

write_env_value() {
  local key="$1"
  local value="$2"
  printf '%s=%s\n' "$key" "$value" >>"$temp_path"
}

if [[ -e "$output_path" ]]; then
  if ! confirm_yes "${output_path} already exists. Overwrite it? Type yes to continue: "; then
    printf 'Aborted without changing %s.\n' "$output_path" >&2
    exit 1
  fi
fi

output_dir="$(dirname "$output_path")"
[[ -d "$output_dir" ]] || fail "output directory does not exist: $output_dir"

admin_dids="$(prompt_value 'ADMIN_DIDS (comma-separated admin DIDs): ')"
if [[ -z "$admin_dids" ]]; then
  printf 'Warning: no DID will be treated as admin.\n' >&2
  if ! confirm_yes "Continue without ADMIN_DIDS? Type yes to continue: "; then
    printf 'Aborted without writing %s.\n' "$output_path" >&2
    exit 1
  fi
fi

secret_key_base="$(prompt_secret_value 'SECRET_KEY_BASE (blank to generate): ')"
if [[ -z "$secret_key_base" ]]; then
  secret_key_base="$(generate_secret)"
elif (( ${#secret_key_base} < 64 )); then
  fail "SECRET_KEY_BASE must be at least 64 characters, or blank to generate a secure value."
fi

admin_api_key="$(prompt_secret_value 'ADMIN_API_KEY (blank to generate): ')"
if [[ -z "$admin_api_key" ]]; then
  admin_api_key="$(generate_secret)"
elif (( ${#admin_api_key} < 16 )); then
  fail "ADMIN_API_KEY must be at least 16 characters, or blank to generate a secure value."
fi

while true; do
  tap_admin_password="$(prompt_secret_value 'TAP_ADMIN_PASSWORD (blank to generate): ')"
  if [[ -z "$tap_admin_password" ]]; then
    tap_admin_password="$(generate_secret)"
    break
  fi
  if (( ${#tap_admin_password} >= 16 )); then
    break
  fi
  printf 'TAP_ADMIN_PASSWORD must be at least 16 characters, or blank to generate a secure value.\n' >&2
done

tap_signal_collection="$(prompt_value 'TAP_SIGNAL_COLLECTION (optional): ')"
tap_collection_filters="$(prompt_value 'TAP_COLLECTION_FILTERS (optional): ')"
external_base_url="$(prompt_value 'EXTERNAL_BASE_URL (optional): ')"

temp_path="$(mktemp "${output_dir}/.generate-env.XXXXXX")"
trap 'rm -f "$temp_path"' EXIT
chmod 600 "$temp_path"

write_env_value "SECRET_KEY_BASE" "$secret_key_base"
write_env_value "ADMIN_API_KEY" "$admin_api_key"
write_env_value "TAP_ADMIN_PASSWORD" "$tap_admin_password"

if [[ -n "$admin_dids" ]]; then
  write_env_value "ADMIN_DIDS" "$admin_dids"
fi
if [[ -n "$tap_signal_collection" ]]; then
  write_env_value "TAP_SIGNAL_COLLECTION" "$tap_signal_collection"
fi
if [[ -n "$tap_collection_filters" ]]; then
  write_env_value "TAP_COLLECTION_FILTERS" "$tap_collection_filters"
fi
if [[ -n "$external_base_url" ]]; then
  write_env_value "EXTERNAL_BASE_URL" "$external_base_url"
fi

mv "$temp_path" "$output_path"
chmod 600 "$output_path"
trap - EXIT

printf 'Wrote Tap Docker environment to %s.\n' "$output_path" >&2
