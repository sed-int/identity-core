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
