# Plan: Phase 1 (API contract) + Phase 2 (Identity core, happy path)

> Status: ✅ Completed 2026-07-17 — M1 milestone passed (scripts/m1_smoke.sh)

## Phase 1 — API contract (`feat/api-contract`)

- `identity.proto` (`identity.v1`), gateway paths per PRD §4.1:
  - `RequestOtp` → `POST /auth/v1/otp/request` — returns expiry; `debug_code` field populated only in dev mode (mock SMS).
  - `VerifyOtp` → `POST /auth/v1/otp/verify` — returns `next_step` enum (TOKENS_ISSUED / SIGNUP_REQUIRED / REACTIVATION_REQUIRED / DEVICE_VERIFICATION_REQUIRED) + `tokens` or short-lived `flow_token` for flow continuation. Implements PRD §4.2 state routing at the contract level.
  - `CompleteSignup` → `POST /auth/v1/signup` — flow_token + nickname → tokens.
  - `IssueToken` → `POST /oauth2/v1/token` — `grant_type=refresh_token` (RTR).
- `board.proto` (`board.v1`): `CreatePost` (`POST /board/v1/posts`), `ListPosts` (`GET /board/v1/posts`). Implemented in Phase 3; contract fixed now.
- JWKS + discovery stay plain HTTP (fixed-shape OIDC endpoints).
- Reactivation / device-verification RPCs are Phase 6 additions (additive = non-breaking).

## Phase 2 — Identity service (`feat/identity-core`)

- **Migrations** (§5): `users` (status ENUM, PENDING default), `user_credentials` (UNIQUE(auth_type, identifier), nullable password_hash), `user_profiles`, `user_devices` (UNIQUE(user_id, fingerprint)), `outbox` (payload JSON, published_at NULL, indexed).
- **Layout** under `services/identity/internal/`:
  - `domain/` — User + Status transition rules as pure functions.
  - `repo/mysql/` — transactional signup (users+credentials+profiles+outbox row in one tx), lookup by phone credential.
  - `token/` — RS256 issuer (kid = pubkey hash), access token 15m / ID token, flow tokens (10m, purpose-scoped), JWKS & discovery handlers; RSA key auto-generated at data/keys/ in dev (gitignored via *.pem).
  - `rtr/` — Redis refresh-token store: `rt:token:<t>` + `rt:family:<f>`, rotate-on-refresh, reuse ⇒ family revoked (PRD §4.3).
  - `otp/` — Redis OTP store: 3-min TTL, 5 attempts, per-phone rate limit; mock SMS logs the code.
  - `usecase/` — auth orchestration (state routing per §4.2; device flow stubbed as always-known until Phase 6).
  - `delivery/grpc/` — generated server impl + request-ID/logging interceptor; `cmd/main.go` wires gRPC (:9090) + gateway/HTTP (:8090; 8080 is taken by go-msa nginx).
- **Deps added**: golang-jwt/v5, go-redis/v9, go-sql-driver/mysql, google/uuid, miniredis (tests).
- **Tests**: status machine, token issue/verify roundtrip, RTR rotation + reuse detection (miniredis), OTP attempts/expiry (miniredis).
- **M1 verification**: `scripts/m1_smoke.sh` — request OTP (dev code) → verify → signup → tokens → refresh rotation (old token rejected, reuse kills family) → JWKS/discovery reachable.
