#!/usr/bin/env bash
set -Eeuo pipefail

environment_file="${1:-${HOME}/.config/lice/dev.env}"
./scripts/dev/validate-env.sh "$environment_file"

compose=(docker compose --env-file "$environment_file" --file compose.yaml)

run_psql() {
  local password_variable="$1"
  local role="$2"
  local database="$3"
  local statement="$4"

  "${compose[@]}" exec -T postgres bash -ceu '
    export PGPASSWORD="${!1}"
    psql \
      --host 127.0.0.1 \
      --username "$2" \
      --dbname "$3" \
      --tuples-only \
      --no-align \
      --set ON_ERROR_STOP=1 \
      --set VERBOSITY=verbose \
      --command "$4"
  ' _ "$password_variable" "$role" "$database" "$statement"
}

expect_connect_denied() {
  local description="$1"
  local password_variable="$2"
  local role="$3"
  local database="$4"
  local catalog_result
  local output
  local status

  catalog_result="$(run_psql POSTGRES_PASSWORD postgres postgres "SELECT has_database_privilege('$role', '$database', 'CONNECT')")"
  catalog_result="${catalog_result//[[:space:]]/}"
  if [[ "$catalog_result" != "f" ]]; then
    echo "Falha: o catalogo ainda concede CONNECT (${description})." >&2
    exit 1
  fi
  set +e
  output="$(run_psql "$password_variable" "$role" "$database" "SELECT 1" 2>&1)"
  status=$?
  set -e
  if [[ $status -eq 0 || "$output" != *"permission denied for database"* ]]; then
    echo "Falha: a tentativa de conexao nao foi negada como esperado (${description})." >&2
    exit 1
  fi
  echo "Negacao de CONNECT comprovada por catalogo e tentativa: ${description}."
}

expect_database_success() {
  local description="$1"
  local password_variable="$2"
  local role="$3"
  local database="$4"
  local statement="$5"

  if ! run_psql "$password_variable" "$role" "$database" "$statement" >/dev/null 2>&1; then
    echo "Falha no controle positivo: ${description}." >&2
    exit 1
  fi
  echo "Controle positivo comprovado: ${description}."
}

expect_permission_denied() {
  local description="$1"
  local password_variable="$2"
  local role="$3"
  local database="$4"
  local statement="$5"
  local output
  local status

  set +e
  output="$(run_psql "$password_variable" "$role" "$database" "$statement" 2>&1)"
  status=$?
  set -e
  if [[ $status -eq 0 ]]; then
    echo "Falha: operacao proibida foi aceita (${description})." >&2
    exit 1
  fi
  if [[ "$output" != *"42501:"* ]]; then
    echo "Falha inesperada em vez de SQLSTATE 42501 (${description})." >&2
    printf '%s\n' "$output" >&2
    exit 1
  fi

  echo "Negacao 42501 comprovada: ${description}."
}

expect_database_success \
  "lice_runtime conecta e consulta o banco lice" \
  LICE_RUNTIME_PASSWORD \
  lice_runtime \
  lice \
  "SELECT count(*) FROM audit.events"

expect_database_success \
  "keycloak conecta e consulta seu proprio banco" \
  KEYCLOAK_DB_PASSWORD \
  keycloak \
  keycloak \
  "SELECT 1"

expect_connect_denied \
  "lice_runtime nao conecta no banco keycloak" \
  LICE_RUNTIME_PASSWORD \
  lice_runtime \
  keycloak

expect_connect_denied \
  "keycloak nao conecta no banco lice" \
  KEYCLOAK_DB_PASSWORD \
  keycloak \
  lice

expect_permission_denied \
  "lice_runtime nao atualiza auditoria" \
  LICE_RUNTIME_PASSWORD \
  lice_runtime \
  lice \
  "UPDATE audit.events SET details = details WHERE false"

expect_permission_denied \
  "lice_runtime nao apaga auditoria" \
  LICE_RUNTIME_PASSWORD \
  lice_runtime \
  lice \
  "DELETE FROM audit.events WHERE false"

expect_permission_denied \
  "lice_runtime nao trunca auditoria" \
  LICE_RUNTIME_PASSWORD \
  lice_runtime \
  lice \
  "TRUNCATE TABLE audit.events"
