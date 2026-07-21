import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { APP_ICON_PATH } from '../../utils/brand'
import type { AuthUser } from '../../services/auth'
import type { AppRoute } from '../../creatorRoutes'
import { cycleFocusIndex } from './focusCycle'
import StartCreationPage from './StartCreationPage'
import VideoLibraryPage from './VideoLibraryPage'
import ProjectWorkspacePage from './ProjectWorkspacePage'
import SettingsPage from '../../pages/SettingsPage'

interface CreatorShellProps {
  user: AuthUser
  serviceStatus: 'unknown' | 'ok' | 'unhealthy'
  route: Extract<AppRoute, { kind: 'creator' }>
  onNavigate: (hash: string) => void
  onLogout: () => void
}

export default function CreatorShell({ user, serviceStatus, route, onNavigate, onLogout }: CreatorShellProps) {
  const [profileOpen, setProfileOpen] = useState(false)
  const [connectionOpen, setConnectionOpen] = useState(false)
  const profileRef = useRef<HTMLDivElement>(null)
  const profileTriggerRef = useRef<HTMLButtonElement>(null)
  const profileMenuRef = useRef<HTMLDivElement>(null)
  const connectionTriggerRef = useRef<HTMLButtonElement>(null)
  const dialogRef = useRef<HTMLElement>(null)
  const dialogCloseButtonRef = useRef<HTMLButtonElement>(null)
  const userName = user.nickname || user.email || '创作者'
  const connectionText = serviceStatus === 'ok' ? '本地服务已连接' : serviceStatus === 'unhealthy' ? '本地服务暂不可用' : '正在确认本地服务状态'

  const closeProfile = useCallback((restoreFocus = false) => {
    setProfileOpen(false)
    if (restoreFocus) window.requestAnimationFrame(() => profileTriggerRef.current?.focus())
  }, [])

  const closeConnection = useCallback((restoreFocus = true) => {
    setConnectionOpen(false)
    if (restoreFocus) window.requestAnimationFrame(() => connectionTriggerRef.current?.focus())
  }, [])

  useEffect(() => {
    if (!profileOpen || connectionOpen) return
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (!profileRef.current?.contains(event.target as Node)) closeProfile()
    }
    document.addEventListener('pointerdown', closeOnOutsidePointer)
    return () => document.removeEventListener('pointerdown', closeOnOutsidePointer)
  }, [closeProfile, connectionOpen, profileOpen])

  useEffect(() => {
    if (!connectionOpen) return
    window.requestAnimationFrame(() => dialogCloseButtonRef.current?.focus())
  }, [connectionOpen])

  const focusProfileMenuItem = (direction: 1 | -1) => {
    const items = Array.from(profileMenuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') || [])
    if (items.length === 0) return
    const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement)
    items[cycleFocusIndex(currentIndex, items.length, direction < 0)]?.focus()
  }

  const openProfileMenu = (focusFirstItem = false) => {
    setProfileOpen(true)
    if (focusFirstItem) {
      window.requestAnimationFrame(() => profileMenuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus())
    }
  }

  const closeTransientUi = () => {
    setProfileOpen(false)
    closeConnection(false)
  }

  const navigate = (hash: '#/create' | '#/videos') => {
    closeTransientUi()
    onNavigate(hash)
  }

  const handleProfileTriggerKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      openProfileMenu(true)
    }
    if (event.key === 'Escape') closeProfile()
  }

  const handleProfileMenuKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      closeProfile(true)
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      focusProfileMenuItem(1)
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      focusProfileMenuItem(-1)
    }
    if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      const items = Array.from(profileMenuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') || [])
      items[event.key === 'Home' ? 0 : items.length - 1]?.focus()
    }
  }

  const handleDialogKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      closeConnection()
      return
    }
    if (event.key !== 'Tab') return
    const controls = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]), [href]') || [])
    const nextIndex = cycleFocusIndex(controls.indexOf(document.activeElement as HTMLElement), controls.length, event.shiftKey)
    if (nextIndex >= 0) {
      event.preventDefault()
      controls[nextIndex]?.focus()
    }
  }

  return (
    <div className="creator-root">
      <div aria-hidden={connectionOpen || undefined}>
        <header className="creator-header">
          <a className="creator-brand" href="#/create" onClick={() => navigate('#/create')}>
            <img src={APP_ICON_PATH} alt="躺营" className="creator-brand-mark" />
            <span>躺营创作</span>
          </a>
          <nav className="creator-nav" aria-label="创作导航">
            <button type="button" className={route.page === 'create' ? 'creator-nav-button is-active' : 'creator-nav-button'} onClick={() => navigate('#/create')}>开始创作</button>
            <button type="button" className={route.page === 'videos' || route.page === 'step' ? 'creator-nav-button is-active' : 'creator-nav-button'} onClick={() => navigate('#/videos')}>我的视频</button>
          </nav>
          <div className="creator-profile" ref={profileRef}>
            <button
              ref={profileTriggerRef}
              type="button"
              className="creator-profile-trigger"
              aria-label="打开账户菜单"
              aria-expanded={profileOpen}
              aria-controls="creator-profile-menu"
              aria-haspopup="menu"
              onClick={() => profileOpen ? closeProfile() : openProfileMenu()}
              onKeyDown={handleProfileTriggerKeyDown}
            >{userName.slice(0, 1).toUpperCase()}</button>
            {profileOpen && (
              <div id="creator-profile-menu" className="creator-profile-menu" ref={profileMenuRef} role="menu" aria-label="账户菜单" onKeyDown={handleProfileMenuKeyDown}>
                <div role="presentation">
                  <strong>{userName}</strong>
                  <span>{user.email}</span>
                </div>
                <button ref={connectionTriggerRef} type="button" role="menuitem" onClick={() => setConnectionOpen(true)}>查看连接状态</button>
                <button type="button" role="menuitem" onClick={() => { closeTransientUi(); onNavigate('#/settings') }}>设置</button>
                <button type="button" role="menuitem" onClick={() => { closeTransientUi(); onLogout() }}>退出登录</button>
              </div>
            )}
          </div>
        </header>

        <main className="creator-main">
          <CreatorOutlet route={route} onNavigate={onNavigate} />
        </main>
      </div>

      {connectionOpen && (
        <div className="creator-dialog-backdrop" role="presentation" onPointerDown={() => closeConnection()}>
          <section className="creator-dialog" ref={dialogRef} role="dialog" tabIndex={-1} aria-modal="true" aria-labelledby="creator-connection-title" onPointerDown={(event) => event.stopPropagation()} onKeyDown={handleDialogKeyDown}>
            <h2 id="creator-connection-title">连接状态</h2>
            <p>{connectionText}</p>
            <p className="creator-dialog-detail">如需帮助，请确认桌面端服务已经启动后再继续创作。</p>
            <button ref={dialogCloseButtonRef} type="button" onClick={() => closeConnection()}>知道了</button>
          </section>
        </div>
      )}
    </div>
  )
}

function CreatorOutlet({ route, onNavigate }: { route: Extract<AppRoute, { kind: 'creator' }>; onNavigate: CreatorShellProps['onNavigate'] }) {
  if (route.page === 'settings') {
    return <SettingsPage variant="creator" onBack={() => onNavigate('#/create')} />
  }
  if (route.page === 'videos') {
    return <VideoLibraryPage onContinueProject={(projectId, stepId) => onNavigate(`#/videos/${encodeURIComponent(projectId)}/steps/${stepId}`)} />
  }
  if (route.page === 'step') {
    return <ProjectWorkspacePage projectId={route.projectId} stepId={route.stepId} onNavigate={onNavigate} />
  }
  return <StartCreationPage onOpenProject={(projectId) => onNavigate(`#/videos/${encodeURIComponent(projectId)}/steps/requirements`)} />
}
