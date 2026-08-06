# Phase 5 Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> Status: ✅ Completed 2026-08-06 — spec: `plan/2026-07-27-phase5-frontend.md`

**Goal:** PRD §6 scenario clickable in a browser — OTP login → signup → post write/list with 401-driven token refresh (Milestone M3).

**Architecture:** Minimal React SPA (no router) in `web/` at the repo root. Screen switching on auth state in `App.tsx`. One fetch wrapper (`api.ts`) attaches the access token and does single-flight refresh-and-retry on 401. Served two ways: Vite dev server with proxy (`:5173`), and nginx container in compose (`:5174`).

**Tech Stack:** React 19, Vite 6, TypeScript, vitest. No runtime deps beyond react/react-dom. Plain CSS.

## Global Constraints

- Branch: `feat/frontend` (created off `docs/phase5-frontend-plan` so the plan files ride along; review gate — no merge to dev, no push).
- No new runtime dependencies beyond `react` + `react-dom`. No router, no state lib, no component lib.
- Gateway JSON: **responses are protojson camelCase** (`debugCode`, `nextStep`, `flowToken`, `tokens.accessToken`, `authorNickname`, `nextPageAfter`); **request bodies use snake_case** (`phone_number`, `flow_token`, `grant_type` — protojson accepts both; smoke scripts already send snake_case). int64 fields (`id`, `page_after`, `nextPageAfter`) serialize as **strings**. Enums serialize as strings (`"NEXT_STEP_SIGNUP_REQUIRED"`).
- Backends: identity HTTP `:8090`, board HTTP `:8091`. No CORS anywhere — all browser traffic goes through a same-origin proxy (Vite dev proxy or nginx).
- Refresh token in localStorage is a **PoC-only** pattern (XSS-readable); access token memory-only. Access-token expiry is NOT tracked client-side — 401 drives refresh (YAGNI; simpler than clock math).
- Commits: Conventional Commits, one logical change each, `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` trailer.
- Manual verification needs the stack up: `make run && make migrate-up` from the repo root.

---

### Task 1: Scaffold Vite app

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx` (stub), `web/src/index.css`
- Modify: `.gitignore` (create if absent)

**Interfaces:**
- Produces: buildable Vite+React+TS app; dev proxy `/auth`,`/oauth2` → `:8090`, `/board` → `:8091`. Later tasks replace `App.tsx` and add files under `web/src/`.

- [ ] **Step 1: Create branch**

```bash
git checkout docs/phase5-frontend-plan && git checkout -b feat/frontend
```

- [ ] **Step 2: Write config files**

`web/package.json`:

```json
{
  "name": "identity-web",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "test": "vitest run",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.3.4",
    "typescript": "^5.7.0",
    "vite": "^6.0.0",
    "vitest": "^3.0.0"
  }
}
```

`web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "skipLibCheck": true,
    "noEmit": true,
    "types": ["vite/client"]
  },
  "include": ["src"]
}
```

`web/vite.config.ts`:

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/auth': 'http://localhost:8090',
      '/oauth2': 'http://localhost:8090',
      '/board': 'http://localhost:8091',
    },
  },
})
```

- [ ] **Step 3: Write app shell**

