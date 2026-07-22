#!/usr/bin/env bash
set -Eeuo pipefail

unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  echo "Arquivos Go sem gofmt:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go mod verify
go vet ./...
go test -race ./...
go build -buildvcs=false ./...
