#!/usr/bin/env bash
set -Eeuo pipefail

curl --fail --silent --show-error http://lice.localhost:8080/health/ready >/dev/null
curl --fail --silent --show-error \
  http://auth.lice.localhost:8080/realms/lice/.well-known/openid-configuration >/dev/null
curl --fail --silent --show-error --dump-header - --output /dev/null \
  http://lice.localhost:8080/controle |
  grep --ignore-case --quiet $'^Referrer-Policy: same-origin\r$'

echo "Health, discovery OIDC e politica same-origin acessiveis pelo Caddy."
