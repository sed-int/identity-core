# Remote VM load-test run plan

> Status: ✅ completed — measurements recorded, including B3/B6 failures

## Goal and scope

Run the existing k6 tests from this machine against the user-provided VM
`10.1.4.34`, preserving the existing thresholds, and record measured results.
The user authorized the test and recording its results. No service changes,
merges, or pushes are included.

## Execution

- [x] Verify Identity :8090 and Board :8091 respond over the network.
- [x] Prepare Docker image `grafana/k6:0.54.0`, matching the previous baseline.
- [x] Preserve test scripts from commit `fa9f8bcfbbcdc3faeb7dd3112e7731fa33d1f94e` before branching from dev.
- [x] Run Board smoke test at 20 iterations/s for 30 seconds.
- [x] Run B1 refresh at 500 iterations/s for 5 minutes, 200 maximum VUs.
- [x] Run B3 Board at 100, 300, 500, and 1,000 iterations/s for 1 minute each.
- [x] Seed 2,000 B2 users through the signup API and run login at 100 iterations/s for 1 minute.
- [x] Run B6 with 50 callers and inspect checks and teardown outcome.
- [x] Record summaries, exit codes, reproduction commands, and generator conditions.
- [x] Verify report numbers against raw JSON and prepare documentation for the review-gated commit.

Run scenarios sequentially. Save raw outputs under
`artifacts/benchmarks/remote-2026-09-22/`. Preserve failures rather than relaxing
thresholds. If smoke testing fails, diagnose before increasing load.

## Limits

B2 fixture creation can use the existing signup helper with suffixes matching
the login script's phone-number format (`LOGIN_RUN=1693`). Seed 2,000 accounts
sequentially before the measured run; this replaces SQL seeding only.
B4 and B5 need VM-side Docker orchestration and are not covered without remote
management access.
VM hardware, deployed revision, server resource utilization, and network
topology are unknown unless supplied separately. Results include network time.
Tests create users and posts in the dedicated test service.

## Outcome

See `docs/benchmark-remote-2026-09-22.md`. Refresh at 500/s and login at 100/s
passed. Board passed through 500/s, but at 1,000/s p99 was 100.73 ms with
188 errors (0.3133%). Token reuse had five successful rotations rather than
one. All measured runs had zero dropped iterations. The reported VM checkout
was `8071c722e07c5eb38e3782e1f20b1da4a9803c5b`, before the feature branch's
atomic rotation fix. No service changes were made.
