#!/usr/bin/env bash
set -Eeuo pipefail

environment_file="${1:-${HOME}/.config/lice/dev.env}"

while IFS= read -r -d '' script; do
  bash -n "$script"
done < <(find infra/postgres scripts -type f -name '*.sh' -print0)

python3 -m json.tool infra/keycloak/realm/lice-realm.json >/dev/null
python3 -c 'import ast, pathlib; ast.parse(pathlib.Path("scripts/keycloak/seed_demo_users.py").read_text(encoding="utf-8"))'

docker run --rm \
  --volume "$PWD/infra/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2.11.4-alpine \
  caddy validate --config /etc/caddy/Caddyfile >/dev/null 2>&1

./scripts/dev/validate-env.sh "$environment_file"

docker compose --env-file "$environment_file" --file compose.yaml config --quiet
echo "Scripts, realm e Compose validados."
