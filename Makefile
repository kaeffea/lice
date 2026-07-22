SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

DEV_ENV ?= $(HOME)/.config/lice/dev.env
COMPOSE := docker compose --env-file "$(DEV_ENV)" --file compose.yaml

.PHONY: doctor dev-env validate dev dev-status dev-logs dev-credentials dev-down dev-reset smoke test test-api test-web test-integration

doctor:
	./scripts/dev/doctor.sh

dev-env:
	./scripts/dev/init-env.sh "$(DEV_ENV)"

validate:
	./scripts/dev/validate.sh "$(DEV_ENV)"

dev:
	@test -f "$(DEV_ENV)" || { echo "Ambiente ausente. Execute 'make dev-env' primeiro." >&2; exit 1; }
	./scripts/dev/validate-env.sh "$(DEV_ENV)"
	./scripts/dev/doctor.sh
	$(COMPOSE) config --quiet
	$(COMPOSE) up --build --detach --wait --wait-timeout 180

dev-status:
	$(COMPOSE) ps

dev-logs:
	$(COMPOSE) logs --follow --tail=200

dev-credentials:
	./scripts/dev/show-credentials.sh "$(DEV_ENV)"

dev-down:
	$(COMPOSE) down --remove-orphans

dev-reset:
	./scripts/dev/reset.sh "$(DEV_ENV)" "$(CONFIRM)"

smoke:
	./scripts/dev/smoke.sh

test-integration:
	./scripts/dev/test-infrastructure.sh "$(DEV_ENV)"
	$(COMPOSE) --profile test run --rm api-integration-tests

test: validate test-api test-web

test-api:
	docker run --rm \
		--volume "$(CURDIR):/workspace" \
		--workdir /workspace/apps/api \
		golang:1.26.5-bookworm \
		/workspace/scripts/dev/test-api.sh

test-web:
	docker run --rm \
		--volume "$(CURDIR):/workspace" \
		--volume /workspace/apps/web/node_modules \
		--workdir /workspace/apps/web \
		node:24.18.0-alpine \
		/workspace/scripts/dev/test-web.sh
