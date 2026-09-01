# Tools installed via `make install-tools` land in GOPATH/bin.
GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

IDENTITY_DSN ?= mysql://root:root@tcp(localhost:3307)/identity
BOARD_DSN    ?= mysql://root:root@tcp(localhost:3307)/board

IDENTITY_MIGRATIONS := services/identity/db/migrations
BOARD_MIGRATIONS    := services/board/db/migrations

.PHONY: install-tools proto migrate-up migrate-down run stop logs test integration build web-dev web-test e2e loadtest loadtest-short benchmark-verifier

## install-tools: install buf + protoc plugins (pinned) into GOPATH/bin
install-tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.50.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.26.1
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.26.1

## proto: lint & generate gRPC / gateway / swagger code from api/proto
proto:
	buf lint
	buf generate

## migrate-up / migrate-down: apply schemas per service DB (no-op while a service has no migrations)
migrate-up:
	@if ls $(IDENTITY_MIGRATIONS)/*.sql >/dev/null 2>&1; then migrate -path $(IDENTITY_MIGRATIONS) -database "$(IDENTITY_DSN)" up; else echo "identity: no migrations yet"; fi
	@if ls $(BOARD_MIGRATIONS)/*.sql >/dev/null 2>&1; then migrate -path $(BOARD_MIGRATIONS) -database "$(BOARD_DSN)" up; else echo "board: no migrations yet"; fi

migrate-down:
	@if ls $(IDENTITY_MIGRATIONS)/*.sql >/dev/null 2>&1; then migrate -path $(IDENTITY_MIGRATIONS) -database "$(IDENTITY_DSN)" down 1; fi
	@if ls $(BOARD_MIGRATIONS)/*.sql >/dev/null 2>&1; then migrate -path $(BOARD_MIGRATIONS) -database "$(BOARD_DSN)" down 1; fi

## run / stop / logs: infrastructure & services via Docker Compose
run:
	docker compose up -d --build

stop:
	docker compose down

logs:
	docker compose logs -f

build:
	go build ./...

test:
	go test ./...

## integration: repository tests against ephemeral MySQL 8 testcontainers
integration:
	go test -tags=integration ./services/identity/internal/repo/mysql ./services/board/internal/repo/mysql

## web-dev / web-test: frontend dev server / unit tests (run `npm install` in web/ once first)
web-dev:
	cd web && npm run dev

web-test:
	cd web && npm run test

## e2e: deterministic Phase 7 signup -> login -> refresh -> post scenario
e2e:
	./scripts/e2e.sh

## loadtest: full Phase 7 k6 matrix (B1-B6); writes artifacts/benchmarks/
loadtest:
	./loadtest/run.sh

## loadtest-short: validation-sized matrix for harness development/CI
loadtest-short:
	SHORT=1 ./loadtest/run.sh

## benchmark-verifier: warm-cache, in-process RS256 verification capacity
benchmark-verifier:
	go test -run '^$$' -bench BenchmarkVerifyWarmCache -benchmem ./pkg/jwks
