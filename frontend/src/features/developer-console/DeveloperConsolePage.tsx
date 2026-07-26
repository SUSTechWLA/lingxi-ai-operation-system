import type { DeveloperDiagnosticsView } from '../../creatorRoutes'
import type { AuthUser } from '../../services/auth'

interface DeveloperConsolePageProps {
  user: AuthUser
  serviceStatus: 'unknown' | 'ok' | 'unhealthy'
  currentView: DeveloperDiagnosticsView
  onViewChange: (view: DeveloperDiagnosticsView) => void
  onLogout: () => void
}

const diagnosticsNavigation: ReadonlyArray<{ view: DeveloperDiagnosticsView; label: string }> = [
  { view: 'summary', label: '摘要' },
  { view: 'timeline', label: '时间线' },
  { view: 'tools', label: '工具与 MCP' },
  { view: 'artifacts', label: '技术产物' },
  { view: 'gates', label: '门禁' },
  { view: 'recovery', label: '恢复' },
]

const serviceStatusLabel = {
  unknown: '正在确认服务状态',
  ok: '服务正常',
  unhealthy: '服务不可用',
} as const

export default function DeveloperConsolePage({
  user,
  serviceStatus,
  currentView,
  onViewChange,
  onLogout,
}: DeveloperConsolePageProps) {
  const activeItem = diagnosticsNavigation.find((item) => item.view === currentView) ?? diagnosticsNavigation[0]
  const userName = user.nickname || user.email

  return (
    <div className="min-h-screen bg-background text-ink">
      <header className="border-b border-line bg-background-card">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-4 px-4 py-4 sm:px-6 lg:px-8">
          <div className="min-w-0">
            <p className="m-0 text-xs font-bold uppercase tracking-[0.18em] text-ink-soft">Developer workspace</p>
            <h1 className="m-0 text-xl font-extrabold text-ink">开发诊断</h1>
          </div>
          <div className="ml-auto flex items-center gap-3 text-sm">
            <span
              className="rounded-full border border-line bg-background px-3 py-1 text-ink-muted"
              role="status"
            >
              {serviceStatusLabel[serviceStatus]}
            </span>
            <span className="hidden max-w-48 truncate text-ink-muted sm:inline" title={user.email}>{userName}</span>
            <button
              type="button"
              className="rounded-md border border-line bg-background-card px-3 py-2 font-bold text-ink hover:border-primary hover:text-primary-dark"
              onClick={onLogout}
            >
              退出登录
            </button>
          </div>
        </div>
        <nav className="mx-auto flex max-w-7xl gap-1 overflow-x-auto px-4 sm:px-6 lg:px-8" aria-label="开发诊断导航">
          {diagnosticsNavigation.map((item) => (
            <button
              key={item.view}
              type="button"
              className={`shrink-0 border-b-2 px-3 py-3 text-sm font-bold ${
                item.view === currentView
                  ? 'border-primary text-primary-dark'
                  : 'border-transparent text-ink-muted hover:border-line hover:text-ink'
              }`}
              aria-current={item.view === currentView ? 'page' : undefined}
              onClick={() => onViewChange(item.view)}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </header>

      <main className="mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <section className="overflow-hidden rounded-xl border border-line bg-background-card shadow-sm" aria-labelledby="diagnostics-view-title">
          <div className="border-b border-line px-5 py-4">
            <p className="m-0 text-xs font-bold uppercase tracking-[0.14em] text-ink-soft">当前视图</p>
            <h2 id="diagnostics-view-title" className="mb-0 mt-1 text-lg font-extrabold text-ink">{activeItem.label}</h2>
          </div>
          <div className="grid min-h-72 place-items-center px-5 py-12 text-center">
            <div className="max-w-md">
              <p className="m-0 text-base font-bold text-ink">诊断视图尚未接入数据</p>
              <p className="mb-0 mt-2 text-sm leading-6 text-ink-muted">
                后续步骤将在所选项目与运行范围内加载诊断信息，避免无边界地展示事件或产物。
              </p>
            </div>
          </div>
        </section>
      </main>
    </div>
  )
}
