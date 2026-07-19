#!/usr/bin/env bash
# Phase 4 eventing smoke: user.created flows outbox → Redis Streams → board's
# authors read model, and ListPosts shows the author nickname — all without
# the board ever calling the identity service.
set -euo pipefail

IDP="${IDENTITY_URL:-http://localhost:8090}"
BOARD="${BOARD_URL:-http://localhost:8091}"
PHONE="+8210$(jot -r 1 10000000 99999999 2>/dev/null || shuf -i 10000000-99999999 -n 1)"
NICK="evt-$RANDOM"

fail() { echo "❌ $1"; exit 1; }
ok() { echo "✅ $1"; }

jwt_payload() {
  local p; p=$(cut -d. -f2 <<<"$1" | tr '_-' '/+')
  while [ $(( ${#p} % 4 )) -ne 0 ]; do p="${p}="; done
  base64 -d <<<"$p"
}

# 1. Signup a fresh user (writes the user + outbox row in one tx)
CODE=$(curl -sf "$IDP/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}" | jq -r .debugCode)
FLOW=$(curl -sf "$IDP/auth/v1/otp/verify" -d "{\"phone_number\":\"$PHONE\",\"code\":\"$CODE\"}" | jq -r .flowToken)
ACCESS=$(curl -sf "$IDP/auth/v1/signup" -d "{\"flow_token\":\"$FLOW\",\"nickname\":\"$NICK\"}" | jq -r .tokens.accessToken)
SUB=$(jwt_payload "$ACCESS" | jq -r .sub)
ok "signed up user $SUB (nickname $NICK)"

# 2. Outbox row gets published by the relay (poll up to 10s)
for i in $(seq 1 10); do
  UNPUB=$(docker exec idsvc-mysql mysql -N -uroot -proot identity \
    -e "SELECT COUNT(*) FROM outbox WHERE aggregate_id='$SUB' AND published_at IS NULL;" 2>/dev/null)
  [ "$UNPUB" = "0" ] && break; sleep 1
done
[ "$UNPUB" = "0" ] || fail "outbox row for user $SUB still unpublished after 10s"
ok "outbox row published by relay"

# 3. Stream carries the event
docker exec idsvc-redis redis-cli XLEN events:user | grep -qv '^0$' || fail "events:user stream is empty"
ok "events:user stream has entries"

# 4. Board consumed it into the authors read model (poll up to 10s)
for i in $(seq 1 10); do
  GOT=$(docker exec idsvc-mysql mysql -N -uroot -proot board \
    -e "SELECT nickname FROM authors WHERE user_id='$SUB';" 2>/dev/null)
  [ "$GOT" = "$NICK" ] && break; sleep 1
done
[ "$GOT" = "$NICK" ] || fail "authors read model missing user $SUB (got '$GOT')"
ok "board consumed user.created → authors read model"

# 5. ListPosts resolves the nickname from the LOCAL read model
curl -sf "$BOARD/board/v1/posts" -H "Authorization: Bearer $ACCESS" \
  -d '{"title":"eventing works","content":"nickname via read model"}' >/dev/null
LISTED=$(curl -sf "$BOARD/board/v1/posts" | jq -r --arg sub "$SUB" \
  '.posts[] | select(.authorId == $sub) | .authorNickname' | head -1)
[ "$LISTED" = "$NICK" ] || fail "ListPosts nickname '$LISTED' != '$NICK'"
ok "ListPosts shows author nickname from board's local read model"

echo
echo "🎉 Phase 4 eventing smoke passed"
