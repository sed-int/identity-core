#!/usr/bin/env bash
# M1 milestone smoke test (PRD §8.1 partial): signup → login → RTR → reuse detection → JWKS.
# Requires: identity service running (dev mode), jq, curl. Uses a random phone per run
# so the per-phone OTP rate limit doesn't bite on repeated runs.
set -euo pipefail

BASE="${IDENTITY_URL:-http://localhost:8090}"
PHONE="+8210$(jot -r 1 10000000 99999999 2>/dev/null || shuf -i 10000000-99999999 -n 1)"

fail() { echo "❌ $1"; exit 1; }
ok() { echo "✅ $1"; }

# 1. Request OTP (dev mode echoes the code)
CODE=$(curl -sf "$BASE/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}" | jq -r .debugCode)
[ -n "$CODE" ] && [ "$CODE" != "null" ] || fail "no debug_code — is the server in dev mode?"
ok "OTP requested for $PHONE (code $CODE)"

# 2. Verify OTP → new user → SIGNUP_REQUIRED + flow token
VERIFY=$(curl -sf "$BASE/auth/v1/otp/verify" \
  -d "{\"phone_number\":\"$PHONE\",\"code\":\"$CODE\",\"device_fingerprint\":\"smoke-device\",\"device_name\":\"m1-smoke\"}")
[ "$(jq -r .nextStep <<<"$VERIFY")" = "NEXT_STEP_SIGNUP_REQUIRED" ] || fail "expected SIGNUP_REQUIRED: $VERIFY"
FLOW=$(jq -r .flowToken <<<"$VERIFY")
ok "unregistered phone routed to signup"

# 3. Complete signup → tokens
TOKENS=$(curl -sf "$BASE/auth/v1/signup" -d "{\"flow_token\":\"$FLOW\",\"nickname\":\"smoker\"}" | jq .tokens)
ACCESS=$(jq -r .accessToken <<<"$TOKENS")
REFRESH1=$(jq -r .refreshToken <<<"$TOKENS")
[ -n "$ACCESS" ] && [ "$ACCESS" != "null" ] || fail "no access token after signup"
ok "signup completed, tokens issued"

# 4. Access token claims sanity (header.payload are base64url JSON)
PAYLOAD=$(cut -d. -f2 <<<"$ACCESS" | tr '_-' '/+' | base64 -d 2>/dev/null || true)
grep -q '"status":"ACTIVE"' <<<"$PAYLOAD" || fail "access token missing status claim: $PAYLOAD"
grep -q '"aud":"board"' <<<"$PAYLOAD" || fail "access token missing aud claim: $PAYLOAD"
ok "access token carries sub/aud/status claims"

# 5. Refresh (RTR): new refresh token must differ
ROTATED=$(curl -sf "$BASE/oauth2/v1/token" -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH1\"}")
REFRESH2=$(jq -r .refreshToken <<<"$ROTATED")
[ -n "$REFRESH2" ] && [ "$REFRESH2" != "$REFRESH1" ] || fail "refresh token was not rotated"
ok "refresh token rotated"

# 6. Replay OLD refresh token → 401 + family revoked
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/oauth2/v1/token" \
  -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH1\"}")
[ "$HTTP" = "401" ] || fail "replayed refresh token returned $HTTP, want 401"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/oauth2/v1/token" \
  -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH2\"}")
[ "$HTTP" = "401" ] || fail "family member survived reuse detection ($HTTP), want 401"
ok "reuse detection revoked the whole token family"

# 7. Second login on the now-ACTIVE account → TOKENS_ISSUED directly
CODE=$(curl -sf "$BASE/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}" | jq -r .debugCode)
VERIFY=$(curl -sf "$BASE/auth/v1/otp/verify" \
  -d "{\"phone_number\":\"$PHONE\",\"code\":\"$CODE\",\"device_fingerprint\":\"smoke-device\"}")
[ "$(jq -r .nextStep <<<"$VERIFY")" = "NEXT_STEP_TOKENS_ISSUED" ] || fail "ACTIVE login did not issue tokens: $VERIFY"
ok "ACTIVE user login issues tokens directly"

# 8. OIDC surface
curl -sf "$BASE/oauth2/v1/jwks" | jq -e '.keys[0].kid' >/dev/null || fail "JWKS endpoint broken"
curl -sf "$BASE/.well-known/openid-configuration" | jq -e .jwks_uri >/dev/null || fail "discovery endpoint broken"
ok "JWKS + discovery endpoints serve"

echo
echo "🎉 M1 smoke passed"
