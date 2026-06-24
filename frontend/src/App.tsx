import { useState, useEffect } from 'react'
import AuthScreen from './components/AuthScreen'
import Sidebar from './components/Sidebar'
import DesktopPage from './pages/DesktopPage'
import CreatorWorkbenchPage from './pages/CreatorWorkbenchPage'
import GuidedVideoStudioPage from './pages/GuidedVideoStudioPage'
import { fetchCurrentUser, getStoredAuthSession, logout, type AuthUser } from './services/auth'
import { isElectron, getElectronAPI } from './utils/electron'

function App() {
  const [activeNav, setActiveNav] = useState('guidedVideo')
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
    if (!isElectron()) return

    const checkHealth = async () => {
      const api = getElectronAPI()
      if (!api) return
      try {
        const status = await api.checkServiceHealth()
        setServiceStatus(status === 'ok' ? 'ok' : 'unhealthy')
      } catch {
        setServiceStatus('unhealthy')
      }
    }

    checkHealth()
    const interval = setInterval(checkHealth, 30000)
    return () => clearInterval(interval)
  }, [])

  if (authChecking) {
    return <div className="flex h-screen items-center justify-center bg-[#FFF8E8] text-sm text-[#735C3D]">正在检查登录状态...</div>
  }

  if (!authUser) {
    return <AuthScreen onAuthenticated={setAuthUser} />
  }

  return (
    <div className="flex h-screen">
      <Sidebar activeNav={activeNav} onNavChange={setActiveNav} user={authUser} onLogout={() => {
        logout()
        setAuthUser(null)
      }} />
      <div className="flex-1 flex flex-col">
        {isElectron() && serviceStatus === 'unhealthy' && (
          <div className="bg-red-50 border-b border-red-200 px-6 py-3 flex items-center gap-2">
            <svg className="w-4 h-4 text-red-500 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
            <span className="text-sm text-red-700">后端服务未连接，部分功能不可用，请检查后端服务是否启动</span>
          </div>
        )}
        {activeNav === 'guidedVideo' && <GuidedVideoStudioPage />}
        {activeNav === 'creator' && <CreatorWorkbenchPage />}
        {activeNav === 'system' && <DesktopPage />}
      </div>
    </div>
  )
}

export default App
