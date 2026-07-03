import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import clsx from 'clsx'
import ReactMarkdown from 'react-markdown'
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
  FiKey,
  FiLayers,
  FiLock,
  FiLogOut,
  FiPlay,
  FiPlayCircle,
  FiRefreshCw,
  FiSearch,
  FiSettings,
  FiShield,
  FiSquare,
  FiUpload,
  FiUserCheck,
  FiUsers,
  FiVideo,
  FiX,
  FiZap,
} from 'react-icons/fi'
import DesktopPage from './DesktopPage'
import {
  approveAgentReview,
  cancelAgentRun,
  createVideoProject,
  fetchVideoProjects,
  fetchArtifactContent,
  fetchArtifactHistory,
  fetchVideoPreflight,
  fetchProjectArtifacts,
  fetchVideoRoleAgents,
  getAgentRun,
  getAgentRunReviews,
  getAgentRunTrace,
  regenerateAgentStage,
  registerExternalGenerationResult,
  rejectAgentReview,
  reviseArtifact,
  startAgentRun,
  submitEditedArtifact,
  type PreflightResponse,
} from '../services/api'
import type { AuthUser } from '../services/auth'
import {
  buildClientModelProvidersForRun,
  checkJiMengLogin,
  fetchJiMengSetupStatus,
  fetchLocalArtifactFile,
  fetchModelProviderSettings,
  installJiMengCLI,
  loginJiMengHeadless,
  openLocalPath,
  registerJiMengMCP,
  uploadLocalArtifactFile,
  type JiMengSetupStatusResponse,
  type LocalArtifactFileResponse,
  type LocalMCPProviderConfig,
  type MCPToolCallResult,
  type ModelCapability,
  type ModelProviderSettingsResponse,
} from '../services/localAgent'
import type { AgentReviewItem, AgentRun, Artifact, VideoProject, VideoRoleAgent } from '../utils/types'
import {
  applyOptimisticRunningStage,
  buildEnvironmentChecklist,
  buildExportDeliveryItems,
  buildDirectorArtifacts,
  buildDirectorStages,
  buildDirectorTraceNodes,
  buildExternalGenerationTaskPackage,
  buildPublishCopies,
  buildShotReviewGroups,
  deriveNextAction,
  displayNameForArtifact,
  downstreamStaleArtifacts,
  extractDirectorErrorDetail,
  externalGenerationGuideSteps,
  getArtifactViewerSelection,
  getStageStateDisplay,
  isActionablePendingReview,
  isProjectSessionStarted,
  localArtifactIdFromStorageRef,
  localServiceStatusDisplay,
  normalizeDirectorErrorMessage,
  nextStageIdAfterReview,
  nextSelectedReviewId,
  overviewProjectStatus,
  projectPrimaryAction,
  publishCopiesToJSON,
  publishCopiesToMarkdown,
  reviewDisplayTitle,
  reviewQualityReportLines,
  reviewOutputText,
  reviewStatusLabel,
  stageActionLabel,
  traceNodeHasError,
  unresolvedMaterialDependencyCount,
  visibleReviewHistory,
  videoCreationProfileForId,
  videoCreationProfiles,
  type DirectorArtifactRecord,
  type DirectorArtifactStatus,
  type DirectorErrorDetail,
  type DirectorNavKey,
  type DirectorShotAssetSlot,
  type DirectorStage,
  type DirectorStageStatus,
  type DirectorTraceNode,
  type EnvironmentChecklistItem,
  type ExportDeliveryItem,
  type VideoCreationProfile,
  type VideoCreationProfileId,
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

const requiredModelProviderCapabilities: ModelCapability[] = ['text_to_text', 'text_to_image', 'text_to_video']
const modelProviderCapabilityLabels: Record<ModelCapability, string> = {
  text_to_text: '文生文',
  text_to_image: '文生图片',
  text_to_video: '文生视频',
}

type ModelProviderStatus = {
  state: 'checking' | 'configured' | 'missing' | 'unavailable'
  missing: ModelCapability[]
}

export default function DirectorStudioPage({ user, onLogout, serviceStatus }: Props) {
  const [activeNav, setActiveNav] = useState<DirectorNavKey>('overview')
  const [topic, setTopic] = useState('')
  const [durationSec, setDurationSec] = useState(45)
  const [roleAgents, setRoleAgents] = useState<VideoRoleAgent[]>(fallbackRoles)
  const [selectedProfileId, setSelectedProfileId] = useState<VideoCreationProfileId>('voice_visual')
  const [project, setProject] = useState<VideoProject | null>(null)
  const [projectArtifacts, setProjectArtifacts] = useState<Artifact[]>([])
  const [run, setRun] = useState<AgentRun | null>(null)
  const [reviews, setReviews] = useState<AgentReviewItem[]>([])
  const [trace, setTrace] = useState<unknown>(null)
  const [preflight, setPreflight] = useState<PreflightResponse | null>(null)
  const [feedback, setFeedback] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [errorDetail, setErrorDetail] = useState<DirectorErrorDetail | undefined>(undefined)
  const [optimisticRunningStageId, setOptimisticRunningStageId] = useState<string | undefined>()
  const [modelProviderStatus, setModelProviderStatus] = useState<ModelProviderStatus>({ state: 'checking', missing: [] })
  const [jimengSetupStatus, setJimengSetupStatus] = useState<JiMengSetupStatusResponse | null>(null)
  const [jimengSetupLoading, setJimengSetupLoading] = useState(false)
  const [jimengSetupError, setJimengSetupError] = useState<string | null>(null)
  const [useJiMengMCP, setUseJiMengMCP] = useState(false)
  const selectedProfile = useMemo(() => videoCreationProfileForId(selectedProfileId), [selectedProfileId])
  const profileOptions = useMemo(() => videoCreationProfiles(), [])
  const jimengReady = useMemo(() => isJiMengReady(jimengSetupStatus), [jimengSetupStatus])

  const refreshModelProviderStatus = useCallback(async () => {
    setModelProviderStatus((current) => ({ ...current, state: 'checking' }))
    try {
      const response = await fetchModelProviderSettings()
      const missing = missingModelProviderCapabilities(response)
      setModelProviderStatus({ state: missing.length > 0 ? 'missing' : 'configured', missing })
    } catch {
      setModelProviderStatus({ state: 'unavailable', missing: requiredModelProviderCapabilities })
    }
  }, [])

  const refreshJiMengSetupStatus = useCallback(async () => {
    setJimengSetupLoading(true)
    setJimengSetupError(null)
    try {
      const status = await fetchJiMengSetupStatus()
      setJimengSetupStatus(status)
      if (!isJiMengReady(status)) setUseJiMengMCP(false)
    } catch (err) {
      setJimengSetupError(normalizeDirectorErrorMessage(err))
      setJimengSetupStatus(null)
      setUseJiMengMCP(false)
    } finally {
      setJimengSetupLoading(false)
    }
  }, [])

  const refreshRun = useCallback(async (runId: string, projectId?: string) => {
    const [nextRun, nextReviews, nextTrace, nextArtifacts] = await Promise.all([
      getAgentRun(runId).catch(() => null),
      getAgentRunReviews(runId).catch(() => ({ runId, reviews: [] })),
      getAgentRunTrace(runId).catch(() => null),
      projectId ? fetchProjectArtifacts(projectId).catch(() => ({ artifacts: [] })) : Promise.resolve({ artifacts: [] }),
    ])
    if (nextRun) setRun(nextRun)
    setReviews(nextReviews.reviews || [])
    setTrace(nextTrace)
    setProjectArtifacts(nextArtifacts.artifacts || [])
  }, [])

  useEffect(() => {
    let mounted = true
    Promise.all([
      fetchVideoRoleAgents().catch(() => ({ roleAgents: fallbackRoles })),
      fetchVideoProjects().catch(() => ({ projects: [] })),
    ]).then(([roles, projects]) => {
      if (!mounted) return
      if (roles.roleAgents?.length) setRoleAgents(roles.roleAgents)
      const latestProject = [...(projects.projects || [])].sort((a, b) => {
        const bTime = Date.parse(b.updatedAt || b.createdAt || '')
        const aTime = Date.parse(a.updatedAt || a.createdAt || '')
        return (Number.isNaN(bTime) ? 0 : bTime) - (Number.isNaN(aTime) ? 0 : aTime)
      })[0]
      if (!latestProject) return
      setProject(latestProject)
      if (latestProject.mode === 'aigc_shot' || latestProject.mode === 'voice_visual') {
        setSelectedProfileId(latestProject.mode)
      }
      const restoredTopic = typeof latestProject.config?.topic === 'string' ? latestProject.config.topic : latestProject.name
      if (restoredTopic) setTopic(restoredTopic)
      if (typeof latestProject.targetDurationSec === 'number' && latestProject.targetDurationSec > 0) {
        setDurationSec(latestProject.targetDurationSec)
      }
      fetchProjectArtifacts(latestProject.id)
        .then((nextArtifacts) => {
          if (!mounted) return
          setProjectArtifacts(nextArtifacts.artifacts || [])
        })
        .catch(() => {
          if (!mounted) return
          setProjectArtifacts([])
        })
      if (latestProject.currentRunId) {
        refreshRun(latestProject.currentRunId, latestProject.id).catch(() => {})
      }
    })
    return () => { mounted = false }
  }, [refreshRun])

  useEffect(() => {
    let mounted = true
    setPreflight(null)
    fetchVideoPreflight(selectedProfile.preflightPipeline)
      .then((nextPreflight) => {
        if (mounted) setPreflight(nextPreflight)
      })
      .catch(() => {
        if (mounted) setPreflight(null)
      })
    return () => { mounted = false }
  }, [selectedProfile.preflightPipeline])

  useEffect(() => {
    if (activeNav === 'overview') {
      refreshModelProviderStatus().catch(() => {})
    }
  }, [activeNav, refreshModelProviderStatus])

  useEffect(() => {
    if (activeNav === 'overview' || activeNav === 'system') {
      refreshJiMengSetupStatus().catch(() => {})
    }
  }, [activeNav, refreshJiMengSetupStatus])

  useEffect(() => {
    if (!run?.id || run.status === 'SUCCESS' || run.status === 'FAILED' || run.status === 'CANCELLED') return undefined
    const timer = window.setInterval(() => {
      refreshRun(run.id, project?.id).catch(() => {})
    }, 2500)
    return () => window.clearInterval(timer)
  }, [project?.id, refreshRun, run?.id, run?.status])

  const projectStarted = isProjectSessionStarted(loading, project?.status, run?.status)
  const stages = useMemo(() => buildDirectorStages(roleAgents, reviews, trace, projectStarted), [roleAgents, reviews, trace, projectStarted])
  const displayStages = useMemo(() => applyOptimisticRunningStage(stages, optimisticRunningStageId), [stages, optimisticRunningStageId])
  const artifacts = useMemo(() => buildDirectorArtifacts(roleAgents, reviews, trace, projectArtifacts as unknown as Array<Record<string, unknown>>), [roleAgents, reviews, trace, projectArtifacts])
  const traceNodes = useMemo(() => buildDirectorTraceNodes(trace), [trace])
  const nextAction = useMemo(() => deriveNextAction(displayStages), [displayStages])
  const pendingReviews = reviews.filter(isActionablePendingReview)
  const activeReview = pendingReviews[0]
  const activeReviewStage = activeReview ? displayStages.find((stage) => stage.reviewId === activeReview.id || stage.id === activeReview.roleAgentId || stage.stage === activeReview.stage) : undefined
  const activeRunId = run?.id || project?.currentRunId
  const basePrimaryProjectAction = projectPrimaryAction({
    preflightCanStart: preflight?.canStart === true,
    loading,
    projectStatus: project?.status,
    runStatus: run?.status,
    stages: displayStages,
    topic,
  })
  const primaryProjectAction = basePrimaryProjectAction.kind === 'stop' && !activeRunId
    ? { ...basePrimaryProjectAction, disabled: true }
    : basePrimaryProjectAction
  const environmentChecklist = useMemo(() => buildEnvironmentChecklist({
    serviceStatus,
    selectedProfile,
    preflight,
    modelProviderState: modelProviderStatus.state,
    missingModelCapabilities: modelProviderStatus.missing,
  }), [modelProviderStatus.missing, modelProviderStatus.state, preflight, selectedProfile, serviceStatus])

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
      const cleanTopic = topic.trim()
      const nextProject = await createVideoProject({
        name: cleanTopic.slice(0, 40) || '视频创作项目',
        description: `${selectedProfile.label}：${cleanTopic}`,
        mode: selectedProfile.projectMode,
        skillName: 'video-creator',
        skillVersion: 'v4.0',
        workflowName: 'dynamic-agent-video-creation',
        workflowVersion: 'v4.0',
        generationMode: selectedProfile.generationMode,
        aspectRatio: '16:9',
        targetDurationSec: durationSec,
        language: 'zh-CN',
        config: {
          entry: 'director_studio',
          topic: cleanTopic,
          durationSec,
          videoType: selectedProfile.id,
          profileId: selectedProfile.id,
          preflightPipeline: selectedProfile.preflightPipeline,
        },
      })
      setProject({ ...nextProject, status: 'RUNNING' })
      const clientModelProviders = await buildClientModelProvidersForRun()
      const result = await startAgentRun({
        message: `${selectedProfile.startMessagePrefix}${durationSec}秒视频：${cleanTopic}`,
        domain: 'video_creation',
        mode: 'dynamic_agent',
        context: {
          projectId: nextProject.id,
          topic: cleanTopic,
          durationSec,
          targetDurationSec: durationSec,
          videoType: selectedProfile.id,
          profileId: selectedProfile.id,
          projectMode: selectedProfile.projectMode,
          generationMode: selectedProfile.generationMode,
          preflightPipeline: selectedProfile.preflightPipeline,
          ...(useJiMengMCP && jimengReady ? { aigcProvider: 'jimeng_mcp' } : {}),
          ...(clientModelProviders ? { modelProviders: clientModelProviders } : {}),
        },
      })
      setProject((current) => current?.id === nextProject.id ? { ...current, status: 'RUNNING', currentRunId: result.runId } : current)
      setOptimisticRunningStageId(undefined)
      await refreshRun(result.runId, nextProject.id)
      setActiveNav('review')
    } catch (err) {
      setError(normalizeDirectorErrorMessage(err))
      setErrorDetail(extractDirectorErrorDetail(err))
    } finally {
      setLoading(false)
    }
  }

  const handleInstallJiMengCLI = async () => {
    setJimengSetupLoading(true)
    setJimengSetupError(null)
    try {
      await installJiMengCLI()
      await refreshJiMengSetupStatus()
    } catch (err) {
      setJimengSetupError(normalizeDirectorErrorMessage(err))
    } finally {
      setJimengSetupLoading(false)
    }
  }

  const handleRegisterJiMengMCP = async () => {
    setJimengSetupLoading(true)
    setJimengSetupError(null)
    try {
      await registerJiMengMCP({ transport: 'stdio' })
      await refreshJiMengSetupStatus()
    } catch (err) {
      setJimengSetupError(normalizeDirectorErrorMessage(err))
    } finally {
      setJimengSetupLoading(false)
    }
  }

  const handleStopProject = async () => {
    const nextRunId = run?.id || project?.currentRunId
    if (!nextRunId) return
    setLoading(true)
    setError(null)
    setErrorDetail(undefined)
    try {
      const result = await cancelAgentRun(nextRunId, project?.id, 'user requested')
      setRun((current) => current ? { ...current, status: 'CANCELLED', updatedAt: new Date().toISOString() } : current)
      setProject((current) => current ? { ...current, status: 'PAUSED', currentRunId: result.runId || current.currentRunId } : current)
      await refreshRun(nextRunId, project?.id)
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
      await refreshRun(run.id, project?.id)
    } catch (err) {
      if (nextOptimisticStageId) setOptimisticRunningStageId(undefined)
      setError(normalizeDirectorErrorMessage(err))
      setErrorDetail(extractDirectorErrorDetail(err))
    } finally {
      setLoading(false)
    }
  }

  const refreshArtifacts = useCallback(async () => {
    if (run?.id) {
      await refreshRun(run.id, project?.id)
      return
    }
    if (!project?.id) return
    const nextArtifacts = await fetchProjectArtifacts(project.id)
    setProjectArtifacts(nextArtifacts.artifacts || [])
  }, [project?.id, refreshRun, run?.id])

  return (
    <div className="director-root flex min-h-screen flex-col gap-4 p-4 lg:flex-row lg:gap-5 lg:p-5">
      <DirectorSidebar active={activeNav} setActive={setActiveNav} user={user} serviceStatus={serviceStatus} preflight={preflight} onLogout={onLogout} />
      <main className="min-w-0 flex-1 lg:p-5 lg:pr-6">
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
        {activeNav !== 'system' && (
          <ModelProviderNotice status={modelProviderStatus} selectedProfile={selectedProfile} onOpenSettings={() => setActiveNav('system')} />
        )}
        <div className="mt-6">
          {activeNav === 'overview' && (
            <OverviewPage
              topic={topic}
              durationSec={durationSec}
              loading={loading}
              primaryAction={primaryProjectAction}
              overviewStatus={overviewProjectStatus(displayStages, project?.status, run?.status)}
              stages={displayStages}
              artifacts={artifacts}
              preflight={preflight}
              selectedProfile={selectedProfile}
              profileOptions={profileOptions}
              environmentChecklist={environmentChecklist}
              jimengSetupStatus={jimengSetupStatus}
              jimengSetupLoading={jimengSetupLoading}
              jimengSetupError={jimengSetupError}
              useJiMengMCP={useJiMengMCP}
              jimengReady={jimengReady}
              nextAction={nextAction}
              onTopicChange={setTopic}
              onDurationChange={setDurationSec}
              onProfileChange={setSelectedProfileId}
              onToggleJiMengMCP={setUseJiMengMCP}
              onRefreshJiMeng={refreshJiMengSetupStatus}
              onInstallJiMengCLI={handleInstallJiMengCLI}
              onRegisterJiMengMCP={handleRegisterJiMengMCP}
              onStart={handleStart}
              onStop={handleStopProject}
              onOpenSettings={() => setActiveNav('system')}
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
          {activeNav === 'trace' && <TracePage traceNodes={traceNodes} run={run} />}
          {activeNav === 'assets' && <AssetsPage artifacts={artifacts} projectId={project?.id} onArtifactsChanged={refreshArtifacts} />}
          {activeNav === 'roles' && <RolesPage stages={displayStages} />}
          {activeNav === 'export' && <ExportPage artifacts={artifacts} durationSec={durationSec} projectId={project?.id} />}
          {activeNav === 'system' && <DesktopPage />}
        </div>
      </main>
    </div>
  )
}

function JiMengSetupPanel(props: {
  status: JiMengSetupStatusResponse | null
  loading: boolean
  error: string | null
  enabled: boolean
  ready: boolean
  selectedProfile: VideoCreationProfile
  onToggle: (enabled: boolean) => void
  onRefresh: () => void
  onInstallCLI: () => void
  onRegisterMCP: () => void
}) {
  const { status, loading, error, enabled, ready, selectedProfile, onToggle, onRefresh, onInstallCLI, onRegisterMCP } = props
  const providerStatus = status?.mcpProviders?.find((item) => item.id === 'jimeng')
  const mcpRegistered = Boolean(status?.mcpProvider)
  const mcpReachable = providerStatus?.reachable === true
  const canUseForProfile = selectedProfile.projectMode === 'aigc_shot' || selectedProfile.generationMode === 'manual_import'
  const startCommand = status?.mcpStartCommand || 'python3 mcp/jimeng/server.py'
  const providerDetail = status?.mcpProvider ? mcpProviderDetail(status.mcpProvider) : 'stdio provider'
  const installCommand = status?.installCommand || 'curl -fsSL https://jimeng.jianying.com/cli | bash'
  const [loginResult, setLoginResult] = useState<MCPToolCallResult | null>(null)
  const [loginLoading, setLoginLoading] = useState(false)
  const [loginError, setLoginError] = useState<string | null>(null)
  const loginData = loginResult?.structuredContent || {}
  const verificationUri = stringRecordValue(loginData, 'verification_uri') || stringRecordValue(loginData, 'verificationUri')
  const userCode = stringRecordValue(loginData, 'user_code') || stringRecordValue(loginData, 'userCode')
  const deviceCode = stringRecordValue(loginData, 'device_code') || stringRecordValue(loginData, 'deviceCode')
  const headline = ready
    ? '即梦自动生成已就绪'
      : status?.dreaminaAvailable
        ? '即梦 CLI 已安装，等待 MCP 连接'
        : '即梦 CLI 未检测到'
  const handleLoginHeadless = async () => {
    setLoginLoading(true)
    setLoginError(null)
    try {
      const result = await loginJiMengHeadless()
      setLoginResult(result)
      if (result.isError) setLoginError(result.content?.[0]?.text || '即梦登录启动失败')
    } catch (err) {
      setLoginError(normalizeDirectorErrorMessage(err))
    } finally {
      setLoginLoading(false)
    }
  }
  const handleCheckLogin = async () => {
    if (!deviceCode) return
    setLoginLoading(true)
    setLoginError(null)
    try {
      const result = await checkJiMengLogin(deviceCode, 30)
      setLoginResult(result)
      if (result.isError) setLoginError(result.content?.[0]?.text || '即梦登录未完成')
    } catch (err) {
      setLoginError(normalizeDirectorErrorMessage(err))
    } finally {
      setLoginLoading(false)
    }
  }

  return (
    <section className="card overflow-hidden p-0">
      <div className="grid gap-0 lg:grid-cols-[1.1fr_0.9fr]">
        <div className="border-b border-line p-5 lg:border-b-0 lg:border-r">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex min-w-0 items-start gap-3">
              <div className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-primary-soft text-primary-dark">
                <FiCpu />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-black text-primary-dark">即梦 AIGC 扩展</p>
                <h3 className="mt-1 text-lg font-black text-ink">{headline}</h3>
                <p className="mt-2 text-sm leading-6 text-ink-muted">
                  用户自己的 Dreamina 登录态保留在本机，躺营只调用已注册的本地 MCP provider。
                </p>
              </div>
            </div>
            <StatusBadge status={ready ? 'valid' : mcpRegistered || status?.dreaminaAvailable ? 'review' : 'pending'} label={ready ? '可自动生成' : '需配置'} />
          </div>
          {error ? <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs font-semibold text-red-700">{error}</div> : null}
          <div className="mt-4 grid gap-3 md:grid-cols-3">
            <JiMengStep label="Dreamina CLI" detail={status?.dreaminaVersion || installCommand} done={status?.dreaminaAvailable === true} />
            <JiMengStep label="MCP 注册" detail={providerDetail} done={mcpRegistered} />
            <JiMengStep label="MCP 连接" detail={providerStatus?.error || (mcpReachable ? 'tools/list 正常' : '等待服务启动')} done={mcpReachable} />
          </div>
          <div className="mt-4 flex flex-wrap gap-2">
            <button
              type="button"
              onClick={onInstallCLI}
              disabled={loading}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-3 py-2 text-xs font-black text-white shadow-glow disabled:cursor-not-allowed disabled:opacity-50"
            >
              <FiDownload /> 安装/更新 CLI
            </button>
            <button
              type="button"
              onClick={onRegisterMCP}
              disabled={loading}
              className="inline-flex items-center gap-2 rounded-lg bg-white px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
            >
              <FiCheck /> 注册 MCP
            </button>
            <button
              type="button"
              onClick={onRefresh}
              disabled={loading}
              className="inline-flex items-center gap-2 rounded-lg bg-white px-3 py-2 text-xs font-black text-ink-muted ring-1 ring-line hover:bg-background-card disabled:cursor-not-allowed disabled:opacity-50"
            >
              <FiRefreshCw className={clsx(loading && 'animate-spin')} /> 刷新状态
            </button>
          </div>
        </div>
        <div className="bg-background-card p-5">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-black text-ink">自动调用即梦生成素材</p>
              <p className="mt-1 text-xs leading-5 text-ink-muted">
                开启后，新项目会把 AIGC 请求交给本地 JiMeng MCP；未开启时继续展示可复制提示词和手动上传。
              </p>
            </div>
            <button
              type="button"
              disabled={!ready || !canUseForProfile}
              onClick={() => onToggle(!enabled)}
              className={clsx(
                'relative h-7 w-12 shrink-0 rounded-full transition disabled:cursor-not-allowed disabled:opacity-50',
                enabled ? 'bg-primary' : 'bg-line',
              )}
              aria-pressed={enabled}
              title="自动调用即梦生成素材"
            >
              <span className={clsx('absolute top-1 h-5 w-5 rounded-full bg-white shadow transition', enabled ? 'left-6' : 'left-1')} />
            </button>
          </div>
          <div className="mt-4 rounded-lg bg-white p-3 ring-1 ring-line">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-black text-primary-dark">MCP 启动命令</span>
              <CopyButton value={startCommand} label="复制命令" />
            </div>
            <code className="mt-2 block break-all rounded bg-ink px-3 py-2 font-mono text-[11px] leading-5 text-white">{startCommand}</code>
          </div>
          <div className="mt-3 rounded-lg bg-white p-3 ring-1 ring-line">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-xs font-black text-primary-dark">首次登录授权</span>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={handleLoginHeadless}
                  disabled={!mcpReachable || loginLoading}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <FiUserCheck /> 获取登录码
                </button>
                <button
                  type="button"
                  onClick={handleCheckLogin}
                  disabled={!deviceCode || loginLoading}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-ink-muted ring-1 ring-line hover:bg-background-card disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <FiRefreshCw className={clsx(loginLoading && 'animate-spin')} /> 检查登录
                </button>
              </div>
            </div>
            {loginError ? <div className="mt-2 text-xs font-semibold text-red-700">{loginError}</div> : null}
            {verificationUri || userCode ? (
              <div className="mt-3 space-y-2">
                {verificationUri ? <LoginCopyRow label="授权页面" value={verificationUri} /> : null}
                {userCode ? <LoginCopyRow label="用户码" value={userCode} /> : null}
              </div>
            ) : (
              <p className="mt-2 text-xs leading-5 text-ink-muted">MCP 连接后可生成登录码，按即梦页面提示完成授权。</p>
            )}
          </div>
          {!canUseForProfile ? (
            <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs font-semibold text-amber-800">
              当前入口以本地编排为主，切到影视/AIGC 入口后可启用即梦自动素材生成。
            </div>
          ) : null}
        </div>
      </div>
    </section>
  )
}

function LoginCopyRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-background-card px-3 py-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-black text-ink-soft">{label}</span>
        <CopyButton value={value} label="复制" />
      </div>
      <div className="mt-1 break-all font-mono text-[11px] leading-4 text-ink">{value}</div>
    </div>
  )
}

