import { useEffect, useMemo, useRef, useState } from 'react'
import type { DeveloperDiagnosticsView } from '../../creatorRoutes'
import {
  fetchProjectArtifacts,
  fetchVideoProjects,
  getAgentRun,
  getAgentRunReviews,
  getAgentRunTrace,
  getTaskContext,
  getTaskDetails,
} from '../../services/api'
import type { AuthUser } from '../../services/auth'
import type { VideoProject } from '../../utils/types'
import {
  isTerminalAgentRunStatus,
  loadProjectDiagnostics,
  mergeProjectDiagnostics,
  selectDiagnosticsProject,
  sortDiagnosticsProjects,
  type DiagnosticsDataSection,
  type ProjectDiagnosticsApi,
  type ProjectDiagnosticsLoadResult,
} from './developerDiagnostics'

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

const diagnosticsApi: ProjectDiagnosticsApi = {
  getRun: getAgentRun,
  getTrace: getAgentRunTrace,
  getReviews: getAgentRunReviews,
  getArtifacts: fetchProjectArtifacts,
  getTask: getTaskDetails,
  getContext: getTaskContext,
}

const failedSectionLabel: Record<DiagnosticsDataSection, string> = {
  run: '运行摘要',
  trace: '执行轨迹',
  reviews: '审核记录',
  artifacts: '产物注册表',
  task: '任务详情',
  context: '任务上下文',
}

type ProjectLoadState = 'loading' | 'ready' | 'backend-unavailable'
type DiagnosticsLoadState = 'idle' | 'loading' | 'ready' | 'backend-unavailable'

