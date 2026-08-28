import { useState } from 'react'
import { completeReactivation } from '../api'
import { setTokens } from '../auth'

interface Props {
  flowToken: string
  onDone: () => void
}

// DORMANT-account reactivation: agreeing to the privacy policy rolls the
// account back to ACTIVE (PRD §4.2 item 3).
export default function Reactivate({ flowToken, onDone }: Props) {
  const [agreed, setAgreed] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    setBusy(true)
    setError('')
    try {
      const res = await completeReactivation(flowToken)
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
      <h1>Welcome back</h1>
      <p>This account was dormant. Re-agree to the privacy policy to reactivate it.</p>
      <label>
        <input type="checkbox" checked={agreed} onChange={(e) => setAgreed(e.target.checked)} /> I
        agree to the privacy policy
      </label>
      <button disabled={!agreed || busy} onClick={submit}>
        Reactivate
      </button>
      {error && <p className="error">{error}</p>}
    </main>
  )
}