function JiMengStep({ label, detail, done }: { label: string; detail: string; done: boolean }) {
  return (
    <div className={clsx('rounded-lg border p-3', done ? 'border-green-100 bg-green-50/70' : 'border-line bg-white')}>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-black text-ink">{label}</span>
        <span className={clsx('grid h-5 w-5 place-items-center rounded-full text-[11px]', done ? 'bg-green-600 text-white' : 'bg-stone-100 text-ink-soft')}>
          {done ? <FiCheck /> : <FiX />}
        </span>
      </div>
      <p className="mt-2 line-clamp-2 break-all text-[11px] leading-4 text-ink-muted">{detail}</p>
    </div>
  )
}

function mcpProviderDetail(provider: LocalMCPProviderConfig): string {
  if (provider.transport === 'stdio') {
    return [provider.command, ...(provider.args || [])].filter(Boolean).join(' ') || 'stdio'
  }
  return provider.endpoint || provider.transport || 'mcp provider'
}

function isJiMengReady(status: JiMengSetupStatusResponse | null): boolean {
  if (!status?.dreaminaAvailable) return false
  const provider = status.mcpProviders?.find((item) => item.id === 'jimeng')
  if (!provider?.reachable) return false
  return Boolean(provider.tools?.some((tool) => tool.name === 'jimeng.generate_video'))
}

