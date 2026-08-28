# Plan: Dormancy sweep (ACTIVE → DORMANT)

> Status: 📋 planned — backlog (agreed 2026-08-28, after phase 6 review discussion)

Phase 6 shipped the recovery path (DORMANT → ACTIVE reactivation) but nothing yet *produces* DORMANT accounts — m6 smoke forces the status via SQL. This adds the missing producer: a periodic job that transitions inactive accounts, per PRD §4.2 ("accounts inactive for over 1 year").

## Sketch

- Ticker goroutine in the identity service, same shape as `internal/outbox/relay.go` (start/stop in `app`, graceful shutdown).
- Each tick:

  ```sql
  UPDATE users SET status='DORMANT'
  WHERE status='ACTIVE' AND last_login_at < NOW() - INTERVAL ? SECOND
  ```

  Period from `DORMANT_AFTER` env var (see [policy-config plan](2026-08-28-policy-config.md)) — default 1 year, set to minutes locally to demo end to end.
- Emit `user.dormant` outbox rows for swept users if Board (or future consumers) should react; skip if nothing consumes it yet — decide at build time (YAGNI check).
- Route the transition through the domain state machine per-user OR document the bulk-UPDATE shortcut with a `ponytail:` comment (bulk SQL bypasses `TransitionTo`; acceptable because ACTIVE→DORMANT is always legal).
- m6 smoke: replace the manual `UPDATE ... SET status='DORMANT'` with the sweep (short `DORMANT_AFTER` + trigger/wait), or keep SQL and add a separate sweep assertion.

## Out of scope

- 30-day advance notice mail (개인정보보호법-style) — needs a real notification channel; PoC has mock SMS only.
- Admin endpoint to force-dormant a user.

Branch: `feat/dormancy-sweep`. Fits before/alongside Phase 7.
