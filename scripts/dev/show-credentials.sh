#!/usr/bin/env bash
set -Eeuo pipefail

environment_file="${1:-${HOME}/.config/lice/dev.env}"
./scripts/dev/validate-env.sh "$environment_file"

set -a
# shellcheck disable=SC1090
source "$environment_file"
set +a

printf 'URL: http://lice.localhost:8080\n'
printf 'Operador: %s\n' "$LICE_DEMO_OPERATOR_USERNAME"
printf 'Senha do operador: %s\n' "$LICE_DEMO_OPERATOR_PASSWORD"
printf 'Usuario sem concessao: %s\n' "$LICE_DEMO_VIEWER_USERNAME"
printf 'Senha do usuario sem concessao: %s\n' "$LICE_DEMO_VIEWER_PASSWORD"