function stringRecordValue(record: Record<string, unknown>, key: string): string {
  const value = record[key]
  return typeof value === 'string' ? value : ''
}

function DirectorSidebar({ active, setActive, user, serviceStatus, preflight, onLogout }: { active: DirectorNavKey; setActive: (key: DirectorNavKey) => void; user: AuthUser; serviceStatus: Props['serviceStatus']; preflight: PreflightResponse | null; onLogout: () => void }) {
  const localStatus = localServiceStatusDisplay(serviceStatus, preflight?.capabilityMenu.localRunner.available)
  const localStatusClassName = localStatus.tone === 'ok'
    ? 'bg-green-50 text-green-700'
    : localStatus.tone === 'error'
      ? 'bg-red-50 text-red-700'
      : 'bg-stone-50 text-ink-muted'

  return (
    <aside className="glass flex w-full shrink-0 flex-col rounded-xl p-4 lg:sticky lg:top-5 lg:h-[calc(100vh-40px)] lg:w-72">
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
      <nav className="mt-5 flex gap-1 overflow-x-auto pb-1 lg:mt-8 lg:block lg:space-y-1 lg:overflow-visible lg:pb-0">
        {navItems.map((item) => {
          const Icon = item.icon
          const isActive = active === item.key
          return (
            <button
              key={item.key}
              onClick={() => setActive(item.key)}
              className={clsx(
                'flex shrink-0 items-center gap-3 rounded-lg px-4 py-3 text-left text-sm font-semibold transition lg:w-full',
                isActive ? 'bg-primary text-white shadow-glow' : 'text-ink-muted hover:bg-primary-soft hover:text-primary-dark',
              )}
            >
              <Icon className="text-lg" />
              <span>{item.label}</span>
            </button>
          )
        })}
      </nav>
      <div className="mt-4 rounded-lg bg-white/70 p-4 ring-1 ring-line lg:mt-auto">
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
          <div className={clsx('rounded-lg px-3 py-2', localStatusClassName)}>{localStatus.label}</div>
          <div className="rounded-lg bg-amber-50 px-3 py-2 text-primary-dark">v0.1.0 内测</div>
        </div>
      </div>
    </aside>
  )
}

function TopBar({ preflight, serviceStatus, run }: { preflight: PreflightResponse | null; serviceStatus: string; run: AgentRun | null }) {
  const localRunnerAvailable = preflight?.capabilityMenu.localRunner.available === true
  const localUnavailable = serviceStatus === 'unhealthy' && !localRunnerAvailable
  const executionReady = preflight?.canStart === true && !localUnavailable
  return (
    <header className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
      <div>
        <div className="flex items-center gap-2 text-sm font-semibold text-primary-dark">
          <FiZap /> 多角色协作 · 可追踪 · 分阶段确认 · 本地可控渲染
        </div>
        <h1 className="mt-2 text-3xl font-black text-gradient md:text-4xl">躺营导演台 v0.1.0</h1>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <div className="hidden items-center gap-2 rounded-lg bg-white/75 px-4 py-3 text-sm text-ink-muted ring-1 ring-line xl:flex">
          <FiSearch /> 搜索项目、产物、过程事件
        </div>
        <StatusPill ok={executionReady} label={localUnavailable ? '服务未连接' : !preflight ? '环境体检中' : preflight.status === 'blocked' ? '环境待处理' : '执行环境就绪'} />
        {run && <div className="rounded-lg bg-white/80 px-4 py-3 text-xs font-bold text-ink-muted ring-1 ring-line">Run {run.id.slice(0, 8)}</div>}
        <button className="rounded-lg bg-white/80 p-3 text-ink-muted ring-1 ring-line hover:text-primary-dark" title="通知"><FiBell /></button>
      </div>
    </header>
  )
}

function ModelProviderNotice({ status, selectedProfile, onOpenSettings }: { status: ModelProviderStatus; selectedProfile: VideoCreationProfile; onOpenSettings: () => void }) {
  if (status.state === 'checking' || status.state === 'configured') return null
  const blockingMissing = selectedProfile.generationMode === 'manual_import'
    ? status.missing.filter((capability) => capability === 'text_to_text')
    : status.missing
  if (status.state === 'missing' && blockingMissing.length === 0) return null
  const missingText = blockingMissing.map((capability) => modelProviderCapabilityLabels[capability]).join('、')
  const title = status.state === 'unavailable' ? '本地模型配置未读取' : '基础模型 API 未配置完整'
  const message = status.state === 'unavailable'
    ? '请先确认本地服务已启动，然后在设置中配置 OpenAI-compatible 接口。'
    : `缺少 ${missingText || '基础模型'} Provider，请在设置中填写接口地址、模型名和 Token。`

  return (
    <div className="mt-4 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-white text-amber-700 ring-1 ring-amber-200">
            <FiKey />
          </span>
          <div className="min-w-0">
            <div className="font-black">{title}</div>
            <div className="mt-1 text-xs leading-5 text-amber-800">{message}</div>
          </div>
        </div>
        <button
          type="button"
          onClick={onOpenSettings}
          className="inline-flex shrink-0 items-center justify-center gap-2 rounded-lg bg-amber-700 px-4 py-2 text-xs font-black text-white hover:bg-amber-800"
        >
          <FiSettings /> 打开设置
        </button>
      </div>
    </div>
  )
}

function missingModelProviderCapabilities(response: ModelProviderSettingsResponse): ModelCapability[] {
  return requiredModelProviderCapabilities.filter((capability) => {
    const provider = response.providers?.[capability]
    return !provider?.hasApiKey && !provider?.apiKey
  })
}

