#!/usr/bin/env bash
set -Eeuo pipefail

environment_file="${1:-${HOME}/.config/lice/dev.env}"

fail() {
  echo "$1" >&2
  exit 1
}

[[ ! -L "$environment_file" ]] || fail "O arquivo de ambiente nao pode ser um link simbolico: ${environment_file}"
[[ -f "$environment_file" ]] || fail "Ambiente ausente ou irregular: ${environment_file}"
[[ "$(stat --format='%a' "$environment_file")" == "600" ]] ||
  fail "Permissao insegura em ${environment_file}; execute: chmod 600 '${environment_file}'"
! grep -q $'\r' "$environment_file" ||
  fail "O arquivo de ambiente contem CRLF; recrie-o no Ubuntu WSL."

set -a
# shellcheck disable=SC1090
source "$environment_file"
set +a

required=(
  POSTGRES_SUPERUSER_PASSWORD
  LICE_MIGRATOR_PASSWORD
  LICE_RUNTIME_PASSWORD
  KEYCLOAK_DB_PASSWORD
  KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_ID
  KEYCLOAK_BOOTSTRAP_ADMIN_CLIENT_SECRET
  LICE_OIDC_CLIENT_SECRET
  LICE_SESSION_HASH_KEY
  LICE_LOGIN_ENCRYPTION_KEY
  LICE_CSRF_KEY
  LICE_DEMO_OPERATOR_USERNAME
  LICE_DEMO_OPERATOR_PASSWORD
  LICE_DEMO_VIEWER_USERNAME
  LICE_DEMO_VIEWER_PASSWORD
)

for name in "${required[@]}"; do
  [[ -n "${!name:-}" ]] || fail "Variavel ausente em ${environment_file}: ${name}"
done

for name in LICE_SESSION_HASH_KEY LICE_LOGIN_ENCRYPTION_KEY LICE_CSRF_KEY; do
  [[ "${!name}" =~ ^[0-9a-f]{64}$ ]] ||
    fail "${name} deve conter exatamente 64 caracteres hexadecimais minusculos."
done
