import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import clsx from 'clsx'
import { APP_ICON_PATH } from '../utils/brand'
import {
  FiActivity,
  FiArchive,
  FiBell,
  FiCheck,
  FiChevronRight,
  FiCopy,
  FiCpu,
  FiDownload,
  FiEdit3,
  FiFileText,
  FiFolder,
  FiHardDrive,
  FiHome,
  FiLayers,
  FiLock,
  FiLogOut,
  FiPlay,
  FiPlayCircle,
  FiRefreshCw,
  FiSearch,
  FiSettings,
  FiShield,
  FiUserCheck,
  FiUsers,
  FiVideo,
  FiX,
  FiZap,
} from 'react-icons/fi'
import DesktopPage from './DesktopPage'
import {
  approveAgentReview,
  fetchVideoPreflight,
  fetchVideoRoleAgents,
  getAgentRun,
  getAgentRunReviews,
  getAgentRunTrace,
  regenerateAgentStage,
  rejectAgentReview,
  startAgentRun,
  submitEditedArtifact,
  type PreflightResponse,
} from '../services/api'
import type { AuthUser } from '../services/auth'
import type { AgentReviewItem, AgentRun, VideoRoleAgent } from '../utils/types'
import {
  applyOptimisticRunningStage,
  buildDirectorArtifacts,
  buildDirectorStages,
  buildDirectorTraceNodes,
  buildPublishCopies,
  deriveNextAction,
  displayNameForArtifact,
  downstreamStaleArtifacts,
  extractDirectorErrorDetail,
  getStageStateDisplay,
  isActionablePendingReview,
  normalizeDirectorErrorMessage,
  nextStageIdAfterReview,
  publishCopiesToJSON,
  publishCopiesToMarkdown,
  reviewDisplayTitle,
  reviewQualityReportLines,
  reviewOutputText,
  reviewStatusLabel,
  stageActionLabel,
  traceNodeHasError,
  visibleReviewHistory,
  type DirectorArtifactRecord,
  type DirectorArtifactStatus,
  type DirectorErrorDetail,
  type DirectorNavKey,
  type DirectorStage,
  type DirectorStageStatus,
  type DirectorTraceNode,
} from './directorStudioLogic'

interface Props {
  user: AuthUser
  onLogout: () => void
  serviceStatus: 'unknown' | 'ok' | 'unhealthy'
}

const navItems: Array<{ key: DirectorNavKey; label: string; icon: React.ComponentType<{ className?: string }> }> = [
  { key: 'overview', label: '项目', icon: FiHome },
  { key: 'review', label: '审核', icon: FiShield },
  { key: 'trace', label: '追踪', icon: FiLayers },
  { key: 'assets', label: '产物', icon: FiArchive },
  { key: 'roles', label: '角色', icon: FiUsers },
  { key: 'export', label: '导出', icon: FiVideo },
  { key: 'system', label: '设置', icon: FiSettings },
]

const fallbackRoles: VideoRoleAgent[] = [
  { id: 'creative_director', name: 'Creative Director', displayName: '创意总监', stage: 'proposal', goal: '理解需求，确定主题、时长、风格与创作方向。', allowedTools: ['proposal_generator', 'capability_preflight'], requiredOutputs: ['VIDEO_PROPOSAL'], humanReview: { required: true, reviewFocus: ['主题是否准确', '目标时长是否合理', '创作方向是否清楚'] } },
  { id: 'script_writer', name: 'Script Writer', displayName: '脚本编剧', stage: 'script', goal: '生成中文口播脚本，控制节奏、观点和表达。', allowedTools: ['video_script_generator', 'script_quality_checker'], requiredOutputs: ['VIDEO_SCRIPT'], humanReview: { required: true, reviewFocus: ['开头是否有吸引力', '表达是否自然', '时长是否合理'] } },
  { id: 'storyboard_artist', name: 'Storyboard Artist', displayName: '卡片设计师', stage: 'storyboard', goal: '把脚本拆成画面页、卡片、字幕与节奏。', allowedTools: ['card_plan_generator', 'caption_splitter'], requiredOutputs: ['CARD_PLAN'], humanReview: { required: true, reviewFocus: ['卡片节奏是否顺畅', '字幕是否适合阅读'] } },
  { id: 'composition_director', name: 'Composition Director', displayName: '结构导演', stage: 'composition', goal: '生成时间轴、画面轨道和安全区布局。', allowedTools: ['video_composition_builder', 'composition_quality_checker'], requiredOutputs: ['VIDEO_COMPOSITION_SPEC'], humanReview: { required: true, reviewFocus: ['结构是否可渲染', '版式是否清晰'] } },
  { id: 'reference_selector', name: 'Reference Selector', displayName: '参考选择', stage: 'reference', goal: '选择背景、图标、字体和参考资产策略。', allowedTools: ['reference_asset_planner'], requiredOutputs: ['REFERENCE_ASSET_PLAN', 'STYLE_PROFILE'] },
  { id: 'continuity_keeper', name: 'Continuity Keeper', displayName: '连续性检查', stage: 'continuity', goal: '维护风格、术语、产物依赖与下游失效规则。', allowedTools: ['continuity_checker', 'stale_tracker'], requiredOutputs: ['CONTINUITY_REPORT'] },
  { id: 'preview_director', name: 'Preview Director', displayName: '预览导演', stage: 'preview', goal: '生成本地预览图，检查可读性和版式。', allowedTools: ['hyperframes_project_generator', 'hyperframes_snapshot', 'preview_quality_checker'], requiredOutputs: ['HYPERFRAMES_PROJECT', 'PREVIEW_SNAPSHOTS'], humanReview: { required: true, reviewFocus: ['画面是否可读', '文字是否溢出', '是否允许进入最终渲染'] } },
  { id: 'render_producer', name: 'Render Producer', displayName: '渲染制片', stage: 'render', goal: '检查渲染依赖，创建本地渲染任务并追踪状态。', allowedTools: ['render_dependency_guard', 'hyperframes_renderer', 'local_job_status_tracker'], requiredOutputs: ['VIDEO', 'RENDER_REPORT'], humanReview: { required: true, reviewFocus: ['预览是否已确认', '渲染依赖是否完整'] } },
  { id: 'quality_reviewer', name: 'Quality Reviewer', displayName: '质量审核', stage: 'quality', goal: '检查文件、时长、分辨率、视频流和产物完整性。', allowedTools: ['ffmpeg_probe', 'final_review_generator'], requiredOutputs: ['FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'] },
  { id: 'package_producer', name: 'Package Producer', displayName: '交付制片', stage: 'package', goal: '打包最终视频、结构说明、预览图、决策日志和审核报告。', allowedTools: ['artifact_packager'], requiredOutputs: ['PROJECT_PACKAGE'] },
]

