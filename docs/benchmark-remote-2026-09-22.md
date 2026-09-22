# Remote VM benchmark — 2026-09-22

> Status: completed — B1/B2 passed; Board passed through 500/s but failed at
> 1,000/s; B6 detected multiple successful rotations of one refresh token.

## Results

| Scenario | Target / duration | Completed iterations | Dropped | p99 | Operation errors | Gate |
| :--- | :--- | ---: | ---: | ---: | ---: | :---: |
| Board smoke | 20/s, 30 s | 600 | 0 | 12.33 ms | 0 | PASS |
| B1 refresh | 500/s, 5 min | 150,001 | 0 | 8.00 ms | 0 | PASS |
| B3 Board | 100/s, 1 min | 6,001 | 0 | 8.49 ms | 0 | PASS |
| B3 Board | 300/s, 1 min | 18,001 | 0 | 7.90 ms | 0 | PASS |
| B3 Board | 500/s, 1 min | 30,001 | 0 | 9.18 ms | 0 | PASS |
| B3 Board | 1,000/s, 1 min | 60,001 | 0 | 100.73 ms | 188 (0.3133%) | FAIL |
| B2 login | 100/s, 1 min | 6,001 | 0 | 21.12 ms | 1 (0.0167%) | PASS |
| B6 token reuse | 50 callers, one attempt each | 50 | 0 | — | 5 successful rotations; expected 1 | FAIL |

The B2 fixture run created 2,000 accounts with zero HTTP failures. B1, B2,
smoke, and Board 100/300/500 returned k6 exit code 0. Board 1,000 and B6
returned exit code 99 for failed thresholds. All tests completed without
dropped iterations. Completed iterations include unsuccessful operations;
for example, Board at 1,000/s had 59,813 successful operations.

The 500/s Board step is the highest tested rate that passed these gates in
this run, not a measured maximum capacity. The 1,000/s step failed both the
50 ms p99 and 0.1% error gates while meeting its iteration-count gate. No
server logs or HTTP status breakdown were captured, so the cause of the 188
failures is not established. Unbounded SQL pools in the reported older
revision are a hypothesis to investigate, not a demonstrated cause.

B6 observed five HTTP 200 responses and 45 HTTP 401 responses. Its teardown
also received the expected 401 for the original token. This reproduces
multiple successful rotations, consistent with the separate Redis read/write
operations in the reported older revision. The newer feature revision
replaces those operations with an atomic Lua script; that revision was not
deployed or tested on this VM during this run.

B2's single failure occurred at the OTP-request stage (6,001 iterations but
6,000 verify checks). The summary does not identify its HTTP status or cause.
Its error rate remains below the existing 0.1% gate; PASS does not mean zero
errors.

## Environment

The k6 generator runs on a separate machine from the application stack.
Identity and Board are reached directly over HTTP at `10.1.4.34:8090` and
`10.1.4.34:8091`. MySQL and Redis run alongside the services on the VM.

| Item | Recorded configuration |
| :--- | :--- |
| VM OS | Ubuntu 24.04.3 LTS, user-reported |
| VM memory | 3.8 GiB RAM and 3.8 GiB swap, user-reported |
| VM CPU | CPU count and model not supplied |
| VM checkout | `8071c722e07c5eb38e3782e1f20b1da4a9803c5b`, user-reported |
| Test scripts | `fa9f8bcfbbcdc3faeb7dd3112e7731fa33d1f94e` |
| Generator | Linux 6.6.87.2 WSL2, x86_64; 12 logical CPUs; 15,925 MiB RAM |
| Docker / k6 | Docker 29.4.3; k6 0.54.0, linux/amd64 |
| k6 image digest | `sha256:1f40432b1cbe7234e977f96c362c9bc550a2d2b583d014dd8669fe40d3e9e755` |

The running container build revision was not independently verified. The VM
checkout predates the Phase 7 atomic refresh-token rotation fix and Board SQL
pool limits. These results must not be treated as a like-for-like comparison
with the newer Phase 7 local baseline.

The generator host also runs unrelated Docker containers. No VM hardware or
application configuration was changed for the test. Initial discovery and
Board-list requests both returned HTTP 200, taking approximately 1.7 and 2.4 ms
respectively from the generator host.

## Scope and interpretation

The existing B1, B2, B3, and B6 scenarios ran sequentially. B2 accounts were
created through the signup API before measurement rather than by SQL fixtures.
B4 replica scaling and B5 Identity outage tests require VM-side orchestration
and are outside this run's coverage.

