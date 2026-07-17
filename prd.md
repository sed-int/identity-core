# 🛡️ Global Identity Service Platform (MSA PoC)

## 1. Project Overview
This project is a Proof of Concept (PoC) for a **Microservices Architecture (MSA)-based Identity Provider (IdP)** designed to handle large-scale traffic (target: 21 million MAU).
To resolve the authentication/authorization bottlenecks typically found in monolithic systems, the authentication system has been decoupled into an independent OIDC-style Provider. It simulates an End-to-End (E2E) flow where other microservices (e.g., a Board Resource Server) verify permissions statelessly using tokens issued by this IdP.

### 🎯 Key Engineering Goals
- **Token Issuer / OIDC-Compatible Provider Role:** Establish a centralized authentication/authorization platform for internal microservices. The PoC implements the OIDC-compatible subset needed for stateless verification (discovery, JWKS, token issuance); the full Authorization Code + PKCE flow for third-party apps is a stretch goal (see §4.1).
- **Protobuf Driven Development:** Utilize `.proto` files as the Single Source of Truth to automatically generate gRPC code, REST proxies (gRPC-Gateway), and Swagger documentation. `buf` enforces lint rules and breaking-change detection on the API contract.
- **Layered Architecture:** Strictly decouple domain logic from infrastructure and delivery layers to maximize testability and maintainability.
- **Scalable Database Design:** Normalize and separate core state information from sub-domain data to reduce the complexity of the legacy single `User` table.

---

## 2. System Architecture

The architecture understands enterprise-grade goals but replaces heavy infrastructure with lightweight, pragmatic alternatives suitable for local testing and PoC purposes.

| Component | Target (Enterprise) | PoC Implementation (Current) |
| :--- | :--- | :--- |
| **Infrastructure / Routing** | Kubernetes (k8s) | **Docker Compose** (Integrated runtime) |
| **Service Communication** | gRPC (Internal) / API Gateway | **gRPC (Internal) / gRPC-Gateway (REST Proxy)** |
| **Circuit Breaker** | Istio (Envoy Proxy) | **Code-level:** Go `sony/gobreaker` wrapping the Board → Identity JWKS fetch (and any future cross-service call) |
| **Event Streaming** | Apache Kafka | **Redis Streams** (consumer groups, at-least-once delivery) |
| **Database** | MySQL Cluster (DB per service) | **MySQL** (Single container, but **separate `identity` and `board` databases** — no cross-DB joins, preserving the DB-per-service boundary) + **go-migrate** |
| **Signing Key Management** | KMS / HSM | **Local PEM file mounted via Docker secret / env** |

---

## 3. Monorepo Project Structure

The Identity Server, Board (Resource) Server, and Frontend are managed within a single repository (`identity-service/`). API specifications are strictly shared across services via the `api/` directory.

```text
identity-service/
├── api/                        # [Shared] Protobuf specs & auto-generated code (gRPC/Swagger)
│   └── buf.yaml                # buf lint & breaking-change configuration
├── services/                   # Go Backend Microservices
│   ├── identity/               # 1. Identity Provider (Auth, Token issuance)
│   │   ├── cmd/                # Entrypoint
│   │   ├── internal/           # Layered Architecture (Domain, Usecase, Repo, Delivery)
│   │   └── db/migrations/      # go-migrate (users, user_credentials, user_devices, outbox)
│   └── board/                  # 2. Resource Server (Board business logic)
│       ├── internal/           # Board logic & JWKS token verification middleware
│       └── db/migrations/      # go-migrate (posts, comments schemas)
├── frontend/                   # 3. Client App (React - Login UI & API integration)
├── pkg/                        # Shared Go Libraries
│   ├── logger/                 # Global structured logger (request-ID propagation)
│   └── jwks/                   # JWKS-based stateless JWT verification (kid-aware cache)
├── docker-compose.yml          # Runs infrastructure (MySQL, Redis) & all services
└── Makefile                    # Automation for protoc/buf builds, DB migrations, etc.
```

---

## 4. Core Authentication Flow (State-Based Routing)

### 4.1 Auth model & endpoints in scope

The frontend is a **first-party client**, so the PoC uses a **direct-grant login API** (phone/OTP in exchange for tokens) rather than the browser-redirect Authorization Code flow. The OIDC-compatible surface exposed by the Identity Service:

| Endpoint | Purpose |
| :--- | :--- |
| `GET /.well-known/openid-configuration` | Discovery document (issuer, JWKS URI, supported algs) |
| `GET /oauth2/v1/jwks` | Public keys for stateless verification |
| `POST /oauth2/v1/token` | Token issuance & refresh (RTR) |
| `POST /auth/v1/otp/request`, `POST /auth/v1/otp/verify` | First-party phone/OTP login & signup |

> **Stretch goal:** standard Authorization Code + PKCE flow for third-party clients. Out of scope for the initial PoC.

### 4.2 State-based routing

After verifying the client's credentials (Phone/OTP), the system dynamically routes the user based on their account `status` and device footprint (`user_devices`, see §5) stored in the Identity DB.

1. **Unregistered (New Signup)**
   - Completes profile setup (nickname, etc.), creates DB records, and publishes a `user.created` event ➔ Transitions to `ACTIVE` state and issues tokens.
   - **Event publishing uses a transactional outbox:** the user rows and an `outbox` row are written in one MySQL transaction; a relay process reads the outbox and publishes to Redis Streams. This avoids the dual-write problem (DB commit succeeding while the event publish fails). The **Board service** consumes `user.created` via a Redis Streams consumer group (at-least-once; consumers must be idempotent).
2. **Normal Login (`ACTIVE`)**
   - Executes Refresh Token Rotation (RTR) logic and issues security tokens.
