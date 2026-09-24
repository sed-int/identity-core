#!/usr/bin/env bash
# Runs the Phase 7 B1-B6 matrix. The full profile uses the reviewed spec rates;
# SHORT=1 validates wiring and thresholds with small, fast loads.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
ARTIFACTS="$ROOT/artifacts/benchmarks"
K6_IMAGE=${K6_IMAGE:-grafana/k6:0.54.0}
IDENTITY_HOST=${IDENTITY_HOST:-http://localhost:8090}
IDENTITY_K6=${IDENTITY_K6:-http://host.docker.internal:8090}
BOARD_K6=${BOARD_K6:-http://host.docker.internal:8091}
SCENARIOS=${SCENARIOS:-"b1 b2 b3 b4 b5 b6"}
COMPOSE=(docker compose -f "$ROOT/docker-compose.yml" -f "$ROOT/docker-compose.benchmark.yml")
IDENTITY_STOPPED=0
BENCH_SERVICES_STARTED=0

fail() { echo "LOADTEST FAIL: $1" >&2; exit 1; }
has_scenario() { [[ " $SCENARIOS " == *" $1 "* ]]; }
need() { command -v "$1" >/dev/null || fail "missing prerequisite: $1"; }
need docker
need curl
need jq
mkdir -p "$ARTIFACTS"

cleanup() {
  if [[ $IDENTITY_STOPPED == 1 ]]; then
    docker compose -f "$ROOT/docker-compose.yml" start identity >/dev/null 2>&1 || true
  fi
  if [[ $BENCH_SERVICES_STARTED == 1 ]]; then
    "${COMPOSE[@]}" stop board-lb-two board-lb-one board-bench-b board-bench-a >/dev/null 2>&1 || true
    "${COMPOSE[@]}" rm -f board-lb-two board-lb-one board-bench-b board-bench-a >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

curl -fsS "$IDENTITY_HOST/.well-known/openid-configuration" >/dev/null || \
  fail "identity stack is not ready; run make run && make migrate-up"

run_k6() {
  local script=$1; shift
  docker run --rm \
    -v "$ROOT/loadtest:/scripts:ro" \
    -v "$ARTIFACTS:/artifacts" \
    -e "IDENTITY_URL=$IDENTITY_K6" \
    -e "BOARD_URL=$BOARD_K6" \
    "$@" "$K6_IMAGE" run "/scripts/$script"
}

issue_access_token() {
  local suffix phone code flow
  suffix=$(( ( $(date +%s) + $$ + RANDOM ) % 100000000 ))
  phone=$(printf '+8215%08d' "$suffix")
  code=$(curl -fsS "$IDENTITY_HOST/auth/v1/otp/request" \
    -H 'Content-Type: application/json' -d "{\"phone_number\":\"$phone\"}" | jq -er .debugCode)
  flow=$(curl -fsS "$IDENTITY_HOST/auth/v1/otp/verify" \
    -H 'Content-Type: application/json' -d "{\"phone_number\":\"$phone\",\"code\":\"$code\"}" | jq -er .flowToken)
  curl -fsS "$IDENTITY_HOST/auth/v1/signup" -H 'Content-Type: application/json' \
    -d "{\"flow_token\":\"$flow\",\"nickname\":\"phase7-scale\"}" | jq -er .tokens.accessToken
}

if [[ ${SHORT:-0} == 1 ]]; then
  B1_RATE=20; B1_DURATION=5s; B1_VUS=20
  B2_RATE=10; B2_DURATION=5s; LOGIN_USERS=100; B2_VUS=20
  B3_RATE=20; B3_DURATION=5s; B3_VUS=50
  SCALE_ONE_RATE=20; SCALE_TWO_RATE=38; SCALE_DURATION=5s
  RESILIENCE_RATE=20; RESILIENCE_DURATION=5s; B6_CALLERS=10
else
  B1_RATE=500; B1_DURATION=5m; B1_VUS=200
  B2_RATE=100; B2_DURATION=1m; LOGIN_USERS=2000; B2_VUS=200
  B3_RATE=1000; B3_DURATION=1m; B3_VUS=1000
  SCALE_ONE_RATE=1000; SCALE_TWO_RATE=1900; SCALE_DURATION=20s
  RESILIENCE_RATE=500; RESILIENCE_DURATION=20s; B6_CALLERS=50
fi

if has_scenario b1; then
  echo "== B1 refresh: ${B1_RATE} RPS for ${B1_DURATION}"
  run_k6 b1_refresh.js -e "RATE=$B1_RATE" -e "DURATION=$B1_DURATION" -e "MAX_VUS=$B1_VUS" \
    -e SUMMARY_PATH=/artifacts/b1_refresh.json
fi

if has_scenario b2; then
  echo "== B2 active full login: ${B2_RATE} RPS for ${B2_DURATION}"
  LOGIN_RUN=$(printf '%04d' $(( ( $(date +%s) + $$ ) % 10000 )))
  docker exec -i idsvc-mysql mysql -uroot -proot \
    --init-command="SET @bench_run='$LOGIN_RUN'; SET @bench_count=$LOGIN_USERS" identity \
    < "$ROOT/loadtest/seed_login.sql"
  run_k6 b2_login.js -e "RATE=$B2_RATE" -e "DURATION=$B2_DURATION" \
    -e "LOGIN_RUN=$LOGIN_RUN" -e "LOGIN_USERS=$LOGIN_USERS" -e "MAX_VUS=$B2_VUS" \
    -e SUMMARY_PATH=/artifacts/b2_login.json
fi

if has_scenario b3; then
  echo "== B3 Board create/list: ${B3_RATE} RPS for ${B3_DURATION}"
  run_k6 b3_board.js -e "RATE=$B3_RATE" -e "DURATION=$B3_DURATION" \
    -e "PREALLOCATED_VUS=$((B3_VUS / 2))" -e "MAX_VUS=$B3_VUS" \
    -e SUMMARY_PATH=/artifacts/b3_board.json
fi

if has_scenario b4 || has_scenario b5; then
  echo "== starting load-balanced Board replicas"
  "${COMPOSE[@]}" stop board-lb-two board-bench-b >/dev/null 2>&1 || true
  "${COMPOSE[@]}" up -d --build board-bench-a board-lb-one
  BENCH_SERVICES_STARTED=1
  ACCESS_TOKEN=$(issue_access_token)
  for _ in {1..30}; do
    if curl -fsS -o /dev/null -H 'Content-Type: application/json' -H "Authorization: Bearer $ACCESS_TOKEN" \
      -d '{"title":"warm-cache","content":"phase7"}' http://localhost:18091/board/v1/posts; then break; fi
    sleep 1
  done
fi

if has_scenario b4; then
  echo "== B4a warm-cache verifier microbenchmark"
  go test -run '^$' -bench BenchmarkVerifyWarmCache -benchmem ./pkg/jwks \
    > "$ARTIFACTS/b4_verifier_benchmark.txt"
  echo "== B4 scale-out: one replica at ${SCALE_ONE_RATE} RPS"
  run_k6 verify.js -e BOARD_URL=http://host.docker.internal:18091 -e "ACCESS_TOKEN=$ACCESS_TOKEN" \
    -e "RATE=$SCALE_ONE_RATE" -e "DURATION=$SCALE_DURATION" -e MAX_P99=none \
    -e SUMMARY_PATH=/artifacts/b4_one.json
  "${COMPOSE[@]}" up -d --build board-bench-b board-lb-two
  echo "== B4 scale-out: two replicas at ${SCALE_TWO_RATE} RPS"
  run_k6 verify.js -e BOARD_URL=http://host.docker.internal:18092 -e "ACCESS_TOKEN=$ACCESS_TOKEN" \
    -e "RATE=$SCALE_TWO_RATE" -e "DURATION=$SCALE_DURATION" -e MAX_P99=none \
    -e SUMMARY_PATH=/artifacts/b4_two.json
  jq -n \
    --argjson one "$(jq '.metrics.iterations.values.count' "$ARTIFACTS/b4_one.json")" \
    --argjson two "$(jq '.metrics.iterations.values.count' "$ARTIFACTS/b4_two.json")" \
    '{one_iterations:$one,two_iterations:$two,throughput_ratio:($two/$one)}' \
    > "$ARTIFACTS/b4_ratio.json"
  jq -e '.throughput_ratio >= 1.8' "$ARTIFACTS/b4_ratio.json" >/dev/null || fail "B4 ratio below 1.8x"
  docker stats --no-stream --format '{{json .}}' idsvc-identity > "$ARTIFACTS/b4_identity_stats.json"
fi

if has_scenario b5; then
  echo "== B5 resilience: warm cache, stop Identity, keep valid traffic green"
  curl -fsS -o /dev/null -H 'Content-Type: application/json' -H "Authorization: Bearer $ACCESS_TOKEN" \
    -d '{"title":"resilience-warm","content":"phase7"}' http://localhost:18091/board/v1/posts
  BOARD_A_ID=$("${COMPOSE[@]}" ps -q board-bench-a)
  docker compose -f "$ROOT/docker-compose.yml" stop identity
  IDENTITY_STOPPED=1
  run_k6 verify.js -e BOARD_URL=http://host.docker.internal:18091 -e "ACCESS_TOKEN=$ACCESS_TOKEN" \
    -e "RATE=$RESILIENCE_RATE" -e "DURATION=$RESILIENCE_DURATION" \
    -e CREATE_ONLY=true \
    -e SUMMARY_PATH=/artifacts/b5_resilience.json &
  K6_PID=$!
  sleep 1
  UNKNOWN_TOKEN='eyJhbGciOiJSUzI1NiIsImtpZCI6InBoYXNlNy11bmtub3duIn0.eyJpc3MiOiJodHRwOi8vbG9jYWxob3N0OjgwOTAiLCJzdWIiOiIxIiwiYXVkIjoiYm9hcmQiLCJleHAiOjQxMDI0NDQ4MDB9.AA'
  for _ in {1..8}; do
    curl -sS -o /dev/null -H 'Content-Type: application/json' -H "Authorization: Bearer $UNKNOWN_TOKEN" \
      -d '{"title":"unknown-kid","content":"breaker probe"}' http://localhost:18091/board/v1/posts || true
    sleep 0.02
  done
  docker logs "$BOARD_A_ID" > "$ARTIFACTS/b5_board.log" 2>&1
  grep -q '"to":"open"' "$ARTIFACTS/b5_board.log" || fail "B5 breaker did not open"
  wait "$K6_PID"
  docker compose -f "$ROOT/docker-compose.yml" start identity
  IDENTITY_STOPPED=0
  sleep 3
  curl -sS -o /dev/null -H 'Content-Type: application/json' -H "Authorization: Bearer $UNKNOWN_TOKEN" \
    -d '{"title":"recovery-probe","content":"phase7"}' http://localhost:18091/board/v1/posts || true
  docker logs "$BOARD_A_ID" > "$ARTIFACTS/b5_board.log" 2>&1
  grep -q '"to":"closed"' "$ARTIFACTS/b5_board.log" || fail "B5 breaker did not recover after Identity restart"
fi

if has_scenario b6; then
  echo "== B6 concurrent RTR reuse flood: $B6_CALLERS callers"
  run_k6 b6_rtr_reuse.js -e "CALLERS=$B6_CALLERS" -e SUMMARY_PATH=/artifacts/b6_rtr_reuse.json
fi

echo "PHASE 7 LOADTEST MATRIX PASSED; artifacts: $ARTIFACTS"
