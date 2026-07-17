# Plan: Revise prd.md — close spec gaps found in review

> Status: ✅ Completed 2026-07-17

## Context
The repo contained only `prd.md` (greenfield MSA IdP PoC, 21M MAU target). Review found the PRD directionally solid but with spec gaps and one internal inconsistency. This plan applied those improvements directly to `prd.md`.

## Changes applied

1. **Clarified the OIDC surface (§4.1)** — enumerated in-scope endpoints (discovery, `/oauth2/v1/token`, `/oauth2/v1/jwks`, first-party OTP APIs); frontend uses a first-party direct-grant flow; Authorization Code + PKCE marked as stretch goal.
2. **Added Token & Key Specification (§4.3)** — RS256, `kid`-based rotation (current + previous key in JWKS), access token TTL 15 min, refresh token TTL 14 days (opaque, Redis), RTR with family-wide invalidation on reuse.
3. **Fixed schema inconsistencies (§5)** — added `user_devices` table backing the device-change flow; defined full `status` enum + transitions (PENDING → ACTIVE ⇄ DORMANT; SUSPENDED; DELETED terminal); `password_hash` nullable/unused for PHONE; OTP state in Redis, not MySQL.
4. **Addressed the dual-write problem (§4.2)** — transactional `outbox` table + relay to Redis Streams; Board service named as `user.created` consumer (consumer group, at-least-once, idempotent).
5. **Added Success Criteria & NFRs (§8)** — k6 targets (Board verification ≥1,000 RPS p99 <50 ms; issuance ≥200 RPS p99 <300 ms), "zero Identity DB queries from Board" as provable property, circuit-breaker resilience check, testing strategy.
6. **Added Security Considerations (§7)** — OTP expiry/attempt caps/rate limits, revocation story, signing key handling.
7. **Smaller cleanups** — gobreaker placement named (Board → JWKS fetch), DB-per-service boundary explicit (separate `identity`/`board` databases), React chosen over "React/Vue", neighborhood-history quiz replaced with Identity-owned verification data, `buf` added to toolchain, repo name reconciled to `identity-service/`.

## Notes
- k6 numbers in §8 are provisional local-machine targets; adjust once real numbers exist.
