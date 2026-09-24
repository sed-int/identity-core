# identity-service — MSA Identity Provider PoC

Go monorepo for an OIDC-style Identity Provider PoC (phone/OTP auth, RS256 JWT + JWKS, stateless verification by a Board resource server). Full spec: `prd.md`. Solo project by hcho.

## Planning convention

- All plans live in `plan/`, named `YYYY-MM-DD-<topic>.md`.
- Before starting any major piece of work, write (or update) a plan file there; mark its status at the top (📋 planned / 🚧 in progress / ✅ completed) so plans double as a project log.
- Current roadmap: `plan/2026-07-17-build-order.md`.

## Git conventions

### Branch model
- **`dev` is the source of truth.** All work branches off `dev` and merges back into `dev`.
- **`main` is the stable branch.** `dev` is merged into `main` at milestones (M1–M3, NFR completion — see build-order plan). Never commit directly to `main`.
- Every feature or code change gets its own branch, prefixed by type:
  - `feat/<short-kebab-desc>` — new functionality
  - `fix/<short-kebab-desc>` — bug fixes
  - `refactor/<short-kebab-desc>` — behavior-preserving restructuring
  - `chore/<short-kebab-desc>` — tooling, deps, CI, Makefile
  - `docs/<short-kebab-desc>` — PRD, plans, README
  - `test/<short-kebab-desc>` — test-only changes
- Example: `feat/otp-login`, `chore/buf-setup`, `docs/build-order-plan`.
- **Review gate:** work is committed on its branch and left there — merging into `dev` and pushing happen only after hcho reviews and approves. Delete branches after merging into `dev`.

### Commits
- Conventional Commits style, matching the branch prefixes: `feat: add OTP verify endpoint`, `fix: handle expired refresh token family`.
- Keep commits scoped to one logical change; the branch tells the story.