The existing thresholds are preserved: refresh p99 below 100 ms, login OTP
verification p99 below 300 ms, Board p99 below 50 ms, operation errors below
0.1%, and completed iterations at least 99% of the requested rate × duration.
Latency includes the network path. B2 performs an OTP request and an OTP verify
per iteration; its custom latency metric covers only the verify/token call.
B3 alternates authenticated creates and public list requests per virtual user,
so the overall mix is approximate, especially during VU startup.

B6 requires exactly one successful rotation across 50 callers and only HTTP
200/401 responses. Its teardown retries the original token and expects 401;
it does not directly attempt to use the winning replacement token.

## Resource snapshot

The user supplied this single server snapshot during the B1 refresh run:

| Container | CPU | Memory |
| :--- | ---: | ---: |
| Identity | 159.53% | 12.33 MiB |
| Board | 0.00% | 7.125 MiB |
| MySQL | 17.69% | 311.9 MiB |
| Redis | 4.79% | 12.79 MiB |

At that time the VM reported 2.8 GiB available RAM. CPU percentages above 100%
represent use of more than one logical CPU. This snapshot is not a peak or
average measurement, and cannot establish the bottleneck on its own.

A saved B1 generator snapshot showed 30.42% CPU and 57.85 MiB memory. A later
attempt to sample Board generator resources occurred after the container had
exited, so no Board resource measurement is available. There is no continuous
CPU, network, disk, or database telemetry for this run.

## Reproduction

The standard runner assumes local Docker management of the application;
these runs used the individual scripts with remote URLs instead. To recover
the exact script revision from this repository and run the full Board gate:

```bash
RUN_DIR=$(mktemp -d /tmp/identity-remote-repro.XXXXXX)
git archive fa9f8bcfbbcdc3faeb7dd3112e7731fa33d1f94e loadtest | tar -x -C "$RUN_DIR"
mkdir -p "$RUN_DIR/results"

docker run --rm --user "$(id -u):$(id -g)" \
  -v "$RUN_DIR/loadtest:/scripts:ro" \
  -v "$RUN_DIR/results:/artifacts" \
  -e IDENTITY_URL=http://10.1.4.34:8090 \
  -e BOARD_URL=http://10.1.4.34:8091 \
  -e RATE=1000 -e DURATION=1m \
  -e PREALLOCATED_VUS=500 -e MAX_VUS=1000 \
  -e SUMMARY_PATH=/artifacts/b3_board_1000.json \
  grafana/k6:0.54.0 run /scripts/b3_board.js
```

Use the same mounts and URLs with the following script/environment settings:

| Scenario | Script | Settings |
| :--- | :--- | :--- |
| Smoke | `b3_board.js` | `RATE=20 DURATION=30s PREALLOCATED_VUS=20 MAX_VUS=100` |
| Refresh | `b1_refresh.js` | `RATE=500 DURATION=5m MAX_VUS=200` |
| Board ramp | `b3_board.js` | `RATE=100`, then `300`, `500`, `1000`; `DURATION=1m PREALLOCATED_VUS=500 MAX_VUS=1000` |
| Login | `b2_login.js` | `RATE=100 DURATION=1m MAX_VUS=200 LOGIN_USERS=2000 LOGIN_RUN=1693` |
| Token reuse | `b6_rtr_reuse.js` | `CALLERS=50` |

Supply each setting with Docker `-e`, and choose a separate `SUMMARY_PATH`
per run. Tests require `DEV_MODE=true` for OTP retrieval and create persistent
users/posts. No cleanup was performed, and Board data accumulated between
steps. Signup phone suffixes depend on wall-clock time; repeated runs are not
guaranteed to start from identical data or avoid account collisions.

For B2, a temporary k6 script imported `signup` from the recorded `lib.js` and
ran one VU for 2,000 shared iterations, calling
`signup(93000000 + __ITER + 1)`. This creates phone numbers
`+821693000001` through `+821693002000`, matching `LOGIN_RUN=1693` and
`LOGIN_USERS=2000`. The exact seeder and sequential runner are in the local
artifact directory. On an already-seeded VM, reuse those accounts; creating
the same accounts again will fail signup. For a new fixture namespace, choose
an unused `LOGIN_RUN` from 1600–1699 and use the suffix
`(LOGIN_RUN - 1600) * 1000000 + __ITER + 1`.

## Artifacts

Raw JSON summaries, logs, copied scripts, environment metadata, and run timing
are retained locally under `artifacts/benchmarks/remote-2026-09-22/` (gitignored).
The documentation branch starts from dev; test scripts were copied from the
recorded feature revision before switching branches.
