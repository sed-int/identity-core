# Plan: Phase 5 — Frontend (React + Vite, M3)

> Status: 📋 Planned

Goal (build-order Phase 5): the full PRD §6 scenario clickable in a browser —
OTP login → signup completion → post write/list, with token refresh on 401.
Demo client, not a product.

## Branch: `feat/frontend`

## Decisions

- **Approach:** minimal SPA, no router — screen switching on auth state.
  TypeScript, plain CSS, no deps beyond React. `web/` at the repo root.
- **Serving: both** dev server and compose.
  - Dev: `npm run dev` on :5173; Vite proxy sends `/auth`, `/oauth2` →
    :8090 and `/board` → :8091. No CORS work on the backend.
  - Compose: multi-stage Dockerfile (node build → nginx). nginx serves the
    static bundle on host :5174 and proxies the same paths to
    `identity:8090` / `board:8091`. Added to `make run`.
  - Tokens carry `iss: http://localhost:8090`; board validates issuer, not
    caller origin, so proxying changes nothing.
- **Token storage:** access token + expiry in memory; refresh token in
  localStorage. localStorage is XSS-readable — acceptable for the PoC demo
  only, not a production pattern.

## Screens & flow (proto contract, `identity/v1` + `board/v1`)

1. **Login** — phone input → `POST /auth/v1/otp/request` → code input.
   Dev mode echoes `debug_code`; prefill it for the demo.
   Verify via `POST /auth/v1/otp/verify` with `device_fingerprint`
   (random UUID persisted in localStorage) and `device_name` (userAgent).
   Route on `next_step`:
   - `TOKENS_ISSUED` → Board
   - `SIGNUP_REQUIRED` → Signup
   - `REACTIVATION_REQUIRED` / `DEVICE_VERIFICATION_REQUIRED` → error banner
     ("phase 6 — not supported in this demo")
2. **Signup** — nickname form → `POST /auth/v1/signup` with `flow_token` →
   Board.
3. **Board** — `GET /board/v1/posts` (paged via `page_after`, "load more"),
   write form (title/content), `author_nickname` shown (proves the phase 4
   read model), logout (drop tokens).

## Token handling (`auth.ts` + `api.ts`)

- Fetch wrapper attaches `Authorization: Bearer <access>`.
- On 401: `POST /oauth2/v1/token` (`grant_type=refresh_token`) → retry the
  original request once. Refresh failure → drop tokens → Login.
- Single in-flight refresh promise — parallel 401s must not double-rotate
  (RTR reuse detection would invalidate the whole family).
- On page load with a stored refresh token: refresh immediately and enter
  Board (session persistence; also exercises RTR visibly).

## Structure

```
web/
  src/
    api.ts        # fetch wrapper: attach token, 401 → refresh → retry once
    auth.ts       # token store (memory + localStorage), device fingerprint
    App.tsx       # screen switch on auth state
    screens/
      Login.tsx
      Signup.tsx
      Board.tsx
    main.tsx
    index.css
  Dockerfile      # node build → nginx
  nginx.conf      # static + /auth,/oauth2,/board proxy
  vite.config.ts  # dev proxy
```

## Verification (Milestone M3)

- Manual: full §6 scenario clickable via dev server AND via compose (:5174).
- One vitest covering `api.ts` 401-refresh-retry + single-flight refresh —
  the only nontrivial logic. Screens stay untested (demo client).
- No `m3_smoke.sh`: M3 is "clickable in a browser"; m1/m2 already cover the
  API path.

## Out of scope

- Reactivation / device-verification UIs (Phase 6).
- Routing, state libs, component libs, i18n, accessibility polish.
