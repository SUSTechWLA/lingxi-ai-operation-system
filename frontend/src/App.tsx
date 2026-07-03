import { useState, useEffect } from 'react'
import AuthScreen from './components/AuthScreen'
import BiaoshuWorkbench from './pages/BiaoshuWorkbench'
import { fetchCurrentUser, getStoredAuthSession, logout, type AuthUser } from './services/auth'

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
        logout()
      } finally {
        setAuthChecking(false)
      }
    }
    restoreSession()
  }, [])

  if (authChecking) {
    return <div className="flex h-screen items-center justify-center bg-[#FFF8E8] text-sm text-[#735C3D]">正在检查登录状态...</div>
  }

  if (!authUser) {
    return <AuthScreen onAuthenticated={setAuthUser} />
  }

  return (
    <BiaoshuWorkbench />
  )
}

export default App
