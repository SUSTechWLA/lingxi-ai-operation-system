import { useState, useEffect, useCallback } from 'react'
import AuthScreen from './components/AuthScreen'
import { BiaoshuAppShell } from './components/BiaoshuAppShell'
import { fetchCurrentUser, getStoredAuthSession, logout as doLogout, type AuthUser } from './services/auth'

function App() {
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
        doLogout()
      } finally {
        setAuthChecking(false)
      }
    }
    restoreSession()
  }, [])

  const handleLogout = useCallback(() => {
    doLogout()
    setAuthUser(null)
  }, [])

  if (authChecking) {
    return <div className="flex h-screen items-center justify-center bg-[#FFF8E8] text-sm text-[#735C3D]">正在检查登录状态...</div>
  }

  if (!authUser) {
    return <AuthScreen onAuthenticated={setAuthUser} />
  }

  return (
    <BiaoshuAppShell user={authUser} onLogout={handleLogout} />
  )
}

export default App
