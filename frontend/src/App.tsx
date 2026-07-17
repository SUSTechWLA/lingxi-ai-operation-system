import { useState, useEffect } from 'react'
import AuthScreen from './components/AuthScreen'
import DirectorStudioPage from './pages/DirectorStudioPage'
import { fetchCurrentUser, getStoredAuthSession, logout, type AuthUser } from './services/auth'
import { fetchLocalAgentHealth } from './services/localAgent'

function App() {
  const [serviceStatus, setServiceStatus] = useState<'unknown' | 'ok' | 'unhealthy'>('unknown')
  const [authUser, setAuthUser] = useState<AuthUser | null>(null)
  const [authChecking, setAuthChecking] = useState(true)

  useEffect(() => {
    const restoreSession = async () => {
      if (!getStoredAuthSession()) {
        setAuthChecking(false)
        return
      }
      try {
        const user = await fetchCurrentUser()
        setAuthUser(user)
      } catch {
        logout()
      } finally {
        setAuthChecking(false)
      }
    }
    restoreSession()
  }, [])

  useEffect(() => {
    const checkHealth = async () => {
      try {
        const health = await fetchLocalAgentHealth()
        setServiceStatus(health.status === 'ok' ? 'ok' : 'unhealthy')
      } catch {
        setServiceStatus('unhealthy')
      }
    }

    checkHealth()
    const interval = setInterval(checkHealth, 30000)
    return () => clearInterval(interval)
  }, [])

  if (authChecking) {
    return <div className="flex h-screen items-center justify-center bg-background text-sm text-ink-muted">正在检查登录状态...</div>
  }

  if (!authUser) {
    return <AuthScreen onAuthenticated={setAuthUser} />
  }

  return (
    <DirectorStudioPage
      user={authUser}
      serviceStatus={serviceStatus}
      onLogout={() => {
        logout()
        setAuthUser(null)
      }}
    />
  )
}

export default App
