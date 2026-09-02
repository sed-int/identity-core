import { useState, type FormEvent } from 'react'
import { ApiError, requestOtp, verifyOtp } from '../api'
import { setTokens } from '../auth'

interface Props {
  onTokens: () => void
  onSignupRequired: (flowToken: string) => void
  onReactivationRequired: (flowToken: string) => void
  onDeviceVerifyRequired: (flowToken: string) => void
}

export default function Login({
  onTokens,
  onSignupRequired,
  onReactivationRequired,
  onDeviceVerifyRequired,
}: Props) {
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
        case 'NEXT_STEP_REACTIVATION_REQUIRED':
          onReactivationRequired(res.flowToken!)
          break
        case 'NEXT_STEP_DEVICE_VERIFICATION_REQUIRED':
          onDeviceVerifyRequired(res.flowToken!)
          break
        default:
          setError(`unexpected next step: ${res.nextStep}`)
      }
    } catch (err) {
      setError(err instanceof ApiError && err.status === 401 ? 'wrong or expired code' : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth-shell">
      <section className="auth-story" aria-label="About Common Ground">
        <div className="brand-lockup">
          <span className="brand-mark">C</span>
          <span className="brand-name">Common Ground</span>
        </div>
        <div className="auth-story-copy">
          <p className="eyebrow">A quieter place to connect</p>
          <h1>Good conversations start here.</h1>
          <p>Share an idea, learn who is behind it, and keep the discussion grounded in a real community.</p>
        </div>
        <div className="auth-proof" aria-label="Authentication benefits">
          <div className="auth-proof-item"><strong>Phone-first</strong><span>No password to remember or reuse.</span></div>
          <div className="auth-proof-item"><strong>Private by design</strong><span>Your phone number stays inside Identity.</span></div>
        </div>
      </section>
      <main className="auth-panel">
        <div className="auth-form-wrap">
          <p className="eyebrow">Step {stage === 'phone' ? '1' : '2'} of 2</p>
          <h2>{stage === 'phone' ? 'Welcome in' : 'Check your code'}</h2>
          <p className="form-intro">
            {stage === 'phone'
              ? 'Use your mobile number to sign in or create a new profile.'
              : `We sent a six-digit code to ${phone}.`}
          </p>
          {stage === 'phone' ? (
            <form onSubmit={submitPhone}>
              <div className="field">
                <label htmlFor="phone">Mobile number</label>
                <input id="phone" type="tel" inputMode="tel" autoComplete="tel" value={phone}
                  onChange={(e) => setPhone(e.target.value)} placeholder="+82 10 1234 5678" required />
                <span className="field-hint">Include the country code, for example +821012345678.</span>
              </div>
              <button className="button button-primary" disabled={busy}>{busy ? 'Sending…' : 'Continue with phone'}</button>
            </form>
          ) : (
            <form onSubmit={submitCode}>
              <div className="field">
                <label htmlFor="code">Verification code</label>
                <input id="code" inputMode="numeric" autoComplete="one-time-code" value={code}
                  onChange={(e) => setCode(e.target.value)} placeholder="000000" required autoFocus />
                <span className="field-hint">In this demo, the development OTP is filled in automatically.</span>
              </div>
              <div className="button-row">
                <button className="button button-secondary" type="button" onClick={() => setStage('phone')}>Back</button>
                <button className="button button-primary" disabled={busy}>{busy ? 'Verifying…' : 'Verify & continue'}</button>
              </div>
            </form>
          )}
          {error && <p className="alert" role="alert">{error}</p>}
        </div>
      </main>
    </div>
  )
}
