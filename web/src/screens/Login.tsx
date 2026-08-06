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
