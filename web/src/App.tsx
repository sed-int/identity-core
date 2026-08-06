import { useEffect, useState } from 'react'
import { refreshSession } from './api'
import { getRefreshToken } from './auth'
import Login from './screens/Login'
import Signup from './screens/Signup'

type Screen = 'loading' | 'login' | 'signup' | 'board'

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
