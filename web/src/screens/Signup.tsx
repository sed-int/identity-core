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