function OverviewPage(props: {
  topic: string
  durationSec: number
  loading: boolean
  primaryAction: ReturnType<typeof projectPrimaryAction>
  overviewStatus: DirectorStageStatus
  stages: DirectorStage[]
  artifacts: DirectorArtifactRecord[]
  preflight: PreflightResponse | null
  selectedProfile: VideoCreationProfile
  profileOptions: VideoCreationProfile[]
  environmentChecklist: EnvironmentChecklistItem[]
  jimengSetupStatus: JiMengSetupStatusResponse | null
  jimengSetupLoading: boolean
  jimengSetupError: string | null
  useJiMengMCP: boolean
  jimengReady: boolean
  nextAction?: ReturnType<typeof deriveNextAction>
  onTopicChange: (value: string) => void
  onDurationChange: (value: number) => void
  onProfileChange: (value: VideoCreationProfileId) => void
  onToggleJiMengMCP: (enabled: boolean) => void
  onRefreshJiMeng: () => void
  onInstallJiMengCLI: () => void
  onRegisterJiMengMCP: () => void
  onStart: () => void
  onStop: () => void
  onOpenSettings: () => void
  onGoReview: () => void
}) {
  const { topic, durationSec, primaryAction, overviewStatus, stages, artifacts, preflight, selectedProfile, profileOptions, environmentChecklist, jimengSetupStatus, jimengSetupLoading, jimengSetupError, useJiMengMCP, jimengReady, nextAction, onTopicChange, onDurationChange, onProfileChange, onToggleJiMengMCP, onRefreshJiMeng, onInstallJiMengCLI, onRegisterJiMengMCP, onStart, onStop, onOpenSettings, onGoReview } = props
  const staleNames = artifacts.filter((artifact) => artifact.status === 'stale').map((artifact) => artifact.name)
  const isStopAction = primaryAction.kind === 'stop'
  return (
    <div className="space-y-5">
      <div className="grid grid-cols-12 gap-5">
        <section className="card col-span-12 p-6 xl:col-span-8">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="grid h-10 w-10 place-items-center rounded-xl bg-primary-soft text-primary-dark"><FiZap /></div>
              <div>
                <p className="text-sm font-bold text-primary-dark">创建新项目</p>
                <p className="text-xs text-ink-soft">一句话描述你想做的视频，AI 会自动规划执行流程</p>
              </div>
            </div>
            <StatusBadge status={overviewStatus} />
          </div>
          <div className="mt-5 grid gap-3 md:grid-cols-2">
            {profileOptions.map((profile) => {
              const active = selectedProfile.id === profile.id
              return (
                <button
                  key={profile.id}
                  type="button"
                  onClick={() => onProfileChange(profile.id)}
                  className={clsx(
                    'min-w-0 rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-primary/30',
                    active ? 'border-primary bg-primary-soft shadow-sm' : 'border-line bg-white hover:border-primary/35 hover:bg-background-card',
                  )}
                >
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="text-sm font-black text-ink">{profile.label}</div>
                      <div className="mt-1 text-xs leading-5 text-ink-muted">{profile.description}</div>
                    </div>
                    <StatusBadge status={active ? 'valid' : 'pending'} label={active ? '当前入口' : '可选择'} />
                  </div>
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {profile.requiredLocalCommands.map((command) => (
                      <span key={command} className="rounded bg-white px-2 py-1 font-mono text-[10px] font-bold text-primary-dark ring-1 ring-line">{command}</span>
                    ))}
                  </div>
                </button>
              )
            })}
          </div>
          <textarea
            className="mt-5 h-36 w-full resize-none rounded-xl border-2 border-line bg-white p-5 text-base leading-7 text-ink outline-none transition placeholder:text-ink-soft/60 focus:border-primary focus:shadow-glow"
            value={topic}
            onChange={(event) => onTopicChange(event.target.value)}
            placeholder="输入你想制作的视频主题..."
          />
          <div className="mt-4 flex flex-wrap items-center gap-3">
            <select className="rounded-lg border border-line bg-white px-4 py-2.5 text-sm text-ink" value={durationSec} onChange={(event) => onDurationChange(Number(event.target.value))}>
              {[30, 45, 60, 90, 120].map((duration) => <option key={duration} value={duration}>{duration} 秒</option>)}
            </select>
            <button
              disabled={primaryAction.disabled}
              onClick={isStopAction ? onStop : onStart}
              className={clsx(
                'flex items-center gap-2 rounded-lg px-6 py-2.5 text-sm font-bold text-white transition disabled:cursor-not-allowed disabled:opacity-50',
                isStopAction ? 'bg-red-600 hover:bg-red-700' : 'bg-primary shadow-glow hover:bg-primary-dark',
              )}
            >
              {isStopAction ? <FiSquare /> : <FiPlay />} {primaryAction.label}
            </button>
            {preflight?.blockers?.length ? <span className="text-xs font-semibold text-red-700">{preflight.blockers[0].message}</span> : !preflight ? <span className="text-xs font-semibold text-primary-dark">正在体检当前视频入口...</span> : null}
          </div>
        </section>
        <section className="card col-span-12 p-6 xl:col-span-4">
          <p className="text-sm font-bold text-primary-dark">项目状态</p>
          <h3 className="mt-2 text-xl font-black text-ink">{nextAction?.label || '准备开始'}</h3>
          <p className="mt-3 text-sm leading-6 text-ink-muted">{nextAction?.description || '输入需求后开始动态 Agent 创作线。'}</p>
          <button onClick={onGoReview} className="mt-5 flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-5 py-3 text-sm font-black text-white shadow-glow transition hover:bg-primary-dark">
            {stages.some((stage) => stage.status === 'review') ? '前往审核' : '查看工作台'} <FiChevronRight />
          </button>
        </section>
      </div>
      <JiMengSetupPanel
        status={jimengSetupStatus}
        loading={jimengSetupLoading}
        error={jimengSetupError}
        enabled={useJiMengMCP}
        ready={jimengReady}
        selectedProfile={selectedProfile}
        onToggle={onToggleJiMengMCP}
        onRefresh={onRefreshJiMeng}
        onInstallCLI={onInstallJiMengCLI}
        onRegisterMCP={onRegisterJiMengMCP}
      />
      <EnvironmentChecklistPanel items={environmentChecklist} onOpenSettings={onOpenSettings} />
      <StageFlow stages={stages} />
      {staleNames.length > 0 && (
        <div className="card border-red-200 bg-red-50/80 p-5">
          <h3 className="text-base font-black text-red-800">下游产物已过期，需要重新生成</h3>
          <div className="mt-3 flex flex-wrap gap-2">
            {staleNames.map((name) => <span key={name} className="rounded-full bg-white px-3 py-1 text-xs font-bold text-red-700 ring-1 ring-red-200">{name}</span>)}
          </div>
        </div>
      )}
      <div className="grid grid-cols-1 gap-5 md:grid-cols-2 xl:grid-cols-4">
        <InfoCard icon={<FiShield />} title="当前角色" value={nextAction?.label || '待启动'} desc="角色边界由后端 StageGuard 校验。" tone="primary" />
        <InfoCard icon={<FiHardDrive />} title="本地执行器" value={preflight?.capabilityMenu.localRunner.available ? '可用' : '待检测'} desc="HyperFrames 预览与渲染走本地执行面。" tone="green" />
        <InfoCard icon={<FiRefreshCw />} title="会话状态" value="可恢复" desc="Run、Trace、Review 由后端持久化。" tone="blue" />
        <InfoCard icon={<FiEdit3 />} title="产物治理" value="产物索引" desc="上游修改后下游产物按上下文标记过期。" tone="violet" />
      </div>
    </div>
  )
}

function EnvironmentChecklistPanel({ items, onOpenSettings }: { items: EnvironmentChecklistItem[]; onOpenSettings: () => void }) {
  const blockedCount = items.filter((item) => item.status === 'blocked').length
  const warningCount = items.filter((item) => item.status === 'warning').length
  return (
    <section className="card p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm font-bold text-primary-dark">启动体检</p>
          <h3 className="mt-1 text-lg font-black text-ink">云端编排、本地工具和模型配置</h3>
        </div>
        <StatusBadge
          status={blockedCount ? 'blocked' : warningCount ? 'review' : 'valid'}
          label={blockedCount ? `${blockedCount} 项待处理` : warningCount ? '可启动但需留意' : '体检通过'}
        />
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {items.map((item) => (
          <div key={item.id} className={clsx('rounded-lg border p-4', environmentItemTone(item.status))}>
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="text-sm font-black text-ink">{item.label}</div>
                <p className="mt-1 text-xs leading-5 text-ink-muted">{item.detail}</p>
                {item.blockerCode ? <div className="mt-2 font-mono text-[10px] font-bold text-red-600">{item.blockerCode}</div> : null}
              </div>
              <StatusBadge status={environmentStatusToBadge(item.status)} label={environmentStatusLabel(item.status)} />
            </div>
            {item.actionLabel ? (
              <button
                type="button"
                onClick={item.actionLabel === '打开设置' ? onOpenSettings : undefined}
                className="mt-3 inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"
              >
                <FiSettings /> {item.actionLabel}
              </button>
            ) : null}
          </div>
        ))}
      </div>
    </section>
  )
}

function environmentItemTone(status: EnvironmentChecklistItem['status']) {
  if (status === 'passed') return 'border-green-100 bg-green-50/70'
  if (status === 'blocked') return 'border-red-200 bg-red-50/75'
  if (status === 'warning') return 'border-amber-200 bg-amber-50/75'
  return 'border-line bg-background-card'
}

function environmentStatusToBadge(status: EnvironmentChecklistItem['status']): DirectorArtifactStatus {
  if (status === 'passed') return 'valid'
  if (status === 'blocked') return 'blocked'
  if (status === 'warning') return 'review'
  return 'pending'
}

