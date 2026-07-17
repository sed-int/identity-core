# Plan: Phase 3 — Stateless verification + Board service (M2)

> Status: 🚧 In progress

Goal (PRD §6): Board creates posts by verifying IdP tokens against cached JWKS —
zero Identity DB access from Board.

## `feat/jwks-verifier` — pkg/jwks
- `Verifier`: parses RS256 JWTs, selects key by `kid` from an in-memory cache.
- Fetches JWKS through `sony/gobreaker` (PRD §2 circuit breaker placement).
- Refetch on unknown `kid` (rotation) with a cooldown; stale cache is served
  when the IdP is unreachable (resilience target §8.4).
- Tests via httptest JWKS server: roundtrip, key rotation, IdP-down-serves-stale.

## `feat/board-service` — services/board
- Migrations: `posts`, `comments` (PRD §3).
- Same layering as identity: domain / repo/mysql / usecase / delivery/grpc / app.
- Auth interceptor: Bearer token from gateway metadata → pkg/jwks → claims in ctx.
  `CreatePost` requires auth (author = token `sub`); `ListPosts` is public.
- Config: gRPC :9091, HTTP :8091, board MySQL DB, JWKS_URL, expected issuer/aud.
- Note: issuer *identifier* (iss claim) is configured separately from the JWKS
  *fetch URL* — inside compose the URL is http://identity:8090 while iss stays
  http://localhost:8090.

## Dockerize + M2 smoke
- Multi-stage Dockerfiles for identity & board; wire into docker-compose with
  healthchecks; named volume for the identity signing key.
- `scripts/m2_smoke.sh` against the compose stack: login on identity →
  CreatePost on board with the access token; no/garbage token → 401;
  ListPosts shows the post. Board has no identity-DB DSN by construction.
