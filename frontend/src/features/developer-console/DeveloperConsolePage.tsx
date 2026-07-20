import DirectorStudioPage from '../../pages/DirectorStudioPage'
import type { DirectorNavKey } from '../../pages/directorStudioLogic'
import type { AuthUser } from '../../services/auth'

interface DeveloperConsolePageProps {
  user: AuthUser
  serviceStatus: 'unknown' | 'ok' | 'unhealthy'
  currentView: DirectorNavKey
  onViewChange: (view: DirectorNavKey) => void
  onLogout: () => void
}

export default function DeveloperConsolePage({ user, serviceStatus, currentView, onViewChange, onLogout }: DeveloperConsolePageProps) {
  return (
    <DirectorStudioPage
      user={user}
      serviceStatus={serviceStatus}
      currentView={currentView}
      onViewChange={onViewChange}
      onLogout={onLogout}
    />
  )
}
