import { useState, type FormEvent } from 'react'
import { ApiError, verifyDevice } from '../api'
import { setTokens } from '../auth'

interface Props {
  flowToken: string
  onDone: () => void
  onRestart: () => void // attempts/flow token exhausted → back to login
}

// Unknown-device check: prove the account creation month (PRD §4.2 item 4).
export default function DeviceVerify({ flowToken, onDone, onRestart }: Props) {
  const [month, setMonth] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await verifyDevice(flowToken, month)
      setTokens(res.tokens)
      onDone()
    } catch (err) {
      setError(err instanceof ApiError && err.status === 401 ? 'wrong answer — try again' : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="card">
      <h1>New device</h1>
      <p>This device isn’t registered to your account. When did you sign up?</p>
      <form onSubmit={submit}>
        <label htmlFor="month">Signup month</label>
        <input id="month" type="month" value={month} onChange={(e) => setMonth(e.target.value)} required />
        <button disabled={busy}>Verify</button>
        <button type="button" onClick={onRestart}>
          Back to login
        </button>
      </form>
      {error && <p className="error">{error}</p>}
    </main>
  )
}
