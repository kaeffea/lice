#!/usr/bin/env bash
set -Eeuo pipefail

for command_name in curl docker make openssl python3; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Dependencia ausente no Ubuntu WSL: ${command_name}" >&2
    exit 1
  fi
done

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose v2 nao esta disponivel (comando: docker compose)." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "O daemon Docker nao esta acessivel a partir do Ubuntu WSL." >&2
  exit 1
fi

echo "Dependencias locais verificadas."
