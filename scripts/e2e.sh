#!/usr/bin/env bash
# Phase 7 executable E2E definition: signup -> known-device login -> RTR ->
# authenticated post -> public list. Requires the Compose stack and migrations.
set -euo pipefail

IDENTITY_URL=${IDENTITY_URL:-http://localhost:8090}
BOARD_URL=${BOARD_URL:-http://localhost:8091}
suffix=$(( ( $(date +%s) + $$ ) % 100000000 ))
PHONE=$(printf '+8217%08d' "$suffix")
DEVICE="phase7-$suffix"

fail() { echo "E2E FAIL: $1" >&2; exit 1; }
need() { command -v "$1" >/dev/null || fail "missing prerequisite: $1"; }
post() { curl --fail --silent --show-error -H 'Content-Type: application/json' "$@"; }

need curl
need jq

echo "== signup: $PHONE"
OTP=$(post "$IDENTITY_URL/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}")
CODE=$(jq -er .debugCode <<<"$OTP") || fail "DEV_MODE must expose debugCode"
VERIFY=$(post "$IDENTITY_URL/auth/v1/otp/verify" \
  -d "{\"phone_number\":\"$PHONE\",\"code\":\"$CODE\",\"device_fingerprint\":\"$DEVICE\",\"device_name\":\"phase7-e2e\"}")
[ "$(jq -r .nextStep <<<"$VERIFY")" = "NEXT_STEP_SIGNUP_REQUIRED" ] || fail "expected signup route"
FLOW=$(jq -er .flowToken <<<"$VERIFY")
TOKENS=$(post "$IDENTITY_URL/auth/v1/signup" \
  -d "{\"flow_token\":\"$FLOW\",\"nickname\":\"phase7-e2e\",\"device_fingerprint\":\"$DEVICE\",\"device_name\":\"phase7-e2e\"}")

echo "== known-device login"
CODE=$(post "$IDENTITY_URL/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}" | jq -er .debugCode)
LOGIN=$(post "$IDENTITY_URL/auth/v1/otp/verify" \
  -d "{\"phone_number\":\"$PHONE\",\"code\":\"$CODE\",\"device_fingerprint\":\"$DEVICE\"}")
[ "$(jq -r .nextStep <<<"$LOGIN")" = "NEXT_STEP_TOKENS_ISSUED" ] || fail "ACTIVE login did not issue tokens"
REFRESH=$(jq -er .tokens.refreshToken <<<"$LOGIN")

echo "== refresh rotation"
ROTATED=$(post "$IDENTITY_URL/oauth2/v1/token" \
  -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH\"}")
ACCESS=$(jq -er .accessToken <<<"$ROTATED")
NEW_REFRESH=$(jq -er .refreshToken <<<"$ROTATED")
[ "$NEW_REFRESH" != "$REFRESH" ] || fail "refresh token did not rotate"

echo "== create and list post"
CREATED=$(post "$BOARD_URL/board/v1/posts" -H "Authorization: Bearer $ACCESS" \
  -d "{\"title\":\"phase7-$suffix\",\"content\":\"signup login refresh post\"}")
POST_ID=$(jq -er .post.id <<<"$CREATED")
post "$BOARD_URL/board/v1/posts?page_size=100" | jq -e --arg id "$POST_ID" \
  '.posts[] | select(.id == $id)' >/dev/null || fail "created post $POST_ID not returned"

echo "PHASE 7 E2E PASSED: user=$PHONE post=$POST_ID"
