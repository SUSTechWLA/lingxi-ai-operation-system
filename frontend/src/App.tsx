import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import AuthScreen from './components/AuthScreen'
import { fetchCurrentUser, getStoredAuthSession, isDefinitiveAuthFailure, logout, type AuthUser } from './services/auth'
import { fetchLocalAgentHealth } from './services/localAgent'
import CreatorShell from './features/creator-studio/CreatorShell'
import { parseAppRoute, replaceHashRoute, type AppRoute, type DeveloperDiagnosticsView } from './creatorRoutes'

const developerConsoleEnabled = import.meta.env.DEV || import.meta.env.VITE_ENABLE_DEVELOPER_CONSOLE === '1'
const DeveloperConsolePage = developerConsoleEnabled ? lazy(() => import('./features/developer-console/DeveloperConsolePage')) : null

function currentRoute(): AppRoute {
  return parseAppRoute(window.location.hash, developerConsoleEnabled)
}

function replaceCreatorHash() {
  const nextHash = replaceHashRoute('#/create')
  if (window.location.hash !== nextHash) window.history.replaceState(null, '', nextHash)
}

function developerHash(view: DeveloperDiagnosticsView) {
  return `#/developer/${view}`
}

function App() {
  const [serviceStatus, setServiceStatus] = useState<'unknown' | 'ok' | 'unhealthy'>('unknown')
  const [authUser, setAuthUser] = useState<AuthUser | null>(null)
  const [authChecking, setAuthChecking] = useState(true)
  const [route, setRoute] = useState<AppRoute>(currentRoute)

  const syncRoute = useCallback(() => {
    const nextRoute = currentRoute()
    if (!window.location.hash || (nextRoute.kind === 'creator' && nextRoute.shouldReplace)) {
      replaceCreatorHash()
      setRoute({ kind: 'creator', page: 'create' })
      return
    }
    setRoute(nextRoute)
  }, [])

  useEffect(() => {
    const restoreSession = async () => {
      const storedSession = getStoredAuthSession()
      if (!storedSession) {
        setAuthChecking(false)
        return
      }
      try {
        const user = await fetchCurrentUser()
        setAuthUser(user)
      } catch (error) {
        if (isDefinitiveAuthFailure(error)) {
          logout()
        } else {
          setAuthUser(storedSession.user)
        }
      } finally {
        setAuthChecking(false)
      }
    }
    restoreSession()
  }, [])

  useEffect(() => {
    syncRoute()
    window.addEventListener('hashchange', syncRoute)
    return () => window.removeEventListener('hashchange', syncRoute)
  }, [syncRoute])

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
    return <AuthScreen onAuthenticated={(user) => {
      setAuthUser(user)
      syncRoute()
    }} />
  }

  const handleLogout = () => {
    logout()
    replaceCreatorHash()
    setRoute({ kind: 'creator', page: 'create' })
    setAuthUser(null)
  }

  if (route.kind === 'developer' && DeveloperConsolePage) {
    return (
      <Suspense fallback={<div className="flex h-screen items-center justify-center bg-background text-sm text-ink-muted">正在打开工作台...</div>}>
        <DeveloperConsolePage
          user={authUser}
          serviceStatus={serviceStatus}
          currentView={route.view}
          onViewChange={(view) => { window.location.hash = developerHash(view) }}
          onLogout={handleLogout}
        />
      </Suspense>
    )
  }

  if (route.kind === 'developer') return null

  return <CreatorShell user={authUser} serviceStatus={serviceStatus} route={route} onNavigate={(hash) => { window.location.hash = hash }} onLogout={handleLogout} />
}

export default App
