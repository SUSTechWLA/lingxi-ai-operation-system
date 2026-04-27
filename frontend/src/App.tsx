import { useState, useEffect } from 'react'
import Sidebar from './components/Sidebar'
import PublishPage from './pages/PublishPage'
import DesktopPage from './pages/DesktopPage'
import { isElectron, getElectronAPI } from './utils/electron'

function App() {
  const [activeNav, setActiveNav] = useState('publish')
  const [serviceStatus, setServiceStatus] = useState<'unknown' | 'ok' | 'unhealthy'>('unknown')

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

  return (
    <div className="flex h-screen">
      <Sidebar activeNav={activeNav} onNavChange={setActiveNav} />
      <div className="flex-1 flex flex-col">
        {isElectron() && serviceStatus === 'unhealthy' && (
          <div className="bg-red-50 border-b border-red-200 px-6 py-3 flex items-center gap-2">
            <svg className="w-4 h-4 text-red-500 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
            <span className="text-sm text-red-700">后端服务未连接，部分功能不可用，请检查后端服务是否启动</span>
          </div>
        )}
        {activeNav === 'publish' ? <PublishPage /> : <DesktopPage />}
      </div>
    </div>
  )
}

export default App
