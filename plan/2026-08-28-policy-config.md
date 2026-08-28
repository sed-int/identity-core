# Plan: Env-var policy config layer

> Status: 📋 planned — backlog (agreed 2026-08-28, after phase 6 review discussion)

Promote hardcoded auth-policy constants to `config.Config` fields (env vars with current values as defaults, same pattern as `DEV_MODE`). Compose then carries policy per environment. No DB-backed runtime policy table — redeploy-to-change is fine for the PoC.

## Knobs

| Env var (proposed) | Today | Where |
| :--- | :--- | :--- |
| `OTP_CODE_TTL` | 3m | `internal/otp/store.go` `CodeTTL` |
| `OTP_MAX_ATTEMPTS` | 5 | `internal/otp/store.go` `MaxAttempts` |
| `OTP_RATE_LIMIT_MAX` / `OTP_RATE_LIMIT_WINDOW` | 5 / 1h | `internal/otp/store.go` |
| `DEVICE_VERIFY_MAX_ATTEMPTS` / `DEVICE_VERIFY_WINDOW` | 3 / 10m | `internal/devverify/limiter.go` |
| `DEVICE_GATE_ENABLED` | always on | `internal/usecase/auth.go` VerifyOtp ACTIVE branch |
| `ACCESS_TOKEN_TTL` | 15m | `internal/token` |
| `REFRESH_TOKEN_TTL` | 14d | `internal/token` / `internal/rtr` |
| `FLOW_TOKEN_TTL` | 10m | `internal/token` |
| `DORMANT_AFTER` | — (no sweep yet) | see [dormancy-sweep plan](2026-08-28-dormancy-sweep.md) |

## Notes

- Constants become fields on the store/limiter/issuer structs, injected from `config.Config` in `app.New`; tests keep current values as defaults.
- Response messages that echo TTLs (`expires_in_seconds`, `expires_in`) must read the configured value, not the old consts.
- Branch: `chore/policy-config`. ~1h mechanical work.
