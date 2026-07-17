#!/usr/bin/env bash
# M2 milestone smoke test (PRD §6/§8): the Board service verifies IdP tokens
# STATELESSLY (JWKS only — it has no identity DB access by construction) and
# authorizes post creation. Run against the docker compose stack.
set -euo pipefail

IDP="${IDENTITY_URL:-http://localhost:8090}"
BOARD="${BOARD_URL:-http://localhost:8091}"
PHONE="+8210$(jot -r 1 10000000 99999999 2>/dev/null || shuf -i 10000000-99999999 -n 1)"

fail() { echo "❌ $1"; exit 1; }
ok() { echo "✅ $1"; }

# 1. Full login on the IdP (signup path)
CODE=$(curl -sf "$IDP/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}" | jq -r .debugCode)
FLOW=$(curl -sf "$IDP/auth/v1/otp/verify" -d "{\"phone_number\":\"$PHONE\",\"code\":\"$CODE\"}" | jq -r .flowToken)
ACCESS=$(curl -sf "$IDP/auth/v1/signup" -d "{\"flow_token\":\"$FLOW\",\"nickname\":\"m2-author\"}" | jq -r .tokens.accessToken)
[ -n "$ACCESS" ] && [ "$ACCESS" != "null" ] || fail "could not obtain access token from IdP"
jwt_payload() { # decode base64url JWT payload with re-added padding
  local p; p=$(cut -d. -f2 <<<"$1" | tr '_-' '/+')
  while [ $(( ${#p} % 4 )) -ne 0 ]; do p="${p}="; done
  base64 -d <<<"$p"
}
SUB=$(jwt_payload "$ACCESS" | jq -r .sub)
ok "IdP issued access token for user $SUB"

# 2. Board rejects unauthenticated / garbage-token writes
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BOARD/board/v1/posts" -d '{"title":"t","content":"c"}')
[ "$HTTP" = "401" ] || fail "no-token create returned $HTTP, want 401"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BOARD/board/v1/posts" \
  -H "Authorization: Bearer garbage.token.here" -d '{"title":"t","content":"c"}')
[ "$HTTP" = "401" ] || fail "garbage-token create returned $HTTP, want 401"
ok "board rejects missing/invalid tokens (401)"

# 3. Board accepts the IdP token — STATELESS verification via JWKS
POST=$(curl -sf "$BOARD/board/v1/posts" \
  -H "Authorization: Bearer $ACCESS" \
  -d '{"title":"stateless works","content":"verified via JWKS, no identity DB touched"}')
AUTHOR=$(jq -r .post.authorId <<<"$POST")
[ "$AUTHOR" = "$SUB" ] || fail "author ($AUTHOR) != token sub ($SUB): $POST"
ok "post created; author taken from verified token sub"

# 4. Public list shows the post
curl -sf "$BOARD/board/v1/posts" | jq -e --arg id "$(jq -r .post.id <<<"$POST")" \
  '.posts[] | select(.id == $id)' >/dev/null || fail "created post missing from list"
ok "public list returns the post"

echo
echo "🎉 M2 smoke passed — stateless verification proven"
