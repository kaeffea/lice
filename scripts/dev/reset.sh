#!/usr/bin/env bash
set -Eeuo pipefail

environment_file="${1:-${HOME}/.config/lice/dev.env}"
confirmation="${2:-}"

if [[ "$confirmation" != "lice-local-data" ]]; then
  echo "Reset cancelado. Para apagar somente os volumes locais do LICE, execute:" >&2
  echo "  make dev-reset CONFIRM=lice-local-data" >&2
  exit 1
fi

./scripts/dev/validate-env.sh "$environment_file"

docker compose --env-file "$environment_file" --file compose.yaml down --volumes --remove-orphans
echo "Dados locais do PostgreSQL e subjects de bootstrap removidos em conjunto."