`web/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>identity-service demo</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

`web/src/main.tsx`:

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

`web/src/App.tsx` (stub — replaced in Task 4):

```tsx
export default function App() {
  return <main className="card">identity-service demo — scaffold OK</main>
}
```

`web/src/index.css`:

```css
:root {
  font-family: system-ui, sans-serif;
  color-scheme: light dark;
}
body {
  margin: 0;
  display: flex;
  justify-content: center;
  padding: 2rem 1rem;
}
#root { width: 100%; max-width: 40rem; }
.card {
  border: 1px solid color-mix(in srgb, currentColor 25%, transparent);
  border-radius: 8px;
  padding: 1rem;
  margin-bottom: 1rem;
}
.error { color: #d33; margin: 0.5rem 0; }
form { display: flex; flex-direction: column; gap: 0.5rem; }
input, textarea, button { font: inherit; padding: 0.5rem; }
button { cursor: pointer; }
button:disabled { cursor: wait; opacity: 0.6; }
.post-meta { font-size: 0.85rem; opacity: 0.7; }
.row { display: flex; justify-content: space-between; align-items: center; }
```

`.gitignore` at repo root — append (create if absent):

```
web/node_modules/
web/dist/
```

- [ ] **Step 4: Install and verify build**

```bash
cd web && npm install && npm run build
```

Expected: `tsc` silent, `vite build` prints `✓ built in …` with a `dist/` output. Also spot-check dev server: `npm run dev`, open http://localhost:5173, page shows "identity-service demo — scaffold OK", Ctrl-C.

- [ ] **Step 5: Commit**

```bash
git add web .gitignore
git commit -m "feat: scaffold web demo client (Vite + React + TS)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Token store (`auth.ts`)

**Files:**
- Create: `web/src/auth.ts`

**Interfaces:**
- Produces (consumed by `api.ts` and screens):
  - `interface TokenPair { accessToken: string; idToken: string; refreshToken: string; tokenType: string; expiresIn: number }`
  - `setTokens(t: TokenPair): void` — access token to memory, refresh token to localStorage
  - `getAccessToken(): string | null`
  - `getRefreshToken(): string | null`
  - `clearTokens(): void`
  - `deviceFingerprint(): string` — random UUID, persisted in localStorage
  - `deviceName(): string`

- [ ] **Step 1: Write `web/src/auth.ts`**

```ts
// Token store. Access token lives in memory only; the refresh token sits in
// localStorage — XSS-readable, acceptable for this PoC demo, not production.

const RT_KEY = 'idsvc.refresh_token'
const DEVICE_KEY = 'idsvc.device_id'

let accessToken: string | null = null

// Gateway (protojson) camelCase response shape.
export interface TokenPair {
  accessToken: string
  idToken: string
  refreshToken: string
  tokenType: string
  expiresIn: number
}

export function setTokens(t: TokenPair): void {
  accessToken = t.accessToken
  localStorage.setItem(RT_KEY, t.refreshToken)
}

export function getAccessToken(): string | null {
  return accessToken
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(RT_KEY)
}

export function clearTokens(): void {
  accessToken = null
  localStorage.removeItem(RT_KEY)
}

export function deviceFingerprint(): string {
  let id = localStorage.getItem(DEVICE_KEY)
  if (!id) {
    id = crypto.randomUUID()
    localStorage.setItem(DEVICE_KEY, id)
  }
  return id
}

export function deviceName(): string {
  return navigator.userAgent.slice(0, 100)
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: silent exit 0.

- [ ] **Step 3: Commit**

```bash
git add web/src/auth.ts
git commit -m "feat: browser token store (memory access token, localStorage refresh token)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: API client with 401 refresh-retry (`api.ts`) — TDD

**Files:**
- Create: `web/src/api.test.ts`, then `web/src/api.ts`
- Test: `web/src/api.test.ts`

**Interfaces:**
- Consumes: everything from `./auth` (Task 2).
- Produces (consumed by screens):
  - `class ApiError extends Error { status: number }`
  - `refreshSession(): Promise<boolean>` — single-flight; on failure clears tokens
  - `requestOtp(phoneNumber: string): Promise<RequestOtpResponse>` where `RequestOtpResponse = { expiresInSeconds: number; debugCode?: string }`
  - `verifyOtp(phoneNumber: string, code: string): Promise<VerifyOtpResponse>` where `VerifyOtpResponse = { nextStep: NextStep; tokens?: TokenPair; flowToken?: string }` and `NextStep` is the proto enum string union
  - `completeSignup(flowToken: string, nickname: string): Promise<{ tokens: TokenPair }>`
  - `listPosts(pageAfter?: string): Promise<ListPostsResponse>` where `ListPostsResponse = { posts?: Post[]; nextPageAfter?: string }` and `Post = { id: string; authorId: string; title: string; content: string; createdAt: string; authorNickname?: string }`
  - `createPost(title: string, content: string): Promise<{ post: Post }>`

- [ ] **Step 1: Write the failing test**

`web/src/api.test.ts`:

```ts
import { beforeEach, expect, test, vi } from 'vitest'
import { listPosts } from './api'
import { clearTokens, getRefreshToken, setTokens, type TokenPair } from './auth'

// node env: minimal localStorage stub (auth.ts only touches it inside functions)
const store = new Map<string, string>()
vi.stubGlobal('localStorage', {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => void store.set(k, v),
  removeItem: (k: string) => void store.delete(k),
})

const tokens = (n: number): TokenPair => ({
  accessToken: `at-${n}`,
  idToken: 'id',
  refreshToken: `rt-${n}`,
  tokenType: 'Bearer',
  expiresIn: 900,
})

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  store.clear()
  clearTokens()
  setTokens(tokens(1))
})

test('401 triggers refresh then retries the original request once', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response('expired', { status: 401 })) // original call
    .mockResolvedValueOnce(json(tokens(2))) // refresh
    .mockResolvedValueOnce(json({ posts: [] })) // retry
  vi.stubGlobal('fetch', fetchMock)

  await expect(listPosts()).resolves.toEqual({ posts: [] })
  expect(fetchMock).toHaveBeenCalledTimes(3)
  expect(getRefreshToken()).toBe('rt-2') // RTR: rotated refresh token stored
  const retryHeaders = new Headers(fetchMock.mock.calls[2][1].headers)
  expect(retryHeaders.get('Authorization')).toBe('Bearer at-2')
})

test('parallel 401s share a single refresh (single-flight)', async () => {
  let refreshCalls = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).startsWith('/oauth2/')) {
        refreshCalls++
        return json(tokens(2))
      }
      const auth = new Headers(init?.headers).get('Authorization')
      return auth === 'Bearer at-2' ? json({ posts: [] }) : new Response('expired', { status: 401 })
    }),
  )

  await Promise.all([listPosts(), listPosts()])
  expect(refreshCalls).toBe(1) // a second rotation would trip RTR reuse detection
})

test('failed refresh clears tokens and surfaces the original 401', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: RequestInfo | URL) =>
      String(url).startsWith('/oauth2/')
        ? new Response('invalid_grant', { status: 400 })
        : new Response('expired', { status: 401 }),
    ),
  )

  await expect(listPosts()).rejects.toMatchObject({ status: 401 })
  expect(getRefreshToken()).toBeNull()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/api.test.ts`
Expected: FAIL — `Cannot find module './api'` (or unresolved import).

- [ ] **Step 3: Write `web/src/api.ts`**

```ts
import {
  clearTokens,
  deviceFingerprint,
  deviceName,
  getAccessToken,
  getRefreshToken,
  setTokens,
  type TokenPair,
} from './auth'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

let refreshInFlight: Promise<boolean> | null = null

// Single-flight: concurrent 401s must share one refresh — a second concurrent
// rotation would trip RTR reuse detection and invalidate the token family.
export function refreshSession(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = doRefresh().finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

async function doRefresh(): Promise<boolean> {
  const rt = getRefreshToken()
  if (!rt) return false
  const res = await fetch('/oauth2/v1/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ grant_type: 'refresh_token', refresh_token: rt }),
  })
  if (!res.ok) {
    clearTokens()
    return false
  }
  setTokens((await res.json()) as TokenPair)
  return true
}

async function apiFetch<T>(path: string, init: RequestInit = {}, retried = false): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  const at = getAccessToken()
  if (at) headers.set('Authorization', `Bearer ${at}`)
  const res = await fetch(path, { ...init, headers })
  // Refresh only when a session exists — a 401 on e.g. a wrong OTP code must
  // not trigger a refresh attempt. Failed refresh falls through to ApiError.
  if (res.status === 401 && !retried && getRefreshToken()) {
    if (await refreshSession()) return apiFetch<T>(path, init, true)
  }
  if (!res.ok) throw new ApiError(res.status, await res.text())
  return res.json() as Promise<T>
}

// ---- identity.v1 ----

export interface RequestOtpResponse {
  expiresInSeconds: number
  debugCode?: string // dev mode only
}

export type NextStep =
  | 'NEXT_STEP_UNSPECIFIED'
  | 'NEXT_STEP_TOKENS_ISSUED'
  | 'NEXT_STEP_SIGNUP_REQUIRED'
  | 'NEXT_STEP_REACTIVATION_REQUIRED'
  | 'NEXT_STEP_DEVICE_VERIFICATION_REQUIRED'

export interface VerifyOtpResponse {
  nextStep: NextStep
  tokens?: TokenPair
  flowToken?: string
}

export const requestOtp = (phoneNumber: string) =>
  apiFetch<RequestOtpResponse>('/auth/v1/otp/request', {
    method: 'POST',
    body: JSON.stringify({ phone_number: phoneNumber }),
  })

export const verifyOtp = (phoneNumber: string, code: string) =>
  apiFetch<VerifyOtpResponse>('/auth/v1/otp/verify', {
    method: 'POST',
    body: JSON.stringify({
      phone_number: phoneNumber,
      code,
      device_fingerprint: deviceFingerprint(),
      device_name: deviceName(),
    }),
  })

export const completeSignup = (flowToken: string, nickname: string) =>
  apiFetch<{ tokens: TokenPair }>('/auth/v1/signup', {
    method: 'POST',
    body: JSON.stringify({ flow_token: flowToken, nickname }),
  })

// ---- board.v1 ----

export interface Post {
  id: string // int64 → string in protojson
  authorId: string
  title: string
  content: string
  createdAt: string
  authorNickname?: string
}

export interface ListPostsResponse {
  posts?: Post[] // protojson omits empty repeated fields
  nextPageAfter?: string // "0" or absent = no more pages
}

export const listPosts = (pageAfter?: string) =>
  apiFetch<ListPostsResponse>(
    `/board/v1/posts${pageAfter && pageAfter !== '0' ? `?page_after=${pageAfter}` : ''}`,
  )

export const createPost = (title: string, content: string) =>
  apiFetch<{ post: Post }>('/board/v1/posts', {
    method: 'POST',
    body: JSON.stringify({ title, content }),
  })
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/api.test.ts`
Expected: 3 passed. Also `npx tsc --noEmit` silent.

- [ ] **Step 5: Commit**

```bash
git add web/src/api.ts web/src/api.test.ts
git commit -m "feat: API client with single-flight 401 refresh-retry

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Login screen + App state routing + session restore

**Files:**
- Create: `web/src/screens/Login.tsx`
- Modify: `web/src/App.tsx` (replace stub)

**Interfaces:**
- Consumes: `requestOtp`, `verifyOtp`, `refreshSession`, `ApiError` from `../api`; `setTokens`, `getRefreshToken` from `../auth`.
- Produces: `Login` props `{ onTokens: () => void; onSignupRequired: (flowToken: string) => void }`. `App` renders `Signup` (Task 5) with `{ flowToken: string; onDone: () => void }` and `Board` (Task 6) with `{ onLogout: () => void }` — build stubs for both inline now, replaced by their tasks.

- [ ] **Step 1: Write `web/src/screens/Login.tsx`**

```tsx
import { useState, type FormEvent } from 'react'
import { ApiError, requestOtp, verifyOtp } from '../api'
import { setTokens } from '../auth'

interface Props {
  onTokens: () => void
  onSignupRequired: (flowToken: string) => void
}

export default function Login({ onTokens, onSignupRequired }: Props) {
  const [phone, setPhone] = useState('+8210')
  const [code, setCode] = useState('')
  const [stage, setStage] = useState<'phone' | 'code'>('phone')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submitPhone(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await requestOtp(phone)
      setCode(res.debugCode ?? '') // dev mode echoes the OTP — prefill for the demo
      setStage('code')
    } catch (err) {
      setError(String(err))
    } finally {
      setBusy(false)
    }
  }

  async function submitCode(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await verifyOtp(phone, code)
      switch (res.nextStep) {
        case 'NEXT_STEP_TOKENS_ISSUED':
          setTokens(res.tokens!)
          onTokens()
          break
        case 'NEXT_STEP_SIGNUP_REQUIRED':
          onSignupRequired(res.flowToken!)
          break
        default:
          setError(`${res.nextStep}: phase 6 flow — not supported in this demo`)
      }
    } catch (err) {
      setError(err instanceof ApiError && err.status === 401 ? 'wrong or expired code' : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="card">
      <h1>Login</h1>
      {stage === 'phone' ? (
        <form onSubmit={submitPhone}>
          <label htmlFor="phone">Phone number (E.164)</label>
          <input id="phone" value={phone} onChange={(e) => setPhone(e.target.value)} required />
          <button disabled={busy}>Send OTP</button>
        </form>
      ) : (
        <form onSubmit={submitCode}>
          <label htmlFor="code">OTP code (prefilled from dev mode)</label>
          <input id="code" value={code} onChange={(e) => setCode(e.target.value)} required />
          <button disabled={busy}>Verify</button>
          <button type="button" onClick={() => setStage('phone')}>Back</button>
        </form>
      )}
      {error && <p className="error">{error}</p>}
    </main>
  )
}
```

- [ ] **Step 2: Replace `web/src/App.tsx`**

Includes temporary Signup/Board stubs — Tasks 5 and 6 replace them with imports from `./screens/`.

```tsx
import { useEffect, useState } from 'react'
import { refreshSession } from './api'
import { getRefreshToken } from './auth'
import Login from './screens/Login'

type Screen = 'loading' | 'login' | 'signup' | 'board'

// Task 5 replaces this stub with: import Signup from './screens/Signup'
function Signup(_: { flowToken: string; onDone: () => void }) {
  return <main className="card">signup — Task 5</main>
}
// Task 6 replaces this stub with: import Board from './screens/Board'
function Board(_: { onLogout: () => void }) {
  return <main className="card">board — Task 6</main>
}

export default function App() {
  // A stored refresh token means a previous session: try to restore it (also
  // demonstrates RTR — the stored token is rotated on every restore).
  const [screen, setScreen] = useState<Screen>(getRefreshToken() ? 'loading' : 'login')
  const [flowToken, setFlowToken] = useState('')

  useEffect(() => {
    if (screen !== 'loading') return
    refreshSession().then((ok) => setScreen(ok ? 'board' : 'login'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  switch (screen) {
    case 'loading':
      return <main className="card">restoring session…</main>
    case 'login':
      return (
        <Login
          onTokens={() => setScreen('board')}
          onSignupRequired={(ft) => {
            setFlowToken(ft)
            setScreen('signup')
          }}
        />
      )
    case 'signup':
      return <Signup flowToken={flowToken} onDone={() => setScreen('board')} />
    case 'board':
      return <Board onLogout={() => setScreen('login')} />
  }
}
```

- [ ] **Step 3: Verify against the running stack**

```bash
make run && make migrate-up   # repo root — skip if already up
cd web && npm run dev
```

Open http://localhost:5173. Enter a fresh phone number (e.g. `+821011112222`) → Send OTP → code prefilled → Verify → "signup — Task 5" stub appears (SIGNUP_REQUIRED path). Typecheck: `npx tsc --noEmit`.

- [ ] **Step 4: Commit**

```bash
git add web/src/App.tsx web/src/screens/Login.tsx
git commit -m "feat: OTP login screen with next_step routing and session restore

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Signup screen

**Files:**
- Create: `web/src/screens/Signup.tsx`
- Modify: `web/src/App.tsx` (swap stub for import)

**Interfaces:**
- Consumes: `completeSignup` from `../api`; `setTokens` from `../auth`.
- Produces: `Signup` props `{ flowToken: string; onDone: () => void }` (exact shape App already passes).

- [ ] **Step 1: Write `web/src/screens/Signup.tsx`**

```tsx
import { useState, type FormEvent } from 'react'
import { completeSignup } from '../api'
import { setTokens } from '../auth'

interface Props {
  flowToken: string
  onDone: () => void
}

export default function Signup({ flowToken, onDone }: Props) {
  const [nickname, setNickname] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await completeSignup(flowToken, nickname)
      setTokens(res.tokens)
      onDone()
    } catch (err) {
      setError(String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="card">
      <h1>Complete signup</h1>
      <form onSubmit={submit}>
        <label htmlFor="nickname">Nickname</label>
        <input id="nickname" value={nickname} onChange={(e) => setNickname(e.target.value)} required />
        <button disabled={busy}>Sign up</button>
      </form>
      {error && <p className="error">{error}</p>}
    </main>
  )
}
```

- [ ] **Step 2: Swap the stub in `web/src/App.tsx`**

Delete the inline `function Signup(...)` stub and add to the imports:

```tsx
import Signup from './screens/Signup'
```

- [ ] **Step 3: Verify against the running stack**

Dev server still running. Fresh phone number → OTP → Verify → nickname form → Sign up → "board — Task 6" stub appears. Reload the page → "restoring session…" then the board stub (session restore via RTR works). `npx tsc --noEmit` silent.

- [ ] **Step 4: Commit**

```bash
git add web/src/App.tsx web/src/screens/Signup.tsx
git commit -m "feat: signup completion screen (flow_token -> tokens)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Board screen (list, pagination, write, logout)

**Files:**
- Create: `web/src/screens/Board.tsx`
- Modify: `web/src/App.tsx` (swap stub for import)

**Interfaces:**
- Consumes: `listPosts`, `createPost`, `ApiError`, type `Post` from `../api`; `clearTokens` from `../auth`.
- Produces: `Board` props `{ onLogout: () => void }`.

- [ ] **Step 1: Write `web/src/screens/Board.tsx`**

```tsx
import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, createPost, listPosts, type Post } from '../api'
import { clearTokens } from '../auth'

interface Props {
  onLogout: () => void
}

export default function Board({ onLogout }: Props) {
  const [posts, setPosts] = useState<Post[]>([])
  const [nextPageAfter, setNextPageAfter] = useState('0') // "0" = no more pages
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  function fail(err: unknown) {
    // A 401 here means refresh already failed (api.ts retried once) — log out.
    if (err instanceof ApiError && err.status === 401) {
      clearTokens()
      onLogout()
    } else {
      setError(String(err))
    }
  }

  async function load(pageAfter?: string) {
    try {
      const res = await listPosts(pageAfter)
      setPosts((prev) => (pageAfter ? [...prev, ...(res.posts ?? [])] : (res.posts ?? [])))
      setNextPageAfter(res.nextPageAfter ?? '0')
    } catch (err) {
      fail(err)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await createPost(title, content)
      setTitle('')
      setContent('')
      await load() // reload first page so the new post shows on top
    } catch (err) {
      fail(err)
    } finally {
      setBusy(false)
    }
  }

  function logout() {
    clearTokens()
    onLogout()
  }

  return (
    <main>
      <div className="card row">
        <h1>Board</h1>
        <button onClick={logout}>Logout</button>
      </div>
      <form className="card" onSubmit={submit}>
        <input
          placeholder="Title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
        />
        <textarea
          placeholder="Content"
          value={content}
          onChange={(e) => setContent(e.target.value)}
          rows={3}
          required
        />
        <button disabled={busy}>Post</button>
      </form>
      {error && <p className="error">{error}</p>}
      {posts.map((p) => (
        <article className="card" key={p.id}>
          <h2>{p.title}</h2>
          <p>{p.content}</p>
          <p className="post-meta">
            {p.authorNickname || p.authorId} · {new Date(p.createdAt).toLocaleString()}
          </p>
        </article>
      ))}
      {nextPageAfter !== '0' && (
        <button onClick={() => load(nextPageAfter)}>Load more</button>
      )}
    </main>
  )
}
```

- [ ] **Step 2: Swap the stub in `web/src/App.tsx`**

Delete the inline `function Board(...)` stub and add to the imports:

```tsx
import Board from './screens/Board'
```

- [ ] **Step 3: Verify — full M3 scenario on the dev server**

Full pass: fresh phone → OTP (prefilled) → signup nickname → board renders → write a post → post appears with **nickname** (proves the phase 4 read model) → reload page → session restored straight to board → logout → back at login. `npx tsc --noEmit` and `npm run test` still green.

- [ ] **Step 4: Commit**

```bash
git add web/src/App.tsx web/src/screens/Board.tsx
git commit -m "feat: board screen with post list, pagination, write form, logout

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: Compose serving (Dockerfile + nginx) and Makefile targets

**Files:**
- Create: `web/Dockerfile`, `web/nginx.conf`, `web/.dockerignore`
- Modify: `docker-compose.yml` (add `web` service), `Makefile` (add `web-dev`, `web-test`)

**Interfaces:**
- Consumes: the built app from Tasks 1–6; compose services `identity` (:8090) and `board` (:8091).
- Produces: demo at http://localhost:5174 via `make run`; `make web-dev` / `make web-test` shortcuts.

- [ ] **Step 1: Write `web/Dockerfile`**

```dockerfile
FROM node:22-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
```

- [ ] **Step 2: Write `web/nginx.conf`**

Same-origin proxying, mirroring the Vite dev proxy — no CORS needed anywhere.

```nginx
server {
    listen 80;

    location / {
        root /usr/share/nginx/html;
        try_files $uri /index.html;
    }

    location /auth/ {
        proxy_pass http://identity:8090;
    }

    location /oauth2/ {
        proxy_pass http://identity:8090;
    }

    location /board/ {
        proxy_pass http://board:8091;
    }
}
```

- [ ] **Step 3: Write `web/.dockerignore`**

```
node_modules
dist
```

- [ ] **Step 4: Add `web` service to `docker-compose.yml`**

Append after the `board` service, matching existing style:

```yaml
  web:
    build: ./web
    container_name: idsvc-web
    depends_on:
      - identity
      - board
    ports:
      - "5174:80" # demo frontend; 5173 stays free for the Vite dev server
```

- [ ] **Step 5: Add Makefile targets**

Update the `.PHONY` line:

```make
.PHONY: install-tools proto migrate-up migrate-down run stop logs test build web-dev web-test
```

Append at the end of the Makefile:

```make
## web-dev / web-test: frontend dev server / unit tests (run `npm install` in web/ once first)
web-dev:
	cd web && npm run dev

web-test:
	cd web && npm run test
```

- [ ] **Step 6: Verify compose path**

```bash
make run
```

Expected: `idsvc-web` builds and starts. Open http://localhost:5174 — run the same M3 pass as Task 6 Step 3 (fresh phone number). Both serving modes now work.

- [ ] **Step 7: Commit**

```bash
git add web/Dockerfile web/nginx.conf web/.dockerignore docker-compose.yml Makefile
git commit -m "chore: serve web demo via nginx in compose (:5174) + web make targets

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: Mark M3 complete in plan docs

**Files:**
- Modify: `plan/2026-07-27-phase5-frontend.md` (status line), `plan/2026-07-27-phase5-frontend-implementation.md` (status line), `plan/2026-07-17-build-order.md` (status line)

**Interfaces:** none — docs only. Only do this task after Task 7's verification actually passed.

- [ ] **Step 1: Update status lines**

- `plan/2026-07-27-phase5-frontend.md`: `> Status: 📋 Planned` → `> Status: ✅ Completed 2026-07-27 — M3 passed (§6 scenario clickable on :5173 and :5174)`
- `plan/2026-07-27-phase5-frontend-implementation.md`: `> Status: 📋 Planned — spec: …` → `> Status: ✅ Completed 2026-07-27 — spec: plan/2026-07-27-phase5-frontend.md`
- `plan/2026-07-17-build-order.md` line 3: `Phases 0–4 ✅ done (M1 + M2 passed, eventing live 2026-07-19) · Phase 5 (frontend) next` → `Phases 0–5 ✅ done (M1–M3 passed, frontend live 2026-07-27) · Phase 6 (edge states) next`

- [ ] **Step 2: Commit**

```bash
git add plan/
git commit -m "docs: mark phase 5 complete (M3 passed)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

Branch stays on `feat/frontend` awaiting hcho's review — review gate: no merge into dev, no push.
