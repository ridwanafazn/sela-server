.DEFAULT_GOAL := help

GO      ?= go
VERSION ?= dev
IMAGE   ?= sela-server:local

.PHONY: help tidy fmt fmt-check vet test test-race test-integration build run \
	migrate-up migrate-down migrate-down-all migrate-version \
	compose-up compose-down compose-reset check docker-build worker worker-once scheduler backfill schedules ops-health ops-usage eval

help:
	@echo "SELA backend"
	@echo ""
	@echo "  make tidy                Download and tidy dependencies"
	@echo "  make fmt                 Format Go sources"
	@echo "  make fmt-check           Fail when formatting is required"
	@echo "  make vet                 Run go vet"
	@echo "  make test                Run unit tests"
	@echo "  make test-race           Run unit tests with the race detector"
	@echo "  make test-integration    Run integration tests (requires Postgres)"
	@echo "  make build               Build API and migration binaries into bin/"
	@echo "  make run                 Run the API locally"
	@echo "  make worker              Run the background worker"
	@echo "  make worker-once         Process available jobs once and exit"
	@echo "  make scheduler           Run the periodic insight fan-out"
	@echo "  make schedules           Enqueue due weekly/monthly insight jobs and exit"
	@echo "  make ops-health          Run local beta health gates"
	@echo "  make ops-usage           Show AI usage and cost summary"
	@echo "  make backfill            Re-enqueue embeddings and memory extraction for active journals"
	@echo "  make eval                Run deterministic safety and prompt-injection quality gates"
	@echo "  make migrate-up          Apply all migrations"
	@echo "  make migrate-down        Roll back one migration"
	@echo "  make migrate-down-all    Roll back every migration (destructive)"
	@echo "  make migrate-version     Print the applied migration version"
	@echo "  make compose-up          Start local PostgreSQL"
	@echo "  make compose-down        Stop local PostgreSQL"
	@echo "  make compose-reset       Stop PostgreSQL and delete its volume (destructive)"
	@echo "  make check               Format check, vet, build, and unit tests"
	@echo "  make docker-build        Build the container image"

tidy:
	$(GO) mod tidy

fmt:
	gofmt -w .

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required for:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration:
	$(GO) test -tags=integration ./...

build:
	@mkdir -p bin
	$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/sela-api ./cmd/api
	$(GO) build -trimpath -o bin/sela-migrate ./cmd/migrate
	$(GO) build -trimpath -o bin/sela-worker ./cmd/worker
	$(GO) build -trimpath -o bin/sela-scheduler ./cmd/scheduler
	$(GO) build -trimpath -o bin/sela-ops ./cmd/ops

run:
	$(GO) run ./cmd/api

worker:
	$(GO) run ./cmd/worker

worker-once:
	$(GO) run ./cmd/worker -once

scheduler:
	$(GO) run ./cmd/scheduler

schedules:
	$(GO) run ./cmd/worker -schedules

ops-health:
	$(GO) run ./cmd/ops health

ops-usage:
	$(GO) run ./cmd/ops usage

backfill:
	$(GO) run ./cmd/worker -backfill

eval:
	$(GO) test -count=1 -v ./internal/evaluation

migrate-up:
	$(GO) run ./cmd/migrate up

migrate-down:
	$(GO) run ./cmd/migrate down

migrate-down-all:
	$(GO) run ./cmd/migrate down -all

migrate-version:
	$(GO) run ./cmd/migrate version

compose-up:
	docker compose up -d

compose-down:
	docker compose down

compose-reset:
	docker compose down -v

check: fmt-check vet build test

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .
