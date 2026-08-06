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
  try {
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
  } catch {
    // Network-level failure (server down) — refresh token may still be valid,
    // so don't clear it. Just resolve false; caller treats session as unrefreshed.
    return false
  }
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