export default function DeveloperConsolePage({
  user,
  serviceStatus,
  currentView,
  onViewChange,
  onLogout,
}: DeveloperConsolePageProps) {
  const activeItem = diagnosticsNavigation.find((item) => item.view === currentView) ?? diagnosticsNavigation[0]
  const userName = user.nickname || user.email
  const [projects, setProjects] = useState<VideoProject[]>([])
  const [projectLoadState, setProjectLoadState] = useState<ProjectLoadState>('loading')
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [diagnostics, setDiagnostics] = useState<ProjectDiagnosticsLoadResult | null>(null)
  const [diagnosticsLoadState, setDiagnosticsLoadState] = useState<DiagnosticsLoadState>('idle')
  const requestScopeRef = useRef('')

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId),
    [projects, selectedProjectId],
  )
  const handleProjectSelect = (projectId: string) => {
    const project = projects.find((candidate) => candidate.id === projectId)
    requestScopeRef.current = project?.currentRunId ? `${project.id}:${project.currentRunId}` : ''
    setSelectedProjectId(projectId)
  }

  useEffect(() => {
    const controller = new AbortController()
    setProjectLoadState('loading')
    void fetchVideoProjects(controller.signal)
      .then((response) => {
        if (controller.signal.aborted) return
        const sortedProjects = sortDiagnosticsProjects(response.projects ?? [])
        setProjects(sortedProjects)
        setSelectedProjectId(selectDiagnosticsProject(sortedProjects)?.id ?? '')
        setProjectLoadState('ready')
      })
      .catch(() => {
        if (controller.signal.aborted) return
        setProjects([])
        setSelectedProjectId('')
        setProjectLoadState('backend-unavailable')
      })

    return () => controller.abort()
  }, [])

  useEffect(() => {
    const projectId = selectedProject?.id
    const runId = selectedProject?.currentRunId
    const requestScope = projectId && runId ? `${projectId}:${runId}` : ''
    requestScopeRef.current = requestScope

    if (!projectId || !runId) {
      setDiagnostics(null)
      setDiagnosticsLoadState('idle')
      return
    }

    const controller = new AbortController()
    setDiagnostics(null)
    setDiagnosticsLoadState('loading')
    void loadProjectDiagnostics({ projectId, runId, signal: controller.signal, api: diagnosticsApi })
      .then((result) => {
        if (controller.signal.aborted || requestScopeRef.current !== requestScope) return
        setDiagnostics((current) => requestScopeRef.current === requestScope ? result : current)
        setDiagnosticsLoadState(result.run ? 'ready' : 'backend-unavailable')
      })
      .catch(() => {
        if (controller.signal.aborted || requestScopeRef.current !== requestScope) return
        setDiagnostics(null)
        setDiagnosticsLoadState('backend-unavailable')
      })

    return () => controller.abort()
  }, [selectedProject?.currentRunId, selectedProject?.id])

  const selectedRunStatus = diagnostics?.run?.status
  useEffect(() => {
    const projectId = selectedProject?.id
    const runId = selectedProject?.currentRunId
    if (
      diagnosticsLoadState !== 'ready' ||
      !projectId ||
      !runId ||
      !selectedRunStatus ||
      isTerminalAgentRunStatus(selectedRunStatus)
    ) return

    const requestScope = `${projectId}:${runId}`
    const controller = new AbortController()
    let polling = false
    const refresh = async () => {
      if (polling || controller.signal.aborted || requestScopeRef.current !== requestScope) return
      polling = true
      try {
        const result = await loadProjectDiagnostics({
          projectId,
          runId,
          signal: controller.signal,
          api: diagnosticsApi,
        })
        if (controller.signal.aborted || requestScopeRef.current !== requestScope) return
        setDiagnostics((current) => (
          requestScopeRef.current === requestScope
            ? mergeProjectDiagnostics(current, result)
            : current
        ))
      } catch {
        // A poll failure leaves the last scoped snapshot visible.
      } finally {
        polling = false
      }
    }
    const timer = window.setInterval(() => { void refresh() }, 3000)

    return () => {
      controller.abort()
      window.clearInterval(timer)
    }
  }, [diagnosticsLoadState, selectedProject?.currentRunId, selectedProject?.id, selectedRunStatus])

  const noProjects = projectLoadState === 'ready' && projects.length === 0
  const noRun = projectLoadState === 'ready' && Boolean(selectedProject) && !selectedProject?.currentRunId
  const backendUnavailable = projectLoadState === 'backend-unavailable' || diagnosticsLoadState === 'backend-unavailable'
  const selectedRunFailed = diagnostics?.run?.status === 'FAILED'
  const partialSections = diagnostics?.errors ?? []

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
        <ProjectSelector projects={projects} selectedProjectId={selectedProjectId} onSelect={handleProjectSelect} />
        <section className="overflow-hidden rounded-xl border border-line bg-background-card shadow-sm" aria-labelledby="diagnostics-view-title">
          <div className="border-b border-line px-5 py-4">
            <p className="m-0 text-xs font-bold uppercase tracking-[0.14em] text-ink-soft">当前视图</p>
            <h2 id="diagnostics-view-title" className="mb-0 mt-1 text-lg font-extrabold text-ink">{activeItem.label}</h2>
          </div>
          <DiagnosticsContent
            projectLoading={projectLoadState === 'loading'}
            diagnosticsLoading={diagnosticsLoadState === 'loading'}
            backendUnavailable={backendUnavailable}
            noProjects={noProjects}
            noRun={noRun}
            diagnostics={diagnostics}
            selectedRunFailed={selectedRunFailed}
            partialSections={partialSections}
          />
        </section>
      </main>
    </div>
  )
}

function ProjectSelector({
  projects,
  selectedProjectId,
  onSelect,
}: {
  projects: VideoProject[]
  selectedProjectId: string
  onSelect: (projectId: string) => void
}) {
  if (projects.length === 0) return null
  const selectedProject = projects.find((project) => project.id === selectedProjectId)
  return (
    <div className="mb-4 flex flex-wrap items-end gap-3 rounded-xl border border-line bg-background-card px-5 py-4">
      <label className="min-w-0 flex-1 text-sm font-bold text-ink" htmlFor="diagnostics-project">
        诊断项目
        <select
          id="diagnostics-project"
          className="mt-2 block w-full rounded-md border border-line bg-background px-3 py-2 font-normal text-ink"
          value={selectedProjectId}
          onChange={(event) => onSelect(event.target.value)}
        >
          {projects.map((project) => (
            <option key={project.id} value={project.id}>
              {project.name}{project.currentRunId ? '' : '（无运行）'}
            </option>
          ))}
        </select>
      </label>
      {selectedProject?.currentRunId ? (
        <p className="m-0 max-w-full truncate text-xs text-ink-muted" title={selectedProject.currentRunId}>
          Run: {selectedProject.currentRunId}
        </p>
      ) : null}
    </div>
  )
}

