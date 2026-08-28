import { useEffect, useState } from 'react'
import { refreshSession } from './api'
import { getRefreshToken } from './auth'
import Login from './screens/Login'
import Signup from './screens/Signup'
import Reactivate from './screens/Reactivate'
import DeviceVerify from './screens/DeviceVerify'
import Board from './screens/Board'

type Screen = 'loading' | 'login' | 'signup' | 'reactivate' | 'deviceVerify' | 'board'

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
          onReactivationRequired={(ft) => {
            setFlowToken(ft)
            setScreen('reactivate')
          }}
          onDeviceVerifyRequired={(ft) => {
            setFlowToken(ft)
            setScreen('deviceVerify')
          }}
        />
      )
    case 'signup':
      return <Signup flowToken={flowToken} onDone={() => setScreen('board')} />
    case 'reactivate':
      return <Reactivate flowToken={flowToken} onDone={() => setScreen('board')} />
    case 'deviceVerify':
      return (
        <DeviceVerify
          flowToken={flowToken}
          onDone={() => setScreen('board')}
          onRestart={() => setScreen('login')}
        />
      )
    case 'board':
      return <Board onLogout={() => setScreen('login')} />
  }
}