3. **Reactivation (`DORMANT`)**
   - Accounts inactive for over 1 year. Requires re-consent to privacy policies before rolling back to the `ACTIVE` state.
4. **Device Change Detected (Security Flow)**
   - If a login from a device not present in `user_devices` is detected, the user must pass an additional verification step **based on Identity-owned data only** (e.g., confirming account creation month, re-verifying OTP on the registered number). Cross-domain data such as neighborhood history is deliberately **out of scope** — pulling it into the IdP would violate the service decoupling this project exists to demonstrate.

### 4.3 Token & Key Specification

- **Signing:** RS256. JWKS serves the **current + previous** key, each with a `kid`; verifiers select by `kid`, enabling zero-downtime rotation.
- **Access Token:** JWT, **TTL 15 minutes**. Claims: `iss`, `sub` (user id), `aud`, `exp`, `iat`, `jti`, plus custom `status` claim.
- **ID Token:** JWT with standard OIDC claims (`sub`, `auth_time`, `nonce` when provided).
- **Refresh Token:** opaque, **TTL 14 days**, stored server-side in **Redis** keyed by token family.
- **RTR (Refresh Token Rotation):** every refresh issues a new refresh token and invalidates the old one. **Reuse detection:** presenting an already-rotated token invalidates the entire token family and forces re-login.
- **Revocation/Logout:** logout deletes the refresh-token family. Access tokens are not denylisted; the 15-minute TTL bounds the exposure window (documented trade-off of stateless verification).

---

## 5. Database Schema & Migration

To solve the high-coupling issues caused by the legacy "massive User table", core states and supplementary data are strictly separated. Schema versioning is managed via `go-migrate`.

- **`users` (Core Master)**
  - Identifier (`id`), account state (`status`), created/updated timestamps. (Highly cacheable due to infrequent changes.)
  - **`status` enum & transitions:** `PENDING` (signup started) → `ACTIVE` ⇄ `DORMANT` (1 year inactive); `ACTIVE`/`DORMANT` → `SUSPENDED` (admin action) → `ACTIVE`; any → `DELETED` (terminal, soft-delete).
- **`user_credentials` (Auth Sub-domain)**
  - Auth method (`auth_type`: PHONE, EMAIL), credential identifier, `password_hash` (**nullable — unused for `PHONE`**, reserved for future EMAIL+password). 1 user : N credentials, to easily expand login methods.
  - **OTP state is NOT stored here** — OTP codes, attempt counters, and rate-limit windows live in **Redis with TTLs**.
- **`user_devices` (Security Sub-domain)**
  - `user_id`, `device_fingerprint`, `device_name`, `last_seen_at`. Backs the device-change detection flow in §4.2.
- **`user_profiles` (Display Sub-domain)**
  - Frequently accessed/updated data for frontend display (nickname, profile image URL, reputation score).
- **`outbox` (Eventing)**
  - `id`, `aggregate_type`, `event_type`, `payload` (JSON), `published_at`. Written transactionally with domain changes; relayed to Redis Streams (§4.2).

---

## 6. End-to-End Scenario (PoC Execution Flow)

This represents the core stateless-authorization workflow observable when running the monorepo.

1. **User (Frontend, React)** completes phone verification (`/auth/v1/otp/*`) and receives OIDC-standard `ID Token`, `Access Token`, and a refresh token from the `Identity Service`.
2. To write a post, the User sends an API request to the **Board Service** with the access token in the Authorization header.
3. **Board Service** *does not* query the Identity DB. Instead, it fetches the public keys from `GET /oauth2/v1/jwks` (wrapped in `gobreaker`), **caches them in-memory keyed by `kid`** (refetching once on an unknown `kid`, with a cache TTL as fallback), performs **Stateless Verification** of the token signature and claims, and subsequently allows the post creation.

---

## 7. Security Considerations

- **OTP hardening:** codes expire in 3 minutes; max 5 verification attempts per code; per-phone-number request rate limiting (e.g., 5/hour) — all enforced via Redis counters.
- **Token theft mitigation:** RTR with family-wide invalidation on reuse (§4.3); short access-token TTL.
- **Key handling:** RSA private key is mounted as a local secret for the PoC; never committed to the repo. Enterprise target: KMS-backed signing.
- **Transport:** plain HTTP inside Docker Compose is accepted for the PoC; TLS termination is assumed at the gateway in the enterprise target.

---

## 8. Success Criteria & NFRs

The 21M MAU target is substantiated by measurable PoC outcomes, not just architecture claims:

1. **Functional:** the full §6 scenario passes via an automated E2E script (signup → login → RTR refresh → post creation).
2. **Statelessness proven:** Board service creates posts with **zero queries to the Identity DB** (verified by DB query logs during the E2E run).
3. **Load test (k6):** token verification path on the Board service sustains **≥ 1,000 RPS with p99 < 50 ms** locally; token issuance sustains **≥ 200 RPS with p99 < 300 ms**. (Local reference numbers — the point is demonstrating the verification path scales independently of the IdP.)
4. **Resilience:** with the Identity service down, the Board service continues verifying tokens from its JWKS cache; `gobreaker` opens and recovery is observable in logs.

### Testing Strategy
- **Unit:** domain & usecase layers (pure Go, no infra).
- **Integration:** repository layer against real MySQL/Redis via testcontainers.
- **E2E:** one scripted run of the §6 scenario against `docker compose up`.

---

## 9. Getting Started

Builds and executions are automated using the `Makefile`.

```bash
# 1. Lint & compile Protobuf (buf + protoc: gRPC code, gateway, Swagger UI)
$ make proto

# 2. Migrate MySQL DB schemas (go-migrate)
$ make migrate-up

# 3. Run all infrastructure and services (Docker Compose)
$ make run

# 4. Run the E2E scenario & load test
$ make e2e
$ make loadtest
```