export default function DirectorStudioPage({ user, onLogout, serviceStatus }: Props) {
  const [activeNav, setActiveNav] = useState<DirectorNavKey>('overview')
  const [topic, setTopic] = useState('')
  const [durationSec, setDurationSec] = useState(45)
  const [roleAgents, setRoleAgents] = useState<VideoRoleAgent[]>(fallbackRoles)
  const [run, setRun] = useState<AgentRun | null>(null)
  const [reviews, setReviews] = useState<AgentReviewItem[]>([])
  const [trace, setTrace] = useState<unknown>(null)
  const [preflight, setPreflight] = useState<PreflightResponse | null>(null)
  const [feedback, setFeedback] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [errorDetail, setErrorDetail] = useState<DirectorErrorDetail | undefined>(undefined)
  const [optimisticRunningStageId, setOptimisticRunningStageId] = useState<string | undefined>()

  const refreshRun = useCallback(async (runId: string) => {
    const [nextRun, nextReviews, nextTrace] = await Promise.all([
      getAgentRun(runId).catch(() => null),
      getAgentRunReviews(runId).catch(() => ({ runId, reviews: [] })),
      getAgentRunTrace(runId).catch(() => null),
    ])
    if (nextRun) setRun(nextRun)
    setReviews(nextReviews.reviews || [])
    setTrace(nextTrace)
  }, [])

  useEffect(() => {
    let mounted = true
    Promise.all([
      fetchVideoRoleAgents().catch(() => ({ roleAgents: fallbackRoles })),
      fetchVideoPreflight('wf-guided-image-text-video').catch(() => null),
    ]).then(([roles, nextPreflight]) => {
      if (!mounted) return
      if (roles.roleAgents?.length) setRoleAgents(roles.roleAgents)
      if (nextPreflight) setPreflight(nextPreflight)
    })
    return () => { mounted = false }
  }, [])

  useEffect(() => {
    if (!run?.id || run.status === 'FAILED') return undefined
    const timer = window.setInterval(() => {
      refreshRun(run.id).catch(() => {})
    }, 2500)
    return () => window.clearInterval(timer)
  }, [refreshRun, run?.id, run?.status])

  const projectStarted = Boolean(run?.id) || loading
  const stages = useMemo(() => buildDirectorStages(roleAgents, reviews, trace, projectStarted), [roleAgents, reviews, trace, projectStarted])
  const displayStages = useMemo(() => applyOptimisticRunningStage(stages, optimisticRunningStageId), [stages, optimisticRunningStageId])
  const artifacts = useMemo(() => buildDirectorArtifacts(roleAgents, reviews, trace), [roleAgents, reviews, trace])
  const traceNodes = useMemo(() => buildDirectorTraceNodes(trace), [trace])
  const nextAction = useMemo(() => deriveNextAction(displayStages), [displayStages])
  const pendingReviews = reviews.filter(isActionablePendingReview)
  const activeReview = pendingReviews[0]
  const activeReviewStage = activeReview ? displayStages.find((stage) => stage.reviewId === activeReview.id || stage.id === activeReview.roleAgentId || stage.stage === activeReview.stage) : undefined
  const canStart = preflight?.canStart !== false && !loading

  useEffect(() => {
    if (!optimisticRunningStageId) return
    const stage = stages.find((item) => item.id === optimisticRunningStageId)
    if (!stage || stage.status !== 'pending') setOptimisticRunningStageId(undefined)
  }, [optimisticRunningStageId, stages])

  const handleStart = async () => {
    if (!topic.trim()) return
    setLoading(true)
    setError(null)
    setErrorDetail(undefined)
    try {
      const result = await startAgentRun({
        message: `请帮我创作一个${durationSec}秒图文视频：${topic.trim()}`,
        domain: 'video_creation',
        mode: 'dynamic_agent',
        context: { topic: topic.trim(), durationSec, targetDurationSec: durationSec },
      })
      setOptimisticRunningStageId(undefined)
      await refreshRun(result.runId)
      setActiveNav('review')
    } catch (err) {
      setError(normalizeDirectorErrorMessage(err))
      setErrorDetail(extractDirectorErrorDetail(err))
    } finally {
      setLoading(false)
    }
  }

  const actOnReview = async (action: 'approve' | 'reject' | 'edit' | 'regenerate') => {
    if (!run?.id || !activeReview) return
    setLoading(true)
    setError(null)
    setErrorDetail(undefined)
    const nextOptimisticStageId = action === 'approve' ? nextStageIdAfterReview(stages, activeReview) : undefined
    if (nextOptimisticStageId) setOptimisticRunningStageId(nextOptimisticStageId)
    try {
      if (action === 'approve') await approveAgentReview(run.id, activeReview.id, feedback || undefined)
      if (action === 'reject') await rejectAgentReview(run.id, activeReview.id, feedback || '请根据审核意见重新生成。')
      if (action === 'edit') await submitEditedArtifact(run.id, activeReview.id, { editedContent: feedback || topic }, feedback || undefined)
      if (action === 'regenerate') await regenerateAgentStage(run.id, activeReview.id, feedback || undefined)
      setFeedback('')
      await refreshRun(run.id)
    } catch (err) {
      if (nextOptimisticStageId) setOptimisticRunningStageId(undefined)
      setError(normalizeDirectorErrorMessage(err))
      setErrorDetail(extractDirectorErrorDetail(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="director-root flex min-h-screen gap-5 p-5">
      <DirectorSidebar active={activeNav} setActive={setActiveNav} user={user} onLogout={onLogout} />
      <main className="min-w-0 flex-1 p-5 pr-6">
        <TopBar preflight={preflight} serviceStatus={serviceStatus} run={run} />
        {error && (
          <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            <div className="flex items-center gap-2 font-semibold">
              <FiShield /> {error}
            </div>
            {errorDetail && (
              <details className="mt-3 border-t border-red-200 pt-3">
                <summary className="cursor-pointer text-xs font-semibold text-red-500 hover:text-red-700">高级详情</summary>
                <div className="mt-2 space-y-1.5 text-xs">
                  <div><span className="font-bold">错误码：</span>{errorDetail.code}</div>
                  {errorDetail.nodeId && <div><span className="font-bold">节点ID：</span>{errorDetail.nodeId}</div>}
                  {errorDetail.artifactKind && <div><span className="font-bold">产物类型：</span>{errorDetail.artifactKind}</div>}
                  {errorDetail.unitId && <div><span className="font-bold">单元ID：</span>{errorDetail.unitId}</div>}
                  {errorDetail.rawMessage && <div className="break-all"><span className="font-bold">原始错误：</span>{errorDetail.rawMessage}</div>}
                </div>
              </details>
            )}
          </div>
        )}
        <div className="mt-6">
          {activeNav === 'overview' && (
            <OverviewPage
              topic={topic}
              durationSec={durationSec}
              loading={loading}
              canStart={canStart}
              stages={displayStages}
              artifacts={artifacts}
              preflight={preflight}
              nextAction={nextAction}
              onTopicChange={setTopic}
              onDurationChange={setDurationSec}
              onStart={handleStart}
              onGoReview={() => setActiveNav('review')}
            />
          )}
          {activeNav === 'review' && (
            <ReviewPage
              review={activeReview}
              stage={activeReviewStage}
              feedback={feedback}
              loading={loading}
              onFeedbackChange={setFeedback}
              onAction={actOnReview}
              allReviews={reviews || []}
              stages={displayStages}
            />
          )}
          {activeNav === 'trace' && <TracePage traceNodes={traceNodes} artifacts={artifacts} run={run} />}
          {activeNav === 'assets' && <AssetsPage artifacts={artifacts} />}
          {activeNav === 'roles' && <RolesPage stages={displayStages} />}
          {activeNav === 'export' && <ExportPage artifacts={artifacts} topic={topic} durationSec={durationSec} />}
          {activeNav === 'system' && <DesktopPage />}
        </div>
      </main>
    </div>
  )
}

function DirectorSidebar({ active, setActive, user, onLogout }: { active: DirectorNavKey; setActive: (key: DirectorNavKey) => void; user: AuthUser; onLogout: () => void }) {
  return (
    <aside className="glass sticky top-5 flex h-[calc(100vh-40px)] w-72 shrink-0 flex-col rounded-xl p-4">
      <div className="flex items-center gap-3">
        <div className="relative h-10 w-10 overflow-hidden rounded-lg shadow-glow">
          <img src={APP_ICON_PATH} alt="躺营" className="h-full w-full object-cover" />
          <span className="absolute -right-1 -top-1 h-3 w-3 rounded-full border-2 border-white bg-success" />
        </div>
        <div>
          <div className="text-base font-black text-ink">躺营导演台</div>
          <div className="text-[11px] font-medium text-ink-soft">AI 多角色视频创作工作台</div>
        </div>
      </div>
      <nav className="mt-8 space-y-1">
        {navItems.map((item) => {
          const Icon = item.icon
          const isActive = active === item.key
          return (
            <button
              key={item.key}
              onClick={() => setActive(item.key)}
              className={clsx(
                'flex w-full items-center gap-3 rounded-lg px-4 py-3 text-left text-sm font-semibold transition',
                isActive ? 'bg-primary text-white shadow-glow' : 'text-ink-muted hover:bg-primary-soft hover:text-primary-dark',
              )}
            >
              <Icon className="text-lg" />
              <span>{item.label}</span>
            </button>
          )
        })}
      </nav>
      <div className="mt-auto rounded-lg bg-white/70 p-4 ring-1 ring-line">
        <div className="flex items-center gap-3">
          <div className="grid h-11 w-11 place-items-center rounded-full bg-gradient-to-br from-primary-dark to-primary font-black text-white">
            {(user.nickname || user.email || '用').slice(0, 1).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-bold text-ink">{user.nickname || '项目负责人'}</div>
            <div className="truncate text-xs text-ink-soft">{user.email}</div>
          </div>
          <button className="rounded-lg p-2 text-ink-soft hover:bg-white hover:text-primary-dark" title="退出登录" onClick={onLogout}>
            <FiLogOut />
          </button>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-2 text-xs">
          <div className="rounded-lg bg-green-50 px-3 py-2 text-green-700">本地在线</div>
          <div className="rounded-lg bg-amber-50 px-3 py-2 text-primary-dark">v1.0 内测</div>
        </div>
      </div>
    </aside>
  )
}

function TopBar({ preflight, serviceStatus, run }: { preflight: PreflightResponse | null; serviceStatus: string; run: AgentRun | null }) {
  return (
    <header className="flex items-center justify-between">
      <div>
        <div className="flex items-center gap-2 text-sm font-semibold text-primary-dark">
          <FiZap /> 多角色协作 · 可追踪 · 分阶段确认 · 本地可控渲染
        </div>
        <h1 className="mt-2 text-4xl font-black text-gradient">躺营导演台 v1.0</h1>
      </div>
      <div className="flex items-center gap-3">
        <div className="hidden items-center gap-2 rounded-lg bg-white/75 px-4 py-3 text-sm text-ink-muted ring-1 ring-line xl:flex">
          <FiSearch /> 搜索项目、产物、过程事件
        </div>
        <StatusPill ok={preflight?.canStart !== false && serviceStatus !== 'unhealthy'} label={serviceStatus === 'unhealthy' ? '服务未连接' : preflight?.status === 'blocked' ? '环境待处理' : '执行环境就绪'} />
        {run && <div className="rounded-lg bg-white/80 px-4 py-3 text-xs font-bold text-ink-muted ring-1 ring-line">Run {run.id.slice(0, 8)}</div>}
        <button className="rounded-lg bg-white/80 p-3 text-ink-muted ring-1 ring-line hover:text-primary-dark" title="命令面板"><FiBell /></button>
      </div>
    </header>
  )
}

function OverviewPage(props: {
  topic: string
  durationSec: number
  loading: boolean
  canStart: boolean
  stages: DirectorStage[]
  artifacts: DirectorArtifactRecord[]
  preflight: PreflightResponse | null
  nextAction?: ReturnType<typeof deriveNextAction>
  onTopicChange: (value: string) => void
  onDurationChange: (value: number) => void
  onStart: () => void
  onGoReview: () => void
}) {
  const { topic, durationSec, loading, canStart, stages, artifacts, preflight, nextAction, onTopicChange, onDurationChange, onStart, onGoReview } = props
  const staleNames = artifacts.filter((artifact) => artifact.status === 'stale').map((artifact) => artifact.name)
  return (
    <div className="space-y-5">
      <div className="grid grid-cols-12 gap-5">
        <section className="card col-span-8 p-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="grid h-10 w-10 place-items-center rounded-xl bg-primary-soft text-primary-dark"><FiZap /></div>
              <div>
                <p className="text-sm font-bold text-primary-dark">创建新项目</p>
                <p className="text-xs text-ink-soft">一句话描述你想做的视频，AI 会自动规划执行流程</p>
              </div>
            </div>
            <StatusBadge status={stages.some((stage) => stage.status === 'running' || stage.status === 'review') ? 'active' : 'pending'} />
          </div>
          <textarea
            className="mt-5 h-36 w-full resize-none rounded-xl border-2 border-line bg-white p-5 text-base leading-7 text-ink outline-none transition placeholder:text-ink-soft/60 focus:border-primary focus:shadow-glow"
            value={topic}
            onChange={(event) => onTopicChange(event.target.value)}
            placeholder="输入你想制作的视频主题..."
          />
          <div className="mt-4 flex items-center gap-3">
            <select className="rounded-lg border border-line bg-white px-4 py-2.5 text-sm text-ink" value={durationSec} onChange={(event) => onDurationChange(Number(event.target.value))}>
              {[30, 45, 60, 90, 120].map((duration) => <option key={duration} value={duration}>{duration} 秒</option>)}
            </select>
            <button disabled={!canStart || !topic.trim()} onClick={onStart} className="flex items-center gap-2 rounded-lg bg-primary px-6 py-2.5 text-sm font-bold text-white shadow-glow transition hover:bg-primary-dark disabled:cursor-not-allowed disabled:opacity-50">
              <FiPlay /> {loading ? '启动中...' : '开始项目'}
            </button>
            {preflight?.blockers?.length ? <span className="text-xs font-semibold text-red-700">{preflight.blockers[0].message}</span> : null}
          </div>
        </section>
        <section className="card col-span-4 p-6">
          <p className="text-sm font-bold text-primary-dark">项目状态</p>
          <h3 className="mt-2 text-xl font-black text-ink">{nextAction?.label || '准备开始'}</h3>
          <p className="mt-3 text-sm leading-6 text-ink-muted">{nextAction?.description || '输入需求后开始动态 Agent 创作线。'}</p>
          <button onClick={onGoReview} className="mt-5 flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-5 py-3 text-sm font-black text-white shadow-glow transition hover:bg-primary-dark">
            {stages.some((stage) => stage.status === 'review') ? '前往审核' : '查看工作台'} <FiChevronRight />
          </button>
        </section>
      </div>
      <StageFlow stages={stages} />
      {staleNames.length > 0 && (
        <div className="card border-red-200 bg-red-50/80 p-5">
          <h3 className="text-base font-black text-red-800">下游产物已过期，需要重新生成</h3>
          <div className="mt-3 flex flex-wrap gap-2">
            {staleNames.map((name) => <span key={name} className="rounded-full bg-white px-3 py-1 text-xs font-bold text-red-700 ring-1 ring-red-200">{name}</span>)}
          </div>
        </div>
      )}
      <div className="grid grid-cols-4 gap-5">
        <InfoCard icon={<FiShield />} title="当前角色" value={nextAction?.label || '待启动'} desc="角色边界由后端 StageGuard 校验。" tone="primary" />
        <InfoCard icon={<FiHardDrive />} title="本地执行器" value={preflight?.capabilityMenu.localRunner.available ? '可用' : '待检测'} desc="HyperFrames 预览与渲染走本地执行面。" tone="green" />
        <InfoCard icon={<FiRefreshCw />} title="会话状态" value="可恢复" desc="Run、Trace、Review 由后端持久化。" tone="blue" />
        <InfoCard icon={<FiEdit3 />} title="产物治理" value="产物索引" desc="上游修改后下游产物按上下文标记过期。" tone="violet" />
      </div>
    </div>
  )
}

function StageFlow({ stages }: { stages: DirectorStage[] }) {
  return (
    <div className="card p-5">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h3 className="text-lg font-black text-ink">多角色创作流程</h3>
          <p className="mt-1 text-sm text-ink-soft">每个角色只处理自己的阶段，产物可审核、可追踪、可恢复。</p>
        </div>
        <span className="rounded-full bg-primary-soft px-3 py-1 text-xs font-bold text-primary-dark">{stages.length} 个角色 · {stages.filter((stage) => stage.reviewFocus.length > 0).length} 个审核门</span>
      </div>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        {stages.map((agent, index) => (
          <div key={agent.id} className={clsx('relative rounded-lg border p-3', stageTone(agent.status))}>
            <div className="flex items-center justify-between">
              <div className={clsx('grid h-8 w-8 place-items-center rounded-lg text-sm font-black', stageIconTone(agent.status))}>{stageIcon(agent.status)}</div>
              <span className="text-xs font-black text-ink-soft">{String(index + 1).padStart(2, '0')}</span>
            </div>
              <div className="mt-3 text-sm font-black text-ink">{stageActionLabel(agent.stage)}</div>
              <div className="mt-1 truncate text-xs text-ink-soft">{agent.status === 'review' ? `当前负责：${agent.displayName}` : agent.stage}</div>
            <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-white/80">
              <div className="h-full rounded-full bg-primary" style={{ width: `${agent.progress}%` }} />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function StateMachineBar({ stages }: { stages: DirectorStage[] }) {
  const runningStage = stages.find((stage) => stage.status === 'running')
  const reviewStage = stages.find((stage) => stage.status === 'review')

  return (
    <section className="card col-span-12 p-5">
      <div className="flex items-center justify-between gap-3 mb-4">
        <div>
          <h3 className="text-base font-black text-ink">任务状态机</h3>
          <p className="mt-1 text-xs text-ink-soft">
            {runningStage ? `${runningStage.displayName} 正在生成，产物完成后进入审核。` : reviewStage ? `${reviewStage.displayName} 产物已输出，等待确认。` : '审核通过后，下个角色立即进入生成中。'}
          </p>
        </div>
        <div className="flex items-center gap-3 text-xs font-semibold text-ink-soft">
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-blue-500" /> 生成中</span>
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-amber-500" /> 待审核</span>
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-green-500" /> 已通过</span>
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-stone-300" /> 等待中</span>
        </div>
      </div>
      <div className="flex items-center gap-1 overflow-x-auto px-1 py-1">
        {stages.map((item, index) => {
          const display = getStageStateDisplay(item.status)
          return (
            <Fragment key={item.id}>
              {index > 0 && (
                <span className="shrink-0 text-stone-300 text-sm font-bold px-1">→</span>
              )}
              <div
                className={clsx(
                  'relative min-w-[110px] shrink-0 rounded-xl border px-4 py-3 text-center transition-all',
                  display.colorClass,
                  display.active && 'shadow-md',
                  display.animate && 'animate-pulse',
                )}
                title={item.goal}
              >
                {/* Colored status indicator dot */}
                <div className={clsx('mx-auto h-2.5 w-2.5 rounded-full mb-2', display.dotColor)} />
                <div className="flex items-center justify-center gap-1.5">
                  <span className="text-lg">
                    {display.icon === 'check' && <FiCheck />}
                    {display.icon === 'shield' && <FiShield />}
                    {display.icon === 'refresh' && <FiRefreshCw className="animate-spin" />}
                    {display.icon === 'x' && <FiX />}
                    {display.icon === 'cpu' && <FiCpu />}
                  </span>
                </div>
                <div className="mt-1.5 text-xs font-black truncate" title={item.displayName}>
                  {stageActionLabel(item.stage)}
                </div>
                <div className="mt-0.5 text-[10px] font-semibold opacity-70">
                  {display.label}
                </div>
              </div>
            </Fragment>
          )
        })}
      </div>
    </section>
  )
}

function NowGeneratingBanner({ stages }: { stages: DirectorStage[] }) {
  const runningStage = stages.find((stage) => stage.status === 'running' || stage.status === 'active')
  const reviewStage = stages.find((stage) => stage.status === 'review')
  const allDone = stages.length > 0 && stages.every((stage) => stage.status === 'done')

  if (allDone) {
    return (
      <div className="col-span-12 rounded-xl border border-green-200 bg-green-50 px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-green-500 text-white">
            <FiCheck />
          </span>
          <div>
            <p className="text-sm font-black text-green-800">全部阶段已完成</p>
            <p className="text-xs text-green-600 mt-0.5">所有审核已通过，可在产物页查看和导出最终视频。</p>
          </div>
        </div>
      </div>
    )
  }

  if (runningStage) {
    return (
      <div className="col-span-12 rounded-xl border border-blue-200 bg-blue-50 px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="relative grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-blue-500 text-white">
            <span className="absolute inset-0 rounded-lg bg-blue-400 animate-ping opacity-30" />
            <FiRefreshCw className="animate-spin relative z-10" />
          </span>
          <div>
            <p className="text-sm font-black text-blue-800">
              系统正在生成【{runningStage.displayName}】的{stageActionLabel(runningStage.stage)}
            </p>
            <p className="text-xs text-blue-600 mt-0.5">生成完成后将自动进入审核阶段，请稍候…</p>
          </div>
        </div>
      </div>
    )
  }

  if (reviewStage) {
    return (
      <div className="col-span-12 rounded-xl border border-amber-200 bg-amber-50 px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-amber-500 text-white">
            <FiShield />
          </span>
          <div>
            <p className="text-sm font-black text-amber-800">
              【{reviewStage.displayName}】产物已输出，等待你的确认
            </p>
            <p className="text-xs text-amber-600 mt-0.5">请审核下方内容，确认后下游阶段将自动继续执行。</p>
          </div>
        </div>
      </div>
    )
  }

  // No stages active yet
  return (
    <div className="col-span-12 rounded-xl border border-stone-200 bg-stone-50 px-5 py-4 transition-all">
      <div className="flex items-center gap-3">
        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-stone-400 text-white">
          <FiCpu />
        </span>
        <div>
          <p className="text-sm font-black text-stone-700">等待任务启动</p>
          <p className="text-xs text-stone-500 mt-0.5">在概览页输入主题并启动项目，审核内容将显示在这里。</p>
        </div>
      </div>
    </div>
  )
}

function ReviewPage({ review, stage, feedback, loading, onFeedbackChange, onAction, allReviews, stages }: { review?: AgentReviewItem; stage?: DirectorStage; feedback: string; loading: boolean; onFeedbackChange: (value: string) => void; onAction: (action: 'approve' | 'reject' | 'edit' | 'regenerate') => void; allReviews: AgentReviewItem[]; stages: DirectorStage[] }) {
  const [selectedReviewId, setSelectedReviewId] = useState<string | undefined>(review?.id)
  const [activeAction, setActiveAction] = useState<string | null>(null)
  const reviewHistory = useMemo(() => visibleReviewHistory(allReviews), [allReviews])
  const selectedReview = reviewHistory.find((item) => item.id === selectedReviewId) || review || reviewHistory[0]
  const selectedStage = useMemo(() => {
    if (!selectedReview) return stage
    return stages.find((item) =>
      item.reviewId === selectedReview.id ||
      item.id === selectedReview.roleAgentId ||
      item.stage === selectedReview.stage ||
      Boolean(selectedReview.tool && item.allowedTools.includes(selectedReview.tool)),
    ) || stage
  }, [selectedReview, stage, stages])
  const primaryOutput = (selectedReview?.requiredOutputs || selectedStage?.requiredOutputs || [])[0]
  const staleAfterChange = primaryOutput ? downstreamStaleArtifacts(primaryOutput) : []
  const outputText = reviewOutputText(selectedReview)
  const qualityLines = reviewQualityReportLines(selectedReview)

  const isPending = selectedReview?.status === 'PENDING'

  useEffect(() => {
    if (review?.id && (selectedReviewId === undefined || !reviewHistory.some((item) => item.id === selectedReviewId))) {
      setSelectedReviewId(review.id)
      return
    }
    if (!review?.id && reviewHistory.length && !reviewHistory.some((item) => item.id === selectedReviewId)) {
      setSelectedReviewId(reviewHistory[0].id)
    }
  }, [review?.id, reviewHistory, selectedReviewId])

  useEffect(() => {
    if (!loading) setActiveAction(null)
    return () => { setActiveAction(null) }
  }, [loading])

  return (
    <div className="grid grid-cols-12 gap-5">
      <NowGeneratingBanner stages={stages} />
      <StateMachineBar stages={stages} />
      <section className="card col-span-12 overflow-visible p-0">
        {selectedReview ? (
          <>
            <div className="rounded-t-lg border-b border-line bg-white/70 px-6 py-5">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <p className="text-xs font-black text-primary-dark">审阅区</p>
                  <h2 className="mt-2 truncate text-2xl font-black text-ink">{reviewDisplayTitle(selectedReview)}</h2>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-xs font-semibold text-ink-soft">
                    <span className="max-w-full rounded-full bg-background-card px-2.5 py-1 ring-1 ring-line [overflow-wrap:anywhere]">{selectedReview.stage || '未标记阶段'}</span>
                    <span className="max-w-full rounded-full bg-background-card px-2.5 py-1 ring-1 ring-line [overflow-wrap:anywhere]">{selectedReview.tool || '未标记工具'}</span>
                    <span className="max-w-md rounded-full bg-background-card px-2.5 py-1 ring-1 ring-line [overflow-wrap:anywhere]" title={selectedReview.id}>{selectedReview.id}</span>
                  </div>
                </div>
                <StatusBadge
                  status={selectedReview.status === 'PENDING' ? 'review' : selectedReview.status === 'REJECTED' ? 'blocked' : 'done'}
                  label={reviewStatusLabel(selectedReview)}
                />
              </div>
            </div>
            <div className="grid grid-cols-12 gap-5 p-6">
              <div className="col-span-8 min-w-0">
                <div className="rounded-lg border border-line bg-white p-5 shadow-sm">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <span className="text-sm font-black text-ink">审核阶段产物</span>
                      <p className="mt-1 text-xs text-ink-soft">选择任一记录即可回看对应产物。</p>
                    </div>
                    {outputText ? <CopyButton value={outputText} label="复制" /> : null}
                  </div>
                  {reviewHistory.length > 1 && (
                    <div className="mt-4 flex gap-2 overflow-x-auto px-1 py-1">
                      {reviewHistory.map((item, index) => {
                        const active = selectedReview?.id === item.id
                        const pending = item.status === 'PENDING'
                        const rejected = item.status === 'REJECTED'
                        return (
                          <button
                            key={item.id}
                            onClick={() => setSelectedReviewId(item.id)}
                            className={clsx(
                              'min-w-[156px] max-w-[224px] rounded-lg border px-3 py-2 text-left text-xs shadow-sm transition focus:outline-none focus:ring-2 focus:ring-primary/30',
                              active ? 'border-primary/55 bg-primary-soft text-primary-dark' : 'border-line bg-background-card text-ink-muted hover:border-primary/30 hover:bg-white',
                            )}
                          >
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-black">{String(index + 1).padStart(2, '0')}</span>
                              <span className={clsx('h-2 w-2 shrink-0 rounded-full', pending ? 'bg-primary' : rejected ? 'bg-red-500' : 'bg-green-500')} />
                            </div>
                            <div className="mt-1 truncate font-black text-ink" title={reviewDisplayTitle(item)}>{reviewDisplayTitle(item)}</div>
                            <div className="mt-0.5 truncate text-[11px]" title={item.tool || item.id}>{reviewStatusLabel(item)}</div>
                          </button>
                        )
                      })}
                    </div>
                  )}
                  <div className="mt-4 max-h-[520px] overflow-auto rounded-lg border border-line bg-background-card p-5 shadow-inner">
                    {outputText ? <ReviewContent text={outputText} /> : <p className="text-sm text-ink-muted">当前审核记录没有可展示正文。</p>}
                  </div>
                </div>
              </div>
              <div className="col-span-4 space-y-3 self-start xl:sticky xl:top-5">
                {isPending && (
                  <section className="rounded-lg border border-primary/35 bg-white p-4 shadow-sm">
                    <div className="flex items-center gap-2">
                      <FiShield className="text-primary" />
                      <h3 className="text-base font-black text-ink">决策操作</h3>
                    </div>
                    <div className="mt-4 grid grid-cols-2 gap-2">
                      <ActionButton color="green" icon={<FiCheck />} label="通过" loadingLabel="通过中…" loading={loading && activeAction === 'approve'} disabled={loading} onClick={() => { setActiveAction('approve'); onAction('approve'); }} />
                      <ActionButton color="red" icon={<FiX />} label="驳回" loadingLabel="驳回中…" loading={loading && activeAction === 'reject'} disabled={loading} onClick={() => { setActiveAction('reject'); onAction('reject'); }} />
                      <ActionButton color="amber" icon={<FiEdit3 />} label="修改提交" loadingLabel="提交中…" loading={loading && activeAction === 'edit'} disabled={loading} onClick={() => { setActiveAction('edit'); onAction('edit'); }} />
                      <ActionButton color="violet" icon={<FiRefreshCw />} label="重新生成" loadingLabel="重新生成中…" loading={loading && activeAction === 'regenerate'} disabled={loading} onClick={() => { setActiveAction('regenerate'); onAction('regenerate'); }} />
                    </div>
                    <label className="mt-4 block text-sm font-black text-ink">反馈意见</label>
                    <textarea
                      value={feedback}
                      onChange={(event) => onFeedbackChange(event.target.value)}
                      placeholder="请输入审核意见或修改建议..."
                      className="mt-2 h-28 w-full resize-none rounded-lg border border-line bg-background-card p-3 text-sm leading-6 outline-none focus:border-primary"
                    />
                  </section>
                )}
                <Panel title="审核原因" items={[selectedReview.reviewReason || '等待人工确认后放行下游阶段。']} />
                {qualityLines.length > 0 && <Panel title="质量门禁" items={qualityLines} />}
                <Panel title="审核重点" items={selectedStage?.reviewFocus?.length ? selectedStage.reviewFocus : ['产物是否符合创作目标', '是否允许进入下游阶段']} />
                <Panel title="输入产物" items={selectedReview.requiredInputs || selectedStage?.requiredInputs || []} />
                <Panel title="输出产物" items={selectedReview.requiredOutputs || selectedStage?.requiredOutputs || []} />
                {staleAfterChange.length > 0 && <Panel title="修改后需重做" items={staleAfterChange} />}
              </div>
            </div>
          </>
        ) : (
          <div className="grid min-h-[460px] place-items-center p-6 text-center">
            <div>
              <div className="mx-auto grid h-12 w-12 place-items-center rounded-lg bg-background-card text-primary-dark ring-1 ring-line"><FiShield /></div>
              <h3 className="mt-4 text-lg font-black text-ink">暂无待回看的审核内容</h3>
              <p className="mt-2 text-sm text-ink-muted">启动项目后，当前待审和已审核产物会按阶段沉淀在左侧记录里。</p>
            </div>
          </div>
        )}
      </section>
    </div>
  )
}

function TracePage({ traceNodes, artifacts, run }: { traceNodes: DirectorTraceNode[]; artifacts: DirectorArtifactRecord[]; run: AgentRun | null }) {
  const [selectedId, setSelectedId] = useState<string | undefined>(traceNodes[0]?.id)
  const selected = traceNodes.find((node) => node.id === selectedId) || traceNodes[0]
  useEffect(() => {
    if (!selectedId && traceNodes[0]) setSelectedId(traceNodes[0].id)
  }, [selectedId, traceNodes])

  const selectedHasError = traceNodeHasError(selected)

  return (
    <div className="grid grid-cols-12 gap-5">
      <section className="card col-span-8 p-6">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-bold text-primary-dark">过程追踪 · 调试视图</p>
            <h2 className="mt-2 text-2xl font-black text-ink">执行追踪</h2>
          </div>
          <span className="rounded-full bg-primary-soft px-3 py-1 text-xs font-bold text-primary-dark">Run {run?.id?.slice(0, 12) || '未启动'}</span>
        </div>
        <div className="mt-6 grid grid-cols-2 gap-3 xl:grid-cols-4 [&>*]:min-w-0">
          {traceNodes.length ? traceNodes.map((node, index) => (
            <button key={node.id} onClick={() => setSelectedId(node.id)}
              className={clsx(
                'rounded-lg border p-4 text-left transition hover:-translate-y-0.5 min-w-0 overflow-hidden',
                selected?.id === node.id ? 'border-primary bg-primary-soft shadow-card' : 'border-line bg-white/70',
                traceNodeHasError(node) && 'border-red-300 bg-red-50/60',
              )}>
              <div className="flex items-center justify-between gap-2 min-w-0">
                <span className={clsx('grid h-8 w-8 shrink-0 place-items-center rounded-lg text-xs font-black text-white', traceNodeHasError(node) ? 'bg-red-500' : 'bg-ink')}>
                  {String(index + 1).padStart(2, '0')}
                </span>
                <StatusBadge status={node.status} />
              </div>
              <div className="mt-3 truncate text-sm font-black text-ink leading-tight" title={node.tool}>{node.tool}</div>
              <div className="mt-1 flex items-center gap-2 min-w-0">
                <span className="truncate text-[11px] text-ink-soft font-mono" title={node.rawName}>{node.rawName}</span>
                {node.rawType && <span className="shrink-0 rounded bg-ink/10 px-1 py-0.5 text-[10px] font-bold text-ink-soft">{node.rawType}</span>}
              </div>
              {node.duration !== '-' && <div className="mt-1 text-[10px] text-ink-muted">{node.duration}</div>}
              {node.error && <div className="mt-2 truncate text-[11px] font-semibold text-red-600" title={node.error}>⚠ {node.error.slice(0, 50)}</div>}
            </button>
          )) : <EmptyState text="还没有执行 trace。启动项目后，每个步骤会显示在这里。" />}
        </div>
        <ArtifactTable artifacts={artifacts} compact />
      </section>
      <aside className="col-span-4 space-y-5">
        <section className="card p-6">
            <div className="flex min-w-0 items-center gap-3">
              <div className={clsx('rounded-lg p-3', selectedHasError ? 'bg-red-50 text-red-600' : 'bg-primary-soft text-primary-dark')}>
                {selectedHasError ? <FiShield /> : <FiActivity />}
              </div>
            <div className="min-w-0">
              <p className="text-sm text-ink-soft">节点详情</p>
              <h3 className="text-lg font-black text-ink [overflow-wrap:anywhere]">{selected?.tool || '-'}</h3>
            </div>
          </div>
          <div className="mt-5 space-y-2 text-sm">
            <DebugField label="节点 ID" value={(selected?.id || '-').replace(/^[a-z0-9]+-/, '')} mono />
            <DebugField label="步骤类型" value={selected?.rawType || '-'} />
            <DebugField label="工具名" value={selected?.rawName || '-'} mono />
            <DebugField label="状态" value={selected?.status || '-'} />
            <DebugField label="执行位置" value={selected ? (selected.plane === 'local' ? '本地' : '云端') : '-'} />
            {selected?.duration && selected.duration !== '-' && <DebugField label="耗时" value={selected.duration} />}
            {selected?.createdAt && <DebugField label="创建时间" value={selected.createdAt} mono />}
            <DebugField label="输入" value={selected?.input || '-'} />
            <DebugField label="输出" value={selected?.output || '-'} />
          </div>
          {selected?.error && (
            <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-4">
              <div className="text-xs font-black text-red-700 mb-2">错误信息</div>
              <pre className="whitespace-pre-wrap break-words text-xs text-red-600">{selected.error}</pre>
            </div>
          )}
        </section>
        <section className="card p-6">
          <div className="flex items-center gap-2"><FiCpu className="text-primary" /><h3 className="text-lg font-black text-ink">时间线</h3></div>
          <div className="mt-4 space-y-2 text-xs">
            {traceNodes.length ? traceNodes.map((node) => (
              <div key={node.id}
                onClick={() => setSelectedId(node.id)}
                className={clsx('cursor-pointer rounded-lg p-3 ring-1 transition min-w-0 overflow-hidden', traceNodeHasError(node) ? 'bg-red-50 ring-red-200' : 'bg-white ring-line', selected?.id === node.id && 'ring-primary bg-primary-soft')}>
                <div className="flex items-center justify-between gap-2 min-w-0">
                  <span className={clsx('truncate font-bold', traceNodeHasError(node) ? 'text-red-700' : 'text-ink')} title={node.tool}>{node.tool}</span>
                  <StatusBadge status={node.status} />
                </div>
                <div className="mt-1 truncate text-ink-soft">{node.output || node.rawName}</div>
              </div>
            )) : <div className="text-ink-muted">暂无事件</div>}
          </div>
        </section>
      </aside>
    </div>
  )
}

function AssetsPage({ artifacts }: { artifacts: DirectorArtifactRecord[] }) {
  const staleCount = artifacts.filter((a) => a.status === 'stale').length
  return (
    <div className="space-y-5">
      <section className="card p-6"><p className="text-sm font-bold text-primary-dark">产物库</p><h2 className="mt-2 text-3xl font-black text-ink">产物索引</h2><p className="mt-2 text-sm text-ink-muted">记录每个中间产物的版本、状态、依赖、审核和本地/云端路径。</p></section>
      {staleCount > 0 && <div className="rounded-lg bg-amber-50 p-4 text-sm font-semibold text-primary-dark ring-1 ring-amber-200">⚠ 有 {staleCount} 个下游产物已过期。上游产物被修改、驳回或重新生成后，下游产物需要重新生成才能使用。</div>}
      <ArtifactTable artifacts={artifacts} />
    </div>
  )
}

function RolesPage({ stages }: { stages: DirectorStage[] }) {
  return (
    <div className="space-y-5">
      <section className="card p-6">
        <p className="text-sm font-bold text-primary-dark">团队与角色</p>
        <h2 className="mt-2 text-3xl font-black text-ink">智能体多角色创作团队</h2>
        <p className="mt-2 text-sm leading-6 text-ink-muted">每个角色都有固定职责、工具权限、输入产物、输出产物和审核边界。阶段守卫防止角色越权调用工具。</p>
      </section>
      <div className="grid grid-cols-2 gap-5 xl:grid-cols-3">
        {stages.map((role) => (
          <div key={role.id} className="card p-5">
            <div className="flex items-start justify-between"><div className="flex items-center gap-3"><div className="grid h-12 w-12 place-items-center rounded-lg tangying-gradient text-white"><FiUserCheck /></div><div><h3 className="font-black text-ink">{role.displayName}</h3><p className="text-xs text-ink-soft">{role.name}</p></div></div><StatusBadge status={role.status} /></div>
            <p className="mt-4 min-h-12 text-sm leading-6 text-ink-muted">{role.goal}</p>
            <div className="mt-4"><b className="text-xs text-ink-soft">允许工具</b><div className="mt-2 flex flex-col gap-2">{role.allowedTools.map((tool) => <span key={tool} className="rounded-full bg-primary-soft px-2.5 py-1 text-[11px] font-bold text-primary-dark">{tool}</span>)}</div></div>
            <div className="mt-4 flex items-center gap-2 text-xs text-ink-muted"><FiLock /> 输出：{role.requiredOutputs.join(' / ') || '-'}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function ExportPage({ artifacts, topic, durationSec }: { artifacts: DirectorArtifactRecord[]; topic: string; durationSec: number }) {
  const video = artifacts.find((artifact) => artifact.kind === 'VIDEO')
  const packageArtifact = artifacts.find((artifact) => artifact.kind === 'PROJECT_PACKAGE')
  const videoReady = video?.status === 'valid' && Boolean(video.storageRef)
  const packageReady = packageArtifact?.status === 'valid' && Boolean(packageArtifact.storageRef)
  const publishCopies = buildPublishCopies(topic, durationSec, artifacts)
  const markdown = publishCopiesToMarkdown(publishCopies)
  const json = publishCopiesToJSON(publishCopies)
  return (
    <div className="space-y-5">
      <div className="grid grid-cols-1 gap-5 xl:grid-cols-12">
      <section className="card p-6 xl:col-span-7">
        <div className="flex items-center justify-between"><div><p className="text-sm font-bold text-primary-dark">最终预览 / 导出</p><h2 className="mt-2 text-2xl font-black text-ink">最终视频预览</h2></div><StatusBadge status={videoReady ? 'valid' : 'pending'} label={videoReady ? 'final.mp4 已生成' : '等待渲染'} /></div>
        <div className="mt-6 overflow-hidden rounded-xl bg-ink shadow-card ring-1 ring-line">
          <div className="relative h-[410px] bg-[radial-gradient(circle_at_70%_30%,rgba(251,191,36,.34),transparent_28%),linear-gradient(135deg,#130b05,#2b1708_40%,#7c3e08)] p-10 text-white">
            <div className="relative z-10 flex h-full flex-col justify-between">
              <div><span className="rounded-full bg-white/15 px-4 py-2 text-xs font-bold ring-1 ring-white/20">智能视频创作工作台</span><h3 className="mt-10 max-w-lg text-5xl font-black">智能体<br />改变的是工作流</h3><p className="mt-5 text-lg text-amber-100">连接工具 · 协同团队 · 释放创造力</p></div>
              <div className="flex items-center gap-4 rounded-lg bg-black/25 p-4 ring-1 ring-white/10"><FiPlayCircle className="text-3xl" /><div className="h-1 flex-1 overflow-hidden rounded-full bg-white/20"><div className="h-full w-[28%] rounded-full bg-primary-light" /></div><span className="text-sm">0:00 / 0:45</span></div>
            </div>
          </div>
        </div>
      </section>
      <aside className="space-y-5 xl:col-span-5">
        <section className="card p-6">
          <h3 className="text-lg font-black text-ink">导出操作</h3>
          {!videoReady && <div className="mb-3 rounded-lg bg-amber-50 p-3 text-xs font-semibold text-primary-dark ring-1 ring-amber-200">最终视频尚未生成</div>}
          <div className="mt-4 grid grid-cols-2 gap-3"><button disabled={!videoReady} className="flex items-center justify-center gap-2 rounded-lg bg-primary px-4 py-3 text-sm font-black text-white shadow-glow disabled:cursor-not-allowed disabled:opacity-45"><FiPlayCircle /> 预览视频</button><button disabled={!videoReady} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"><FiFolder /> 打开文件夹</button></div>
          <button disabled={!videoReady} className="mt-3 flex w-full items-center justify-center gap-2 rounded-lg bg-violet px-4 py-3 text-sm font-black text-white disabled:cursor-not-allowed disabled:opacity-45"><FiDownload /> {packageReady ? '下载交付包' : '导出交付包'}</button>
          <div className="mt-3 grid grid-cols-2 gap-3">
            <button onClick={() => downloadTextFile('publish-copy.md', markdown, 'text/markdown')} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line"><FiFileText /> Markdown</button>
            <button onClick={() => downloadTextFile('publish-copy.json', json, 'application/json')} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line"><FiDownload /> JSON</button>
          </div>
        </section>
        <section className="card p-6">
          <h3 className="text-lg font-black text-ink">交付物清单</h3>
          <div className="mt-4 space-y-3 text-sm">
            {artifacts.filter((artifact) => ['VIDEO', 'PROJECT_PACKAGE', 'RENDER_REPORT', 'FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'].includes(artifact.kind)).map((artifact) => <div key={artifact.id} className="flex items-center justify-between rounded-lg bg-background-card px-4 py-3 ring-1 ring-line"><span>{artifact.name}</span><StatusBadge status={artifact.status} /></div>)}
          </div>
        </section>
      </aside>
      </div>
      <section className="card p-6">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-bold text-primary-dark">发布素材</p>
            <h2 className="mt-2 text-2xl font-black text-ink">小红书 / B站</h2>
          </div>
          <CopyButton value={markdown} label="复制全部" />
        </div>
        <div className="mt-5 grid grid-cols-1 gap-5 xl:grid-cols-2">
          {publishCopies.map((copy) => (
            <div key={copy.platform} className="rounded-lg bg-white p-5 ring-1 ring-line">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-black text-ink">{copy.platformName}</h3>
                <CopyButton value={JSON.stringify(copy, null, 2)} label="复制 JSON" />
              </div>
              <PublishCopyField label="标题" value={copy.title} />
              <PublishCopyField label="正文" value={copy.description} multiline />
              <PublishCopyField label="标签" value={copy.tags.map((tag) => `#${tag}`).join(' ')} />
              <PublishCopyField label="封面文案" value={copy.coverText} />
              <PublishCopyField label="发布建议" value={copy.publishTips.join('\n')} multiline />
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

function ArtifactTable({ artifacts, compact = false }: { artifacts: DirectorArtifactRecord[]; compact?: boolean }) {
  const headers = ['ID', '名称', '类型', '状态', '负责人', '操作']
  return (
    <section className={clsx('card overflow-hidden p-0', compact && 'mt-6')}>
      <table className="w-full text-left text-sm">
        <thead className="bg-background-mist text-xs text-ink-soft">
          <tr>
            {headers.map((h) => (
              <th className="whitespace-nowrap px-4 py-3" key={h}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-line bg-white/70">
          {artifacts.map((artifact) => (
            <tr key={artifact.id}>
              <td className="whitespace-nowrap px-4 py-3 font-mono text-xs font-bold">{artifact.id}</td>
              <td className="whitespace-nowrap px-4 py-3 font-semibold text-ink">{artifact.name}</td>
              <td className="whitespace-nowrap px-4 py-3 text-ink-muted">{displayNameForArtifact(artifact.kind)}</td>
              <td className="whitespace-nowrap px-4 py-3"><StatusBadge status={artifact.status} /></td>
              <td className="whitespace-nowrap px-4 py-3 text-ink-muted">{artifact.owner}</td>
              <td className="whitespace-nowrap px-4 py-3"><CopyButton value={artifactToCopyText(artifact)} label="复制" /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}

function PublishCopyField({ label, value, multiline = false }: { label: string; value: string; multiline?: boolean }) {
  return (
    <div className="mt-4 rounded-lg bg-background-card p-4 ring-1 ring-line">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-black text-primary-dark">{label}</span>
        <CopyButton value={value} label="复制" />
      </div>
      <p className={clsx('mt-2 whitespace-pre-wrap text-sm leading-6 text-ink-muted', !multiline && 'line-clamp-2')}>{value}</p>
    </div>
  )
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = async () => {
    await copyText(value)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1200)
  }
  return <button onClick={handleCopy} className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"><FiCopy /> {copied ? '已复制' : label}</button>
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

function downloadTextFile(filename: string, content: string, mimeType: string) {
  const blob = new Blob([content], { type: `${mimeType};charset=utf-8` })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

function artifactToCopyText(artifact: DirectorArtifactRecord) {
  return JSON.stringify({
    id: artifact.id,
    name: artifact.name,
    kind: artifact.kind,
    version: artifact.version,
    status: artifact.status,
    owner: artifact.owner,
    humanApproved: artifact.humanApproved,
    storageRef: artifact.storageRef,
    dependsOn: artifact.dependsOn,
  }, null, 2)
}

function StatusBadge({ status, label }: { status: DirectorArtifactStatus | DirectorStageStatus; label?: string }) {
  const labelMap: Record<DirectorArtifactStatus | DirectorStageStatus, string> = {
    done: '已完成',
    active: '生成中',
    review: '待审核',
    blocked: '已阻断',
    pending: '待开始',
    running: '生成中',
    valid: '有效',
    stale: '已过期',
    failed: '失败',
  }
  return <span className={clsx('inline-flex items-center rounded-full px-2.5 py-1 text-xs font-bold ring-1', statusBadgeTone(status))}>{label ?? labelMap[status]}</span>
}

function InfoCard({ icon, title, value, desc, tone }: { icon: React.ReactNode; title: string; value: string; desc: string; tone: 'primary' | 'green' | 'blue' | 'violet' }) {
  const toneMap = { primary: 'bg-primary-soft text-primary-dark', green: 'bg-green-50 text-green-700', blue: 'bg-blue-50 text-blue-700', violet: 'bg-violet-50 text-violet' }
  return <div className="card p-5"><div className="flex items-center gap-3"><div className={`rounded-lg p-3 ${toneMap[tone]}`}>{icon}</div><div><p className="text-sm text-ink-soft">{title}</p><p className="font-black text-ink">{value}</p></div></div><p className="mt-4 text-sm leading-6 text-ink-muted">{desc}</p></div>
}

function Panel({ title, items }: { title: string; items: string[] }) {
  return <div className="min-w-0 rounded-lg border border-line bg-background-card p-4 shadow-sm"><b className="text-sm text-ink">{title}</b><ul className="mt-3 space-y-2 text-xs text-ink-muted">{(items.length ? items : ['-']).map((item) => <li key={item} className="[overflow-wrap:anywhere]">{item}</li>)}</ul></div>
}

function ActionButton({ color, icon, label, disabled, onClick, loading = false, loadingLabel = '' }: { color: 'green' | 'red' | 'amber' | 'violet'; icon: React.ReactNode; label: string; disabled: boolean; onClick: () => void; loading?: boolean; loadingLabel?: string }) {
  const map = { green: 'border-green-200 bg-green-50 text-green-700 hover:bg-green-100', red: 'border-red-200 bg-red-50 text-red-700 hover:bg-red-100', amber: 'border-amber-200 bg-amber-50 text-primary-dark hover:bg-amber-100', violet: 'border-violet-200 bg-violet-50 text-violet hover:bg-violet-100' }
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={clsx(
        'flex min-w-0 items-center justify-center gap-2 rounded-lg border px-3 py-3 text-sm font-black leading-tight transition disabled:cursor-not-allowed',
        loading ? 'opacity-70' : '',
        disabled && !loading ? 'opacity-45' : '',
        map[color],
      )}
    >
      <span className="shrink-0">{loading ? <FiRefreshCw className="animate-spin" /> : icon}</span>
      <span className="[overflow-wrap:anywhere]">{loading && loadingLabel ? loadingLabel : label}</span>
    </button>
  )
}

function DebugField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-lg bg-background-card px-4 py-2.5 ring-1 ring-line">
      <div className="text-[11px] font-bold text-primary-dark">{label}</div>
      <div className={clsx('mt-1 whitespace-pre-wrap text-xs text-ink-muted [overflow-wrap:anywhere]', mono && 'font-mono')}>{value}</div>
    </div>
  )
}

function StatusPill({ ok, label }: { ok: boolean; label: string }) {
  return <div className={clsx('flex items-center gap-2 rounded-lg px-4 py-3 text-sm font-bold ring-1', ok ? 'bg-green-50 text-green-700 ring-green-200' : 'bg-red-50 text-red-700 ring-red-200')}><span className={clsx('h-2 w-2 rounded-full', ok ? 'bg-green-500' : 'bg-red-500')} /> {label}</div>
}

function EmptyState({ text }: { text: string }) {
  return <div className="col-span-full rounded-lg bg-white/70 p-6 text-sm text-ink-muted ring-1 ring-line">{text}</div>
}

// ReviewContent renders review text with auto-detection of JSON and Markdown.
function ReviewContent({ text }: { text: string }) {
  // Try to detect and pretty-format JSON
  const trimmed = text.trim()
  if ((trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
    try {
      const parsed = JSON.parse(trimmed)
      return <pre className="whitespace-pre-wrap rounded-lg border border-line bg-white p-4 font-mono text-xs leading-6 text-ink-muted [overflow-wrap:anywhere]">{JSON.stringify(parsed, null, 2)}</pre>
    } catch { /* not valid JSON, fall through */ }
  }

  // Render markdown-like content
  return <div className="text-sm leading-7 text-ink [overflow-wrap:anywhere]" dangerouslySetInnerHTML={{ __html: simpleMarkdown(text) }} />
}

// simpleMarkdown converts basic markdown to HTML.
function simpleMarkdown(text: string): string {
  let html = text
    // Escape HTML
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    // Headers
    .replace(/^### (.+)$/gm, '<h4 class="font-bold text-ink mt-3 mb-1">$1</h4>')
    .replace(/^## (.+)$/gm, '<h3 class="font-bold text-lg text-ink mt-4 mb-2">$1</h3>')
    .replace(/^# (.+)$/gm, '<h2 class="font-black text-xl text-ink mt-5 mb-2">$1</h2>')
    // Bold and italic
    .replace(/\*\*(.+?)\*\*/g, '<strong class="font-bold">$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    // Inline code
    .replace(/`([^`]+)`/g, '<code class="bg-amber-50 text-amber-800 px-1 rounded text-xs">$1</code>')
    // Lists
    .replace(/^- (.+)$/gm, '<li class="ml-4 list-disc">$1</li>')
    .replace(/^(\d+)\. (.+)$/gm, '<li class="ml-4 list-decimal">$1</li>')
    // Line breaks
    .replace(/\n\n/g, '</p><p class="mt-2">')
    .replace(/\n/g, '<br/>')
    // Horizontal rules
    .replace(/^---$/gm, '<hr class="my-3 border-line"/>')
    // Email/URL auto-link
    .replace(/(https?:\/\/[^\s<]+)/g, '<a href="$1" class="text-primary underline" target="_blank">$1</a>')

  return '<p class="mt-2">' + html + '</p>'
}

function stageIcon(status: DirectorStageStatus) {
  if (status === 'done') return <FiCheck />
  if (status === 'review') return <FiShield />
  if (status === 'blocked' || status === 'failed') return <FiLock />
  if (status === 'running') return <FiRefreshCw className="animate-spin" />
  if (status === 'active') return <FiPlay />
  return <FiCpu />
}

function stageTone(status: DirectorStageStatus) {
  if (status === 'review') return 'border-primary bg-primary-soft'
  if (status === 'running' || status === 'active') return 'border-blue-200 bg-blue-50'
  if (status === 'done') return 'border-green-200 bg-green-50'
  if (status === 'blocked' || status === 'failed') return 'border-red-100 bg-red-50/60'
  return 'border-line bg-white/65'
}

function stageIconTone(status: DirectorStageStatus) {
  if (status === 'done') return 'bg-green-600 text-white'
  if (status === 'review') return 'bg-primary text-white'
  if (status === 'running' || status === 'active') return 'bg-blue-600 text-white'
  if (status === 'blocked' || status === 'failed') return 'bg-red-500 text-white'
  return 'bg-stone-200 text-stone-600'
}

function statusBadgeTone(status: DirectorArtifactStatus | DirectorStageStatus) {
  if (status === 'done' || status === 'valid') return 'bg-green-50 text-green-700 ring-green-200'
  if (status === 'review') return 'bg-amber-50 text-primary-dark ring-amber-200'
  if (status === 'running' || status === 'active') return 'bg-blue-50 text-blue-700 ring-blue-200'
  if (status === 'blocked' || status === 'stale' || status === 'failed') return 'bg-red-50 text-red-700 ring-red-200'
  return 'bg-stone-50 text-stone-600 ring-stone-200'
}
