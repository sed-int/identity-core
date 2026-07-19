# Plan: SLO & Benchmark Spec — substantiating the 21M MAU claim

> Status: 📋 Planned — spec for Phase 7 (NFR verification); numbers reviewed by hcho before implementation

The README/PRD claim "designed for 21M MAU". This document turns that into
(1) a traffic model, (2) SLOs per operation, and (3) a benchmark matrix whose
pass/fail is the definition of done for Phase 7.

## 1. Traffic model (why these numbers)

Assumptions for a Daangn-scale consumer app:

| Parameter | Value | Rationale |
| :--- | :--- | :--- |
| MAU | 21,000,000 | PRD target |
| DAU/MAU | 50% → 10.5M DAU | high-engagement local commerce app |
| Peak factor | 5× average | evening peak, KR single-timezone traffic |
| Full login (OTP) frequency | 1 per user / 14 days | refresh TTL 14d + RTR keeps sessions alive |
| Token refresh frequency | 3 per DAU / day | 15-min access TTL over ~2-3 app sessions/day |
| API calls per DAU / day | 100 | every one is a token **verification** |

Derived load:

| Operation | Avg RPS | Peak RPS | Served by |
| :--- | ---: | ---: | :--- |
| Full login (OTP request+verify+issue) | ~17 | **~90** | Identity (MySQL+Redis+RSA sign) |
| Token refresh (RTR) | ~365 | **~1,800** | Identity (Redis+RSA sign) |
| Token verification | ~12,000 | **~60,000** | **Resource servers, in-process** — zero IdP load |
| JWKS fetch | ~0 | pods × 1/5min | Identity (static JSON, CDN-able) |

**The architectural point the numbers prove:** verification — the 60k RPS
problem — never touches the IdP. The IdP's real write load is ~2k RPS peak,
which a modest MySQL/Redis footprint handles. Stateless verification converts
an O(traffic) auth problem into an O(logins) one.

## 2. SLOs (production targets)

| SLI | SLO | Notes |
| :--- | :--- | :--- |
| Login availability | 99.9% monthly | error budget ≈ 43 min/month |
| Login latency | p99 < 300 ms | OTP verify → tokens issued |
| Refresh availability | 99.95% | breaks active sessions if violated |
| Refresh latency | p99 < 100 ms | Redis RTT + RSA sign |
| Verification latency | p99 < 5 ms in-process | no network by design |
| Verification availability | 100% while JWKS cache warm | must survive total IdP outage (breaker + stale cache) |
| JWKS endpoint availability | 99.99% | clients tolerate hours of staleness |

## 3. PoC benchmark matrix (Phase 7, local scale)

Local hardware ≠ production, so the PoC proves **shape, not absolute scale**:
latency distributions, linear scaling, and resilience behavior. Local
pass/fail gates (≈ PRD §8, tightened to the traffic model):

| # | Scenario (k6) | Gate |
| :--- | :--- | :--- |
| B1 | Refresh grant sustained 5 min | ≥ 500 RPS, p99 < 100 ms, errors < 0.1% |
| B2 | Full login flow sustained | ≥ 100 RPS, p99 < 300 ms |
| B3 | Board create+list with tokens | ≥ 1,000 RPS, p99 < 50 ms (gateway path) |
| B4 | **Scale-out proof**: board ×1 vs ×2 replicas | ~2× throughput; identity CPU & JWKS hit-rate flat |
| B5 | **Resilience**: kill identity during B3 | board error rate stays 0%; breaker opens; recovery on restart |
| B6 | RTR reuse-flood (attack sim) | family revocations correct under concurrency, no 5xx |

Report: `docs/benchmark-results.md` with raw k6 summaries + docker stats;
README roadmap table links measured numbers next to the gates.

## 4. Honest limitations to document with results

- Single-node MySQL/Redis: production needs read replicas / Redis Cluster;
  the PoC measures the *stateless tier's* independence from that scaling.
- RS256 sign cost dominates issuance CPU → production would batch/pool or use
  ES256; note measured sign µs in results.
- No network hops between LB tiers locally — absolute latencies flatter
  than production; distributions and scaling ratios are the signal.
