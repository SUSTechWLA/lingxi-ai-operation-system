import { useCallback, useEffect, useMemo, useState } from 'react'
import clsx from 'clsx'
import { APP_ICON_PATH } from '../utils/brand'
import {
  FiActivity,
  FiArchive,
  FiBell,
  FiCheck,
  FiChevronRight,
  FiCpu,
  FiDownload,
  FiEdit3,
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
  buildDirectorArtifacts,
  buildDirectorStages,
  buildDirectorTraceNodes,
  deriveNextAction,
  displayNameForArtifact,
  downstreamStaleArtifacts,
  stageActionLabel,
  type DirectorArtifactRecord,
  type DirectorArtifactStatus,
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
  const [topic, setTopic] = useState('请帮我做一个45秒视频，讲智能体改变的是工作流')
  const [durationSec, setDurationSec] = useState(45)
  const [roleAgents, setRoleAgents] = useState<VideoRoleAgent[]>(fallbackRoles)
  const [run, setRun] = useState<AgentRun | null>(null)
  const [reviews, setReviews] = useState<AgentReviewItem[]>([])
  const [trace, setTrace] = useState<unknown>(null)
  const [preflight, setPreflight] = useState<PreflightResponse | null>(null)
  const [feedback, setFeedback] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

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

  const stages = useMemo(() => buildDirectorStages(roleAgents, reviews, trace), [roleAgents, reviews, trace])
  const artifacts = useMemo(() => buildDirectorArtifacts(roleAgents, reviews, trace), [roleAgents, reviews, trace])
  const traceNodes = useMemo(() => buildDirectorTraceNodes(trace), [trace])
  const nextAction = useMemo(() => deriveNextAction(stages), [stages])
  const pendingReviews = reviews.filter((review) => review.status === 'PENDING')
  const activeReview = pendingReviews[0]
  const activeReviewStage = activeReview ? stages.find((stage) => stage.reviewId === activeReview.id || stage.id === activeReview.roleAgentId || stage.stage === activeReview.stage) : undefined
  const canStart = preflight?.canStart !== false && !loading

  const handleStart = async () => {
    if (!topic.trim()) return
    setLoading(true)
    setError(null)
    try {
      const result = await startAgentRun({
        message: `请帮我创作一个${durationSec}秒图文视频：${topic.trim()}`,
        domain: 'video_creation',
        mode: 'dynamic_agent',
        context: { topic: topic.trim(), durationSec, targetDurationSec: durationSec },
      })
      await refreshRun(result.runId)
      setActiveNav('review')
    } catch (err) {
      setError(errorMessage(err, '启动失败，请检查云端服务和模型配置。'))
    } finally {
      setLoading(false)
    }
  }

  const actOnReview = async (action: 'approve' | 'reject' | 'edit' | 'regenerate') => {
    if (!run?.id || !activeReview) return
    setLoading(true)
    setError(null)
    try {
      if (action === 'approve') await approveAgentReview(run.id, activeReview.id, feedback || undefined)
      if (action === 'reject') await rejectAgentReview(run.id, activeReview.id, feedback || '请根据审核意见重新生成。')
      if (action === 'edit') await submitEditedArtifact(run.id, activeReview.id, { editedContent: feedback || topic }, feedback || undefined)
      if (action === 'regenerate') await regenerateAgentStage(run.id, activeReview.id, feedback || undefined)
      setFeedback('')
      await refreshRun(run.id)
    } catch (err) {
      setError(errorMessage(err, '审核操作失败。'))
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
          <div className="mt-4 flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm font-semibold text-red-700">
            <FiShield /> {error}
          </div>
        )}
        <div className="mt-6">
          {activeNav === 'overview' && (
            <OverviewPage
              topic={topic}
              durationSec={durationSec}
              loading={loading}
              canStart={canStart}
              stages={stages}
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
            />
          )}
          {activeNav === 'trace' && <TracePage traceNodes={traceNodes} artifacts={artifacts} run={run} />}
          {activeNav === 'assets' && <AssetsPage artifacts={artifacts} />}
          {activeNav === 'roles' && <RolesPage stages={stages} />}
          {activeNav === 'export' && <ExportPage artifacts={artifacts} />}
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
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm font-bold text-primary-dark">项目总览 / 启动</p>
              <h2 className="mt-2 text-3xl font-black text-ink">智能体改变的是工作流</h2>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-ink-muted">一句话启动中文 16:9 图文视频，分阶段确认创意方案、脚本、卡片、结构、预览、渲染和交付包。</p>
            </div>
            <StatusBadge status={stages.some((stage) => stage.status === 'running' || stage.status === 'review') ? 'active' : 'pending'} />
          </div>
          <div className="mt-6 rounded-lg bg-background-mist p-4 ring-1 ring-line">
            <label className="text-sm font-black text-ink">一句话需求</label>
            <textarea
              className="mt-3 h-24 w-full resize-none rounded-lg border border-line bg-white p-3 text-sm text-ink outline-none focus:border-primary"
              value={topic}
              onChange={(event) => onTopicChange(event.target.value)}
              placeholder="例如：做一个 45 秒图文视频，讲 AI Agent 为什么改变的是工作流"
            />
            <div className="mt-3 flex items-center gap-3">
              <select className="rounded-lg border border-line bg-white px-3 py-2 text-sm text-ink" value={durationSec} onChange={(event) => onDurationChange(Number(event.target.value))}>
                {[30, 45, 60, 90, 120].map((duration) => <option key={duration} value={duration}>{duration} 秒</option>)}
              </select>
              <button disabled={!canStart || !topic.trim()} onClick={onStart} className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-bold text-white shadow-glow disabled:cursor-not-allowed disabled:opacity-50">
                <FiPlay /> {loading ? '启动中...' : '开始项目'}
              </button>
              {preflight?.blockers?.length ? <span className="text-xs font-semibold text-red-700">{preflight.blockers[0].message}</span> : null}
            </div>
          </div>
        </section>
        <section className="card col-span-4 p-6">
          <p className="text-sm font-bold text-primary-dark">下一步动作</p>
          <h3 className="mt-2 text-xl font-black text-ink">{nextAction?.label || '启动新项目'}</h3>
          <p className="mt-3 text-sm leading-6 text-ink-muted">{nextAction?.description || '输入需求后开始动态 Agent 创作线。'}</p>
          <button onClick={onGoReview} className="mt-5 flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-5 py-3 text-sm font-black text-white shadow-glow">
            进入审核工作台 <FiChevronRight />
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

function ReviewPage({ review, stage, feedback, loading, onFeedbackChange, onAction }: { review?: AgentReviewItem; stage?: DirectorStage; feedback: string; loading: boolean; onFeedbackChange: (value: string) => void; onAction: (action: 'approve' | 'reject' | 'edit' | 'regenerate') => void }) {
  const primaryOutput = (review?.requiredOutputs || stage?.requiredOutputs || [])[0]
  const staleAfterChange = primaryOutput ? downstreamStaleArtifacts(primaryOutput) : []

  return (
    <div className="grid grid-cols-12 gap-5">
      <section className="card col-span-8 p-6">
        <div className="flex items-center justify-between border-b border-line pb-5">
          <div>
            <p className="text-sm font-bold text-primary-dark">审核工作台</p>
            <h2 className="mt-2 text-2xl font-black text-ink">当前角色：{stage?.displayName || '暂无待审核'}</h2>
          </div>
          <div className="flex items-center gap-2"><StatusBadge status={review ? 'review' : 'done'} label={review ? '待审核' : '暂无阻塞'} /></div>
        </div>
        {review ? (
          <div className="mt-6 grid grid-cols-12 gap-5">
            <div className="col-span-8 rounded-lg bg-white p-6 ring-1 ring-line">
              <div className="mb-5 flex items-center justify-between">
                <h3 className="text-lg font-black text-ink">{review.reviewPhase || stage?.stage || review.tool}</h3>
                <span className="text-xs font-bold text-ink-soft">{review.id}</span>
              </div>
              <div className="space-y-4">
                <ReviewField label="工具" value={review.tool || '-'} />
                <ReviewField label="审核原因" value={review.reviewReason || '等待人工确认后放行下游阶段。'} />
                <ReviewField label="输出产物" value={(review.requiredOutputs || review.reviewArtifactKinds || []).join(' / ') || '-'} />
                <ReviewField label="阻塞下游" value={review.blocksDownstream === false ? '否' : '是'} />
              </div>
            </div>
            <div className="col-span-4 space-y-4">
              <Panel title="审核重点" items={stage?.reviewFocus.length ? stage.reviewFocus : ['产物是否符合创作目标', '是否允许进入下游阶段']} />
              <Panel title="输入产物" items={review.requiredInputs || stage?.requiredInputs || []} />
              <Panel title="输出产物" items={review.requiredOutputs || stage?.requiredOutputs || []} />
              <Panel title="修改后需重做" items={staleAfterChange} />
            </div>
          </div>
        ) : (
          <div className="mt-6 rounded-lg bg-background-card p-6 text-sm leading-7 text-ink-muted ring-1 ring-line">当前没有待审核节点。启动项目或等待当前阶段完成后，这里会显示需要你确认的产物。</div>
        )}
      </section>
      <aside className="col-span-4 space-y-5">
        <section className="card p-6">
          <h3 className="text-lg font-black text-ink">决策操作</h3>
          <div className="mt-5 grid grid-cols-2 gap-3">
            <ActionButton color="green" icon={<FiCheck />} label="通过" disabled={!review || loading} onClick={() => onAction('approve')} />
            <ActionButton color="red" icon={<FiX />} label="驳回" disabled={!review || loading} onClick={() => onAction('reject')} />
            <ActionButton color="amber" icon={<FiEdit3 />} label="修改提交" disabled={!review || loading} onClick={() => onAction('edit')} />
            <ActionButton color="violet" icon={<FiRefreshCw />} label="重新生成" disabled={!review || loading} onClick={() => onAction('regenerate')} />
          </div>
          <label className="mt-5 block text-sm font-black text-ink">反馈意见</label>
          <textarea value={feedback} onChange={(event) => onFeedbackChange(event.target.value)} placeholder="请输入审核意见或修改建议，供下个角色优化..." className="mt-3 h-36 w-full resize-none rounded-lg border border-line bg-white p-4 text-sm outline-none focus:border-primary" />
        </section>
      </aside>
    </div>
  )
}

function TracePage({ traceNodes, artifacts, run }: { traceNodes: DirectorTraceNode[]; artifacts: DirectorArtifactRecord[]; run: AgentRun | null }) {
  const [selectedId, setSelectedId] = useState<string | undefined>(traceNodes[0]?.id)
  const selected = traceNodes.find((node) => node.id === selectedId) || traceNodes[0]
  useEffect(() => {
    if (!selectedId && traceNodes[0]) setSelectedId(traceNodes[0].id)
  }, [selectedId, traceNodes])

  return (
    <div className="grid grid-cols-12 gap-5">
      <section className="card col-span-8 p-6">
        <div className="flex items-center justify-between">
          <div><p className="text-sm font-bold text-primary-dark">过程追踪 / 中间件</p><h2 className="mt-2 text-2xl font-black text-ink">工作流总览</h2></div>
          <span className="rounded-full bg-primary-soft px-3 py-1 text-xs font-bold text-primary-dark">运行编号：{run?.id.slice(0, 8) || '未启动'}</span>
        </div>
        <div className="mt-6 grid grid-cols-2 gap-3 xl:grid-cols-4">
          {traceNodes.length ? traceNodes.map((node) => (
            <button key={node.id} onClick={() => setSelectedId(node.id)} className={clsx('rounded-lg border p-4 text-left transition hover:-translate-y-0.5', selected?.id === node.id ? 'border-primary bg-primary-soft shadow-card' : 'border-line bg-white/70')}>
              <div className="flex items-center justify-between"><span className="grid h-8 w-8 place-items-center rounded-lg bg-ink text-xs font-black text-white">{node.id.slice(0, 2)}</span><StatusBadge status={node.status} /></div>
              <div className="mt-3 text-sm font-black text-ink">{node.role}</div>
              <div className="text-xs text-ink-soft">{node.tool}</div>
              <div className="mt-3 flex items-center gap-2 text-xs text-ink-muted"><FiChevronRight /> {node.output}</div>
            </button>
          )) : <EmptyState text="还没有执行 trace。启动项目后，工具调用会显示在这里。" />}
        </div>
        <ArtifactTable artifacts={artifacts} compact />
      </section>
      <aside className="col-span-4 space-y-5">
        <section className="card p-6">
          <div className="flex items-center gap-3"><div className="rounded-lg bg-primary-soft p-3 text-primary-dark"><FiActivity /></div><div><p className="text-sm text-ink-soft">中间件详情</p><h3 className="text-xl font-black text-ink">{selected?.role || '-'}</h3></div></div>
          <div className="mt-5 space-y-3 text-sm">
            <Field label="角色" value={selected?.role || '-'} />
            <Field label="工具名称" value={selected?.tool || '-'} />
            <Field label="执行平面" value={selected?.plane === 'local' ? '本地' : '云端'} />
            <Field label="输入产物" value={selected?.input || '-'} />
            <Field label="输出产物" value={selected?.output || '-'} />
            <Field label="人工审核" value={selected?.review ? '需要' : '不需要'} />
          </div>
        </section>
        <section className="card p-6">
          <div className="flex items-center gap-2"><FiCpu className="text-primary" /><h3 className="text-lg font-black text-ink">事件流</h3></div>
          <div className="mt-4 space-y-3 text-xs text-ink-muted">
            {traceNodes.slice(-5).map((node) => <div key={node.id} className="rounded-lg bg-white p-3 ring-1 ring-line">[{node.status}] {node.tool} · {node.output}</div>)}
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
            <div className="mt-4"><b className="text-xs text-ink-soft">允许工具</b><div className="mt-2 flex flex-wrap gap-2">{role.allowedTools.map((tool) => <span key={tool} className="rounded-full bg-primary-soft px-2.5 py-1 text-[11px] font-bold text-primary-dark">{tool}</span>)}</div></div>
            <div className="mt-4 flex items-center gap-2 text-xs text-ink-muted"><FiLock /> 输出：{role.requiredOutputs.join(' / ') || '-'}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function ExportPage({ artifacts }: { artifacts: DirectorArtifactRecord[] }) {
  const video = artifacts.find((artifact) => artifact.kind === 'VIDEO')
  const packageArtifact = artifacts.find((artifact) => artifact.kind === 'PROJECT_PACKAGE')
  const videoReady = video?.status === 'valid' && Boolean(video.storageRef)
  const packageReady = packageArtifact?.status === 'valid' && Boolean(packageArtifact.storageRef)
  return (
    <div className="grid grid-cols-12 gap-5">
      <section className="card col-span-7 p-6">
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
      <aside className="col-span-5 space-y-5">
        <section className="card p-6">
          <h3 className="text-lg font-black text-ink">导出操作</h3>
          {!videoReady && <div className="mb-3 rounded-lg bg-amber-50 p-3 text-xs font-semibold text-primary-dark ring-1 ring-amber-200">最终视频尚未生成</div>}
          <div className="mt-4 grid grid-cols-2 gap-3"><button disabled={!videoReady} className="flex items-center justify-center gap-2 rounded-lg bg-primary px-4 py-3 text-sm font-black text-white shadow-glow disabled:cursor-not-allowed disabled:opacity-45"><FiPlayCircle /> 预览视频</button><button disabled={!videoReady} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"><FiFolder /> 打开文件夹</button></div>
          <button disabled={!videoReady} className="mt-3 flex w-full items-center justify-center gap-2 rounded-lg bg-violet px-4 py-3 text-sm font-black text-white disabled:cursor-not-allowed disabled:opacity-45"><FiDownload /> {packageReady ? '下载交付包' : '导出交付包'}</button>
        </section>
        <section className="card p-6">
          <h3 className="text-lg font-black text-ink">交付物清单</h3>
          <div className="mt-4 space-y-3 text-sm">
            {artifacts.filter((artifact) => ['VIDEO', 'PROJECT_PACKAGE', 'RENDER_REPORT', 'FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'].includes(artifact.kind)).map((artifact) => <div key={artifact.id} className="flex items-center justify-between rounded-lg bg-background-card px-4 py-3 ring-1 ring-line"><span>{artifact.name}</span><StatusBadge status={artifact.status} /></div>)}
          </div>
        </section>
      </aside>
    </div>
  )
}

function ArtifactTable({ artifacts, compact = false }: { artifacts: DirectorArtifactRecord[]; compact?: boolean }) {
  return (
    <section className={clsx('card overflow-hidden p-0', compact && 'mt-6')}>
      <table className="w-full text-left text-sm"><thead className="bg-background-mist text-xs text-ink-soft"><tr>{['ID', '名称', '类型', '版本', '状态', '负责人', '已审核', '存储位置'].map((header) => <th className="px-5 py-4" key={header}>{header}</th>)}</tr></thead><tbody className="divide-y divide-line bg-white/70">{artifacts.map((artifact) => <tr key={artifact.id}><td className="px-5 py-4 font-bold">{artifact.id}</td><td className="px-5 py-4 font-black text-ink">{artifact.name}</td><td className="px-5 py-4 text-ink-muted">{displayNameForArtifact(artifact.kind)}</td><td className="px-5 py-4">{artifact.version}</td><td className="px-5 py-4"><StatusBadge status={artifact.status} /></td><td className="px-5 py-4 text-ink-muted">{artifact.owner}</td><td className="px-5 py-4">{artifact.humanApproved ? '是' : '否'}</td><td className="px-5 py-4 text-xs text-ink-soft">{artifact.storageRef}</td></tr>)}</tbody></table>
    </section>
  )
}

function StatusBadge({ status, label }: { status: DirectorArtifactStatus | DirectorStageStatus; label?: string }) {
  const labelMap: Record<DirectorArtifactStatus | DirectorStageStatus, string> = {
    done: '已完成',
    active: '进行中',
    review: '待审核',
    blocked: '已阻断',
    pending: '待开始',
    running: '运行中',
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
  return <div className="rounded-lg bg-background-card p-4 ring-1 ring-line"><b className="text-sm text-ink">{title}</b><ul className="mt-3 space-y-2 text-xs text-ink-muted">{(items.length ? items : ['-']).map((item) => <li key={item}>{item}</li>)}</ul></div>
}

function ReviewField({ label, value }: { label: string; value: string }) {
  return <div className="rounded-lg bg-background-card p-4 ring-1 ring-line"><span className="text-xs font-black text-primary-dark">{label}</span><p className="mt-2 text-sm leading-7 text-ink-muted">{value}</p></div>
}

function ActionButton({ color, icon, label, disabled, onClick }: { color: 'green' | 'red' | 'amber' | 'violet'; icon: React.ReactNode; label: string; disabled: boolean; onClick: () => void }) {
  const map = { green: 'border-green-200 bg-green-50 text-green-700', red: 'border-red-200 bg-red-50 text-red-700', amber: 'border-amber-200 bg-amber-50 text-primary-dark', violet: 'border-violet-200 bg-violet-50 text-violet' }
  return <button disabled={disabled} onClick={onClick} className={`flex items-center justify-center gap-2 rounded-lg border px-4 py-3 text-sm font-black transition disabled:cursor-not-allowed disabled:opacity-45 ${map[color]}`}>{icon}{label}</button>
}

function Field({ label, value }: { label: string; value: string }) {
  return <div className="flex items-center justify-between rounded-lg bg-background-card px-4 py-3 ring-1 ring-line"><span className="text-ink-soft">{label}</span><b className="text-right text-ink">{value}</b></div>
}

function StatusPill({ ok, label }: { ok: boolean; label: string }) {
  return <div className={clsx('flex items-center gap-2 rounded-lg px-4 py-3 text-sm font-bold ring-1', ok ? 'bg-green-50 text-green-700 ring-green-200' : 'bg-red-50 text-red-700 ring-red-200')}><span className={clsx('h-2 w-2 rounded-full', ok ? 'bg-green-500' : 'bg-red-500')} /> {label}</div>
}

function EmptyState({ text }: { text: string }) {
  return <div className="col-span-full rounded-lg bg-white/70 p-6 text-sm text-ink-muted ring-1 ring-line">{text}</div>
}

function stageIcon(status: DirectorStageStatus) {
  if (status === 'done') return <FiCheck />
  if (status === 'blocked' || status === 'failed') return <FiLock />
  if (status === 'running' || status === 'active') return <FiPlay />
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

function errorMessage(err: unknown, fallback: string) {
  if (err && typeof err === 'object' && 'message' in err && typeof err.message === 'string') {
    if (err.message.includes('ARTIFACT_MANIFEST_INVALID')) {
      return '本地任务返回的产物信息不完整，无法写入项目产物库。请重新执行该步骤。'
    }
    return err.message
  }
  return fallback
}
