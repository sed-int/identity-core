# Phase 7 local benchmark results

> Run date: 2026-09-01 (Asia/Seoul)<br>
> Profile: full local acceptance matrix (`make loadtest`)<br>
> Host: MacBook Air, Apple M1 (8 cores), 16 GB RAM, macOS 26.3.1<br>
> Runtime: Docker Desktop 25.0.2, Go 1.25.0, k6 0.54.0

These are **local PoC gates**, not production capacity numbers. The load
generator, services, MySQL, Redis, and Docker VM share one laptop. The run
substantiates architecture and local latency shape; an AWS deployment with
separate load generators, ALB/TLS, RDS, and ElastiCache is required before
making production sizing or cost claims for 21M MAU.

## Result summary

| Gate | Full-run result | Status |
| :--- | :--- | :---: |
| B1 refresh, 500 RPS for 5 min | 149,964 completed; 37 scheduler drops (0.025%); p99 **42.10 ms**; **0% errors** | PASS |
| B2 active login, 100 RPS for 1 min | 6,000 completed; no drops; OTP-verify-to-token p99 **7.96 ms**; **0% errors** | PASS |
| B3 Board create/list, 1,000 RPS for 1 min | 60,001 completed; no drops; p99 **17.53 ms**; **0.032% errors** | PASS |
| B4 Board x1 vs x2 | x1: 1,000.1 RPS; x2: 1,893.6 RPS; **1.89x** completed-throughput ratio; 0% errors in the isolated rerun | PASS |
| B5 stop Identity under authenticated Board writes | 10,001/10,001 completed at 500 RPS; p99 **12.61 ms**; 0% errors; breaker `closed -> open -> half-open -> closed` | PASS |
| B6 50-way concurrent RTR reuse | exactly one rotation won; every other response was a reuse rejection; winner's family revoked; no 5xx | PASS |

## B4: component capacity versus system tail latency

The warm-cache verifier benchmark uses the real RS256 parser/verifier with a
cached JWKS key:

```text
BenchmarkVerifyWarmCache-8  147769  8047 ns/op  4216 B/op  70 allocs/op
```

That is roughly 124k verifications/second on this host, above the traffic
model's 60k-RPS peak verification load. It is a component benchmark, not an
HTTP capacity promise.

The scale-out k6 test uses only real Board operations through nginx, with the
same 50/50 create/list mix as B3. One replica held p99 **11.39 ms** at 1,000
RPS. Two replicas delivered 1.89x throughput at the requested 1,900 RPS with
0% request errors, but p99 rose to **129.40 ms** and 130 iterations (0.34%)
were not scheduled. This still passes B4's throughput-ratio gate, while showing
that the shared laptop/MySQL tier cannot preserve B3's separate 50 ms tail
target at 1.9x load.

The first B4 attempt also exposed unbounded Go SQL pools hitting MySQL's
151-connection ceiling. Phase 7 now budgets each Board replica to 40 open / 10
idle connections and gives benchmark replicas a clean lifecycle. The isolated
rerun had zero errors. This is a concrete NFR hardening result, not benchmark
tuning: production replicas must divide the database connection budget.

## B5: cached-key outage behavior

The harness warms Board's JWKS cache with a real authenticated `CreatePost`,
stops the Identity container, and continues real authenticated writes. Forged
unknown-`kid` requests are sent separately so they exercise the JWKS fetch
breaker without contaminating the valid-traffic SLI. Structured Board logs
recorded:

```text
closed -> open -> half-open -> closed
```

Valid traffic remained at 0% errors throughout the Identity outage. This
demonstrates that request-time verification has no Identity dependency once the
cache is warm.

## B6: concurrency defect found and fixed

The original RTR implementation performed Redis `GET`, comparison, and token
replacement as separate operations. Concurrent presentations of one current
refresh token could therefore produce multiple winners. Phase 7 moved the
compare/rotate/revoke decision into one Redis Lua operation. A 32-goroutine Go
test and the 50-client k6 flood now prove that one rotation wins and the first
reuse atomically revokes the winning token's family.

## Reproduction

```bash
make run
make migrate-up
make e2e
make integration     # ephemeral MySQL 8 repository tests
make loadtest-short  # wiring/CI-sized profile
make loadtest        # full local acceptance profile; B1 alone runs 5 minutes
```

Machine-readable summaries and service-stat snapshots are written to the
gitignored `artifacts/benchmarks/` directory. Use the committed report for the
reviewed baseline; raw files are intentionally machine-local.

## Limits and next validation tier

- MySQL and Redis are single containers, not RDS/ElastiCache clusters.
- Docker Desktop adds a VM boundary and shares CPU with k6 and every service.
- No ALB, TLS, cross-AZ latency, production data volume, or autoscaling is
  represented.
- The verifier still allocates 4.2 KB / 70 allocations per verification;
  reducing parser allocations is a valid optimization before very high scale.
- The next capacity run should use the same thresholds unchanged on AWS, with
  load generators isolated from the system under test.
