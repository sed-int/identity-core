# Plan: Build order

> Status: 🚧 In progress — Phase 0 ✅ done 2026-07-17 · Phase 1 next
>
> Phase 0 notes: logger uses zerolog (user preference over slog); Redis exposed on host port **6380** (6379 taken by the go-msa project); generated code is committed under `api/gen/`; `make install-tools` installs pinned buf + protoc plugins.

Organizing principle: get the PRD §6 E2E scenario (login → token → stateless verification → post) working end-to-end as early as possible with the happy path only, then broaden to edge states, eventing, frontend, and NFRs. Each phase ends with something runnable.

## Phase 0 — Scaffolding & toolchain
- Repo layout from PRD §3; single Go module at the root (PoC-simple: `pkg/` and generated `api/` code import without go.work juggling).
- `docker-compose.yml` with just MySQL + Redis; init script creates separate `identity` and `board` databases.
- `Makefile` targets: `proto`, `migrate-up`, `run` (stubs fine); `buf.yaml`.
- `pkg/logger` (structured, request-ID field).
- **Done when:** `docker compose up` gives healthy MySQL/Redis; `make proto` runs.

## Phase 1 — API contract (proto-first)
- `identity.proto`: OTP request/verify, token (issue + refresh), signup completion, HTTP annotations for §4.1 gateway paths.
- `board.proto`: post create/list.
- `buf generate` → gRPC stubs, grpc-gateway, Swagger. Discovery + JWKS stay plain HTTP handlers (fixed-shape OIDC endpoints, not worth proto-ing).
- **Done when:** generated code compiles; Swagger UI renders.

## Phase 2 — Identity service, happy path only (biggest phase, ~⅓ of total work)
- Migrations: all §5 tables now (`users`, `user_credentials`, `user_profiles`, `user_devices`, `outbox`) even though device/outbox logic comes later.
- Layers bottom-up: domain (status enum + transitions as pure functions) → repo (MySQL) → usecase (signup, OTP login with mock SMS that logs the code, OTP state in Redis) → delivery (gRPC + gateway).
- Token stack: RSA keygen, RS256 signer with `kid`, `/oauth2/v1/jwks`, `/.well-known/openid-configuration`, token endpoint, RTR with token families in Redis.
- Unit tests for domain/usecase as written — state machine especially.
- **Milestone M1:** signup + login via curl; tokens issued; JWKS serves keys.

## Phase 3 — Stateless verification + Board service (proves the thesis)
- `pkg/jwks`: kid-aware in-memory cache, refetch-on-unknown-kid, gobreaker-wrapped fetch.
- Board service: migrations (posts, comments), auth middleware using `pkg/jwks`, post CRUD. Both services in docker-compose.
- **Milestone M2 (core of the PoC):** post created with a real token, zero Identity DB queries from Board.

## Phase 4 — Eventing
- Outbox relay goroutine (poll → publish to Redis Streams → mark published).
- Board-side `user.created` consumer via consumer group, idempotent.

## Phase 5 — Frontend (React, Vite)
- OTP login → signup completion → post write/list; token refresh on 401. Demo client, not a product.
- **Milestone M3:** full §6 scenario clickable in a browser.

## Phase 6 — Edge states & security hardening
- DORMANT reactivation, device-change verification (`user_devices`), OTP expiry/attempt caps/rate limits (§7).
- After E2E works — these are variations on a proven pipeline; Phase 2 state-routing leaves the seams.

## Phase 7 — Prove the NFRs (§8)
- `make e2e`: scripted signup → login → refresh → post.
- `make loadtest`: k6 vs §8 targets.
- Resilience demo: kill Identity; Board keeps verifying from cache; gobreaker opens.
- testcontainers integration tests for repo layers if not added along the way.
