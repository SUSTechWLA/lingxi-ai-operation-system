import { useState } from 'react'
import { APP_ICON_PATH } from '../../utils/brand'
import type { AuthUser } from '../../services/auth'
import type { AppRoute } from '../../creatorRoutes'

interface CreatorShellProps {
  user: AuthUser
  serviceStatus: 'unknown' | 'ok' | 'unhealthy'
  route: Extract<AppRoute, { kind: 'creator' }>
  onNavigate: (hash: '#/create' | '#/videos') => void
  onLogout: () => void
}

export default function CreatorShell({ user, serviceStatus, route, onNavigate, onLogout }: CreatorShellProps) {
  const [connectionOpen, setConnectionOpen] = useState(false)
  const userName = user.nickname || user.email || '创作者'
  const connectionText = serviceStatus === 'ok' ? '本地服务已连接' : serviceStatus === 'unhealthy' ? '本地服务暂不可用' : '正在确认本地服务状态'

  return (
    <div className="creator-root">
      <header className="creator-header">
        <a className="creator-brand" href="#/create" onClick={() => onNavigate('#/create')}>
          <img src={APP_ICON_PATH} alt="躺营" className="creator-brand-mark" />
          <span>躺营创作</span>
        </a>
        <nav className="creator-nav" aria-label="创作导航">
          <button type="button" className={route.page === 'create' ? 'creator-nav-button is-active' : 'creator-nav-button'} onClick={() => onNavigate('#/create')}>开始创作</button>
          <button type="button" className={route.page === 'videos' || route.page === 'step' ? 'creator-nav-button is-active' : 'creator-nav-button'} onClick={() => onNavigate('#/videos')}>我的视频</button>
        </nav>
        <details className="creator-profile">
          <summary aria-label="打开账户菜单">{userName.slice(0, 1).toUpperCase()}</summary>
          <div className="creator-profile-menu">
            <strong>{userName}</strong>
            <span>{user.email}</span>
            <button type="button" onClick={() => setConnectionOpen(true)}>查看连接状态</button>
            <button type="button" onClick={onLogout}>退出登录</button>
          </div>
        </details>
      </header>

      <main className="creator-main">
        <CreatorOutlet route={route} />
      </main>

      {connectionOpen && (
        <div className="creator-dialog-backdrop" role="presentation" onMouseDown={() => setConnectionOpen(false)}>
          <section className="creator-dialog" role="dialog" aria-modal="true" aria-labelledby="creator-connection-title" onMouseDown={(event) => event.stopPropagation()}>
            <h2 id="creator-connection-title">连接状态</h2>
            <p>{connectionText}</p>
            <p className="creator-dialog-detail">如需帮助，请确认桌面端服务已经启动后再继续创作。</p>
            <button type="button" onClick={() => setConnectionOpen(false)}>知道了</button>
          </section>
        </div>
      )}
    </div>
  )
}

function CreatorOutlet({ route }: { route: Extract<AppRoute, { kind: 'creator' }> }) {
  if (route.page === 'videos') {
    return <CreatorPlaceholder eyebrow="我的视频" title="视频项目将在这里呈现" detail="项目列表会在下一步接入。" />
  }
  if (route.page === 'step') {
    return <CreatorPlaceholder eyebrow="创作进度" title={`项目 ${route.projectId}`} detail={`当前步骤：${route.stepId}`} />
  }
  return <CreatorPlaceholder eyebrow="开始创作" title="准备好讲一个好故事" detail="创作入口会在下一步接入。" />
}

function CreatorPlaceholder({ eyebrow, title, detail }: { eyebrow: string; title: string; detail: string }) {
  return (
    <section className="creator-placeholder" aria-labelledby="creator-page-title">
      <p>{eyebrow}</p>
      <h1 id="creator-page-title">{title}</h1>
      <span>{detail}</span>
    </section>
  )
}
