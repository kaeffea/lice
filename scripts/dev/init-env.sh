#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

destination="${1:-${HOME}/.config/lice/dev.env}"

fail() {
  echo "$1" >&2
  exit 1
}

validate_existing() {
  "$(dirname "$0")/validate-env.sh" "$destination"
}

if [[ -e "$destination" || -L "$destination" ]]; then
  validate_existing
  echo "Ambiente local existente validado em ${destination}."
  exit 0
fi

command -v openssl >/dev/null 2>&1 || fail "openssl nao encontrado. Instale-o no Ubuntu WSL."

directory="$(dirname "$destination")"
mkdir -p "$directory"
chmod 0700 "$directory"

temporary_file="$(mktemp "${directory}/.dev.env.XXXXXX")"
cleanup() {
  rm -f "$temporary_file"
}
trap cleanup EXIT

random_hex() {
  openssl rand -hex "$1"
}

{
  printf 'COMPOSE_PROJECT_NAME=lice\n'
  printf 'POSTGRES_SUPERUSER_PASSWORD=%s\n' "$(random_hex 32)"
  printf 'LICE_MIGRATOR_PASSWORD=%s\n' "$(random_hex 32)"
  printf 'LICE_RUNTIME_PASSWORD=%s\n' "$(random_hex 32)"
  printf 'KEYCLOAK_DB_PASSWORD=%s\n' "$(random_hex 32)"
  printf 'KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_ID=lice-local-bootstrap\n'
  printf 'KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_SECRET=%s\n' "$(random_hex 32)"
  printf 'LICE_OIDC_CLIENT_SECRET=%s\n' "$(random_hex 48)"
  printf 'LICE_SESSION_HASH_KEY=%s\n' "$(random_hex 32)"
  printf 'LICE_LOGIN_ENCRYPTION_KEY=%s\n' "$(random_hex 32)"
  printf 'LICE_CSRF_KEY=%s\n' "$(random_hex 32)"
  printf 'LICE_DEMO_OPERATOR_USERNAME=operator@lice.local\n'
  printf 'LICE_DEMO_OPERATOR_PASSWORD=%s\n' "$(random_hex 24)"
  printf 'LICE_DEMO_VIEWER_USERNAME=viewer@lice.local\n'
  printf 'LICE_DEMO_VIEWER_PASSWORD=%s\n' "$(random_hex 24)"
} >"$temporary_file"

chmod 0600 "$temporary_file"
mv "$temporary_file" "$destination"
trap - EXIT

validate_existing
echo "Ambiente local criado com permissao 0600 em ${destination}."
echo "As credenciais so sao exibidas por 'make dev-credentials'."