function DiagnosticsContent({
  projectLoading,
  diagnosticsLoading,
  backendUnavailable,
  noProjects,
  noRun,
  diagnostics,
  selectedRunFailed,
  partialSections,
}: {
  projectLoading: boolean
  diagnosticsLoading: boolean
  backendUnavailable: boolean
  noProjects: boolean
  noRun: boolean
  diagnostics: ProjectDiagnosticsLoadResult | null
  selectedRunFailed: boolean
  partialSections: DiagnosticsDataSection[]
}) {
  if (projectLoading || diagnosticsLoading) {
    return <DiagnosticsNotice title="正在加载诊断数据" detail="正在读取所选项目的运行摘要与关联记录。" />
  }
  if (backendUnavailable) {
    return <DiagnosticsNotice title="诊断后端不可用" detail="无法读取所选范围，请确认服务状态后重试。" />
  }
  if (noProjects) return <DiagnosticsNotice title="暂无项目" detail="当前账号还没有可供诊断的视频项目。" />
  if (noRun) return <DiagnosticsNotice title="项目暂无运行" detail="所选项目尚未关联运行，因而没有运行诊断数据。" />
  if (!diagnostics?.run) return <DiagnosticsNotice title="暂无诊断摘要" detail="所选运行尚未返回可显示的数据。" />

  return (
    <div className="space-y-5 px-5 py-6">
      {selectedRunFailed ? (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3" role="alert">
          <p className="m-0 font-bold text-red-800">所选运行已失败</p>
          <p className="mb-0 mt-1 text-sm text-red-700">已保留可用摘要与证据，供进一步定位失败原因。</p>
        </div>
      ) : null}
      {partialSections.length > 0 ? (
        <div className="rounded-lg border border-amber-300 bg-amber-50 px-4 py-3" role="status">
          <p className="m-0 font-bold text-amber-900">部分诊断数据可用</p>
          <p className="mb-0 mt-1 text-sm text-amber-800">
            暂时无法读取：{partialSections.map((section) => failedSectionLabel[section]).join('、')}。已加载的运行摘要保持可见。
          </p>
        </div>
      ) : null}
      <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <DiagnosticsMetric label="运行状态" value={diagnostics.run.status} />
        <DiagnosticsMetric label="任务" value={diagnostics.run.taskId ?? '未关联'} />
        <DiagnosticsMetric label="轨迹节点" value={String(diagnostics.nodes?.length ?? 0)} />
        <DiagnosticsMetric label="审核记录" value={String(diagnostics.reviews?.length ?? 0)} />
        <DiagnosticsMetric label="项目产物" value={String(diagnostics.artifacts?.length ?? 0)} />
        <DiagnosticsMetric label="上下文记录" value={String(diagnostics.context?.length ?? 0)} />
      </dl>
    </div>
  )
}

function DiagnosticsNotice({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="grid min-h-72 place-items-center px-5 py-12 text-center" role="status">
      <div className="max-w-md">
        <p className="m-0 text-base font-bold text-ink">{title}</p>
        <p className="mb-0 mt-2 text-sm leading-6 text-ink-muted">{detail}</p>
      </div>
    </div>
  )
}

function DiagnosticsMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-lg border border-line bg-background px-4 py-3">
      <dt className="text-xs font-bold text-ink-soft">{label}</dt>
      <dd className="mb-0 mt-1 break-words text-sm font-bold text-ink">{value}</dd>
    </div>
  )
}
