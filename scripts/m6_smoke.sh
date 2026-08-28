#!/usr/bin/env bash
# m6 smoke: phase 6 edge flows (PRD §4.2 items 3–4).
#   A. DORMANT reactivation: dormant user → REACTIVATION_REQUIRED → consent → tokens.
#   B. Unknown device: new fingerprint → DEVICE_VERIFICATION_REQUIRED →
#      wrong month rejected → right month → tokens → board post works.
set -euo pipefail

IDENTITY=${IDENTITY:-http://localhost:8090}
BOARD=${BOARD:-http://localhost:8091}
PHONE="+8210$(date +%s | tail -c 8)" # unique per run (stays under the 5/hour OTP rate limit)
# Same access pattern as m4_smoke.sh (container idsvc-mysql, root:root — PoC only).
MYSQL=(docker exec idsvc-mysql mysql -N -uroot -proot identity)

step() { echo; echo "== $1"; }
fail() { echo "SMOKE FAIL: $1" >&2; exit 1; }

otp_login() { # $1=fingerprint $2=device_name → verify response JSON
  local code
  code=$(curl -sf "$IDENTITY/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}" | jq -r .debugCode)
  curl -sf "$IDENTITY/auth/v1/otp/verify" \
    -d "{\"phone_number\":\"$PHONE\",\"code\":\"$code\",\"device_fingerprint\":\"$1\",\"device_name\":\"$2\"}"
}

step "signup (registers device dev-a)"
V=$(otp_login dev-a m6-primary)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_SIGNUP_REQUIRED" ] || fail "expected signup step"
FLOW=$(jq -r .flowToken <<<"$V")
curl -sf "$IDENTITY/auth/v1/signup" \
  -d "{\"flow_token\":\"$FLOW\",\"nickname\":\"m6user\",\"device_fingerprint\":\"dev-a\",\"device_name\":\"m6-primary\"}" \
  | jq -e '.tokens.accessToken' >/dev/null || fail "signup issued no tokens"
USER_ID=$("${MYSQL[@]}" -e "SELECT user_id FROM user_credentials WHERE identifier='$PHONE'")
echo "user id: $USER_ID"

step "A. force DORMANT via SQL, login → REACTIVATION_REQUIRED"
"${MYSQL[@]}" -e "UPDATE users SET status='DORMANT' WHERE id=$USER_ID"
V=$(otp_login dev-a m6-primary)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_REACTIVATION_REQUIRED" ] || fail "expected reactivation step"
FLOW=$(jq -r .flowToken <<<"$V")

step "A. reactivate without consent → 4xx"
curl -sf "$IDENTITY/auth/v1/reactivate" \
  -d "{\"flow_token\":\"$FLOW\",\"privacy_consent\":false}" >/dev/null && fail "consentless reactivation accepted"

step "A. reactivate with consent → tokens, status ACTIVE"
R=$(curl -sf "$IDENTITY/auth/v1/reactivate" \
  -d "{\"flow_token\":\"$FLOW\",\"privacy_consent\":true,\"device_fingerprint\":\"dev-a\",\"device_name\":\"m6-primary\"}")
[ "$(jq -r '.tokens.accessToken' <<<"$R")" != "null" ] || fail "no tokens after reactivation"
[ "$("${MYSQL[@]}" -e "SELECT status FROM users WHERE id=$USER_ID")" = "ACTIVE" ] || fail "status not ACTIVE"

step "B. login from unknown device dev-b → DEVICE_VERIFICATION_REQUIRED, no tokens"
V=$(otp_login dev-b m6-laptop)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_DEVICE_VERIFICATION_REQUIRED" ] || fail "expected device verification step"
[ "$(jq -r .tokens <<<"$V")" = "null" ] || fail "tokens leaked before device verification"
FLOW=$(jq -r .flowToken <<<"$V")

step "B. wrong creation month → 401"
curl -sf "$IDENTITY/auth/v1/device/verify" \
  -d "{\"flow_token\":\"$FLOW\",\"registration_month\":\"1999-01\",\"device_fingerprint\":\"dev-b\",\"device_name\":\"m6-laptop\"}" \
  >/dev/null && fail "wrong month accepted"

step "B. right creation month → tokens; board accepts them"
MONTH=$("${MYSQL[@]}" -e "SELECT DATE_FORMAT(created_at,'%Y-%m') FROM users WHERE id=$USER_ID")
R=$(curl -sf "$IDENTITY/auth/v1/device/verify" \
  -d "{\"flow_token\":\"$FLOW\",\"registration_month\":\"$MONTH\",\"device_fingerprint\":\"dev-b\",\"device_name\":\"m6-laptop\"}")
AT=$(jq -r '.tokens.accessToken' <<<"$R")
[ "$AT" != "null" ] || fail "no tokens after device verification"
curl -sf "$BOARD/board/v1/posts" -H "Authorization: Bearer $AT" \
  -d '{"title":"m6","content":"edge flows work"}' >/dev/null || fail "board rejected verified-device token"

step "B. dev-b now known: fresh login goes straight to tokens"
V=$(otp_login dev-b m6-laptop)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_TOKENS_ISSUED" ] || fail "re-login from verified device not trusted"

echo; echo "M6 SMOKE PASSED ✅"
