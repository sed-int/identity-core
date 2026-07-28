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