function environmentStatusLabel(status: EnvironmentChecklistItem['status']) {
  if (status === 'passed') return '通过'
  if (status === 'blocked') return '待处理'
  if (status === 'warning') return '注意'
  return '检测中'
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
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-5">
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
  const blockedStage = stages.find((stage) => stage.status === 'blocked' || stage.status === 'failed')
  const runningStage = stages.find((stage) => stage.status === 'running')
  const reviewStage = stages.find((stage) => stage.status === 'review')

  return (
    <section className="card col-span-12 p-5">
      <div className="flex items-center justify-between gap-3 mb-4">
        <div>
          <h3 className="text-base font-black text-ink">任务状态机</h3>
          <p className="mt-1 text-xs text-ink-soft">
            {blockedStage ? `${blockedStage.displayName} 执行失败或被阻断，请查看追踪页错误并重新生成。` : runningStage ? `${runningStage.displayName} 正在生成，产物完成后进入审核。` : reviewStage ? `${reviewStage.displayName} 产物已输出，等待确认。` : '审核通过后，下个角色立即进入生成中。'}
          </p>
        </div>
        <div className="flex items-center gap-3 text-xs font-semibold text-ink-soft">
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-primary" /> 生成中</span>
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
  const blockedStage = stages.find((stage) => stage.status === 'blocked' || stage.status === 'failed')
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

  if (blockedStage) {
    return (
      <div className="col-span-12 rounded-xl border border-red-200 bg-red-50 px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-red-500 text-white">
            <FiX />
          </span>
          <div>
            <p className="text-sm font-black text-red-800">
              【{blockedStage.displayName}】执行失败或被阻断
            </p>
            <p className="text-xs text-red-600 mt-0.5">请切到追踪页查看错误详情，或在审核页重新生成当前阶段。</p>
          </div>
        </div>
      </div>
    )
  }

  if (runningStage) {
    return (
      <div className="col-span-12 rounded-xl border border-line bg-primary-soft px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="relative grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-primary text-white">
            <span className="absolute inset-0 rounded-lg bg-primary-light animate-ping opacity-30" />
            <FiRefreshCw className="animate-spin relative z-10" />
          </span>
          <div>
            <p className="text-sm font-black text-primary-dark">
              系统正在生成【{runningStage.displayName}】的{stageActionLabel(runningStage.stage)}
            </p>
            <p className="text-xs text-ink-muted mt-0.5">生成完成后将自动进入审核阶段，请稍候…</p>
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
  const lastAutoSelectedActiveReviewIdRef = useRef<string | undefined>(review?.id)
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
    const nextId = nextSelectedReviewId(
      selectedReviewId,
      review?.id,
      reviewHistory,
      lastAutoSelectedActiveReviewIdRef.current,
    )
    if (nextId !== selectedReviewId) {
      setSelectedReviewId(nextId)
    }
    if (review?.id && review.id !== lastAutoSelectedActiveReviewIdRef.current) {
      lastAutoSelectedActiveReviewIdRef.current = review.id
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
              <div className="col-span-12 min-w-0 xl:col-span-8">
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
              <div className="col-span-12 space-y-3 self-start xl:sticky xl:top-5 xl:col-span-4">
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

function TracePage({ traceNodes, run }: { traceNodes: DirectorTraceNode[]; run: AgentRun | null }) {
  const [selectedId, setSelectedId] = useState<string | undefined>(traceNodes[0]?.id)
  const selected = traceNodes.find((node) => node.id === selectedId) || traceNodes[0]
  useEffect(() => {
    if (!selectedId && traceNodes[0]) setSelectedId(traceNodes[0].id)
  }, [selectedId, traceNodes])

  const selectedHasError = traceNodeHasError(selected)

  return (
    <div className="grid grid-cols-12 gap-5">
      <section className="card col-span-12 p-6 xl:col-span-8">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-bold text-primary-dark">过程追踪 · 调试视图</p>
            <h2 className="mt-2 text-2xl font-black text-ink">执行追踪</h2>
          </div>
          <span className="rounded-full bg-primary-soft px-3 py-1 text-xs font-bold text-primary-dark">Run {run?.id?.slice(0, 12) || '未启动'}</span>
        </div>
        <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4 [&>*]:min-w-0">
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
      </section>
      <aside className="col-span-12 space-y-5 xl:col-span-4">
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

function AssetsPage({ artifacts, projectId, onArtifactsChanged }: { artifacts: DirectorArtifactRecord[]; projectId?: string; onArtifactsChanged?: () => Promise<void> | void }) {
  const staleCount = artifacts.filter((a) => a.status === 'stale').length
  const materialDependencyCount = useMemo(() => unresolvedMaterialDependencyCount(buildShotReviewGroups(artifacts)), [artifacts])
  return (
    <div className="space-y-5">
      <section className="card p-6">
        <p className="text-sm font-bold text-primary-dark">Shot 素材工作台</p>
        <h2 className="mt-2 text-3xl font-black text-ink">按 shot 回填与查看</h2>
        <p className="mt-2 text-sm leading-6 text-ink-muted">先按 SHOT 查看脚本、提示词、参考图、故事板、基础画面和文字叠层；底部保留原始产物索引用于查 ID、版本和路径。</p>
      </section>
      {staleCount > 0 && <div className="rounded-lg bg-amber-50 p-4 text-sm font-semibold text-primary-dark ring-1 ring-amber-200">⚠ 有 {staleCount} 个下游产物已过期。上游产物被修改、驳回或重新生成后，下游产物需要重新生成才能使用。</div>}
      {materialDependencyCount > 0 && (
        <div className="rounded-lg border border-primary/25 bg-white p-4 shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="text-sm font-black text-primary-dark">待回填素材</p>
              <p className="mt-1 text-sm leading-6 text-ink-muted">
                {materialDependencyCount} 个外部生成请求已按 shot 放入下方槽位。复制 Prompt 到网页端生成后，直接在对应 shot 的参考图、故事板或基础画面槽上传回填。
              </p>
            </div>
            <StatusBadge status="review" label="待用户回填" />
          </div>
        </div>
      )}
      <ShotAssetWorkbench artifacts={artifacts} projectId={projectId} onArtifactsChanged={onArtifactsChanged} />
      <section className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="text-sm font-bold text-primary-dark">原始产物索引</p>
            <h3 className="mt-1 text-xl font-black text-ink">ID、状态与版本</h3>
          </div>
          <span className="rounded-full bg-background-card px-3 py-1 text-xs font-bold text-ink-muted ring-1 ring-line">{artifacts.length} artifacts</span>
        </div>
        <ArtifactTable artifacts={artifacts} projectId={projectId} onArtifactsChanged={onArtifactsChanged} />
      </section>
    </div>
  )
}

interface ShotPromptPreview {
  artifactId: string
  loading: boolean
  error?: string
  request?: ExternalGenerationRequestContent
}

function ShotAssetWorkbench({ artifacts, projectId, onArtifactsChanged }: { artifacts: DirectorArtifactRecord[]; projectId?: string; onArtifactsChanged?: () => Promise<void> | void }) {
  const groups = useMemo(() => buildShotReviewGroups(artifacts), [artifacts])
  const [openShotId, setOpenShotId] = useState<string | undefined>(groups[0]?.shotId)
  const [promptPreviews, setPromptPreviews] = useState<Record<string, ShotPromptPreview>>({})
  const [uploadingKey, setUploadingKey] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (openShotId && groups.some((group) => group.shotId === openShotId)) return
    setOpenShotId(groups[0]?.shotId)
  }, [groups, openShotId])

  const openGroup = groups.find((group) => group.shotId === openShotId) || groups[0]

  useEffect(() => {
    if (!openGroup) return
    const requests = openGroup.slots.flatMap((slot) => slot.dependencyRequests)
    for (const artifact of requests) {
      if (promptPreviews[artifact.id]) continue
      setPromptPreviews((current) => ({
        ...current,
        [artifact.id]: { artifactId: artifact.id, loading: true },
      }))
      fetchArtifactContent(artifact.id)
        .then((response) => {
          const request = externalGenerationRequestFromContent(response.content)
          setPromptPreviews((current) => ({
            ...current,
            [artifact.id]: { artifactId: artifact.id, loading: false, request: request || undefined, error: request ? undefined : '无法解析生成请求' },
          }))
        })
        .catch((err) => {
          setPromptPreviews((current) => ({
            ...current,
            [artifact.id]: { artifactId: artifact.id, loading: false, error: normalizeDirectorErrorMessage(err) },
          }))
        })
    }
  }, [openGroup, promptPreviews])

  const uploadShotAsset = async (
    event: ChangeEvent<HTMLInputElement>,
    shotId: string,
    slot: DirectorShotAssetSlot,
    request?: ExternalGenerationRequestContent,
  ) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || !slot.uploadKind) return
    if (!projectId) {
      setError('缺少项目 ID，无法登记 shot 素材。')
      return
    }
    const uploadKey = `${shotId}-${slot.kind}-${request?.requestId || 'manual'}`
    setUploadingKey(uploadKey)
    setMessage(null)
    setError(null)
    try {
      const registered = await uploadAndRegisterShotAsset({
        projectId,
        shotId,
        slot,
        request,
        file,
      })
      setMessage(`已回填 ${shotId} · ${slot.label}：${registered.artifact?.name || registered.artifact?.id || file.name}`)
      await onArtifactsChanged?.()
    } catch (err) {
      setError(normalizeDirectorErrorMessage(err))
    } finally {
      setUploadingKey(null)
    }
  }

  if (!groups.length) return null
  return (
    <section className="card p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-sm font-bold text-primary-dark">Shot 素材工作台</p>
          <h3 className="mt-1 text-xl font-black text-ink">逐 shot 查看、复制、上传</h3>
        </div>
        <span className="rounded-full bg-primary-soft px-3 py-1 text-xs font-bold text-primary-dark">{groups.length} shots</span>
      </div>
      <div className="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-4">
        {groups.map((group) => (
          <button
            key={group.shotId}
            type="button"
            onClick={() => setOpenShotId(group.shotId)}
            className={clsx(
              'rounded-lg p-4 text-left ring-1 transition',
              openGroup?.shotId === group.shotId ? 'bg-primary-soft ring-primary' : 'bg-white ring-line hover:bg-background-card',
            )}
          >
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="font-mono text-xs font-black text-primary-dark">{group.shotId}</div>
                <div className="mt-1 truncate text-sm font-black text-ink" title={group.title}>{group.title}</div>
              </div>
              <StatusBadge status={group.status} />
            </div>
            {group.narrationText ? <p className="mt-3 line-clamp-2 text-sm leading-6 text-ink-muted">{group.narrationText}</p> : null}
            <div className="mt-3 grid grid-cols-2 gap-1 text-center text-[10px] font-black text-ink-muted sm:grid-cols-3">
              {group.slots.map((slot) => (
                <span key={slot.kind} className={clsx('rounded px-2 py-1 ring-1', slot.status === 'valid' ? 'bg-green-50 text-green-700 ring-green-100' : slot.status === 'review' ? 'bg-amber-50 text-primary-dark ring-amber-100' : 'bg-background-card ring-line')}>
                  {slot.label}
                </span>
              ))}
            </div>
          </button>
        ))}
      </div>
      {openGroup ? (
        <div className="mt-5 rounded-lg border border-line bg-background-card p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="font-mono text-xs font-black text-primary-dark">{openGroup.shotId}</div>
              <h4 className="mt-1 text-lg font-black text-ink [overflow-wrap:anywhere]">{openGroup.title}</h4>
              <div className="mt-2 flex flex-wrap gap-2 text-[11px] font-bold text-ink-muted">
                {openGroup.durationSec ? <span className="rounded bg-white px-2 py-1 ring-1 ring-line">{openGroup.durationSec}s</span> : null}
                <span className="rounded bg-white px-2 py-1 ring-1 ring-line">产物 {openGroup.artifactCounts.total}</span>
                <span className="rounded bg-white px-2 py-1 ring-1 ring-line">参考 {openGroup.artifactCounts.references}</span>
                <span className="rounded bg-white px-2 py-1 ring-1 ring-line">媒体 {openGroup.artifactCounts.media}</span>
              </div>
              {openGroup.generationStrategy && (
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <span className="rounded-full bg-primary-soft px-3 py-1 text-xs font-black text-primary-dark">
                    {openGroup.generationStrategy.label}
                  </span>
                  {openGroup.generationStrategy.reason && (
                    <span className="max-w-full text-xs leading-5 text-ink-muted [overflow-wrap:anywhere]">{openGroup.generationStrategy.reason}</span>
                  )}
                </div>
              )}
            </div>
            <StatusBadge status={openGroup.status} />
          </div>
          <div className="mt-4 grid gap-3 lg:grid-cols-2">
            {openGroup.narrationText ? <ShotTextBlock title="口播脚本" value={openGroup.narrationText} /> : null}
            {openGroup.visualText ? <ShotTextBlock title="画面说明" value={openGroup.visualText} /> : null}
          </div>
          <div className="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-2">
            {openGroup.slots.map((slot) => (
              <ShotAssetSlotCard
                key={slot.kind}
                shotId={openGroup.shotId}
                slot={slot}
                promptPreviews={promptPreviews}
                projectReady={Boolean(projectId)}
                uploadingKey={uploadingKey}
                onUpload={uploadShotAsset}
              />
            ))}
          </div>
          {message ? <p className="mt-4 text-sm font-semibold text-green-700">{message}</p> : null}
          {error ? <p className="mt-4 text-sm font-semibold text-red-600">{error}</p> : null}
        </div>
      ) : null}
    </section>
  )
}

function ShotTextBlock({ title, value }: { title: string; value: string }) {
  return (
    <div className="rounded-lg bg-white p-4 ring-1 ring-line">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-black text-primary-dark">{title}</span>
        <CopyButton value={value} label="复制" />
      </div>
      <p className="mt-2 text-sm leading-6 text-ink-muted">{value}</p>
    </div>
  )
}

function ShotAssetSlotCard({
  shotId,
  slot,
  promptPreviews,
  projectReady,
  uploadingKey,
  onUpload,
}: {
  shotId: string
  slot: DirectorShotAssetSlot
  promptPreviews: Record<string, ShotPromptPreview>
  projectReady: boolean
  uploadingKey: string | null
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
}) {
  const requests = slot.dependencyRequests
    .map((artifact) => promptPreviews[artifact.id]?.request)
    .filter((request): request is ExternalGenerationRequestContent => Boolean(request))
  const copyValue = requests.length
    ? requests.map((request) => buildExternalGenerationTaskPackage(request).fullText).join('\n\n---\n\n')
    : slot.artifacts.map(artifactToCopyText).join('\n\n')
  const canUpload = Boolean(slot.uploadKind)
  const accept = slot.uploadKind === 'video' ? 'video/*' : 'image/*'
  const manualUploadKey = `${shotId}-${slot.kind}-manual`

  return (
    <div className="rounded-lg border border-line bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="grid h-8 w-8 place-items-center rounded-lg bg-primary-soft text-primary-dark">{slotIcon(slot.kind)}</span>
            <div>
              <div className="text-sm font-black text-ink">{slot.label}</div>
              <div className="mt-0.5 text-xs text-ink-muted">{slot.description}</div>
            </div>
          </div>
        </div>
        <StatusBadge status={slot.status} label={slot.kind === 'prompt' ? '可复制' : slot.status === 'review' && canUpload ? '可回填' : undefined} />
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        {copyValue ? <CopyButton value={copyValue} label={slot.kind === 'prompt' ? '复制任务包' : '复制信息'} /> : null}
        {canUpload ? (
          <label className={clsx(
            'inline-flex cursor-pointer items-center gap-1.5 rounded-lg bg-primary px-2.5 py-1.5 text-xs font-black text-white',
            (!projectReady || uploadingKey === manualUploadKey) && 'cursor-not-allowed opacity-50',
          )}>
            <FiUpload /> {uploadingKey === manualUploadKey ? '上传中...' : `上传${slot.label}`}
            <input
              type="file"
              accept={accept}
              data-smoke-id={`shot-${slot.kind}-upload`}
              disabled={!projectReady || uploadingKey === manualUploadKey}
              className="sr-only"
              onChange={(event) => onUpload(event, shotId, slot)}
            />
          </label>
        ) : null}
      </div>
      <div className="mt-3 space-y-3">
        {slot.kind === 'prompt' && !requests.length && slot.artifacts.length === 0 ? (
          <p className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">当前 shot 的提示词还未生成；生成后会在这里展示，可直接复制到外部网站。</p>
        ) : null}
        {requests.map((request) => (
          <ShotExternalRequestCard
            key={request.requestId}
            shotId={shotId}
            slot={slot}
            request={request}
            projectReady={projectReady}
            allowUpload={slot.kind !== 'prompt'}
            uploading={uploadingKey === `${shotId}-${slot.kind}-${request.requestId}`}
            onUpload={onUpload}
          />
        ))}
        {slot.dependencyRequests.map((artifact) => {
          const preview = promptPreviews[artifact.id]
          if (!preview || preview.request) return null
          return (
            <div key={artifact.id} className="rounded-lg bg-background-card p-3 text-xs text-ink-muted ring-1 ring-line">
              {preview.loading ? '正在读取外部生成请求...' : preview.error || '生成请求暂不可读'}
            </div>
          )
        })}
        {slot.artifacts.filter((artifact) => !slot.dependencyRequests.some((request) => request.id === artifact.id)).map((artifact) => (
          <div key={artifact.id} className="min-w-0 rounded-lg bg-background-card p-3 ring-1 ring-line">
            <div className="flex items-center justify-between gap-2">
              <div className="truncate text-xs font-black text-ink" title={artifact.name}>{artifact.name}</div>
              <StatusBadge status={artifact.status} />
            </div>
            <div className="mt-1 truncate font-mono text-[11px] text-ink-soft" title={artifact.storageRef}>{artifact.storageRef || artifact.id}</div>
          </div>
        ))}
        {slot.kind !== 'prompt' && !slot.artifacts.length ? (
          <p className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">暂无已登记素材。可以先在本地或外部网页生成，再用上方按钮上传到这个 shot。</p>
        ) : null}
      </div>
    </div>
  )
}

function ShotExternalRequestCard({
  shotId,
  slot,
  request,
  projectReady,
  allowUpload,
  uploading,
  onUpload,
}: {
  shotId: string
  slot: DirectorShotAssetSlot
  request: ExternalGenerationRequestContent
  projectReady: boolean
  allowUpload: boolean
  uploading: boolean
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
}) {
  const targetText = [
    request.target?.aspectRatio,
    request.target?.resolution,
    request.target?.durationSec ? `${request.target.durationSec}s` : '',
  ].filter(Boolean).join(' / ')
  const accept = request.kind === 'video' ? 'video/*' : 'image/*'
  const guideSteps = externalGenerationGuideSteps(request)
  const taskPackage = buildExternalGenerationTaskPackage(request)

  return (
    <div className="rounded-lg border border-primary/20 bg-primary-soft/40 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate font-mono text-[11px] font-black text-primary-dark">{request.requestId}</div>
          <div className="mt-0.5 text-xs text-ink-muted">{request.kind === 'image' ? '图片生成请求' : '视频生成请求'}{targetText ? ` · ${targetText}` : ''}</div>
        </div>
        <div className="flex flex-wrap gap-2">
          <CopyButton value={taskPackage.fullText} label="复制完整任务包" />
          <CopyButton value={taskPackage.positivePrompt} label="正向词" />
          {request.negativePrompt ? <CopyButton value={taskPackage.negativePrompt} label="负向词" /> : null}
          <CopyButton value={taskPackage.parameterText} label="参数" />
          <CopyButton value={taskPackage.referenceManifest} label="参考图" />
          <button
            type="button"
            onClick={() => downloadTextFile(`${request.requestId || shotId}-reference-package.md`, taskPackage.fullText, 'text/markdown')}
            className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"
          >
            <FiDownload /> 下载包
          </button>
          {allowUpload ? (
            <label className={clsx(
              'inline-flex cursor-pointer items-center gap-1.5 rounded-lg bg-primary px-2.5 py-1.5 text-xs font-black text-white',
              (!projectReady || uploading) && 'cursor-not-allowed opacity-50',
            )}>
              <FiUpload /> {uploading ? '上传中...' : '上传结果'}
              <input
                type="file"
                accept={accept}
                data-smoke-id="external-generation-upload"
                disabled={!projectReady || uploading}
                className="sr-only"
                onChange={(event) => onUpload(event, shotId, slot, request)}
              />
            </label>
          ) : null}
        </div>
      </div>
      <pre className="mt-3 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-white p-3 text-xs leading-5 text-ink ring-1 ring-line">{request.prompt}</pre>
      {request.references.length ? (
        <div className="mt-3 grid gap-2 sm:grid-cols-2">
          {request.references.map((ref) => (
            <div key={`${ref.id}-${ref.storageRef}`} className="min-w-0 rounded-lg bg-white p-2 ring-1 ring-line">
              <div className="truncate text-xs font-black text-ink">{ref.label || ref.id}</div>
              <div className="mt-1 truncate font-mono text-[11px] text-ink-soft" title={ref.storageRef}>{ref.storageRef}</div>
            </div>
          ))}
        </div>
      ) : null}
      <details className="mt-3">
        <summary className="cursor-pointer text-xs font-black text-primary-dark">网页端生成步骤</summary>
        <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs leading-5 text-primary-dark">
          {guideSteps.map((step) => <li key={step}>{step}</li>)}
        </ol>
      </details>
    </div>
  )
}

function slotIcon(kind: DirectorShotAssetSlot['kind']) {
  if (kind === 'base-media') return <FiVideo />
  if (kind === 'overlay') return <FiLayers />
  if (kind === 'video') return <FiVideo />
  if (kind === 'storyboard') return <FiLayers />
  if (kind === 'reference') return <FiFolder />
  return <FiFileText />
}

async function uploadAndRegisterShotAsset({
  projectId,
  shotId,
  slot,
  request,
  file,
}: {
  projectId: string
  shotId: string
  slot: DirectorShotAssetSlot
  request?: ExternalGenerationRequestContent
  file: File
}) {
  if (!slot.uploadKind) throw new Error('该素材槽不支持文件上传')
  const kind = request?.kind || slot.uploadKind
  const referenceAssetIds = (request?.references || []).map((ref) => ref.id).filter(Boolean).slice(0, 6)
  const uploadId = safeLocalUploadId([
    shotId,
    slot.kind,
    request?.requestId || file.name,
  ].filter(Boolean).join('-'))
  const tags = ['manual_shot_upload', `shot_${slot.kind}`]
  if (request?.requestId) tags.push('external_manual_upload', 'material_dependency_result')
  const localArtifact = await uploadLocalArtifactFile({
    projectId,
    id: uploadId,
    file,
    mimeType: file.type || (kind === 'image' ? 'image/png' : 'video/mp4'),
    metadata: {
      artifactType: request?.requestId ? 'external_manual_generation_result' : `manual_shot_${slot.kind}`,
      externalGenerationRequestId: request?.requestId,
      generationKind: kind,
      relatedShotId: shotId,
      shotAssetSlot: slot.kind,
      source: request?.requestId ? 'external_manual_upload' : 'manual_shot_upload',
      cloudPayloadStored: false,
      localOnly: true,
      tags,
    },
  })
  if (!localArtifact.storageRef) {
    throw new Error('local agent 未返回 storageRef')
  }
  return registerExternalGenerationResult(projectId, {
    kind,
    storageType: 'local',
    storageRef: localArtifact.storageRef,
    mimeType: localArtifact.mimeType || file.type || undefined,
    sizeBytes: localArtifact.sizeBytes,
    contentHash: localArtifact.contentHash,
    relatedShotId: shotId,
    generationRequestId: request?.requestId,
    source: request?.requestId ? 'external_manual_upload' : 'manual_shot_upload',
    description: `${shotId} ${slot.label}`,
    tags,
    referenceAssetIds,
  })
}

function RolesPage({ stages }: { stages: DirectorStage[] }) {
  return (
    <div className="space-y-5">
      <section className="card p-6">
        <p className="text-sm font-bold text-primary-dark">团队与角色</p>
        <h2 className="mt-2 text-3xl font-black text-ink">智能体多角色创作团队</h2>
        <p className="mt-2 text-sm leading-6 text-ink-muted">每个角色都有固定职责、工具权限、输入产物、输出产物和审核边界。阶段守卫防止角色越权调用工具。</p>
      </section>
      <div className="grid grid-cols-1 gap-5 md:grid-cols-2 xl:grid-cols-3">
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

function ExportPage({ artifacts, durationSec, projectId }: { artifacts: DirectorArtifactRecord[]; durationSec: number; projectId?: string }) {
  const deliveryItems = useMemo(() => buildExportDeliveryItems(artifacts), [artifacts])
  const video = deliveryItems.find((item) => item.id === 'final-video')?.artifact
  const packageArtifact = deliveryItems.find((item) => item.id === 'project-package')?.artifact
  const publishArtifact = deliveryItems.find((item) => item.id === 'publish-copy')?.artifact
  const previewVideoRef = useRef<HTMLVideoElement>(null)
  const [videoPreviewUrl, setVideoPreviewUrl] = useState<string | null>(null)
  const [videoPreviewLoading, setVideoPreviewLoading] = useState(false)
  const [videoPreviewError, setVideoPreviewError] = useState<string | null>(null)
  const [videoLocalPath, setVideoLocalPath] = useState<string | null>(null)
  const [publishContent, setPublishContent] = useState<unknown>(null)
  const [publishLoading, setPublishLoading] = useState(false)
  const [publishError, setPublishError] = useState<string | null>(null)
  const videoReady = video?.status === 'valid' && Boolean(video.storageRef)
  const packageReady = packageArtifact?.status === 'valid' && Boolean(packageArtifact.storageRef)
  const videoStorageRef = video?.storageRef || ''
  useEffect(() => {
    let cancelled = false
    let objectUrl: string | null = null
    setVideoPreviewUrl(null)
    setVideoPreviewError(null)
    setVideoPreviewLoading(false)
    setVideoLocalPath(null)

    if (!videoReady || !videoStorageRef) return undefined
    const directUrl = directMediaPreviewUrl(videoStorageRef)
    if (directUrl) {
      setVideoPreviewUrl(directUrl)
      return undefined
    }

    const localArtifactId = localArtifactIdFromStorageRef(videoStorageRef)
    if (!localArtifactId) {
      setVideoPreviewError('最终视频已登记，但还没有可读取的本地视频文件。')
      return undefined
    }
    if (!projectId) {
      setVideoPreviewError('缺少项目 ID，无法读取本地视频文件。')
      return undefined
    }

    setVideoPreviewLoading(true)
    fetchLocalArtifactFile({ projectId, id: localArtifactId })
      .then((localArtifact) => {
        if (cancelled) return
        const blob = localArtifactFileToBlob(localArtifact, video?.metadata)
        objectUrl = URL.createObjectURL(blob)
        setVideoLocalPath(localArtifact.path || null)
        setVideoPreviewUrl(objectUrl)
      })
      .catch((err) => {
        if (!cancelled) setVideoPreviewError(normalizeDirectorErrorMessage(err))
      })
      .finally(() => {
        if (!cancelled) setVideoPreviewLoading(false)
      })

    return () => {
      cancelled = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [projectId, video?.id, video?.metadata, videoReady, videoStorageRef])
  useEffect(() => {
    let cancelled = false
    setPublishContent(null)
    setPublishError(null)
    if (!publishArtifact) return undefined
    if (!isInspectableArtifact(publishArtifact)) {
      setPublishError('发布文案产物还未写入项目产物库。')
      return undefined
    }
    setPublishLoading(true)
    fetchArtifactContent(publishArtifact.id)
      .then((result) => {
        if (!cancelled) setPublishContent(result.content)
      })
      .catch((err) => {
        if (!cancelled) setPublishError(normalizeDirectorErrorMessage(err))
      })
      .finally(() => {
        if (!cancelled) setPublishLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [publishArtifact])
  const publishCopies = useMemo(() => buildPublishCopies(publishContent), [publishContent])
  const publishReady = publishCopies.length > 0
  const publishLocalOnlyPointer = isLocalOnlyArtifactPointer(publishContent)
  const markdown = publishCopiesToMarkdown(publishCopies)
  const json = publishCopiesToJSON(publishCopies)
  const openFinalVideoFolder = async () => {
    if (!videoLocalPath) {
      setVideoPreviewError('当前环境无法直接打开本地文件夹，请复制交付物清单中的 storageRef 定位文件。')
      return
    }
    const opened = await openLocalPath(videoLocalPath)
    if (!opened) setVideoPreviewError('当前浏览器环境不能打开本地文件夹，请在桌面端使用该功能。')
  }
  return (
    <div className="space-y-5">
      <div className="grid grid-cols-1 gap-5 xl:grid-cols-12">
      <section className="card p-6 xl:col-span-7">
        <div className="flex items-center justify-between"><div><p className="text-sm font-bold text-primary-dark">最终预览 / 导出</p><h2 className="mt-2 text-2xl font-black text-ink">最终视频预览</h2></div><StatusBadge status={videoReady ? (videoPreviewError ? 'review' : 'valid') : 'pending'} label={videoReady ? (videoPreviewError ? '已登记待读取' : 'final.mp4 已生成') : '等待渲染'} /></div>
        <div className="mt-6 overflow-hidden rounded-xl bg-ink shadow-card ring-1 ring-line">
          {videoPreviewUrl ? (
            <video
              ref={previewVideoRef}
              data-testid="final-video-preview"
              className="h-[410px] w-full bg-black object-contain"
              src={videoPreviewUrl}
              controls
              playsInline
              preload="metadata"
            />
          ) : (
            <div className="relative h-[410px] bg-[linear-gradient(135deg,#1A0B02,#2B1606_42%,#8B4A12_74%,#E89412)] p-10 text-white">
              <div className="relative z-10 flex h-full flex-col justify-between">
                <div><span className="rounded-full bg-white/15 px-4 py-2 text-xs font-bold ring-1 ring-white/20">智能视频创作工作台</span><h3 className="mt-10 max-w-lg text-5xl font-black">等待最终视频</h3><p className="mt-5 text-lg text-amber-100">{videoPreviewLoading ? '正在读取本地视频文件...' : videoPreviewError || '生成完成后会在这里出现播放器'}</p></div>
                <div className="flex items-center gap-4 rounded-lg bg-black/25 p-4 ring-1 ring-white/10"><FiPlayCircle className="text-3xl" /><div className="h-1 flex-1 overflow-hidden rounded-full bg-white/20"><div className="h-full w-[28%] rounded-full bg-primary-light" /></div><span className="text-sm">0:00 / {formatSeconds(durationSec)}</span></div>
              </div>
            </div>
          )}
        </div>
      </section>
      <aside className="space-y-5 xl:col-span-5">
        <section className="card p-6">
          <h3 className="text-lg font-black text-ink">导出操作</h3>
          {!videoReady && <div className="mb-3 rounded-lg bg-amber-50 p-3 text-xs font-semibold text-primary-dark ring-1 ring-amber-200">最终视频尚未生成</div>}
          {videoPreviewError && <div className="mb-3 rounded-lg bg-amber-50 p-3 text-xs font-semibold text-primary-dark ring-1 ring-amber-200">{videoPreviewError}</div>}
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2"><button disabled={!videoPreviewUrl} onClick={() => { void previewVideoRef.current?.play().catch(() => undefined) }} className="flex items-center justify-center gap-2 rounded-lg bg-primary px-4 py-3 text-sm font-black text-white shadow-glow disabled:cursor-not-allowed disabled:opacity-45"><FiPlayCircle /> 预览视频</button><button disabled={!videoReady} onClick={() => { void openFinalVideoFolder() }} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"><FiFolder /> 打开文件夹</button></div>
          <button disabled={!videoReady} className="mt-3 flex w-full items-center justify-center gap-2 rounded-lg bg-violet px-4 py-3 text-sm font-black text-white disabled:cursor-not-allowed disabled:opacity-45"><FiDownload /> {packageReady ? '下载交付包' : '导出交付包'}</button>
          <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <button disabled={!publishReady} onClick={() => downloadTextFile('publish-copy.md', markdown, 'text/markdown')} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"><FiFileText /> Markdown</button>
            <button disabled={!publishReady} onClick={() => downloadTextFile('publish-copy.json', json, 'application/json')} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"><FiDownload /> JSON</button>
          </div>
        </section>
        <section className="card p-6">
          <h3 className="text-lg font-black text-ink">交付物清单</h3>
          <div className="mt-4 space-y-3 text-sm">
            {deliveryItems.map((item) => <ExportDeliveryRow key={item.id} item={item} />)}
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
          {publishReady ? <CopyButton value={markdown} label="复制全部" /> : null}
        </div>
        <div className="mt-5 grid grid-cols-1 gap-5 xl:grid-cols-2">
          {publishLoading ? (
            <div className="rounded-lg bg-white p-5 text-sm font-semibold text-ink-muted ring-1 ring-line">正在读取发布文案产物...</div>
          ) : publishError ? (
            <div className="rounded-lg bg-red-50 p-5 text-sm font-semibold text-red-700 ring-1 ring-red-200">{publishError}</div>
          ) : !publishReady ? (
            <div className="rounded-lg bg-amber-50 p-5 text-sm font-semibold text-primary-dark ring-1 ring-amber-200">
              {publishLocalOnlyPointer ? '发布文案产物已生成，但正文只返回了本地索引，当前客户端无法读取完整文案。' : '等待 publish_copy_generator 根据口播稿生成标题、简介和关键词。'}
            </div>
          ) : publishCopies.map((copy) => (
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

function ExportDeliveryRow({ item }: { item: ExportDeliveryItem }) {
  return (
    <div className="rounded-lg bg-background-card px-4 py-3 ring-1 ring-line">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-sm font-black text-ink">{item.label}</div>
          <div className="mt-1 text-xs leading-5 text-ink-muted">{item.description}</div>
          {item.storageRef ? <div className="mt-1 truncate font-mono text-[11px] text-ink-soft" title={item.storageRef}>{item.storageRef}</div> : null}
        </div>
        <StatusBadge status={item.status} />
      </div>
    </div>
  )
}

function directMediaPreviewUrl(storageRef: string): string | null {
  const ref = storageRef.trim()
  return /^(https?:|blob:|data:)/u.test(ref) ? ref : null
}

function isLocalOnlyArtifactPointer(content: unknown): boolean {
  if (!content || typeof content !== 'object') return false
  const record = content as Record<string, unknown>
  return record.contentAvailability === 'local-agent' ||
    (record.localOnly === true && typeof record.storageRef === 'string' && record.storageRef.startsWith('local://'))
}

function localArtifactFileToBlob(localArtifact: LocalArtifactFileResponse, metadata?: Record<string, unknown>): Blob {
  const metadataMimeType =
    typeof metadata?.mimeType === 'string' ? metadata.mimeType :
      typeof metadata?.mime_type === 'string' ? metadata.mime_type : ''
  const mimeType = localArtifact.mimeType || metadataMimeType || 'video/mp4'
  if (localArtifact.contentBase64) {
    return blobFromBase64(localArtifact.contentBase64, mimeType)
  }
  if (typeof localArtifact.content === 'string') {
    return new Blob([localArtifact.content], { type: mimeType })
  }
  throw new Error('本地视频文件没有可读取内容')
}

function blobFromBase64(contentBase64: string, mimeType: string): Blob {
  const binary = window.atob(contentBase64)
  const chunks: ArrayBuffer[] = []
  const chunkSize = 8192
  for (let offset = 0; offset < binary.length; offset += chunkSize) {
    const slice = binary.slice(offset, offset + chunkSize)
    const buffer = new ArrayBuffer(slice.length)
    const bytes = new Uint8Array(buffer)
    for (let index = 0; index < slice.length; index += 1) {
      bytes[index] = slice.charCodeAt(index)
    }
    chunks.push(buffer)
  }
  return new Blob(chunks, { type: mimeType })
}

function ArtifactTable({ artifacts, compact = false, projectId, onArtifactsChanged }: { artifacts: DirectorArtifactRecord[]; compact?: boolean; projectId?: string; onArtifactsChanged?: () => Promise<void> | void }) {
  const [selectedId, setSelectedId] = useState<string | undefined>()
  const [content, setContent] = useState<unknown>(null)
  const [history, setHistory] = useState<Artifact[]>([])
  const [viewerLoading, setViewerLoading] = useState(false)
  const [viewerError, setViewerError] = useState<string | null>(null)
  const [revisionMessage, setRevisionMessage] = useState('')
  const [revisionLoading, setRevisionLoading] = useState(false)
  const [externalUploading, setExternalUploading] = useState(false)
  const [externalUploadMessage, setExternalUploadMessage] = useState<string | null>(null)
  const headers = ['ID', '名称', '类型', '状态', '负责人', '操作']
  const selected = artifacts.find((artifact) => artifact.id === selectedId)
  const externalRequest = useMemo(() => externalGenerationRequestFromContent(content), [content])

  useEffect(() => {
    if (selectedId && !artifacts.some((artifact) => artifact.id === selectedId)) {
      setSelectedId(undefined)
      setContent(null)
      setHistory([])
      setViewerError(null)
    }
  }, [artifacts, selectedId])

  const loadArtifact = async (artifact: DirectorArtifactRecord) => {
    const selection = getArtifactViewerSelection(selectedId, artifact)
    setSelectedId(selection.selectedId)
    setExternalUploadMessage(null)
    setViewerError(null)
    if (!selection.shouldLoad) {
      setViewerLoading(false)
      setContent(selection.placeholder ?? null)
      setHistory([])
      return
    }
    setContent(null)
    setHistory([])
    setViewerLoading(true)
    try {
      const [nextContent, nextHistory] = await Promise.all([
        fetchArtifactContent(artifact.id),
        fetchArtifactHistory(artifact.id),
      ])
      setContent(nextContent.content)
      setHistory(nextHistory.history || [])
    } catch (err) {
      setContent(null)
      setHistory([])
      setViewerError(normalizeDirectorErrorMessage(err))
    } finally {
      setViewerLoading(false)
    }
  }

  const submitRevision = async () => {
    if (!selected || !revisionMessage.trim() || !isInspectableArtifact(selected)) return
    setRevisionLoading(true)
    setViewerError(null)
    try {
      const clientModelProviders = await buildClientModelProvidersForRun()
      const revised = await reviseArtifact(selected.id, revisionMessage.trim(), clientModelProviders as Record<string, unknown> | undefined)
      setContent(revised.content)
      const nextHistory = await fetchArtifactHistory(selected.id)
      setHistory(nextHistory.history || [])
      setRevisionMessage('')
      await onArtifactsChanged?.()
    } catch (err) {
      setViewerError(normalizeDirectorErrorMessage(err))
    } finally {
      setRevisionLoading(false)
    }
  }

  const uploadExternalResult = async (event: ChangeEvent<HTMLInputElement>, request: ExternalGenerationRequestContent) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || !selected) return
    if (!projectId) {
      setViewerError('缺少项目 ID，无法登记外部生成结果。')
      return
    }
    setExternalUploading(true)
    setViewerError(null)
    setExternalUploadMessage(null)
    try {
      const kind = request.kind === 'image' ? 'image' : 'video'
      const referenceAssetIds = (request.references || []).map((ref) => ref.id).filter(Boolean).slice(0, 6)
      const localArtifact = await uploadLocalArtifactFile({
        projectId,
        id: safeLocalUploadId(`${request.requestId || selected.id}-result`),
        file,
        mimeType: file.type || (kind === 'image' ? 'image/png' : 'video/mp4'),
        metadata: {
          artifactType: 'external_manual_generation_result',
          externalGenerationRequestId: request.requestId,
          generationKind: kind,
          relatedShotId: request.shotId,
          referenceAssetIds,
          source: 'external_manual_upload',
          cloudPayloadStored: false,
          localOnly: true,
        },
      })
      if (!localArtifact.storageRef) {
        throw new Error('local agent 未返回 storageRef')
      }
      const registered = await registerExternalGenerationResult(projectId, {
        kind,
        storageType: 'local',
        storageRef: localArtifact.storageRef,
        mimeType: localArtifact.mimeType || file.type || undefined,
        sizeBytes: localArtifact.sizeBytes,
        contentHash: localArtifact.contentHash,
        relatedShotId: request.shotId,
        generationRequestId: request.requestId,
        source: 'external_manual_upload',
        tags: ['external_manual_upload', 'material_dependency_result'],
        referenceAssetIds,
      })
      setExternalUploadMessage(`已登记 ${registered.artifact?.name || registered.artifact?.id || '外部生成结果'}`)
      await onArtifactsChanged?.()
    } catch (err) {
      setViewerError(normalizeDirectorErrorMessage(err))
    } finally {
      setExternalUploading(false)
    }
  }

  return (
    <section className={clsx('card overflow-x-auto p-0', compact && 'mt-6')}>
      <table className="min-w-[760px] w-full text-left text-sm">
        <thead className="bg-background-mist text-xs text-ink-soft">
          <tr>
            {headers.map((h) => (
              <th className="whitespace-nowrap px-4 py-3" key={h}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-line bg-white/70">
          {artifacts.map((artifact) => {
            const active = selectedId === artifact.id
            return (
              <Fragment key={artifact.id}>
                <tr>
                  <td className="whitespace-nowrap px-4 py-3 font-mono text-xs font-bold">{artifact.id}</td>
                  <td className="whitespace-nowrap px-4 py-3 font-semibold text-ink">{artifact.name}</td>
                  <td className="whitespace-nowrap px-4 py-3 text-ink-muted">{displayNameForArtifact(artifact.kind)}</td>
                  <td className="whitespace-nowrap px-4 py-3">
                    <StatusBadge
                      status={artifact.status}
                      label={isMaterialDependencyRequest(artifact) && artifact.status === 'review' ? '素材待回填' : undefined}
                    />
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-ink-muted">{artifact.owner}</td>
                  <td className="whitespace-nowrap px-4 py-3">
                    <div className="flex flex-wrap gap-2">
                      <button
                        onClick={() => loadArtifact(artifact)}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"
                      >
                        <FiFileText /> {active ? '收起' : '查看'}
                      </button>
                      <CopyButton value={artifactToCopyText(artifact)} label="复制" />
                    </div>
                  </td>
                </tr>
                {active && (
                  <tr>
                    <td colSpan={headers.length} className="bg-background-card px-4 py-4">
                      <div className={clsx('grid gap-4', compact ? 'grid-cols-1' : 'grid-cols-12')}>
                        <div className={clsx('min-w-0 rounded-lg border border-line bg-white p-4 shadow-sm', compact ? '' : 'col-span-12 xl:col-span-8')}>
                          <div className="flex items-center justify-between gap-3">
                            <div className="min-w-0">
                              <div className="truncate text-sm font-black text-ink">{artifact.name}</div>
                              <div className="mt-1 truncate font-mono text-[11px] text-ink-soft">{artifact.storageRef || 'storage_ref pending'}</div>
                            </div>
                            {content !== null && content !== undefined ? <CopyButton value={artifactContentText(content)} label="复制正文" /> : null}
                          </div>
                          <div className="mt-4 max-h-[420px] overflow-auto rounded-lg border border-line bg-background-card p-4">
                            {viewerLoading ? (
                              <p className="text-sm text-ink-muted">正在加载产物正文...</p>
                            ) : viewerError ? (
                              <p className="text-sm font-semibold text-red-600">{viewerError}</p>
                            ) : content !== null && content !== undefined ? (
                              <ReviewContent text={artifactContentText(content)} />
                            ) : (
                              <p className="text-sm text-ink-muted">暂无可展示正文。</p>
                            )}
                          </div>
                          {externalRequest ? (
                            <ExternalGenerationRequestPanel
                              request={externalRequest}
                              disabled={!projectId || externalUploading}
                              uploading={externalUploading}
                              message={externalUploadMessage}
                              onUpload={uploadExternalResult}
                            />
                          ) : null}
                        </div>
                        <div className={clsx('space-y-3', compact ? '' : 'col-span-12 xl:col-span-4')}>
                          <div className="rounded-lg border border-line bg-white p-4 shadow-sm">
                            <div className="flex items-center gap-2 text-sm font-black text-ink"><FiLayers /> 版本历史</div>
                            <div className="mt-3 space-y-2">
                              {history.length ? history.map((item) => (
                                <div key={item.id} className="rounded-lg bg-background-card p-3 ring-1 ring-line">
                                  <div className="flex items-center justify-between gap-2">
                                    <span className="font-mono text-xs font-bold text-ink">v{item.version}</span>
                                    <span className="text-xs text-ink-soft">{artifactHistoryStatus(item)}</span>
                                  </div>
                                  <div className="mt-1 truncate text-xs text-ink-muted" title={item.storageRef}>{formatArtifactHistoryLabel(item)}</div>
                                </div>
                              )) : <p className="text-xs text-ink-muted">暂无历史版本。</p>}
                            </div>
                          </div>
                          <div className="rounded-lg border border-primary/25 bg-white p-4 shadow-sm">
                            <div className="flex items-center gap-2 text-sm font-black text-ink"><FiEdit3 /> 返工意见</div>
                            <textarea
                              value={revisionMessage}
                              onChange={(event) => setRevisionMessage(event.target.value)}
                              placeholder="输入希望修改的方向..."
                              className="mt-3 h-24 w-full resize-none rounded-lg border border-line bg-background-card p-3 text-sm leading-6 outline-none focus:border-primary"
                            />
                            <button
                              onClick={submitRevision}
                              disabled={revisionLoading || !revisionMessage.trim() || !selected || !isInspectableArtifact(selected)}
                              className="mt-3 inline-flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-3 py-2 text-sm font-black text-white disabled:cursor-not-allowed disabled:opacity-50"
                            >
                              <FiRefreshCw /> {revisionLoading ? '返工中...' : '提交返工'}
                            </button>
                          </div>
                        </div>
                      </div>
                    </td>
                  </tr>
                )}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </section>
  )
}

interface ExternalGenerationReference {
  id: string
  label?: string
  role?: string
  storageRef: string
  artifactId?: string
}

interface ExternalGenerationRequestContent {
  requestId: string
  kind: 'image' | 'video'
  shotId?: string
  prompt: string
  negativePrompt?: string
  references: ExternalGenerationReference[]
  target?: {
    aspectRatio?: string
    durationSec?: number
    resolution?: string
  }
  promptCharLimit?: number
  referenceImageLimit?: number
}

function ExternalGenerationRequestPanel({
  request,
  disabled,
  uploading,
  message,
  onUpload,
}: {
  request: ExternalGenerationRequestContent
  disabled: boolean
  uploading: boolean
  message: string | null
  onUpload: (event: ChangeEvent<HTMLInputElement>, request: ExternalGenerationRequestContent) => void
}) {
  const accept = request.kind === 'image' ? 'image/*' : 'video/*'
  const targetText = [
    request.target?.aspectRatio,
    request.target?.resolution,
    request.target?.durationSec ? `${request.target.durationSec}s` : '',
  ].filter(Boolean).join(' / ')
  const guideSteps = externalGenerationGuideSteps(request)
  const taskPackage = buildExternalGenerationTaskPackage(request)

  return (
    <div className="mt-4 rounded-lg border border-primary/25 bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-sm font-black text-ink">素材依赖点</div>
          <div className="mt-1 text-xs text-ink-muted">
            {request.kind === 'image' ? '图片' : '视频'} · 参考图 {request.references.length}/{request.referenceImageLimit || 6}
            {targetText ? ` · ${targetText}` : ''}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <StatusBadge status="review" label="待用户回填" />
          <CopyButton value={taskPackage.fullText} label="复制完整任务包" />
          <CopyButton value={taskPackage.positivePrompt} label="正向词" />
          {request.negativePrompt ? <CopyButton value={taskPackage.negativePrompt} label="负向词" /> : null}
          <CopyButton value={taskPackage.parameterText} label="参数" />
          <CopyButton value={taskPackage.referenceManifest} label="参考图" />
          <button
            type="button"
            onClick={() => downloadTextFile(`${request.requestId}-external-task.md`, taskPackage.fullText, 'text/markdown')}
            className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"
          >
            <FiDownload /> 下载包
          </button>
          <label className={clsx(
            'inline-flex cursor-pointer items-center gap-1.5 rounded-lg bg-primary px-2.5 py-1.5 text-xs font-black text-white',
            disabled && 'cursor-not-allowed opacity-50'
          )}>
            <FiUpload /> {uploading ? '上传中...' : '上传回此依赖点'}
            <input
              type="file"
              accept={accept}
              data-smoke-id="external-generation-upload"
              disabled={disabled}
              className="sr-only"
              onChange={(event) => onUpload(event, request)}
            />
          </label>
        </div>
      </div>
      <p className="mt-3 text-xs leading-5 text-ink-muted">
        流程在这里等待用户提供素材。你可以把 Prompt 和参考图复制到任意图片或视频生成网站，生成后上传文件回填；系统只登记本地引用和依赖关系。
      </p>
      <div className="mt-3 rounded-lg border border-line bg-background-card p-3">
        <div className="text-xs font-black text-ink-soft">Prompt</div>
        <pre className="mt-2 max-h-48 whitespace-pre-wrap break-words text-xs leading-5 text-ink">{request.prompt}</pre>
      </div>
      <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 p-3">
        <div className="text-xs font-black text-primary-dark">浏览器手动生成步骤</div>
        <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs leading-5 text-primary-dark">
          {guideSteps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
      </div>
      {request.negativePrompt ? (
        <div className="mt-3 rounded-lg border border-line bg-background-card p-3">
          <div className="text-xs font-black text-ink-soft">Negative Prompt</div>
          <pre className="mt-2 max-h-32 whitespace-pre-wrap break-words text-xs leading-5 text-ink">{request.negativePrompt}</pre>
        </div>
      ) : null}
      {request.references.length ? (
        <div className="mt-3 grid gap-2 sm:grid-cols-2">
          {request.references.slice(0, 6).map((ref) => (
            <div key={`${ref.id}-${ref.storageRef}`} className="min-w-0 rounded-lg bg-background-card p-3 ring-1 ring-line">
              <div className="truncate text-xs font-black text-ink">{ref.label || ref.id}</div>
              <div className="mt-1 text-[11px] text-ink-muted">{ref.role || 'reference'}</div>
              <div className="mt-1 truncate font-mono text-[11px] text-ink-soft" title={ref.storageRef}>{ref.storageRef}</div>
            </div>
          ))}
        </div>
      ) : null}
      {message ? <p className="mt-3 text-xs font-semibold text-green-700">{message}</p> : null}
    </div>
  )
}

function isMaterialDependencyRequest(artifact: DirectorArtifactRecord): boolean {
  return artifact.kind === 'EXTERNAL_GENERATION_REQUEST' ||
    artifact.metadata?.artifactType === 'external_generation_request' ||
    artifact.metadata?.artifact_kind === 'external_generation_request'
}

function isInspectableArtifact(artifact: DirectorArtifactRecord) {
  return Boolean(artifact.id && !/^A\d{2}/.test(artifact.id))
}

function externalGenerationRequestFromContent(content: unknown): ExternalGenerationRequestContent | null {
  const raw = parseMaybeJSON(content)
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null
  const data = raw as Record<string, unknown>
  const prompt = stringField(data.prompt)
  const kind = stringField(data.kind)
  if (!prompt || (kind !== 'image' && kind !== 'video')) return null
  const references = Array.isArray(data.references)
    ? data.references
      .map((item) => referenceFromUnknown(item))
      .filter((item): item is ExternalGenerationReference => Boolean(item))
      .slice(0, 6)
    : []
  const target = objectField(data.target)
  return {
    requestId: stringField(data.requestId) || safeLocalUploadId(`${kind}-${prompt}`).slice(0, 48),
    kind,
    shotId: stringField(data.shotId) || undefined,
    prompt,
    negativePrompt: stringField(data.negativePrompt) || undefined,
    references,
    target: target ? {
      aspectRatio: stringField(target.aspectRatio) || undefined,
      durationSec: numberField(target.durationSec),
      resolution: stringField(target.resolution) || undefined,
    } : undefined,
    promptCharLimit: numberField(data.promptCharLimit),
    referenceImageLimit: numberField(data.referenceImageLimit),
  }
}

function referenceFromUnknown(value: unknown): ExternalGenerationReference | null {
  const item = objectField(value)
  if (!item) return null
  const storageRef = stringField(item.storageRef)
  if (!storageRef) return null
  return {
    id: stringField(item.id) || safeLocalUploadId(storageRef).slice(0, 32),
    label: stringField(item.label) || undefined,
    role: stringField(item.role) || undefined,
    storageRef,
    artifactId: stringField(item.artifactId) || undefined,
  }
}

function parseMaybeJSON(value: unknown): unknown {
  if (typeof value !== 'string') return value
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function objectField(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

function stringField(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function numberField(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function safeLocalUploadId(value: string): string {
  const cleaned = value.trim().replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^[._-]+|[._-]+$/g, '')
  return cleaned || 'external-result'
}

function artifactContentText(content: unknown): string {
  if (typeof content === 'string') return content
  if (content == null) return ''
  try {
    return JSON.stringify(content, null, 2)
  } catch {
    return String(content)
  }
}

function formatSeconds(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds))
  const minutes = Math.floor(total / 60)
  const rest = total % 60
  return `${minutes}:${String(rest).padStart(2, '0')}`
}

function formatArtifactHistoryLabel(artifact: Artifact): string {
  return artifact.createdAt || artifact.storageRef || artifact.name
}

function artifactHistoryStatus(artifact: Artifact): string {
  const metadataStatus = typeof artifact.metadata?.status === 'string' ? artifact.metadata.status : ''
  if (metadataStatus) return metadataStatus
  return artifact.isCurrent ? 'current' : 'archived'
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
    missing: '产物缺失',
  }
  return <span className={clsx('inline-flex items-center rounded-full px-2.5 py-1 text-xs font-bold ring-1', statusBadgeTone(status))}>{label ?? labelMap[status]}</span>
}

function InfoCard({ icon, title, value, desc, tone }: { icon: React.ReactNode; title: string; value: string; desc: string; tone: 'primary' | 'green' | 'blue' | 'violet' }) {
  const toneMap = { primary: 'bg-primary-soft text-primary-dark', green: 'bg-green-50 text-green-700', blue: 'bg-background-mist text-primary-dark', violet: 'bg-primary-soft text-violet' }
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

  return (
    <div className="markdown-body text-sm leading-7 text-ink [overflow-wrap:anywhere]">
      <ReactMarkdown>{text}</ReactMarkdown>
    </div>
  )
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
  if (status === 'running' || status === 'active') return 'border-line bg-primary-soft'
  if (status === 'done') return 'border-green-200 bg-green-50'
  if (status === 'blocked' || status === 'failed') return 'border-red-100 bg-red-50/60'
  return 'border-line bg-white/65'
}

function stageIconTone(status: DirectorStageStatus) {
  if (status === 'done') return 'bg-green-600 text-white'
  if (status === 'review') return 'bg-primary text-white'
  if (status === 'running' || status === 'active') return 'bg-primary text-white'
  if (status === 'blocked' || status === 'failed') return 'bg-red-500 text-white'
  return 'bg-stone-200 text-stone-600'
}

function statusBadgeTone(status: DirectorArtifactStatus | DirectorStageStatus) {
  if (status === 'done' || status === 'valid') return 'bg-green-50 text-green-700 ring-green-200'
  if (status === 'review') return 'bg-amber-50 text-primary-dark ring-amber-200'
  if (status === 'running' || status === 'active') return 'bg-primary-soft text-primary-dark ring-line'
  if (status === 'blocked' || status === 'stale' || status === 'failed' || status === 'missing') return 'bg-red-50 text-red-700 ring-red-200'
  return 'bg-stone-50 text-stone-600 ring-stone-200'
}
