import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent, type Dispatch, type MouseEvent, type ReactNode, type SetStateAction } from 'react'
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
import { TalkingHeadLayerInspector } from '../features/director-studio/talking-head/components/TalkingHeadLayerInspector'
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
  createLocalDiagnostics,
  fetchJiMengSetupStatus,
  fetchLocalArtifactFile,
  fetchModelProviderSettings,
  localArtifactRawUrl,
  openLocalPath,
  uploadLocalArtifactFile,
  type JiMengSetupStatusResponse,
  type LocalArtifactFileResponse,
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
  buildProjectAssemblySummary,
  buildPublishCopies,
  buildShotReviewGroups,
  creationProfileSummary,
  buildImageRegenerationInstruction,
  deriveNextAction,
  displayNameForArtifact,
  downstreamStaleArtifacts,
  extractDirectorErrorDetail,
  externalGenerationReferencesFromArtifacts,
  getArtifactViewerSelection,
  getStageStateDisplay,
  isShotProductionReview,
  isProjectSessionStarted,
  localArtifactIdFromStorageRef,
  localServiceStatusDisplay,
  mergeExternalGenerationTaskReferences,
  normalizeVideoCreationProfileId,
  normalizeDirectorErrorMessage,
  nextStageIdAfterReview,
  nextSelectedReviewId,
  overviewProjectStatus,
  preferredActiveReview,
  projectPrimaryAction,
  qualityGateTargetLines,
  publishCopiesToJSON,
  publishCopiesToMarkdown,
  reviewDisplayTitle,
  reviewOutputPanelHint,
  reviewOutputPanelTitle,
  reviewQualityReportLines,
  reviewOutputText,
  reviewStatusLabel,
  stageActionLabel,
  traceNodeHasError,
  unresolvedMaterialDependencyCount,
  visibleEnvironmentIssues,
  visibleReviewHistory,
  videoCreationProfileForId,
  videoCreationProfiles,
  type DirectorArtifactRecord,
  type DirectorArtifactStatus,
  type DirectorErrorDetail,
  type DirectorNavKey,
  type DirectorShotReviewGroup,
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
  { id: 'quality_reviewer', name: 'Quality Reviewer', displayName: '质量审核', stage: 'quality', goal: '检查抽帧清晰度、文字安全区、文件、时长、分辨率、视频流和产物完整性。', allowedTools: ['video_frame_qa', 'ffmpeg_probe', 'final_review_generator'], requiredOutputs: ['VIDEO_VISUAL_QA_REPORT', 'VIDEO_VISUAL_QA_CONTACT_SHEET', 'FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'] },
  { id: 'package_producer', name: 'Package Producer', displayName: '交付制片', stage: 'package', goal: '打包最终视频、结构说明、预览图、决策日志和审核报告。', allowedTools: ['artifact_packager'], requiredOutputs: ['PROJECT_PACKAGE'] },
]

const requiredModelProviderCapabilities: ModelCapability[] = ['text_to_text', 'text_to_image', 'text_to_video']
const modelProviderCapabilityLabels: Record<ModelCapability, string> = {
  text_to_text: '文生文',
  text_to_image: '文生图片',
  text_to_video: '文生视频',
}

type ShotWorkspaceMode = 'voice_visual' | 'aigc_shot'
type ShotWorkspaceModeSource = 'llm_profile' | 'artifact_metadata' | 'selected_profile'

interface ShotWorkspaceModeDetection {
  mode: ShotWorkspaceMode
  source: ShotWorkspaceModeSource
  label: string
  detail: string
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
  const [selectedProfileId, setSelectedProfileId] = useState<VideoCreationProfileId>('talking_head')
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
    try {
      const status = await fetchJiMengSetupStatus()
      setJimengSetupStatus(status)
    } catch {
      setJimengSetupStatus(null)
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
      const configuredProfile = typeof latestProject.config?.canonicalProfileId === 'string'
        ? latestProject.config.canonicalProfileId
        : undefined
      setSelectedProfileId(normalizeVideoCreationProfileId(latestProject.canonicalProfileId || configuredProfile || latestProject.mode))
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
  }, [activeNav, refreshModelProviderStatus, selectedProfile.id])

  useEffect(() => {
    if (activeNav === 'overview') {
      refreshJiMengSetupStatus().catch(() => {})
    }
  }, [activeNav, refreshJiMengSetupStatus, selectedProfile.id])

  useEffect(() => {
    if (!run?.id || run.status === 'SUCCESS' || run.status === 'FAILED' || run.status === 'CANCELLED') return undefined
    const timer = window.setInterval(() => {
      refreshRun(run.id, project?.id).catch(() => {})
    }, 2500)
    return () => window.clearInterval(timer)
  }, [project?.id, refreshRun, run?.id, run?.status])

  const projectStarted = isProjectSessionStarted(loading, project?.status, run?.status)
  const stages = useMemo(() => buildDirectorStages(roleAgents, reviews, trace, projectStarted, run?.status), [roleAgents, reviews, trace, projectStarted, run?.status])
  const displayStages = useMemo(() => applyOptimisticRunningStage(stages, optimisticRunningStageId), [stages, optimisticRunningStageId])
  const artifacts = useMemo(() => buildDirectorArtifacts(roleAgents, reviews, trace, projectArtifacts as unknown as Array<Record<string, unknown>>), [roleAgents, reviews, trace, projectArtifacts])
  const traceNodes = useMemo(() => buildDirectorTraceNodes(trace), [trace])
  const nextAction = useMemo(() => deriveNextAction(displayStages, run?.status), [displayStages, run?.status])
  const activeReview = preferredActiveReview(reviews)
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
  const shouldUseJiMengMCPForRun = jimengReady && canUseJiMengForProfile(selectedProfile)

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
      const contentTypeRouting = {
        enabled: true,
        decisionArtifactKind: 'VIDEO_CREATION_PROFILE',
        instruction: '在创作开始先判断视频属于口播/知识类视频还是影视化/AIGC shot 创作视频，并把 canonical 判定写入 VIDEO_CREATION_PROFILE。口播/知识类使用 profileId=talking_head；影视化/AIGC shot 使用 profileId=cinematic_story。后续 shot 产物页会根据这个判定自动进入对应工作台。',
        options: [
          { profileId: 'talking_head', projectMode: 'voice_visual', label: '口播/知识类视频', focus: '口播稿、HyperFrames 确定性文字层、AIGC 插入素材' },
          { profileId: 'cinematic_story', projectMode: 'aigc_shot', label: '影视化/AIGC shot 视频', focus: '剧本、跨 shot 一致性、角色/场景/道具参考图、AIGC 主画面' },
        ],
      }
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
          contentTypeRouting,
        },
      })
      setProject({ ...nextProject, status: 'RUNNING' })
      const clientModelProviders = await buildClientModelProvidersForRun()
      const result = await startAgentRun({
        message: `请先判断创作类型，再创作一个${durationSec}秒视频：${cleanTopic}`,
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
          contentTypeRouting,
          ...(shouldUseJiMengMCPForRun ? { aigcProvider: 'jimeng_mcp' } : {}),
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

  const actOnReview = async (action: 'approve' | 'reject' | 'edit' | 'regenerate', targetReview?: AgentReviewItem) => {
    const reviewToActOn = targetReview || activeReview
    if (!run?.id || !reviewToActOn) return
    setLoading(true)
    setError(null)
    setErrorDetail(undefined)
    const nextOptimisticStageId = action === 'approve' ? nextStageIdAfterReview(stages, reviewToActOn) : undefined
    if (nextOptimisticStageId) setOptimisticRunningStageId(nextOptimisticStageId)
    try {
      if (action === 'approve') await approveAgentReview(run.id, reviewToActOn.id, feedback || undefined)
      if (action === 'reject') await rejectAgentReview(run.id, reviewToActOn.id, feedback || '请根据审核意见重新生成。')
      if (action === 'edit') await submitEditedArtifact(run.id, reviewToActOn.id, { editedContent: feedback || topic }, feedback || undefined)
      if (action === 'regenerate') await regenerateAgentStage(run.id, reviewToActOn.id, feedback || undefined)
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
        {activeNav !== 'system' && <ModelProviderNotice status={modelProviderStatus} onOpenSettings={() => setActiveNav('system')} />}
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
              jimengReady={jimengReady}
              nextAction={nextAction}
              onTopicChange={setTopic}
              onDurationChange={setDurationSec}
              onProfileChange={setSelectedProfileId}
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
              onGoAssets={() => setActiveNav('assets')}
              allReviews={reviews || []}
              stages={displayStages}
            />
          )}
          {activeNav === 'trace' && <TracePage traceNodes={traceNodes} run={run} />}
          {activeNav === 'assets' && <AssetsPage artifacts={artifacts} projectId={project?.id} selectedProfile={selectedProfile} onArtifactsChanged={refreshArtifacts} />}
          {activeNav === 'roles' && <RolesPage stages={displayStages} />}
          {activeNav === 'export' && <ExportPage artifacts={artifacts} durationSec={durationSec} projectId={project?.id} />}
          {activeNav === 'system' && <DesktopPage />}
        </div>
      </main>
    </div>
  )
}

function JiMengProjectNotice({ ready, selectedProfile, onOpenSettings }: { ready: boolean; selectedProfile: VideoCreationProfile; onOpenSettings: () => void }) {
  const availableForProfile = canUseJiMengForProfile(selectedProfile)
  return (
    <section className="card p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="text-sm font-bold text-primary-dark">即梦 CLI</p>
          <h3 className="mt-1 text-lg font-black text-ink">图片 / 视频生成配置在设置中管理</h3>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-ink-muted">
            即梦 CLI 和 MCP 登录属于生成能力配置，和文生图片、文生视频 Provider 一起维护。配置完成后，影视 / AIGC 项目会自动使用已就绪的本地即梦能力。
          </p>
        </div>
        <StatusBadge
          status={ready && availableForProfile ? 'valid' : ready ? 'review' : 'pending'}
          label={ready ? '已配置' : '需配置'}
        />
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={onOpenSettings}
          className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-black text-white shadow-glow transition hover:bg-primary-dark"
        >
          <FiSettings /> 配置即梦 CLI
        </button>
        {!availableForProfile ? (
          <span className="text-xs font-semibold text-ink-soft">当前入口不需要自动 AIGC 生成；切换到影视 / AIGC 入口后会使用该配置。</span>
        ) : ready ? (
          <span className="text-xs font-semibold text-green-700">本地即梦生成能力已可用于当前入口。</span>
        ) : (
          <span className="text-xs font-semibold text-primary-dark">打开设置后安装 CLI、注册 MCP 并完成登录。</span>
        )}
      </div>
    </section>
  )
}

function isJiMengReady(status: JiMengSetupStatusResponse | null): boolean {
  if (!status?.dreaminaAvailable) return false
  const provider = status.mcpProviders?.find((item) => item.id === 'jimeng')
  if (!provider?.reachable) return false
  return Boolean(provider.tools?.some((tool) => tool.name === 'jimeng.generate_video'))
}

function canUseJiMengForProfile(profile: VideoCreationProfile): boolean {
  return profile.projectMode === 'aigc_shot' || profile.generationMode === 'manual_import'
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

function ModelProviderNotice({ status, onOpenSettings }: { status: ModelProviderStatus; onOpenSettings: () => void }) {
  if (status.state === 'checking' || status.state === 'configured') return null
  const missing = status.missing
  if (status.state === 'missing' && missing.length === 0) return null
  const missingText = missing.map((capability) => modelProviderCapabilityLabels[capability]).join('、')
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
  jimengReady: boolean
  nextAction?: ReturnType<typeof deriveNextAction>
  onTopicChange: (value: string) => void
  onDurationChange: (value: number) => void
  onProfileChange: (value: VideoCreationProfileId) => void
  onStart: () => void
  onStop: () => void
  onOpenSettings: () => void
  onGoReview: () => void
}) {
  const { topic, durationSec, primaryAction, overviewStatus, stages, artifacts, preflight, selectedProfile, profileOptions, environmentChecklist, jimengReady, nextAction, onTopicChange, onDurationChange, onProfileChange, onStart, onStop, onOpenSettings, onGoReview } = props
  const staleNames = artifacts.filter((artifact) => artifact.status === 'stale').map((artifact) => artifact.name)
  const isStopAction = primaryAction.kind === 'stop'
  const shotGroups = useMemo(() => buildShotReviewGroups(artifacts), [artifacts])
  const assemblySummary = useMemo(() => buildProjectAssemblySummary(shotGroups, artifacts), [artifacts, shotGroups])
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
                  <div className="mt-3 flex flex-wrap gap-2">
                    <span className="rounded-full bg-white px-2.5 py-1 text-[11px] font-bold text-primary-dark ring-1 ring-line">
                      {profile.projectMode === 'aigc_shot' ? '逐 shot 生成' : '脚本到成片'}
                    </span>
                    <span className="rounded-full bg-white px-2.5 py-1 text-[11px] font-bold text-ink-muted ring-1 ring-line">
                      {profile.generationMode === 'manual_import' ? '可手动上传结果' : '自动走本地渲染'}
                    </span>
                  </div>
                </button>
              )
            })}
          </div>
          <div className="mt-3 rounded-lg bg-background-card px-4 py-3 text-xs leading-5 text-ink-muted ring-1 ring-line">
            启动后 LLM 会先判断这是口播/知识类视频还是影视化/AIGC shot 视频，并写入创作主线；进入产物页后，shot 工作台会按该判定自动切换到对应流程。
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
            {preflight?.blockers?.length ? (
              <span className="text-xs font-semibold text-red-700">当前入口还有配置未完成，请查看下方体检问题或打开设置处理。</span>
            ) : !preflight ? (
              <span className="text-xs font-semibold text-primary-dark">正在体检当前视频入口...</span>
            ) : null}
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
      <JiMengProjectNotice ready={jimengReady} selectedProfile={selectedProfile} onOpenSettings={onOpenSettings} />
      <EnvironmentChecklistPanel items={environmentChecklist} onOpenSettings={onOpenSettings} />
      <StageFlow stages={stages} />
      {shotGroups.length > 0 && (
        <section className="card p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-sm font-bold text-primary-dark">Final assembly</p>
              <h3 className="mt-1 text-lg font-black text-ink">Accepted shots / final QA / provenance</h3>
            </div>
            <StatusBadge status={assemblySummary.allShotsAccepted ? 'valid' : 'review'} label={assemblySummary.allShotsAccepted ? 'all shots accepted' : 'waiting shots'} />
          </div>
          <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <AssemblyMetric label="Shots" value={`${assemblySummary.acceptedShotCount}/${assemblySummary.totalShotCount}`} />
            <AssemblyMetric label="Assembly" value={assemblySummary.finalAssemblyStatus} />
            <AssemblyMetric label="Final QA" value={assemblySummary.finalQAStatus} />
            <AssemblyMetric label="Provenance" value={`${assemblySummary.finalVideoSourceType || 'pending'}${assemblySummary.finalVideoIsFallback ? ' fallback' : ''}`} />
          </div>
          <p className="mt-3 text-xs font-semibold text-ink-muted">
            AIGC video {assemblySummary.realAIGCVideoCount} · fallback {assemblySummary.fallbackCount}
          </p>
        </section>
      )}
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
  const issueItems = visibleEnvironmentIssues(items)
  if (!issueItems.length) return null

  const blockedCount = issueItems.filter((item) => item.status === 'blocked').length
  const warningCount = issueItems.filter((item) => item.status === 'warning').length
  const checkingCount = issueItems.filter((item) => item.status === 'unknown').length
  return (
    <section className="card p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm font-bold text-primary-dark">启动体检</p>
          <h3 className="mt-1 text-lg font-black text-ink">开始前需要处理的问题</h3>
          <p className="mt-1 text-xs leading-5 text-ink-muted">系统会自动检查当前入口，只展示会影响启动或生成质量的事项。</p>
        </div>
        <StatusBadge
          status={blockedCount ? 'blocked' : warningCount ? 'review' : 'pending'}
          label={blockedCount ? `${blockedCount} 项待处理` : warningCount ? `${warningCount} 项需留意` : checkingCount ? '检测中' : '待处理'}
        />
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {issueItems.map((item) => (
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

function AssemblyMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-background-card p-3 ring-1 ring-line">
      <div className="text-[11px] font-black text-primary-dark">{label}</div>
      <div className="mt-1 truncate text-sm font-black text-ink" title={value}>{value}</div>
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

function ReviewPage({ review, stage, feedback, loading, onFeedbackChange, onAction, onGoAssets, allReviews, stages }: { review?: AgentReviewItem; stage?: DirectorStage; feedback: string; loading: boolean; onFeedbackChange: (value: string) => void; onAction: (action: 'approve' | 'reject' | 'edit' | 'regenerate', targetReview?: AgentReviewItem) => void; onGoAssets: () => void; allReviews: AgentReviewItem[]; stages: DirectorStage[] }) {
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
  const qualityTargetLines = qualityGateTargetLines(selectedReview)

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
                      <span className="text-sm font-black text-ink">{reviewOutputPanelTitle(selectedReview)}</span>
                      <p className="mt-1 text-xs text-ink-soft">{reviewOutputPanelHint(selectedReview)}</p>
                    </div>
                    <div className="flex flex-wrap justify-end gap-2">
                      {isShotProductionReview(selectedReview) ? (
                        <button
                          type="button"
                          onClick={onGoAssets}
                          className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-2.5 py-1.5 text-xs font-black text-white shadow-sm hover:bg-primary-dark"
                        >
                          <FiArchive /> 查看产物页提示词和上传入口
                        </button>
                      ) : null}
                      {outputText ? <CopyButton value={outputText} label="复制" /> : null}
                    </div>
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
                      <ActionButton color="green" icon={<FiCheck />} label="通过" loadingLabel="通过中…" loading={loading && activeAction === 'approve'} disabled={loading} onClick={() => { setActiveAction('approve'); onAction('approve', selectedReview); }} />
                      <ActionButton color="red" icon={<FiX />} label="驳回" loadingLabel="驳回中…" loading={loading && activeAction === 'reject'} disabled={loading} onClick={() => { setActiveAction('reject'); onAction('reject', selectedReview); }} />
                      <ActionButton color="amber" icon={<FiEdit3 />} label="修改提交" loadingLabel="提交中…" loading={loading && activeAction === 'edit'} disabled={loading} onClick={() => { setActiveAction('edit'); onAction('edit', selectedReview); }} />
                      <ActionButton color="violet" icon={<FiRefreshCw />} label="重新生成" loadingLabel="重新生成中…" loading={loading && activeAction === 'regenerate'} disabled={loading} onClick={() => { setActiveAction('regenerate'); onAction('regenerate', selectedReview); }} />
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
                {qualityTargetLines.length > 0 && <Panel title="门禁目标产物" items={qualityTargetLines} />}
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

function shotWorkspaceModeForProfile(profile: VideoCreationProfile): ShotWorkspaceMode {
  return profile.projectMode === 'aigc_shot' ? 'aigc_shot' : 'voice_visual'
}

function detectShotWorkspaceMode(artifacts: DirectorArtifactRecord[], selectedProfile: VideoCreationProfile): ShotWorkspaceModeDetection {
  const profileSummary = creationProfileSummary(artifacts)
  const llmMode = shotWorkspaceModeForProfileId(profileSummary.profileId)
  if (llmMode) {
    return {
      mode: llmMode,
      source: 'llm_profile',
      label: `LLM 自动识别：${shotWorkspaceModeLabel(llmMode)}`,
      detail: profileSummary.label && profileSummary.label !== '未选择'
        ? `来自创作主线：${profileSummary.label}`
        : '来自创作主线产物',
    }
  }
  const metadataMode = shotWorkspaceModeFromArtifactMetadata(artifacts)
  if (metadataMode) {
    return {
      mode: metadataMode,
      source: 'artifact_metadata',
      label: `自动识别：${shotWorkspaceModeLabel(metadataMode)}`,
      detail: '来自项目或产物元数据',
    }
  }
  const fallbackMode = shotWorkspaceModeForProfile(selectedProfile)
  return {
    mode: fallbackMode,
    source: 'selected_profile',
    label: `当前项目类型：${shotWorkspaceModeLabel(fallbackMode)}`,
    detail: '尚未读取到 LLM 创作类型判定，暂按启动时选择的项目类型展示。',
  }
}

function shotWorkspaceModeForProfileId(profileId: string | undefined): ShotWorkspaceMode | undefined {
  const normalized = profileId?.trim().toLowerCase().replace(/[-\s]+/g, '_') || ''
  if (!normalized) return undefined
  if (['aigc_shot', 'cinematic_story', 'cinematic', 'film', 'film_story', 'movie', 'narrative', 'story_video', 'short_film'].includes(normalized)) return 'aigc_shot'
  if (['voice_visual', 'talking_head', 'voice', 'voiceover', 'knowledge', 'explainer', 'tutorial', 'faceless_explainer'].includes(normalized)) return 'voice_visual'
  if (normalized.includes('cinematic') || normalized.includes('film') || normalized.includes('movie') || normalized.includes('aigc_shot')) return 'aigc_shot'
  if (normalized.includes('talking') || normalized.includes('voice') || normalized.includes('knowledge') || normalized.includes('explainer')) return 'voice_visual'
  return undefined
}

function shotWorkspaceModeFromArtifactMetadata(artifacts: DirectorArtifactRecord[]): ShotWorkspaceMode | undefined {
  for (const artifact of artifacts) {
    const metadata = artifact.metadata || {}
    const candidates = [
      stringField(metadata.profileId),
      stringField(metadata.videoType),
      stringField(metadata.projectMode),
      stringField(metadata.mode),
      stringField(objectField(metadata.creationProfile)?.profileId),
      stringField(objectField(metadata.creationProfile)?.projectMode),
    ]
    if (artifact.inlineJson) {
      const inline = objectField(parseMaybeJSON(artifact.inlineJson))
      if (inline) {
        candidates.push(
          stringField(inline.profileId),
          stringField(inline.videoType),
          stringField(inline.projectMode),
          stringField(objectField(inline.creationProfile)?.profileId),
          stringField(objectField(inline.creationProfile)?.projectMode),
        )
      }
    }
    const matched = candidates.map(shotWorkspaceModeForProfileId).find(Boolean)
    if (matched) return matched
  }
  return undefined
}

function shotWorkspaceModeLabel(mode: ShotWorkspaceMode): string {
  return mode === 'aigc_shot' ? '影视创作' : '口播视频'
}

function shotWorkspaceCopy(mode: ShotWorkspaceMode) {
  if (mode === 'aigc_shot') {
    return {
      eyebrow: '影视分镜 shot 工作台',
      title: '剧本、连续性参考图与 AIGC 镜头提示词',
      description: '每个 shot 先锁定剧本片段、角色 / 场景 / 道具参考图和故事板，再检查 AIGC 主画面层的文学化提示词、运镜、景别和跨 shot 一致性；HyperFrames 层主要承担字幕和少量说明。',
      primary: 'AIGC 主画面层',
      secondary: '全局一致性参考',
      tertiary: 'HyperFrames 字幕层',
    }
  }
  return {
    eyebrow: '口播知识 shot 工作台',
    title: '口播稿、HyperFrames 时间线与 AIGC 插入点',
    description: '每个 shot 先围绕口播稿校验内容节奏，再检查 HyperFrames 可控层的素材、字幕、图形和时间线变化；AIGC 只作为插入素材服务口播，不要求跨 shot 连续性。',
    primary: '口播稿主线',
    secondary: 'HyperFrames 可控层',
    tertiary: 'AIGC 插入素材',
  }
}

function AssetsPage({ artifacts, projectId, selectedProfile, onArtifactsChanged }: { artifacts: DirectorArtifactRecord[]; projectId?: string; selectedProfile: VideoCreationProfile; onArtifactsChanged?: () => Promise<void> | void }) {
  const staleCount = artifacts.filter((a) => a.status === 'stale').length
  const materialDependencyCount = useMemo(() => unresolvedMaterialDependencyCount(buildShotReviewGroups(artifacts)), [artifacts])
  const modeDetection = useMemo(() => detectShotWorkspaceMode(artifacts, selectedProfile), [artifacts, selectedProfile])
  const [mode, setMode] = useState<ShotWorkspaceMode>(modeDetection.mode)

  useEffect(() => {
    setMode(modeDetection.mode)
  }, [modeDetection.mode])

  return (
    <div className="space-y-5">
      {staleCount > 0 && <div className="rounded-lg bg-amber-50 p-4 text-sm font-semibold text-primary-dark ring-1 ring-amber-200">⚠ 有 {staleCount} 个下游产物已过期。上游产物被修改、驳回或重新生成后，下游产物需要重新生成才能使用。</div>}
      {materialDependencyCount > 0 && (
        <div className="rounded-lg border border-primary/25 bg-white p-4 shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="text-sm font-black text-primary-dark">待回填素材</p>
              <p className="mt-1 text-sm leading-6 text-ink-muted">
                {materialDependencyCount} 个外部生成请求已按 shot 放入下方槽位。复制提示词和参考图到图片 / 视频生成工具，生成后直接在对应 shot 的素材槽上传回填。
              </p>
            </div>
            <StatusBadge status="review" label="待用户回填" />
          </div>
        </div>
      )}
      <ShotAssetWorkbench artifacts={artifacts} projectId={projectId} mode={mode} modeDetection={modeDetection} onModeChange={setMode} onArtifactsChanged={onArtifactsChanged} />
      <details className="card p-4">
        <summary className="cursor-pointer text-sm font-black text-primary-dark">开发详情：原始产物索引（{artifacts.length}）</summary>
        <div className="mt-4">
          <ArtifactTable artifacts={artifacts} projectId={projectId} mode={mode} onArtifactsChanged={onArtifactsChanged} />
        </div>
      </details>
    </div>
  )
}

interface ShotPromptPreview {
  artifactId: string
  loading: boolean
  error?: string
  request?: ExternalGenerationRequestContent
}

function ShotAssetWorkbench({ artifacts, projectId, mode, modeDetection, onModeChange, onArtifactsChanged }: { artifacts: DirectorArtifactRecord[]; projectId?: string; mode: ShotWorkspaceMode; modeDetection: ShotWorkspaceModeDetection; onModeChange?: (mode: ShotWorkspaceMode) => void; onArtifactsChanged?: () => Promise<void> | void }) {
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
  const openRequestEntries = openGroup ? shotRequestPreviewEntries(openGroup, promptPreviews) : []
  const openIndex = Math.max(0, groups.findIndex((group) => group.shotId === openGroup?.shotId))
  const openNarrationFallback = openGroup ? neighborNarrationForShot(groups, openIndex) : ''
  const openNarrationText = openGroup ? shotNarrationForDisplay(openGroup, openRequestEntries, openNarrationFallback) : ''

  useEffect(() => {
    if (!openGroup) return
    const requests = openGroup.slots.flatMap((slot) => externalGenerationRequestArtifactsForSlot(slot))
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

  const handleRequestRevised = async (artifactId: string, request: ExternalGenerationRequestContent) => {
    setPromptPreviews((current) => ({
      ...current,
      [artifactId]: {
        ...(current[artifactId] || { artifactId }),
        artifactId,
        loading: false,
        request,
        error: undefined,
      },
    }))
    await onArtifactsChanged?.()
  }

  if (!groups.length) return null
  const previousGroup = groups[openIndex - 1]
  const nextGroup = groups[openIndex + 1]
  const copy = shotWorkspaceCopy(mode)
  const loadedRequestCount = openRequestEntries.filter((entry) => entry.request).length
  const referenceCount = openGroup ? Math.max(openGroup.artifactCounts.references, shotReferenceCount(openRequestEntries)) : 0
  const globalReady = referenceCount > 0 || loadedRequestCount > 0 || Boolean(openGroup?.production.canEnterAssembly)
  const activeStatusLabel = openGroup?.production.canEnterAssembly ? '可进入拼接' : openGroup?.status === 'valid' ? '已通过' : '待校验'
  const usingAutoMode = mode === modeDetection.mode
  const qaStatusLabel = openGroup ? shotQaStatusLabel(openGroup.production.qaStatus) : ''
  const sourceTypeLabel = openGroup ? shotSourceTypeLabel(openGroup.production.sourceType, openGroup.production.isFallback) : ''

  return (
    <section className="space-y-4">
      <div className="card overflow-hidden p-0">
        <div className="border-b border-primary/15 bg-[linear-gradient(180deg,#fffaf0,#fffdf7)] p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2 text-xs font-black text-ink-muted">
                <span>项目</span>
                <FiChevronRight className="text-primary" />
                <span>产物</span>
                <FiChevronRight className="text-primary" />
                <span className="font-mono text-primary-dark">{openGroup?.shotId || 'SHOT'}</span>
              </div>
              <h3 className="mt-3 text-2xl font-black text-ink">{mode === 'aigc_shot' ? '影视创作 Shot 工作台' : '口播视频 Shot 工作台'}</h3>
              <p className="mt-2 max-w-4xl text-sm leading-6 text-ink-muted">{copy.description}</p>
              <div className={clsx(
                'mt-3 inline-flex max-w-full flex-wrap items-center gap-2 rounded-lg px-3 py-2 text-xs font-black ring-1',
                usingAutoMode ? 'bg-green-50 text-green-700 ring-green-100' : 'bg-amber-50 text-amber-800 ring-amber-100',
              )}>
                <FiCpu />
                <span>{usingAutoMode ? modeDetection.label : `手动查看：${shotWorkspaceModeLabel(mode)}`}</span>
                <span className="font-semibold opacity-80">{usingAutoMode ? modeDetection.detail : `LLM 建议：${shotWorkspaceModeLabel(modeDetection.mode)} · ${modeDetection.detail}`}</span>
              </div>
            </div>
            <div className="flex rounded-lg bg-white p-1 ring-1 ring-line">
              {([
                ['voice_visual', '口播视频', FiLayers],
                ['aigc_shot', '影视创作', FiVideo],
              ] as const).map(([itemMode, label, Icon]) => (
                <button
                  key={itemMode}
                  type="button"
                  onClick={() => onModeChange?.(itemMode)}
                  className={clsx(
                    'inline-flex items-center gap-1.5 rounded-md px-3 py-2 text-xs font-black transition',
                    mode === itemMode ? 'bg-primary text-white shadow-sm' : 'text-primary-dark hover:bg-primary-soft',
                  )}
                >
                  <Icon /> {label}
                </button>
              ))}
            </div>
          </div>
          <div className="mt-5 grid gap-3 xl:grid-cols-[minmax(0,1fr)_360px]">
            <div className="rounded-lg bg-white p-4 ring-1 ring-primary/20">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-black text-primary-dark"><FiUserCheck /> 全局参考包同步</div>
                  <p className="mt-2 text-sm leading-6 text-ink-muted">
                    {mode === 'aigc_shot'
                      ? '当前 shot 以 AIGC 主画面为核心。先确认角色、场景、道具和故事板参考，保持跨 shot 连续性；HyperFrames 只承担字幕和少量说明。'
                      : '当前 shot 以口播稿为核心。先确认口播内容和 HyperFrames 可控层时间线，再在合适位置补 AIGC b-roll 素材。'}
                  </p>
                </div>
                <span className={clsx(
                  'rounded-full px-3 py-1 text-xs font-black ring-1',
                  globalReady ? 'bg-green-50 text-green-700 ring-green-100' : 'bg-amber-50 text-amber-800 ring-amber-100',
                )}>{globalReady ? '已同步' : '待补充'}</span>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <ShotFocusMetric label="参考图" value={`${referenceCount}`} />
              <ShotFocusMetric label={mode === 'aigc_shot' ? '角色' : '口播稿'} value={mode === 'aigc_shot' ? `${openGroup?.referenceRoles.length || 0}` : openNarrationText ? '已读取' : '待确认'} />
              <ShotFocusMetric label="素材任务" value={`${loadedRequestCount}`} />
              <ShotFocusMetric label="Shot 时长" value={openGroup?.durationSec ? `${openGroup.durationSec}s` : '未标注'} />
            </div>
          </div>
        </div>

        <div className="border-b border-line bg-white/80 px-5 py-3">
          <div className="flex gap-2 overflow-x-auto pb-1">
            {groups.map((group) => (
              <button
                key={group.shotId}
                type="button"
                onClick={() => setOpenShotId(group.shotId)}
                className={clsx(
                  'min-w-[184px] rounded-lg p-3 text-left ring-1 transition',
                  openGroup?.shotId === group.shotId ? 'bg-primary-soft ring-primary' : 'bg-white ring-line hover:bg-background-card',
                )}
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="font-mono text-xs font-black text-primary-dark">{group.shotId}</div>
                  <StatusBadge status={group.status} label={group.status === 'valid' ? '有效' : undefined} />
                </div>
                <div className="mt-2 truncate text-sm font-black text-ink" title={group.title}>{group.title}</div>
                <p className="mt-2 line-clamp-2 min-h-10 text-xs leading-5 text-ink-muted">
                  {shotCardPreviewText(group)}
                </p>
              </button>
            ))}
          </div>
        </div>
      </div>

      {openGroup ? (
        <div className="card p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="font-mono text-xs font-black text-primary-dark">{openGroup.shotId}</div>
              <h4 className="mt-1 text-2xl font-black text-ink [overflow-wrap:anywhere]">{openGroup.title}</h4>
              <div className="mt-3 flex flex-wrap gap-2 text-[11px] font-bold text-ink-muted">
                {openGroup.durationSec ? <span className="rounded bg-white px-2 py-1 ring-1 ring-line">{openGroup.durationSec}s</span> : null}
                {qaStatusLabel ? <span className="rounded bg-white px-2 py-1 ring-1 ring-line">{qaStatusLabel}</span> : null}
                {openGroup.production.attemptCount > 0 ? <span className="rounded bg-white px-2 py-1 ring-1 ring-line">尝试 {openGroup.production.attemptCount}</span> : null}
                {openGroup.production.acceptedCandidateId ? <span className="rounded bg-white px-2 py-1 ring-1 ring-line">已采纳版本</span> : null}
                {sourceTypeLabel ? <span className={clsx('rounded px-2 py-1 ring-1', openGroup.production.isFallback ? 'bg-amber-50 text-amber-800 ring-amber-100' : 'bg-white ring-line')}>{sourceTypeLabel}</span> : null}
                <span className={clsx('rounded px-2 py-1 ring-1', openGroup.production.canEnterAssembly ? 'bg-green-50 text-green-700 ring-green-100' : 'bg-background-card ring-line')}>{activeStatusLabel}</span>
                <span className="rounded bg-white px-2 py-1 ring-1 ring-line">产物 {openGroup.artifactCounts.total}</span>
                <span className="rounded bg-white px-2 py-1 ring-1 ring-line">参考 {openGroup.artifactCounts.references}</span>
                <span className="rounded bg-white px-2 py-1 ring-1 ring-line">媒体 {openGroup.artifactCounts.media}</span>
              </div>
              {openGroup.production.lockedDimensions.length ? (
                <div className="mt-2 flex flex-wrap gap-1.5 text-[11px] font-bold text-ink-muted">
                  {openGroup.production.lockedDimensions.map((dimension) => (
                    <span key={dimension} className="rounded bg-green-50 px-2 py-1 text-green-700 ring-1 ring-green-100">lock {dimension}</span>
                  ))}
                </div>
              ) : null}
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
            <div className="flex flex-wrap items-center justify-end gap-2">
              <button
                type="button"
                disabled={!previousGroup}
                onClick={() => previousGroup && setOpenShotId(previousGroup.shotId)}
                className="inline-flex items-center gap-1.5 rounded-lg bg-white px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"
              >
                上一 shot
              </button>
              <button
                type="button"
                disabled={!nextGroup}
                onClick={() => nextGroup && setOpenShotId(nextGroup.shotId)}
                className="inline-flex items-center gap-1.5 rounded-lg bg-white px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"
              >
                下一 shot
              </button>
              <button type="button" className="inline-flex items-center gap-1.5 rounded-lg bg-white px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"><FiCheck /> 保存</button>
              <button type="button" className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-2 text-xs font-black text-white shadow-sm"><FiRefreshCw /> 重新生成</button>
              <button type="button" className="inline-flex items-center gap-1.5 rounded-lg bg-white px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"><FiDownload /> 导出</button>
            </div>
          </div>
          <ShotProductionGuideCard
            group={openGroup}
            requestEntries={openRequestEntries}
            narrationFallbackText={openNarrationFallback}
            mode={mode}
            projectId={projectId}
            projectReady={Boolean(projectId)}
            uploadingKey={uploadingKey}
            promptPreviews={promptPreviews}
            onUpload={uploadShotAsset}
            onRequestRevised={handleRequestRevised}
          />
          {message ? <p className="mt-4 text-sm font-semibold text-green-700">{message}</p> : null}
          {error ? <p className="mt-4 text-sm font-semibold text-red-600">{error}</p> : null}
        </div>
      ) : null}
    </section>
  )
}

interface ShotRequestPreviewEntry {
  slot: DirectorShotAssetSlot
  artifact: DirectorArtifactRecord
  preview?: ShotPromptPreview
  request?: ExternalGenerationRequestContent
}

function externalGenerationRequestArtifactsForSlot(slot: DirectorShotAssetSlot): DirectorArtifactRecord[] {
  const byId = new Map<string, DirectorArtifactRecord>()
  for (const artifact of [...slot.dependencyRequests, ...slot.artifacts]) {
    if (!isMaterialDependencyRequest(artifact)) continue
    byId.set(artifact.id, artifact)
  }
  return Array.from(byId.values())
}

function shotRequestPreviewEntries(
  group: DirectorShotReviewGroup,
  promptPreviews: Record<string, ShotPromptPreview>,
): ShotRequestPreviewEntry[] {
  const shotReferences = externalGenerationReferencesFromArtifacts(group.slots.flatMap((slot) => slot.artifacts))
  const entries = group.slots.flatMap((slot) => externalGenerationRequestArtifactsForSlot(slot).map((artifact) => {
    const preview = promptPreviews[artifact.id]
    const request = preview?.request
      ? mergeExternalGenerationTaskReferences(preview.request, shotReferences) as ExternalGenerationRequestContent
      : undefined
    return {
      slot,
      artifact,
      preview,
      request,
    }
  }))
  return dedupeShotRequestPreviewEntries(entries)
}

function dedupeShotRequestPreviewEntries(entries: ShotRequestPreviewEntry[]): ShotRequestPreviewEntry[] {
  const byKey = new Map<string, ShotRequestPreviewEntry>()
  for (const entry of entries) {
    const request = entry.request
    const promptKey = normalizeReadableText(request?.prompt).slice(0, 240)
    const key = request?.requestId || (promptKey ? `${request?.kind || 'request'}:${promptKey}` : entry.artifact.id)
    const existing = byKey.get(key)
    if (!existing || requestEntryPriority(entry) > requestEntryPriority(existing)) {
      byKey.set(key, entry)
    }
  }
  return Array.from(byKey.values())
}

function requestEntryPriority(entry: ShotRequestPreviewEntry): number {
  let score = 0
  if (entry.request) score += 20
  if (entry.slot.uploadKind) score += 10
  if (entry.slot.kind === 'base-media' || entry.slot.kind === 'video') score += 6
  if (entry.slot.kind === 'storyboard' || entry.slot.kind === 'reference') score += 4
  if (entry.slot.kind === 'prompt') score -= 8
  return score
}

function ShotProductionGuideCard({
  group,
  requestEntries,
  narrationFallbackText,
  mode,
  projectId,
  projectReady,
  uploadingKey,
  promptPreviews,
  onUpload,
  onRequestRevised,
}: {
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  narrationFallbackText?: string
  mode: ShotWorkspaceMode
  projectId?: string
  projectReady: boolean
  uploadingKey: string | null
  promptPreviews: Record<string, ShotPromptPreview>
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
  onRequestRevised?: (artifactId: string, request: ExternalGenerationRequestContent) => Promise<void> | void
}) {
  const narrationText = shotNarrationForDisplay(group, requestEntries, narrationFallbackText)
  const visualText = shotVisualForDisplay(group, requestEntries, narrationText)
  if (mode === 'aigc_shot') {
    return (
      <CinematicShotGuide
        group={group}
        requestEntries={requestEntries}
        narrationText={narrationText}
        visualText={visualText}
        projectId={projectId}
        projectReady={projectReady}
        uploadingKey={uploadingKey}
        promptPreviews={promptPreviews}
        onUpload={onUpload}
        onRequestRevised={onRequestRevised}
      />
    )
  }
  return (
    <VoiceShotGuide
      group={group}
      requestEntries={requestEntries}
      narrationText={narrationText}
      visualText={visualText}
      projectId={projectId}
      projectReady={projectReady}
      uploadingKey={uploadingKey}
      promptPreviews={promptPreviews}
      onUpload={onUpload}
      onRequestRevised={onRequestRevised}
    />
  )
}

function CinematicShotGuide({
  group,
  requestEntries,
  narrationText,
  visualText,
  projectId,
  projectReady,
  uploadingKey,
  promptPreviews,
  onUpload,
  onRequestRevised,
}: {
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  narrationText: string
  visualText: string
  projectId?: string
  projectReady: boolean
  uploadingKey: string | null
  promptPreviews: Record<string, ShotPromptPreview>
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
  onRequestRevised?: (artifactId: string, request: ExternalGenerationRequestContent) => Promise<void> | void
}) {
  const references = uniqueShotReferences(requestEntries)
  const characterRefs = referencesByDependency(references, ['角色参考图'])
  const scenePropRefs = referencesByDependency(references, ['场景参考图', '道具参考图'])
  const storyboardRefs = referencesByDependency(references, ['故事板 / 首帧'])
  const mediaArtifacts = shotMediaArtifacts(group)
  const videoPrompt = cinematicPromptText(requestEntries, visualText, narrationText)
  const previewArtifacts = mediaArtifacts.filter((artifact) => artifactPreviewKind(artifact)).slice(0, 6)
  const hyperFramesArtifacts = hyperFramesOutputArtifacts(group)

  return (
    <div className="mt-4 space-y-4">
      <div className="rounded-lg border border-primary/20 bg-white p-4 ring-1 ring-primary/10">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-xs font-black text-primary-dark">影视创作产物视图</div>
            <p className="mt-2 text-sm leading-6 text-ink-muted">{cinematicShotActionSummary(group, requestEntries)}</p>
          </div>
          <StatusBadge status={group.status} />
        </div>
        <ShotReferenceStrategyPanel mode="aigc_shot" group={group} requestEntries={requestEntries} narrationOverride={narrationText} />
      </div>

      <ShotPrototypePanel index="1" title="剧本 / Shot 意图" subtitle="先确认叙事事实、情绪目标和本 shot 在前后镜头里的位置。">
        <ShotTextBlock
          title="剧本片段"
          value={narrationText || '尚未从上游产物读取到剧本文本。请回到审核页确认剧本或 shot list。'}
          mode="aigc_shot"
        />
      </ShotPrototypePanel>

      <ShotPrototypePanel index="2" title="跨 Shot 一致性" subtitle="先锁定角色、场景、道具和物理状态，再进入单个 shot 的提示词。">
        <ContinuityChecklist group={group} requestEntries={requestEntries} />
      </ShotPrototypePanel>

      <ShotPrototypePanel index="3" title="角色 / 场景 / 道具参考图" subtitle="多视角参考图用于稳定主体、空间关系和关键物件。">
        <div className="space-y-3">
          <ReferenceGallery references={characterRefs} mode="aigc_shot" projectId={projectId} emptyText="暂无角色参考图。影视创作建议补充主角正面、侧面、背面和表情参考。" />
          <ReferenceGallery references={scenePropRefs} mode="aigc_shot" projectId={projectId} emptyText="暂无场景或道具参考图。请补充场景、道具和关键物理状态。" />
          <StepSlotUploadPanel
            group={group}
            kinds={['reference']}
            mode="aigc_shot"
            projectId={projectId}
            projectReady={projectReady}
            uploadingKey={uploadingKey}
            promptPreviews={promptPreviews}
            onUpload={onUpload}
            onRequestRevised={onRequestRevised}
          />
        </div>
      </ShotPrototypePanel>

      <ShotPrototypePanel index="4" title="AIGC 层画面描述与素材任务" subtitle="展开处理素材任务：这里是视频 / 图片生成的核心提示词，可划词局部修改并上传回填。">
        <AigcPromptDimensionGrid group={group} requestEntries={requestEntries} visualText={visualText} />
        <div className="mt-3">
          <ShotTextBlock
            title="优美提示词"
            value={videoPrompt}
            mode="aigc_shot"
          />
        </div>
        <div className="mt-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
          <div className="text-xs font-black text-primary-dark">本步骤上传 / 回填</div>
          <p className="mt-1 text-[11px] leading-5 text-ink-muted">在这里复制提示词、局部修改、生成外部 AIGC 素材，并把结果上传回当前 shot。</p>
          <div className="mt-3">
          <CompactRequestTaskStack
            group={group}
            requestEntries={requestEntries}
            mode="aigc_shot"
            projectId={projectId}
            projectReady={projectReady}
            uploadingKey={uploadingKey}
            emptyText="当前 shot 暂无需要手动生成的 AIGC 图片或视频素材。"
            onUpload={onUpload}
            onRequestRevised={onRequestRevised}
          />
          </div>
        </div>
      </ShotPrototypePanel>

      <ShotPrototypePanel index="5" title="分镜故事板 / 首帧" subtitle="用于锁定构图、景别、首帧和跨 shot 过渡。">
        <ReferenceGallery references={storyboardRefs} mode="aigc_shot" projectId={projectId} emptyText="暂无故事板参考图。可以先生成分镜图或首帧，再在本步骤上传回填。" />
        <StepSlotUploadPanel
          group={group}
          kinds={['storyboard']}
          mode="aigc_shot"
          projectId={projectId}
          projectReady={projectReady}
          uploadingKey={uploadingKey}
          promptPreviews={promptPreviews}
          onUpload={onUpload}
          onRequestRevised={onRequestRevised}
        />
      </ShotPrototypePanel>

      <ShotPrototypePanel index="6" title="HyperFrames 辅助层与产物预览" subtitle="字幕、说明和可控图形交给确定性渲染层；最终产物只展示可看的图片和视频。">
        <ShotLayerTimeline mode="aigc_shot" group={group} requestEntries={requestEntries} narrationOverride={narrationText} />
        <LayerPromptBlock
          title="HyperFrames 辅助层提示词"
          value={hyperFramesPromptForDisplay(requestEntries) || 'HyperFrames 辅助层只负责字幕、说明和少量可控图形，不抢 AIGC 主画面。'}
          mode="aigc_shot"
        />
        <HyperFramesOutputPreview artifacts={hyperFramesArtifacts} mode="aigc_shot" projectId={projectId} durationSec={group.durationSec} narrationText={narrationText} />
        <StepSlotUploadPanel
          group={group}
          kinds={['overlay', 'video']}
          mode="aigc_shot"
          hideMaterializedKinds={['overlay']}
          projectId={projectId}
          projectReady={projectReady}
          uploadingKey={uploadingKey}
          promptPreviews={promptPreviews}
          onUpload={onUpload}
          onRequestRevised={onRequestRevised}
        />
        <div className="mt-3">
          <ArtifactGallery artifacts={previewArtifacts} mode="aigc_shot" projectId={projectId} emptyText="暂无可播放或可预览产物。生成后会在这里出现。" />
        </div>
      </ShotPrototypePanel>
    </div>
  )
}

function VoiceShotGuide({
  group,
  requestEntries,
  narrationText,
  visualText,
  projectId,
  projectReady,
  uploadingKey,
  promptPreviews,
  onUpload,
  onRequestRevised,
}: {
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  narrationText: string
  visualText: string
  projectId?: string
  projectReady: boolean
  uploadingKey: string | null
  promptPreviews: Record<string, ShotPromptPreview>
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
  onRequestRevised?: (artifactId: string, request: ExternalGenerationRequestContent) => Promise<void> | void
}) {
  const references = uniqueShotReferences(requestEntries)
  const mediaArtifacts = shotMediaArtifacts(group).filter((artifact) => artifactPreviewKind(artifact)).slice(0, 6)
  const subtitleArtifacts = slotArtifactsByKind(group, ['overlay']).filter(isSubtitleArtifact)
  const hyperFramesArtifacts = hyperFramesOutputArtifacts(group)

  return (
    <div className="mt-4 space-y-4">
      <div className="rounded-lg border border-primary/20 bg-white p-4 ring-1 ring-primary/10">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-xs font-black text-primary-dark">口播视频产物视图</div>
            <p className="mt-2 text-sm leading-6 text-ink-muted">{shotActionSummary(group, requestEntries)}</p>
          </div>
          <StatusBadge status={group.status} />
        </div>
        <ShotReferenceStrategyPanel mode="voice_visual" group={group} requestEntries={requestEntries} narrationOverride={narrationText} />
      </div>

      <TalkingHeadLayerInspector artifacts={group.artifacts} />

      <ShotPrototypePanel index="1" title="口播稿" subtitle="口播是主线。划词后只局部修改，不重写整个 shot。">
        <ShotTextBlock
          title="口播稿"
          value={voiceScriptTextForDisplay(narrationText)}
          mode="voice_visual"
        />
      </ShotPrototypePanel>

      <ShotPrototypePanel index="2" title="HyperFrames 层时间线" subtitle="中文文字、字幕、卡片、流程标签和浏览器组件由确定性渲染层完成。">
        <VoiceHyperFramesTimeline group={group} requestEntries={requestEntries} narrationText={narrationText} />
        <LayerPromptBlock
          title="HyperFrames 层提示词"
          value={hyperFramesPromptForDisplay(requestEntries) || 'HyperFrames 层负责字幕、标题、UI 卡片、流程标签和安全文字。请在这里补充组件、素材和时间线变化。'}
          mode="voice_visual"
        />
        <HyperFramesOutputPreview artifacts={hyperFramesArtifacts} mode="voice_visual" projectId={projectId} durationSec={group.durationSec} narrationText={narrationText} />
        <StepSlotUploadPanel
          group={group}
          kinds={['overlay']}
          mode="voice_visual"
          hideMaterializedKinds={['overlay']}
          projectId={projectId}
          projectReady={projectReady}
          uploadingKey={uploadingKey}
          promptPreviews={promptPreviews}
          onUpload={onUpload}
          onRequestRevised={onRequestRevised}
        />
      </ShotPrototypePanel>

      <ShotPrototypePanel index="3" title="AIGC 插入点" subtitle="先定义插入位置和服务的口播信息点，再进入素材生成。">
        <VoiceAigcInsertTimeline group={group} requestEntries={requestEntries} visualText={visualText} />
      </ShotPrototypePanel>

      <ShotPrototypePanel index="4" title="AIGC 素材任务" subtitle="展开处理素材任务：复制提示词生成素材，生成完成后上传回填；这一步不要求跨 shot 一致性。">
        <div className="rounded-lg bg-background-card p-3 ring-1 ring-line">
          <div className="text-xs font-black text-primary-dark">本步骤上传 / 回填</div>
          <p className="mt-1 text-[11px] leading-5 text-ink-muted">围绕口播插入点生成 AIGC 素材，完成后直接上传回当前 shot。</p>
          <div className="mt-3">
        <CompactRequestTaskStack
          group={group}
          requestEntries={requestEntries}
          mode="voice_visual"
          projectId={projectId}
          projectReady={projectReady}
          uploadingKey={uploadingKey}
          emptyText="当前 shot 暂无 AIGC 插入素材任务。"
          onUpload={onUpload}
          onRequestRevised={onRequestRevised}
        />
          </div>
        </div>
      </ShotPrototypePanel>

      <ShotPrototypePanel index="5" title="字幕层" subtitle="字幕由 HyperFrames 负责，不交给 AIGC 生成。">
        <SubtitleLayerPreview
          narrationText={narrationText}
          durationSec={group.durationSec}
          subtitleArtifacts={subtitleArtifacts}
          projectId={projectId}
        />
      </ShotPrototypePanel>

      <ShotPrototypePanel index="6" title="预览与产物" subtitle="视频、图片、overlay 直接预览；内部文件索引已隐藏。">
        <VoiceReferencePanel requestEntries={requestEntries} references={references} narrationText={narrationText} projectId={projectId} />
        <StepSlotUploadPanel
          group={group}
          kinds={['video']}
          mode="voice_visual"
          projectId={projectId}
          projectReady={projectReady}
          uploadingKey={uploadingKey}
          promptPreviews={promptPreviews}
          onUpload={onUpload}
          onRequestRevised={onRequestRevised}
        />
        <div className="mt-3">
          <ArtifactGallery artifacts={mediaArtifacts} mode="voice_visual" projectId={projectId} emptyText="暂无可播放或可预览产物。生成后会在这里出现。" />
        </div>
      </ShotPrototypePanel>
    </div>
  )
}

function ShotPrototypePanel({
  index,
  title,
  subtitle,
  children,
  className,
}: {
  index: string
  title: string
  subtitle?: string
  children: ReactNode
  className?: string
}) {
  const [completed, setCompleted] = useState(false)
  const [open, setOpen] = useState(index === '1')
  return (
    <details data-shot-step={index} open={open} onToggle={(event) => setOpen(event.currentTarget.open)} className={clsx('group rounded-lg border border-line bg-white shadow-sm', className)}>
      <summary className="cursor-pointer list-none p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className={clsx(
                'grid h-7 w-7 place-items-center rounded-lg font-mono text-xs font-black ring-1',
                completed ? 'bg-green-50 text-green-700 ring-green-100' : 'bg-primary-soft text-primary-dark ring-primary/15',
              )}>{index}</span>
              <h5 className="text-base font-black text-ink">{title}</h5>
            </div>
            {subtitle ? <p className="mt-2 text-xs leading-5 text-ink-muted">{subtitle}</p> : null}
          </div>
          <div className="flex items-center gap-2">
            <span className={clsx(
              'rounded-full px-3 py-1 text-[11px] font-black ring-1',
              completed ? 'bg-green-50 text-green-700 ring-green-100' : 'bg-background-card text-primary-dark ring-line',
            )}>{completed ? '已完成' : '展开处理'}</span>
            <FiChevronRight className="mt-1 text-primary transition group-open:rotate-90" />
          </div>
        </div>
      </summary>
      <div className="border-t border-line px-4 pb-4 pt-3">
        {children}
        <div className="mt-4 flex justify-end">
          <button
            type="button"
            onClick={() => setCompleted((current) => !current)}
            className={clsx(
              'inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-black ring-1',
              completed ? 'bg-green-50 text-green-700 ring-green-100 hover:bg-white' : 'bg-primary text-white ring-primary shadow-sm',
            )}
          >
            <FiCheck /> {completed ? '取消完成' : '完成本步骤'}
          </button>
        </div>
      </div>
    </details>
  )
}

function LayerPromptBlock({ title, value, mode }: { title: string; value: string; mode: ShotWorkspaceMode }) {
  return (
    <div className="mt-3">
      <ShotTextBlock title={title} value={value} mode={mode} />
    </div>
  )
}

function StepSlotUploadPanel({
  group,
  kinds,
  mode,
  hideMaterializedKinds = [],
  projectId,
  projectReady,
  uploadingKey,
  promptPreviews,
  onUpload,
  onRequestRevised,
}: {
  group: DirectorShotReviewGroup
  kinds: DirectorShotAssetSlot['kind'][]
  mode: ShotWorkspaceMode
  hideMaterializedKinds?: DirectorShotAssetSlot['kind'][]
  projectId?: string
  projectReady: boolean
  uploadingKey: string | null
  promptPreviews: Record<string, ShotPromptPreview>
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
  onRequestRevised?: (artifactId: string, request: ExternalGenerationRequestContent) => Promise<void> | void
}) {
  const slots = group.slots.filter((slot) => kinds.includes(slot.kind))
  if (!slots.length) return null
  const shotReferences = externalGenerationReferencesFromArtifacts(group.slots.flatMap((slot) => slot.artifacts))
  return (
    <div className="mt-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-xs font-black text-primary-dark">本步骤上传 / 回填</div>
          <p className="mt-1 text-[11px] leading-5 text-ink-muted">只处理当前步骤需要的素材，生成完成后直接在这里上传回填。</p>
        </div>
        <span className="rounded-full bg-white px-2.5 py-1 text-[11px] font-black text-primary-dark ring-1 ring-line">{slots.length} 个入口</span>
      </div>
      <div className="mt-3 grid grid-cols-1 gap-3 xl:grid-cols-2">
        {slots.map((slot) => (
          <ShotAssetSlotCard
            key={`${group.shotId}-${slot.kind}`}
            shotId={group.shotId}
            slot={slot}
            durationSec={group.durationSec}
            projectId={projectId}
            hideMaterializedArtifacts={hideMaterializedKinds.includes(slot.kind)}
            shotReferences={shotReferences}
            promptPreviews={promptPreviews}
            projectReady={projectReady}
            uploadingKey={uploadingKey}
            mode={mode}
            onRequestRevised={onRequestRevised}
            onUpload={onUpload}
          />
        ))}
      </div>
    </div>
  )
}

function AigcPromptDimensionGrid({
  group,
  requestEntries,
  visualText,
}: {
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  visualText: string
}) {
  const requestText = firstReadableRequestText(requestEntries, ['overallShotPrompt', 'visualText', 'prompt'])
  const fields = [
    { label: '角色', value: group.referenceRoles.slice(0, 3).join('、') || firstPromptSentence(requestText) || '待补充主要角色' },
    { label: '场景', value: firstRequestLine(requestEntries, ['visualText']) || visualText || '待补充空间、时代、天气和环境状态' },
    { label: '道具', value: lockedDimensionLine(group, ['prop', '道具']) || '标注关键道具、位置和物理状态' },
    { label: '镜头运动', value: cameraMotionHint(requestText) },
    { label: '拍摄范围', value: shotSizeHint(requestText) },
    { label: '光线情绪', value: moodHint(requestText || visualText) },
    { label: '时间线变化', value: timelineChangeHint(requestEntries, group.durationSec) },
  ]
  return (
    <div className="grid gap-2 md:grid-cols-2">
      {fields.map((field) => (
        <div key={field.label} className="rounded-lg bg-background-card p-3 ring-1 ring-line">
          <div className="text-[11px] font-black text-primary-dark">{field.label}</div>
          <p className="mt-1 line-clamp-3 text-xs leading-5 text-ink-muted">{field.value}</p>
        </div>
      ))}
    </div>
  )
}

function ContinuityChecklist({ group, requestEntries }: { group: DirectorShotReviewGroup; requestEntries: ShotRequestPreviewEntry[] }) {
  const buckets = referenceBuckets(requestEntries)
  const rows = [
    { label: '主角', ready: buckets.character > 0 || group.referenceRoles.length > 0, detail: group.referenceRoles.slice(0, 3).join('、') || '补充主角多视角参考' },
    { label: '场景', ready: buckets.scene > 0, detail: lockedDimensionLine(group, ['scene', '场景']) || '确认空间、位置和环境状态' },
    { label: '道具', ready: buckets.prop > 0, detail: lockedDimensionLine(group, ['prop', '道具']) || '确认关键道具是否跨 shot 延续' },
    { label: '服装', ready: group.production.lockedDimensions.some((item) => /服装|costume|clothes/u.test(item)), detail: lockedDimensionLine(group, ['服装', 'costume']) || '主角服装、发型和伤痕保持一致' },
    { label: '光线', ready: group.production.lockedDimensions.some((item) => /光|light|tone/u.test(item)), detail: lockedDimensionLine(group, ['光', 'light']) || '确认昼夜、色温和情绪光线' },
    { label: '物理关系', ready: group.production.canEnterAssembly || group.status === 'valid', detail: group.production.canEnterAssembly ? '当前 shot 已可进入拼接' : '留意人物位置、道具方向和运动惯性' },
  ]
  return (
    <div className="space-y-2">
      {rows.map((row) => (
        <div key={row.label} className="flex items-start gap-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
          <span className={clsx('mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs', row.ready ? 'bg-green-50 text-green-700 ring-1 ring-green-100' : 'bg-amber-50 text-amber-800 ring-1 ring-amber-100')}>
            {row.ready ? <FiCheck /> : <FiSquare />}
          </span>
          <div className="min-w-0">
            <div className="text-xs font-black text-ink">{row.label}</div>
            <p className="mt-1 text-xs leading-5 text-ink-muted">{row.detail}</p>
          </div>
        </div>
      ))}
    </div>
  )
}

function VoiceHyperFramesTimeline({
  group,
  requestEntries,
  narrationText,
}: {
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  narrationText: string
}) {
  const duration = group.durationSec || firstRequestDuration(requestEntries) || 8
  const half = Math.max(1, Math.round(duration / 2))
  const hyperFramesLine = firstHyperFramesLine(requestEntries)
  const segments = [
    { time: '0s', title: '口播进入', detail: firstPromptSentence(narrationText) || '承接口播内容，字幕保持稳定可读。' },
    { time: `${Math.max(1, half - 1)}s`, title: '可控素材变化', detail: hyperFramesLine || '标题、数据卡片、流程标签或浏览器组件按口播节奏出现。' },
    { time: `${half}s`, title: '强调信息点', detail: '用 HyperFrames 层处理中文、数字、箭头和重点词，不让 AIGC 生成文字。' },
    { time: `${duration}s`, title: '收束到下个 shot', detail: firstFfmpegLine(requestEntries) || '保留字幕安全区，等待 FFmpeg 合成完整 shot。' },
  ]
  return (
    <div className="rounded-lg bg-background-card p-3 ring-1 ring-line">
      <div className="grid grid-cols-4 gap-1 text-center font-mono text-[11px] font-black text-primary-dark">
        <span>0s</span>
        <span>{Math.round(duration * 0.33)}s</span>
        <span>{Math.round(duration * 0.66)}s</span>
        <span>{duration}s</span>
      </div>
      <div className="mt-2 h-2 overflow-hidden rounded-full bg-white ring-1 ring-line">
        <div className="h-full w-full bg-[linear-gradient(90deg,#F59E0B,#16A34A_52%,#8B5CF6)]" />
      </div>
      <div className="mt-3 grid gap-2 md:grid-cols-2">
        {segments.map((segment) => (
          <div key={`${segment.time}-${segment.title}`} className="rounded-lg bg-white p-3 ring-1 ring-line">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-black text-ink">{segment.title}</span>
              <span className="font-mono text-[11px] font-black text-primary-dark">{segment.time}</span>
            </div>
            <p className="mt-2 line-clamp-3 text-xs leading-5 text-ink-muted">{segment.detail}</p>
          </div>
        ))}
      </div>
      <div className="mt-3">
        <ShotLayerTimeline mode="voice_visual" group={group} requestEntries={requestEntries} narrationOverride={narrationText} />
      </div>
    </div>
  )
}

function VoiceAigcInsertTimeline({
  group,
  requestEntries,
  visualText,
}: {
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  visualText: string
}) {
  const duration = group.durationSec || firstRequestDuration(requestEntries) || 8
  const entries = requestEntries.filter((entry) => entry.request)
  const insertSlots = entries.length ? entries.slice(0, 4).map((entry, index) => ({
    time: `${Math.round((duration / (entries.length + 1)) * (index + 1))}s`,
    title: entry.request?.kind === 'image' ? '图片插入' : '视频插入',
    detail: entry.request ? richVoiceAigcInsertDescription(entry.request, visualText) : '',
    beats: entry.request ? promptTimelineBeats(entry.request) : [],
    notes: entry.request ? voiceAigcInsertNotes(entry.request) : [],
  })) : [
    {
      time: `${Math.round(duration * 0.45)}s`,
      title: '可选 b-roll',
      detail: visualText || '画面单调处可插入无文字 AIGC 素材，服务口播信息点。',
      beats: [],
      notes: ['服务口播', '无文字画面', '保留字幕安全区'],
    },
  ]
  return (
    <div className="space-y-3">
      {insertSlots.map((slot) => (
        <div key={`${slot.time}-${slot.title}`} className="rounded-lg bg-background-card p-3 ring-1 ring-line">
          <div className="grid gap-3 md:grid-cols-[76px_110px_1fr]">
            <div className="font-mono text-xs font-black text-primary-dark">{slot.time}</div>
            <div className="text-xs font-black text-ink">{slot.title}</div>
            <div className="text-xs leading-5 text-ink">{slot.detail}</div>
          </div>
          {slot.beats.length ? (
            <div className="mt-3 grid gap-2 md:grid-cols-3">
              {slot.beats.map((beat) => (
                <div key={`${slot.time}-${beat.time}`} className="rounded-lg bg-white p-3 ring-1 ring-line">
                  <div className="font-mono text-[11px] font-black text-primary-dark">{beat.time}</div>
                  <div className="mt-1 text-xs leading-5 text-ink-muted">{beat.text}</div>
                </div>
              ))}
            </div>
          ) : null}
          {slot.notes.length ? (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {slot.notes.map((note) => (
                <span key={`${slot.time}-${note}`} className="rounded bg-white px-2 py-1 text-[11px] font-black text-primary-dark ring-1 ring-line">{note}</span>
              ))}
            </div>
          ) : null}
        </div>
      ))}
    </div>
  )
}

function VoiceReferencePanel({
  requestEntries,
  references,
  narrationText,
  projectId,
}: {
  requestEntries: ShotRequestPreviewEntry[]
  references: ExternalGenerationReference[]
  narrationText: string
  projectId?: string
}) {
  const rows = [
    { label: '口播稿', value: narrationText ? '已读取' : '待确认' },
    { label: 'HyperFrames 策略', value: hyperFramesTimelineSummary(requestEntries) },
    { label: 'AIGC 插入', value: `${requestEntries.filter((entry) => entry.request).length} 个素材任务` },
  ]
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-3 gap-2">
        {rows.map((row) => (
          <div key={row.label} className="rounded-lg bg-background-card p-3 ring-1 ring-line">
            <div className="text-[11px] font-black text-primary-dark">{row.label}</div>
            <div className="mt-1 truncate text-xs font-black text-ink" title={row.value}>{row.value}</div>
          </div>
        ))}
      </div>
      <ReferenceGallery references={references.slice(0, 3)} mode="voice_visual" projectId={projectId} emptyText="口播类无需强跨 shot 参考；只有需要外部素材时才显示参考图。" />
    </div>
  )
}

function SubtitleLayerPreview({
  narrationText,
  durationSec,
  subtitleArtifacts = [],
  projectId,
}: {
  narrationText: string
  durationSec?: number
  subtitleArtifacts?: DirectorArtifactRecord[]
  projectId?: string
}) {
  if (subtitleArtifacts.length > 0) {
    const fallbackCues = subtitleFallbackCuesFromNarration(narrationText, durationSec)
    return (
      <div className="space-y-3">
        {subtitleArtifacts.map((artifact) => (
          <ShotArtifactPreview key={artifact.id} artifact={artifact} projectId={projectId} subtitleFallbackCues={fallbackCues} />
        ))}
      </div>
    )
  }
  const lines = splitSubtitleLines(narrationText || '等待口播稿后生成字幕层。', 4)
  const duration = durationSec || 8
  return (
    <div className="space-y-2">
      {lines.map((line, index) => {
        const start = Math.round((duration / lines.length) * index)
        const end = Math.round((duration / lines.length) * (index + 1))
        return (
          <div key={`${index}-${line}`} className="grid gap-3 rounded-lg bg-background-card p-3 ring-1 ring-line md:grid-cols-[90px_1fr]">
            <div className="font-mono text-[11px] font-black text-primary-dark">{start}s-{end}s</div>
            <div className="line-clamp-2 text-xs leading-5 text-ink-muted">{line}</div>
          </div>
        )
      })}
    </div>
  )
}

function CompactRequestTaskStack({
  group,
  requestEntries,
  mode,
  projectId,
  projectReady,
  uploadingKey,
  emptyText = '当前 shot 暂无外部生成任务。',
  onUpload,
  onRequestRevised,
}: {
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  mode: ShotWorkspaceMode
  projectId?: string
  projectReady: boolean
  uploadingKey: string | null
  emptyText?: string
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
  onRequestRevised?: (artifactId: string, request: ExternalGenerationRequestContent) => Promise<void> | void
}) {
  if (!requestEntries.length) {
    return <p className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">{emptyText}</p>
  }
  return (
    <div className="space-y-3">
      {requestEntries.map((entry, index) => {
        const request = entry.request
        if (!request) {
          return (
            <div key={`${entry.slot.kind}-${entry.artifact.id}-${index}`} className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">
              {entry.preview?.loading ? '正在读取生成提示词...' : entry.preview?.error || `生成请求 ${entry.artifact.name} 暂不可读。`}
            </div>
          )
        }
        return (
          <ShotRequestSummaryCard
            key={`${entry.slot.kind}-${entry.artifact.id}-${request.requestId}-${index}`}
            shotId={group.shotId}
            slot={entry.slot}
            request={request}
            sequence={index + 1}
            artifactId={entry.artifact.id}
            mode={mode}
            projectId={projectId}
            projectReady={projectReady}
            allowUpload={Boolean(entry.slot.uploadKind)}
            uploading={uploadingKey === `${group.shotId}-${entry.slot.kind}-${request.requestId}`}
            onUpload={onUpload}
            onRequestRevised={onRequestRevised}
          />
        )
      })}
    </div>
  )
}

function ReferenceGallery({ references, mode, projectId, emptyText }: { references: ExternalGenerationReference[]; mode: ShotWorkspaceMode; projectId?: string; emptyText: string }) {
  if (!references.length) {
    return <p className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">{emptyText}</p>
  }
  return (
    <div className="grid gap-3 md:grid-cols-2">
      {references.slice(0, 6).map((reference, index) => (
        <ExternalReferenceCard key={`${reference.id}-${index}`} reference={reference} index={index + 1} mode={mode} projectId={projectId} />
      ))}
    </div>
  )
}

function ArtifactGallery({ artifacts, mode, projectId, emptyText }: { artifacts: DirectorArtifactRecord[]; mode: ShotWorkspaceMode; projectId?: string; emptyText: string }) {
  if (!artifacts.length) {
    return <p className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">{emptyText}</p>
  }
  return (
    <div className="grid gap-3 md:grid-cols-2">
      {artifacts.slice(0, 6).map((artifact) => (
        <ShotArtifactPreview key={artifact.id} artifact={artifact} mode={mode} projectId={projectId} />
      ))}
    </div>
  )
}

function HyperFramesOutputPreview({
  artifacts,
  mode = 'aigc_shot',
  projectId,
  durationSec,
  narrationText = '',
}: {
  artifacts: DirectorArtifactRecord[]
  mode?: ShotWorkspaceMode
  projectId?: string
  durationSec?: number
  narrationText?: string
}) {
  const visibleArtifacts = uniqueArtifactsById(artifacts.filter((artifact) => !isMaterialDependencyRequest(artifact)))
  const videoArtifacts = visibleArtifacts.filter((artifact) => artifactPreviewKind(artifact) === 'video')
  const subtitleArtifacts = visibleArtifacts.filter(isSubtitleArtifact)
  const readableArtifacts = visibleArtifacts.filter((artifact) => readableArtifactKind(artifact) && !isSubtitleArtifact(artifact) && artifactPreviewKind(artifact) !== 'video')
  const fallbackCues = subtitleFallbackCuesFromNarration(narrationText, durationSec)

  if (!visibleArtifacts.length) {
    return <p className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">HyperFrames 产物还没有生成；生成后会直接展示视频层、字幕和文本内容。</p>
  }

  return (
    <div className="space-y-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-xs font-black text-primary-dark">HyperFrames 可读预览</div>
          <p className="mt-1 text-[11px] leading-5 text-ink-muted">视频层可直接播放，字幕文件会解析成时间轴；不再只显示“已生成”。</p>
        </div>
        <div className="flex flex-wrap gap-1.5 text-[11px] font-black text-primary-dark">
          <span className="rounded bg-white px-2 py-1 ring-1 ring-line">视频 {videoArtifacts.length}</span>
          <span className="rounded bg-white px-2 py-1 ring-1 ring-line">字幕 {subtitleArtifacts.length}</span>
          {durationSec ? <span className="rounded bg-white px-2 py-1 ring-1 ring-line">{durationSec}s</span> : null}
        </div>
      </div>
      {videoArtifacts.length ? (
        <div className="grid gap-3 md:grid-cols-2">
          {videoArtifacts.map((artifact) => (
            <ShotArtifactPreview key={artifact.id} artifact={artifact} mode={mode} projectId={projectId} />
          ))}
        </div>
      ) : null}
      {subtitleArtifacts.length ? (
        <div className="space-y-3">
          {subtitleArtifacts.map((artifact) => (
            <ShotArtifactPreview key={artifact.id} artifact={artifact} mode={mode} projectId={projectId} subtitleFallbackCues={fallbackCues} />
          ))}
        </div>
      ) : null}
      {readableArtifacts.length ? (
        <div className="grid gap-3 md:grid-cols-2">
          {readableArtifacts.map((artifact) => (
            <ShotArtifactPreview key={artifact.id} artifact={artifact} mode={mode} projectId={projectId} />
          ))}
        </div>
      ) : null}
    </div>
  )
}

function uniqueShotReferences(requestEntries: ShotRequestPreviewEntry[]): ExternalGenerationReference[] {
  const refs = new Map<string, ExternalGenerationReference>()
  for (const entry of requestEntries) {
    for (const reference of entry.request?.references || []) {
      const key = reference.artifactId || reference.storageRef || reference.id
      if (!refs.has(key)) refs.set(key, reference)
    }
  }
  return Array.from(refs.values())
}

function referencesByDependency(references: ExternalGenerationReference[], labels: string[]): ExternalGenerationReference[] {
  return references.filter((reference) => labels.includes(referenceDependencyLabel(reference)))
}

function slotArtifactsByKind(group: DirectorShotReviewGroup, kinds: DirectorShotAssetSlot['kind'][]): DirectorArtifactRecord[] {
  return group.slots
    .filter((slot) => kinds.includes(slot.kind))
    .flatMap((slot) => slot.artifacts)
    .filter((artifact) => !isMaterialDependencyRequest(artifact))
}

function hyperFramesOutputArtifacts(group: DirectorShotReviewGroup): DirectorArtifactRecord[] {
  return uniqueArtifactsById(
    group.artifacts.filter((artifact) => {
      if (isMaterialDependencyRequest(artifact)) return false
      return isHyperFramesArtifact(artifact) || isSubtitleArtifact(artifact)
    }),
  )
}

function uniqueArtifactsById(artifacts: DirectorArtifactRecord[]): DirectorArtifactRecord[] {
  const byId = new Map<string, DirectorArtifactRecord>()
  for (const artifact of artifacts) byId.set(artifact.id, artifact)
  return Array.from(byId.values())
}

function shotMediaArtifacts(group: DirectorShotReviewGroup): DirectorArtifactRecord[] {
  const byId = new Map<string, DirectorArtifactRecord>()
  for (const artifact of group.slots.flatMap((slot) => slot.artifacts)) {
    if (isMaterialDependencyRequest(artifact)) continue
    if (!artifactPreviewKind(artifact)) continue
    byId.set(artifact.id, artifact)
  }
  return Array.from(byId.values())
}

function cinematicPromptText(requestEntries: ShotRequestPreviewEntry[], visualText: string, narrationText: string): string {
  const prompt = firstReadableRequestText(requestEntries, ['overallShotPrompt', 'prompt', 'visualText'])
  if (prompt) return prompt
  const parts = [
    narrationText ? `剧本意图：${narrationText}` : '',
    visualText ? `画面描述：${visualText}` : '',
    '提示词需要明确主要角色、场景、道具、景别、镜头移动、光线情绪和时间线变化；保持跨 shot 的人物、空间和物理状态一致；字幕和可读文字交给 HyperFrames 层。',
  ].filter(Boolean)
  return parts.join('\n\n')
}

function firstPromptSentence(value: string): string {
  const text = normalizeReadableText(value)
  if (!text) return ''
  const match = text.match(/^(.{18,120}?[。.!！?？；;])/u)
  return (match?.[1] || text.slice(0, 120)).trim()
}

function lockedDimensionLine(group: DirectorShotReviewGroup, keywords: string[]): string {
  const lowerKeywords = keywords.map((item) => item.toLowerCase())
  return group.production.lockedDimensions.find((dimension) => {
    const lower = dimension.toLowerCase()
    return lowerKeywords.some((keyword) => lower.includes(keyword))
  }) || ''
}

function cameraMotionHint(prompt: string): string {
  const text = normalizeReadableText(prompt)
  if (/推|推进|push|dolly|zoom|跟拍|摇镜|pan|tilt|环绕|orbit|移镜|tracking/iu.test(text)) {
    return firstPromptSentence(text) || '已有运镜描述'
  }
  return '明确镜头从哪里开始、如何移动、在哪里结束，例如推近、跟拍、横移或环绕。'
}

function shotSizeHint(prompt: string): string {
  const text = normalizeReadableText(prompt)
  if (/特写|近景|中景|远景|全景|wide|close|medium|establishing/iu.test(text)) {
    return firstPromptSentence(text) || '已有景别描述'
  }
  return '补充景别变化：远景建立空间，中景展示动作，近景强调情绪。'
}

function moodHint(prompt: string): string {
  const text = normalizeReadableText(prompt)
  if (/光|阴影|冷|暖|昏暗|明亮|情绪|压抑|温柔|紧张|light|shadow|mood|tone/iu.test(text)) {
    return firstPromptSentence(text) || '已有光线情绪'
  }
  return '补充光线、色温、空气质感和情绪细节，让模型进行 vibe create。'
}

function timelineChangeHint(requestEntries: ShotRequestPreviewEntry[], durationSec?: number): string {
  const request = requestEntries.find((entry) => entry.request?.target?.durationSec || entry.request?.aigcPlan?.keyframeStrategy)?.request
  if (request?.aigcPlan?.keyframeStrategy) return request.aigcPlan.keyframeStrategy
  const duration = durationSec || request?.target?.durationSec || 8
  return `按 ${duration}s 设计起始画面、中段动作变化和结束画面，明确角色、道具、场景和相机的变化。`
}

function firstRequestDuration(requestEntries: ShotRequestPreviewEntry[]): number | undefined {
  return requestEntries.find((entry) => entry.request?.target?.durationSec)?.request?.target?.durationSec
}

function richVoiceAigcInsertDescription(request: ExternalGenerationRequestContent, fallbackVisualText: string): string {
  const source = normalizeReadableText([request.visualText, request.overallShotPrompt, request.prompt].filter(Boolean).join(' '))
  const targetText = [
    request.target?.durationSec ? `${request.target.durationSec}s` : '',
    request.target?.aspectRatio,
    request.target?.resolution,
  ].filter(Boolean).join(' / ')
  if (/AI\s*内容流程|一次跑到底|选题、脚本、分镜|创作流程/u.test(source)) {
    return [
      targetText ? `${targetText}。` : '',
      '把“AI 内容流程，一次跑到底”处理成一段寓言式机械喜剧：明亮创作桌像一座微型剧场，脚本纸、分镜卡、素材贴纸和 QA 放大镜先拥挤成温柔的混乱；圆滚滚的小机器人像默片里的机关师按下开始键，桌面随即展开成发光传送带。',
      '画面参考童话寓言的隐喻、默片喜剧的连锁动作和立体绘本的层次：便利贴从乱跳到排队盖章，纸团被整理成视频胶片，最后弹出干净的视频胶囊与开源星标。整体轻快、明亮、解压，右侧或下方留出干净字幕区，不让 AIGC 生成任何可读文字。',
    ].filter(Boolean).join('')
  }
  const concept = quotedConcept(source) || firstPromptSentence(source) || fallbackVisualText || '当前口播信息点'
  return [
    targetText ? `${targetText}。` : '',
    `把“${concept}”转成服务口播的动态视觉隐喻：先给观众一个能立刻读懂的反差动作，再让主体、道具和环境产生连锁变化，最后收束到清爽稳定的构图，方便字幕和 HyperFrames 文字层叠加。`,
    '画面应像一段短篇寓言或绘本剧场：细节具体、动作有因果、镜头有呼吸，情绪积极但不喧闹；AIGC 只负责背景、运动、氛围和镜头变化，不生成标题、Logo、按钮或可读文字。',
  ].filter(Boolean).join('')
}

function promptTimelineBeats(request: ExternalGenerationRequestContent): Array<{ time: string; text: string }> {
  const prompt = normalizeReadableText(request.prompt || request.overallShotPrompt || request.visualText || '')
  const matches = Array.from(prompt.matchAll(/(\d+(?:\.\d+)?\s*[-~—]\s*\d+(?:\.\d+)?)\s*秒[：:]\s*([^。；;]+[。]?)/gu))
  const beats = matches.map((match) => ({
    time: `${match[1].replace(/\s+/gu, '')}s`,
    text: match[2].trim(),
  })).filter((beat) => beat.text).slice(0, 3)
  if (beats.length) return beats
  const duration = request.target?.durationSec || 8
  return [
    { time: `0-${Math.round(duration * 0.3)}s`, text: '建立清晰主体和情绪钩子，让观众第一眼看懂视觉隐喻。' },
    { time: `${Math.round(duration * 0.3)}-${Math.round(duration * 0.7)}s`, text: '主体、道具和环境发生连锁变化，推动信息从混乱走向清晰。' },
    { time: `${Math.round(duration * 0.7)}-${duration}s`, text: '动作稳定收束，保留字幕安全区，方便进入下一段内容。' },
  ]
}

function voiceAigcInsertNotes(request: ExternalGenerationRequestContent): string[] {
  const source = normalizeReadableText([request.prompt, request.visualText, request.overallShotPrompt, request.textSafeLayout].filter(Boolean).join(' '))
  const notes = new Set<string>()
  notes.add('服务口播')
  if (/非真人|动画|风格化|绘本|卡通/u.test(source)) notes.add('非真人风格化')
  if (/不要生成文字|不要.*文字|字幕|Logo|水印|可读汉字/u.test(source)) notes.add('无文字乱码风险')
  if (/留白|安全区|25%|35%/u.test(source)) notes.add('保留字幕安全区')
  if (/推近|横移|镜头|运镜/u.test(source)) notes.add('镜头有运动')
  if (/0\.5\s*秒|稳定收束|末尾/u.test(source)) notes.add('末尾稳定收束')
  return Array.from(notes).slice(0, 6)
}

function quotedConcept(text: string): string {
  const match = text.match(/[“"]([^”"]{4,42})[”"]/u)
  return match?.[1]?.trim() || ''
}

function splitSubtitleLines(text: string, maxLines: number): string[] {
  const clean = normalizeReadableText(text)
  if (!clean) return []
  const sentenceParts = clean.split(/(?<=[。！？!?；;])/u).map((item) => item.trim()).filter(Boolean)
  const parts = sentenceParts.length >= maxLines ? sentenceParts : clean.match(new RegExp(`.{1,${Math.max(18, Math.ceil(clean.length / maxLines))}}`, 'gu')) || [clean]
  return parts.slice(0, maxLines)
}

function subtitleFallbackCuesFromNarration(narrationText: string, durationSec?: number): SubtitleCue[] {
  const lines = splitSubtitleLines(narrationText, 4)
  const duration = durationSec || 8
  return lines.map((line, index) => {
    const start = Math.round((duration / Math.max(1, lines.length)) * index)
    const end = Math.round((duration / Math.max(1, lines.length)) * (index + 1))
    return {
      start: `${start}s`,
      end: `${end}s`,
      text: line,
    }
  })
}

function ShotFocusMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-background-card px-3 py-2 ring-1 ring-line">
      <div className="text-[11px] font-black text-primary-dark">{label}</div>
      <div className="mt-1 truncate text-xs font-black text-ink" title={value}>{value}</div>
    </div>
  )
}

function shotReferenceCount(requestEntries: ShotRequestPreviewEntry[]): number {
  const ids = new Set<string>()
  for (const entry of requestEntries) {
    for (const ref of entry.request?.references || []) {
      ids.add(ref.artifactId || ref.storageRef || ref.id)
    }
  }
  return ids.size
}

function hyperFramesTimelineSummary(requestEntries: ShotRequestPreviewEntry[]): string {
  const withHyperFrames = requestEntries.filter((entry) => {
    const request = entry.request
    return Boolean(request?.hyperframesPlan?.prompt || request?.hyperframesPlan?.plan || request?.textSafeLayout)
  }).length
  return withHyperFrames ? `${withHyperFrames} 段 HyperFrames 约束` : '字幕 / 图形待确认'
}

function ShotLayerTimeline({
  mode,
  group,
  requestEntries,
  narrationOverride,
}: {
  mode: ShotWorkspaceMode
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  narrationOverride?: string
}) {
  const narrationText = narrationOverride || shotNarrationForDisplay(group, requestEntries)
  const rows = mode === 'aigc_shot'
    ? [
      {
        label: '剧本',
        time: 'source',
        detail: narrationText || group.title || '确认剧本片段和情绪目标',
        tone: 'bg-white',
      },
      {
        label: 'AIGC 主画面',
        time: '0-100%',
        detail: firstRequestLine(requestEntries, ['visualText', 'overallShotPrompt', 'prompt']) || '描述角色、场景、景别、运镜、情绪和物理连续性',
        tone: 'bg-primary-soft/65',
      },
      {
        label: '全局参考',
        time: 'lock',
        detail: referenceStrategyLine(mode, group, requestEntries),
        tone: 'bg-green-50',
      },
      {
        label: 'HyperFrames 字幕',
        time: 'overlay',
        detail: firstHyperFramesLine(requestEntries) || '只做字幕、说明和少量可控图形，不抢 AIGC 主画面',
        tone: 'bg-background-card',
      },
    ]
    : [
      {
        label: '口播稿',
        time: 'source',
        detail: narrationText || '口播稿待确认',
        tone: 'bg-white',
      },
      {
        label: 'HyperFrames 可控层',
        time: '0-100%',
        detail: firstHyperFramesLine(requestEntries) || '字幕、标题、卡片、UI 素材和可控浏览器组件按时间线变化',
        tone: 'bg-primary-soft/65',
      },
      {
        label: 'AIGC 插入点',
        time: 'b-roll',
        detail: firstRequestLine(requestEntries, ['visualText', 'prompt']) || '仅在画面单调处插入素材，服务口播内容，不要求跨 shot 一致性',
        tone: 'bg-green-50',
      },
      {
        label: 'FFmpeg 合成',
        time: 'final',
        detail: firstFfmpegLine(requestEntries) || '把 AIGC 素材、HyperFrames 层和字幕层融合成完整 shot',
        tone: 'bg-background-card',
      },
    ]

  return (
    <div className="rounded-lg bg-background-card p-3 ring-1 ring-line">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-xs font-black text-primary-dark">{mode === 'aigc_shot' ? '镜头连续性时间线' : '口播服务型时间线'}</div>
          <p className="mt-1 text-[11px] leading-5 text-ink-muted">
            {mode === 'aigc_shot' ? '重点是 AIGC 画面、参考图和跨 shot 物理一致性。' : '重点是口播内容、HyperFrames 组件变化和 AIGC 插入位置。'}
          </p>
        </div>
        <span className="rounded-full bg-white px-2.5 py-1 text-[11px] font-black text-primary-dark ring-1 ring-line">{group.durationSec ? `${group.durationSec}s` : 'shot'}</span>
      </div>
      <div className="mt-3 space-y-2">
        {rows.map((row) => (
          <div key={row.label} className={clsx('grid gap-3 rounded-lg p-3 ring-1 ring-line md:grid-cols-[88px_76px_1fr]', row.tone)}>
            <div className="text-xs font-black text-ink">{row.label}</div>
            <div className="font-mono text-[11px] font-black text-primary-dark">{row.time}</div>
            <div className="line-clamp-2 text-xs leading-5 text-ink-muted">{row.detail}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function ShotReferenceStrategyPanel({
  mode,
  group,
  requestEntries,
  narrationOverride,
}: {
  mode: ShotWorkspaceMode
  group: DirectorShotReviewGroup
  requestEntries: ShotRequestPreviewEntry[]
  narrationOverride?: string
}) {
  const buckets = referenceBuckets(requestEntries)
  const narrationText = narrationOverride || shotNarrationForDisplay(group, requestEntries)
  const locked = group.production.lockedDimensions.length ? group.production.lockedDimensions.join('、') : '暂无锁定维度'
  const rows = mode === 'aigc_shot'
    ? [
      { label: '角色多视角', value: `${buckets.character} 张`, hint: '主角脸、服装、发型、年龄和情绪必须跨 shot 一致。' },
      { label: '场景 / 道具', value: `${buckets.scene + buckets.prop} 张`, hint: '空间关系、关键道具和物理状态要连续。' },
      { label: '故事板 / 首帧', value: `${buckets.storyboard} 张`, hint: '用于锁定景别、构图、起止状态和转场。' },
      { label: '连续性锁定', value: locked, hint: '修改本 shot 时不要破坏前后 shot 的角色、场景、道具状态。' },
    ]
    : [
      { label: '口播优先', value: narrationText ? '已读取' : '待确认', hint: '画面只服务口播，不制造新的信息负担。' },
      { label: 'HyperFrames 素材', value: hyperFramesTimelineSummary(requestEntries), hint: '标题、字幕、图形、流程标签和浏览器组件保持可控。' },
      { label: 'AIGC 插入', value: `${requestEntries.filter((entry) => entry.request).length} 个任务`, hint: '只补充 b-roll 或背景动态，不要求跨 shot 角色一致性。' },
      { label: '文字安全', value: 'HyperFrames 负责', hint: '中文文字、字幕、按钮和标签不交给 AIGC 生成。' },
    ]

  return (
    <div className="mt-3 grid gap-2 md:grid-cols-2 xl:grid-cols-4">
      {rows.map((row) => (
        <div key={row.label} className="rounded-lg bg-background-card p-3 ring-1 ring-line">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-black text-ink">{row.label}</span>
            <span className="max-w-[128px] truncate rounded bg-white px-2 py-1 text-[11px] font-black text-primary-dark ring-1 ring-line" title={row.value}>{row.value}</span>
          </div>
          <p className="mt-2 text-[11px] leading-5 text-ink-muted">{row.hint}</p>
        </div>
      ))}
    </div>
  )
}

function referenceBuckets(requestEntries: ShotRequestPreviewEntry[]) {
  const buckets = { character: 0, scene: 0, prop: 0, storyboard: 0, style: 0, other: 0 }
  for (const entry of requestEntries) {
    for (const ref of entry.request?.references || []) {
      const label = referenceDependencyLabel(ref)
      if (label.includes('角色')) buckets.character += 1
      else if (label.includes('场景')) buckets.scene += 1
      else if (label.includes('道具')) buckets.prop += 1
      else if (label.includes('故事板') || label.includes('首帧')) buckets.storyboard += 1
      else if (label.includes('风格')) buckets.style += 1
      else buckets.other += 1
    }
  }
  return buckets
}

function firstRequestLine(requestEntries: ShotRequestPreviewEntry[], keys: Array<keyof ExternalGenerationRequestContent>): string {
  return firstReadableRequestText(requestEntries, keys).slice(0, 180)
}

function firstHyperFramesLine(requestEntries: ShotRequestPreviewEntry[]): string {
  for (const entry of requestEntries) {
    const request = entry.request
    const text = normalizeReadableText(request?.hyperframesPlan?.prompt) ||
      normalizeReadableText(request?.hyperframesPlan?.plan) ||
      normalizeReadableText(request?.textSafeLayout)
    if (text) return text.slice(0, 180)
  }
  return ''
}

function firstFfmpegLine(requestEntries: ShotRequestPreviewEntry[]): string {
  for (const entry of requestEntries) {
    const text = normalizeReadableText(entry.request?.ffmpegFusionPlan?.plan)
    if (text) return text.slice(0, 180)
  }
  return ''
}

function referenceStrategyLine(mode: ShotWorkspaceMode, group: DirectorShotReviewGroup, requestEntries: ShotRequestPreviewEntry[]): string {
  if (mode === 'voice_visual') return 'AIGC 插入素材不承担跨 shot 连续性，生成时只需贴合本段口播。'
  const count = shotReferenceCount(requestEntries)
  const roles = group.referenceRoles.length ? group.referenceRoles.slice(0, 4).join('、') : ''
  return count ? `${count} 张参考图${roles ? `：${roles}` : ''}，用于角色、场景、道具和风格一致性。` : '建议补充角色、场景、道具和故事板参考图。'
}

function shotNarrationForDisplay(group: DirectorShotReviewGroup, requestEntries: ShotRequestPreviewEntry[], fallbackText?: string): string {
  const requestNarration = firstReadableRequestText(requestEntries, ['narrationText', 'sourceScriptSegment'])
  if (requestNarration && !isVisualBridgeInstruction(requestNarration)) return requestNarration
  const groupNarration = normalizeReadableText(group.narrationText)
  if (groupNarration && !isVisualBridgeInstruction(groupNarration)) return groupNarration
  const fallbackNarration = normalizeReadableText(fallbackText)
  if (fallbackNarration && !isVisualBridgeInstruction(fallbackNarration)) return fallbackNarration
  return ''
}

function voiceScriptTextForDisplay(narrationText: string): string {
  if (narrationText) return narrationText
  return '当前 shot 未读取到对应口播正文。请回到脚本 / shot list 阶段补齐本 shot 的口播内容；画面承接、视觉钩子和素材提示词不会再显示为口播稿。'
}

function neighborNarrationForShot(groups: DirectorShotReviewGroup[], currentIndex: number): string {
  const candidates: Array<DirectorShotReviewGroup | undefined> = [
    groups[currentIndex + 1],
    groups[currentIndex - 1],
  ]
  for (const group of candidates) {
    const narration = normalizeReadableText(group?.narrationText)
    if (narration && !isVisualBridgeInstruction(narration)) return narration
  }
  return ''
}

function hyperFramesPromptForDisplay(requestEntries: ShotRequestPreviewEntry[]): string {
  const parts: string[] = []
  for (const entry of requestEntries) {
    const request = entry.request
    if (!request) continue
    const plan = request.hyperframesPlan
    const title = request.requestId ? `任务 ${request.requestId}` : ''
    const text = [
      plan?.prompt ? `HyperFrames 提示词：${plan.prompt}` : '',
      plan?.plan ? `组件计划：${plan.plan}` : '',
      plan?.textSafeLayout || request.textSafeLayout ? `文字安全区：${plan?.textSafeLayout || request.textSafeLayout}` : '',
      plan?.locks?.length ? `锁定内容：${plan.locks.join('、')}` : '',
    ].filter(Boolean).join('\n')
    if (text) parts.push([title, text].filter(Boolean).join('\n'))
  }
  return Array.from(new Set(parts)).join('\n\n')
}

function shotVisualForDisplay(group: DirectorShotReviewGroup, requestEntries: ShotRequestPreviewEntry[], narrationText: string): string {
  const visualText = normalizeReadableText(group.visualText) ||
    firstReadableRequestText(requestEntries, ['visualText'])
  if (!visualText) return ''
  return isSameReadableText(visualText, narrationText) ? '' : visualText
}

function firstReadableRequestText(requestEntries: ShotRequestPreviewEntry[], keys: Array<keyof ExternalGenerationRequestContent>): string {
  for (const entry of requestEntries) {
    const request = entry.request
    if (!request) continue
    for (const key of keys) {
      const value = request[key]
      const text = normalizeReadableText(value)
      if (text) return text
    }
  }
  return ''
}

function normalizeReadableText(value: unknown): string {
  return typeof value === 'string' ? value.trim().replace(/\s+/g, ' ') : ''
}

function isSameReadableText(a: string, b: string): boolean {
  const left = normalizeReadableText(a)
  const right = normalizeReadableText(b)
  return Boolean(left && right && left === right)
}

function isVisualBridgeInstruction(value: string): boolean {
  const text = normalizeReadableText(value)
  if (!text) return false
  return text.length <= 40 && /可视化反差动作承接口播/u.test(text)
}

function shotCardPreviewText(group: DirectorShotReviewGroup): string {
  const narrationText = normalizeReadableText(group.narrationText)
  if (narrationText && !isVisualBridgeInstruction(narrationText)) return narrationText
  const visualText = normalizeReadableText(group.visualText)
  if (visualText && !isVisualBridgeInstruction(visualText)) return visualText
  return '等待 shot 内容'
}

function shotActionSummary(group: DirectorShotReviewGroup, requestEntries: ShotRequestPreviewEntry[]): string {
  const loadedRequests = requestEntries.filter((entry) => entry.request).length
  if (loadedRequests > 0) {
    return `本 shot 有 ${loadedRequests} 个素材任务。先确认口播和画面参考，再处理下方 AIGC 素材提示词、参考图和上传结果。`
  }
  if (group.slots.some((slot) => slot.kind === 'overlay' && slot.artifacts.length > 0)) {
    return '本 shot 当前主要由 HyperFrames 本地内容组成。展开下方槽位查看预览，确认后继续处理完整 shot 或最终拼接。'
  }
  return '本 shot 暂无需要手动生成的 AIGC 素材。展开下方信息确认脚本和画面意图，再查看已有产物或继续下一步。'
}

function cinematicShotActionSummary(group: DirectorShotReviewGroup, requestEntries: ShotRequestPreviewEntry[]): string {
  const loadedRequests = requestEntries.filter((entry) => entry.request).length
  const referenceCount = shotReferenceCount(requestEntries)
  if (loadedRequests > 0) {
    return `本 shot 有 ${loadedRequests} 个 AIGC 主画面任务和 ${referenceCount} 张一致性参考。先确认剧本片段、角色 / 场景 / 道具和故事板，再处理镜头提示词与上传结果。`
  }
  if (referenceCount > 0 || group.referenceRoles.length > 0) {
    return '本 shot 已读取到影视参考资产。先确认剧本、连续性和故事板，再查看主画面视频或补充 AIGC 生成任务。'
  }
  return '本 shot 暂无完整影视生成资料。请先补齐剧本片段、角色 / 场景 / 道具参考图和故事板。'
}

function ShotTextBlock({ title, value, mode }: { title: string; value: string; mode: ShotWorkspaceMode }) {
  const [draft, setDraft] = useState(value)
  const [selection, setSelection] = useState<TextSelectionDraft | null>(null)

  useEffect(() => {
    setDraft(value)
    setSelection(null)
  }, [value])

  const handleSelection = (event: MouseEvent<HTMLParagraphElement>) => {
    const nextSelection = textSelectionFromDocument(event.currentTarget, title)
    if (nextSelection) setSelection(nextSelection)
  }

  const applySelectionRewrite = async (instruction: string) => {
    if (!selection) return
    setDraft((current) => replaceFirstSelectedText(current, selection.text, buildSelectionRewriteText(selection.text, instruction, mode)))
    setSelection(null)
    window.getSelection()?.removeAllRanges()
  }

  return (
    <div className="relative rounded-lg bg-white p-4 ring-1 ring-line">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-black text-primary-dark">{title}</span>
        <CopyButton value={draft} label="复制" />
      </div>
      <p
        className="mt-2 cursor-text whitespace-pre-wrap text-sm leading-6 text-ink-muted selection:bg-primary-soft"
        onMouseUp={handleSelection}
      >
        {draft}
      </p>
      {selection ? (
        <SelectionFloatingAssistant
          selection={selection}
          mode={mode}
          actionLabel="替换选中段"
          onClose={() => setSelection(null)}
          onApply={applySelectionRewrite}
        />
      ) : null}
    </div>
  )
}

interface TextSelectionDraft {
  text: string
  sourceLabel: string
  anchor: {
    top: number
    left: number
  }
  start?: number
  end?: number
}

function textSelectionFromDocument(container: HTMLElement, sourceLabel: string): TextSelectionDraft | null {
  const selection = window.getSelection()
  const selectedText = selection?.toString().trim() || ''
  if (!selection || !selectedText || !selection.rangeCount) return null
  const anchorNode = selection.anchorNode
  const focusNode = selection.focusNode
  if ((anchorNode && !container.contains(anchorNode)) || (focusNode && !container.contains(focusNode))) return null
  const rect = selection.getRangeAt(0).getBoundingClientRect()
  return {
    text: selectedText.slice(0, 1200),
    sourceLabel,
    anchor: clampFloatingAnchor(rect.left, rect.bottom + 8),
  }
}

function textareaSelectionDraft(textarea: HTMLTextAreaElement, sourceLabel: string): TextSelectionDraft | null {
  const start = textarea.selectionStart
  const end = textarea.selectionEnd
  if (start === end) return null
  const selectedText = textarea.value.slice(start, end).trim()
  if (!selectedText) return null
  const rect = textarea.getBoundingClientRect()
  return {
    text: selectedText.slice(0, 1200),
    sourceLabel,
    start,
    end,
    anchor: clampFloatingAnchor(rect.left + 20, rect.top + 44),
  }
}

function clampFloatingAnchor(left: number, top: number): TextSelectionDraft['anchor'] {
  const width = 360
  const height = 260
  return {
    left: Math.max(16, Math.min(left, window.innerWidth - width - 16)),
    top: Math.max(16, Math.min(top, window.innerHeight - height - 16)),
  }
}

function replaceFirstSelectedText(current: string, selected: string, replacement: string): string {
  const index = current.indexOf(selected)
  if (index < 0) return current
  return `${current.slice(0, index)}${replacement}${current.slice(index + selected.length)}`
}

function replaceRange(current: string, start: number, end: number, replacement: string): string {
  return `${current.slice(0, start)}${replacement}${current.slice(end)}`
}

function buildSelectionRewriteText(selectedText: string, instruction: string, mode: ShotWorkspaceMode): string {
  const cleanInstruction = instruction.trim() || '提升可执行性和清晰度'
  if (mode === 'aigc_shot') {
    return [
      selectedText,
      `局部修改要求：${cleanInstruction}。补足角色、场景、道具、景别、镜头移动、光线情绪和前后 shot 连续性；保持物理状态一致，不改动未选中的叙事事实。`,
    ].join('\n')
  }
  return [
    selectedText,
    `局部修改要求：${cleanInstruction}。让该段更服务口播稿，明确 HyperFrames 可控素材的时间线变化和 AIGC 插入位置；AIGC 不生成文字，不承担跨 shot 一致性。`,
  ].join('\n')
}

function requestTitleForLayer(request: ExternalGenerationRequestContent, sequence: number, mode: ShotWorkspaceMode): string {
  if (mode === 'voice_visual') {
    return request.kind === 'image'
      ? `AIGC 插入层图片提示词${sequence > 1 ? ` ${sequence}` : ''}`
      : `AIGC 插入层视频提示词${sequence > 1 ? ` ${sequence}` : ''}`
  }
  return request.kind === 'image'
    ? `AIGC 主画面参考图提示词${sequence > 1 ? ` ${sequence}` : ''}`
    : `AIGC 主画面视频提示词${sequence > 1 ? ` ${sequence}` : ''}`
}

function SelectionFloatingAssistant({
  selection,
  mode,
  loading = false,
  error,
  actionLabel,
  onClose,
  onApply,
}: {
  selection: TextSelectionDraft
  mode: ShotWorkspaceMode
  loading?: boolean
  error?: string | null
  actionLabel: string
  onClose: () => void
  onApply: (instruction: string) => Promise<void> | void
}) {
  const [instruction, setInstruction] = useState('')

  return (
    <div
      className="fixed z-50 w-[min(360px,calc(100vw-32px))] rounded-lg border border-primary/30 bg-white p-3 shadow-card ring-1 ring-primary/10"
      style={{ left: selection.anchor.left, top: selection.anchor.top }}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-xs font-black text-primary-dark">AI 划词修改 · {selection.sourceLabel}</div>
          <p className="mt-1 line-clamp-2 text-[11px] leading-5 text-ink-muted">{selection.text}</p>
        </div>
        <button type="button" onClick={onClose} className="rounded p-1 text-ink-soft hover:bg-background-card hover:text-primary-dark" aria-label="关闭划词修改">
          <FiX />
        </button>
      </div>
      <textarea
        value={instruction}
        onChange={(event) => setInstruction(event.target.value)}
        placeholder={mode === 'aigc_shot' ? '例如：加强镜头推进和主角情绪，但保持场景一致。' : '例如：改成 2 秒出现的数据卡片，别影响口播节奏。'}
        className="mt-3 h-20 w-full resize-none rounded-lg border border-line bg-background-card p-2 text-xs leading-5 text-ink outline-none focus:border-primary"
      />
      {error ? <p className="mt-2 text-xs font-semibold text-red-600">{error}</p> : null}
      <div className="mt-3 flex flex-wrap justify-end gap-2">
        <button type="button" onClick={() => setInstruction('更明确时间线、动作变化和素材边界。')} className="rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft">
          时间线
        </button>
        <button type="button" onClick={() => setInstruction(mode === 'aigc_shot' ? '加强跨 shot 一致性，明确角色、场景、道具的锁定状态。' : '更贴合口播内容，减少喧宾夺主的画面变化。')} className="rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft">
          聚焦
        </button>
        <button
          type="button"
          disabled={loading}
          onClick={() => { void onApply(instruction) }}
          className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-2.5 py-1.5 text-xs font-black text-white shadow-sm disabled:cursor-not-allowed disabled:opacity-60"
        >
          {loading ? <FiRefreshCw className="animate-spin" /> : <FiEdit3 />} {loading ? '处理中...' : actionLabel}
        </button>
      </div>
    </div>
  )
}

function ShotRequestSummaryCard({
  shotId,
  slot,
  request,
  sequence,
  artifactId,
  mode,
  projectId,
  projectReady,
  allowUpload,
  uploading,
  onUpload,
  onRequestRevised,
}: {
  shotId: string
  slot: DirectorShotAssetSlot
  request: ExternalGenerationRequestContent
  sequence: number
  artifactId?: string
  mode: ShotWorkspaceMode
  projectId?: string
  projectReady: boolean
  allowUpload: boolean
  uploading?: boolean
  onUpload?: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
  onRequestRevised?: (artifactId: string, request: ExternalGenerationRequestContent) => Promise<void> | void
}) {
  const [promptDraft, setPromptDraft] = useState(() => safePromptText(request.prompt))
  const [promptInstruction, setPromptInstruction] = useState('')
  const [referenceDrafts, setReferenceDrafts] = useState<ExternalGenerationReference[]>(() => cloneReferenceDrafts(request.references))
  const [approved, setApproved] = useState(false)
  const [promptSelection, setPromptSelection] = useState<TextSelectionDraft | null>(null)
  const [partialRevisionLoading, setPartialRevisionLoading] = useState(false)
  const [partialRevisionError, setPartialRevisionError] = useState<string | null>(null)

  useEffect(() => {
    setPromptDraft(safePromptText(request.prompt))
    setPromptInstruction('')
    setReferenceDrafts(cloneReferenceDrafts(request.references))
    setApproved(false)
    setPromptSelection(null)
    setPartialRevisionError(null)
  }, [request.references, request.requestId, request.prompt])

  const accept = request.kind === 'video' ? 'video/*' : 'image/*'
  const usableReferences = referenceDrafts.filter((ref) => ref.storageRef.trim())
  const editableRequest: ExternalGenerationRequestContent = { ...request, references: usableReferences }
  const targetText = [
    request.target?.aspectRatio,
    request.target?.resolution,
    request.target?.durationSec ? `${request.target.durationSec}s` : '',
  ].filter(Boolean).join(' / ')
  const dependencySummary = requestDependencySummary(editableRequest, slot)
  const referenceRows = requestReferenceRows(editableRequest)
  const promptSourceLabel = mode === 'aigc_shot' ? 'AIGC 镜头提示词' : 'AIGC 插入素材提示词'
  const requestCardTitle = requestTitleForLayer(request, sequence, mode)

  const applyPromptSelectionRewrite = async (instruction: string) => {
    if (!promptSelection) return
    const fallbackReplacement = buildSelectionRewriteText(promptSelection.text, instruction, mode)
    if (!artifactId) {
      if (promptSelection.start !== undefined && promptSelection.end !== undefined) {
        setPromptDraft((current) => replaceRange(current, promptSelection.start || 0, promptSelection.end || 0, fallbackReplacement))
      }
      setPromptSelection(null)
      return
    }
    setPartialRevisionLoading(true)
    setPartialRevisionError(null)
    try {
      const clientModelProviders = await buildClientModelProvidersForRun()
      const revised = await reviseArtifact(
        artifactId,
        buildPartialRevisionInstruction(promptSelection.text, instruction, mode, promptSourceLabel),
        clientModelProviders as Record<string, unknown> | undefined,
      )
      const nextRequest = externalGenerationRequestFromContent(revised.content)
      if (nextRequest) {
        setPromptDraft(safePromptText(nextRequest.prompt))
        setReferenceDrafts(cloneReferenceDrafts(nextRequest.references))
        await onRequestRevised?.(artifactId, nextRequest)
      } else if (promptSelection.start !== undefined && promptSelection.end !== undefined) {
        setPromptDraft((current) => replaceRange(current, promptSelection.start || 0, promptSelection.end || 0, fallbackReplacement))
      }
      setPromptSelection(null)
    } catch (err) {
      setPartialRevisionError(normalizeDirectorErrorMessage(err))
    } finally {
      setPartialRevisionLoading(false)
    }
  }

  return (
    <details className="rounded-lg border border-line bg-white p-3 shadow-sm">
      <summary className="cursor-pointer list-none">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="text-sm font-black text-ink">{requestCardTitle}</div>
            <div className="mt-1 text-xs leading-5 text-ink-muted">
              对应：{slot.label}{targetText ? ` · ${targetText}` : ''} · {dependencySummary}
            </div>
          </div>
          <span className={clsx(
            'rounded-full px-3 py-1 text-[11px] font-black',
            approved ? 'bg-green-50 text-green-700 ring-1 ring-green-100' : 'bg-primary-soft text-primary-dark',
          )}>{approved ? '已通过，内容已锁定' : '展开处理'}</span>
        </div>
      </summary>

      {request.kind === 'video' && mode === 'aigc_shot' ? <ShotLayerPlanPanel request={editableRequest} mode={mode} /> : null}

      <div className="mt-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div className="text-xs font-black text-ink-soft">素材操作 · {mode === 'aigc_shot' ? '镜头生成' : '插入生成'}</div>
            <div className="mt-1 text-[11px] text-ink-muted">
              {mode === 'aigc_shot' ? '划词修改镜头提示词，生成视频后上传；通过后锁定当前版本。' : '划词修改 AIGC 插入素材提示词，生成后上传；通过后锁定当前版本。'}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <CopyButton value={promptDraft} label="复制提示词" />
            <button
              type="button"
              onClick={() => setApproved(true)}
              disabled={approved}
              className={clsx(
                'inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-black ring-1',
                approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
              )}
            >
              <FiCheck /> {approved ? '已通过，内容已锁定' : '通过并锁定'}
            </button>
          </div>
        </div>
        <div className="mt-3 rounded-lg bg-white p-3 ring-1 ring-line">
          <div className="text-xs font-black text-ink-soft">优化要求</div>
          <textarea
            value={promptInstruction}
            disabled={approved}
            onChange={(event) => setPromptInstruction(event.target.value)}
            placeholder={mode === 'aigc_shot' ? '例如：加强镜头从中景推到近景，主角表情更压抑，保持道具位置。' : '例如：改成 2 秒 b-roll，背景更现代，右侧留给字幕。'}
            className="mt-2 min-h-20 w-full resize-y rounded-lg border border-line bg-background-card p-3 text-xs leading-5 text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
            aria-label={`${request.requestId} 提示词生成要求`}
          />
          <button
            type="button"
            disabled={approved}
            onClick={() => setPromptDraft(regeneratePromptDraft(request, referenceDrafts, promptInstruction, promptDraft, mode))}
            className={clsx(
              'mt-2 inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-black ring-1',
              approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
            )}
          >
            <FiRefreshCw /> 按要求重新生成提示词
          </button>
        </div>
        <div className="mt-3 text-xs font-black text-ink-soft">{promptSourceLabel}</div>
        <div className="relative">
          <textarea
            value={promptDraft}
            maxLength={EXTERNAL_PROMPT_MAX_CHARS}
            disabled={approved}
            onChange={(event) => setPromptDraft(safePromptText(event.target.value))}
            onSelect={(event) => setPromptSelection(textareaSelectionDraft(event.currentTarget, promptSourceLabel))}
            onMouseUp={(event) => setPromptSelection(textareaSelectionDraft(event.currentTarget, promptSourceLabel))}
            className="mt-3 min-h-36 w-full resize-y rounded-lg border border-line bg-white p-3 text-xs leading-5 text-ink outline-none selection:bg-primary-soft focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
            aria-label={`${request.requestId} 可编辑提示词`}
          />
          {promptSelection ? (
            <SelectionFloatingAssistant
              selection={promptSelection}
              mode={mode}
              loading={partialRevisionLoading}
              error={partialRevisionError}
              actionLabel={artifactId ? 'AI 局部返工' : '替换选中段'}
              onClose={() => setPromptSelection(null)}
              onApply={applyPromptSelectionRewrite}
            />
          ) : null}
        </div>
        {request.negativePrompt ? (
          <details className="mt-3 rounded-lg bg-white p-3 ring-1 ring-line">
            <summary className="cursor-pointer text-xs font-black text-ink-soft">可选负面提示词</summary>
            <p className="mt-2 whitespace-pre-wrap text-xs leading-5 text-ink-muted">{safePromptText(request.negativePrompt)}</p>
          </details>
        ) : null}
      </div>

      <div className="mt-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div className="text-xs font-black text-ink-soft">编辑参考图</div>
            <div className="mt-1 text-[11px] text-ink-muted">可以修改参考图名称、类型和路径；通过后这些内容会锁定。</div>
          </div>
          <button
            type="button"
            disabled={approved}
            onClick={() => setReferenceDrafts((current) => [...current, newReferenceDraft(current.length + 1)])}
            className={clsx(
              'inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-black ring-1',
              approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
            )}
          >
            <FiUpload /> 新增参考图
          </button>
        </div>
        {referenceDrafts.length ? (
          <div className="mt-3 space-y-3">
            {referenceDrafts.map((ref, index) => (
              <div key={ref.id || `${request.requestId}-ref-${index}`} className="rounded-lg bg-white p-3 ring-1 ring-line">
                <div className="grid gap-2 md:grid-cols-[1fr_0.8fr_1.4fr_auto]">
                  <input
                    value={ref.label || ''}
                    disabled={approved}
                    onChange={(event) => updateReferenceDraft(setReferenceDrafts, index, { label: event.target.value })}
                    placeholder="参考图名称"
                    className="rounded-lg border border-line bg-background-card px-3 py-2 text-xs text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
                    aria-label={`参考图 ${index + 1} 名称`}
                  />
                  <input
                    value={ref.role || ''}
                    disabled={approved}
                    onChange={(event) => updateReferenceDraft(setReferenceDrafts, index, { role: event.target.value })}
                    placeholder="类型，如 storyboard"
                    className="rounded-lg border border-line bg-background-card px-3 py-2 text-xs text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
                    aria-label={`参考图 ${index + 1} 类型`}
                  />
                  {isInternalStorageRef(ref.storageRef) ? (
                    <div className="rounded-lg border border-line bg-green-50 px-3 py-2 text-xs font-semibold text-green-700">
                      已选择参考图
                    </div>
                  ) : (
                    <input
                      value={ref.storageRef}
                      disabled={approved}
                      onChange={(event) => updateReferenceDraft(setReferenceDrafts, index, { storageRef: event.target.value })}
                      placeholder="图片 URL 或上传后自动登记"
                      className="rounded-lg border border-line bg-background-card px-3 py-2 text-xs text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
                      aria-label={`参考图 ${index + 1} 来源`}
                    />
                  )}
                  <button
                    type="button"
                    disabled={approved}
                    onClick={() => setReferenceDrafts((current) => current.filter((_, itemIndex) => itemIndex !== index))}
                    className={clsx(
                      'rounded-lg px-3 py-2 text-xs font-black ring-1',
                      approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
                    )}
                  >
                    移除
                  </button>
                </div>
                {ref.storageRef.trim() ? (
                  <div className="mt-3">
                    <ExternalReferenceCard reference={ref} index={index + 1} mode={mode} projectId={projectId} />
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        ) : (
          <p className="mt-3 rounded-lg bg-white p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">本素材没有图片依赖，可以直接使用上方提示词生成，也可以新增参考图。</p>
        )}
        {referenceRows.length ? (
          <div className="mt-3 space-y-2">
            {referenceRows.map((row) => (
              <div key={row} className="rounded-lg bg-white px-3 py-2 text-xs leading-5 text-ink-muted ring-1 ring-line">{row}</div>
            ))}
          </div>
        ) : null}
      </div>

      {allowUpload && onUpload ? (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <label className={clsx(
            'inline-flex cursor-pointer items-center gap-1.5 rounded-lg bg-primary px-2.5 py-1.5 text-xs font-black text-white',
            (!projectReady || uploading) && 'cursor-not-allowed opacity-50',
          )}>
            <FiUpload /> {uploading ? '上传中...' : request.kind === 'image' ? '上传图片结果' : '上传视频结果'}
            <input
              type="file"
              accept={accept}
              data-smoke-id="external-generation-upload"
              disabled={!projectReady || uploading}
              className="sr-only"
              onChange={(event) => onUpload(event, shotId, slot, request)}
            />
          </label>
          <span className="text-xs leading-5 text-ink-muted">生成完成后上传到这里，系统会把它关联到当前 shot。</span>
        </div>
      ) : null}
    </details>
  )
}

function ShotLayerPlanPanel({ request, mode }: { request: ExternalGenerationRequestContent; mode: ShotWorkspaceMode }) {
  const textSafeLayout = request.textSafeLayout || request.aigcPlan?.textSafeLayout || 'AIGC 视频层需要给 HyperFrames 标题、字幕和流程标签留出干净区域。'
  const aigcDetail = [
    request.aigcPlan?.prompt || request.prompt,
    request.aigcPlan?.avoidGeneratedText ? '必须不要生成文字、字幕、Logo、水印或可读汉字，避免乱码。' : '',
    request.aigcPlan?.requiresBlankArea ? textSafeLayout : '',
  ].filter(Boolean).join('\n\n')
  const hyperframesDetail = [
    request.hyperframesPlan?.prompt || 'HyperFrames 负责本 shot 的精确文字、关键帧、字幕、UI 卡片和图形包装。',
    request.hyperframesPlan?.locks?.length ? `锁定内容：${request.hyperframesPlan.locks.join('、')}` : '',
  ].filter(Boolean).join('\n\n')
  const ffmpegDetail = [
    request.ffmpegFusionPlan?.plan || 'FFmpeg 会把 AIGC 背景或局部素材与 HyperFrames 文字层合成为完整 shot。',
    request.ffmpegFusionPlan?.mode ? `融合模式：${request.ffmpegFusionPlan.mode}` : '',
    request.ffmpegFusionPlan?.outputArtifactKind ? `输出产物：${request.ffmpegFusionPlan.outputArtifactKind}` : '',
  ].filter(Boolean).join('\n\n')

  return (
    <div className="mt-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-xs font-black text-ink-soft">分层创作计划</div>
          <p className="mt-1 text-xs leading-5 text-ink-muted">
            {mode === 'aigc_shot' ? 'AIGC 负责主画面和镜头表达，HyperFrames 只做字幕 / 简单说明，最后 FFmpeg 融合。' : 'HyperFrames 负责可控文字、字幕和素材时间线，AIGC 只做插入素材，最后 FFmpeg 融合。'}
          </p>
        </div>
        <span className="rounded-full bg-primary-soft px-2.5 py-1 text-[11px] font-black text-primary-dark">{mode === 'aigc_shot' ? '连续性优先' : '文字留白'}</span>
      </div>
      <div className="mt-3 rounded-lg bg-white p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">
        {textSafeLayout}
      </div>
      <div className="mt-3 space-y-2">
        <CompactLayerPlanRow
          icon={<FiVideo />}
          title={mode === 'aigc_shot' ? 'AIGC 视频层 / 主画面层' : 'AIGC 视频层 / 插入层'}
          summary={mode === 'aigc_shot' ? '角色、场景、镜头、情绪和运动。' : '背景或局部动态，不生成文字。'}
          detail={aigcDetail}
        />
        <CompactLayerPlanRow
          icon={<FiLayers />}
          title="HyperFrames 文字 / 图形层"
          summary={mode === 'aigc_shot' ? '字幕、少量说明和安全文字。' : '中文标题、字幕、关键帧和 UI 图形。'}
          detail={hyperframesDetail}
        />
        <CompactLayerPlanRow
          icon={<FiCpu />}
          title="FFmpeg 融合"
          summary="裁剪、叠加、统一规格后输出完整 shot。"
          detail={ffmpegDetail}
        />
      </div>
    </div>
  )
}

function CompactLayerPlanRow({
  icon,
  title,
  summary,
  detail,
}: {
  icon: ReactNode
  title: string
  summary: string
  detail: string
}) {
  return (
    <details className="rounded-lg bg-white px-3 py-2 ring-1 ring-line">
      <summary className="cursor-pointer list-none">
        <div className="flex items-center gap-2">
          <span className="mt-0.5 text-primary">{icon}</span>
          <div className="min-w-0 flex-1">
            <div className="text-xs font-black text-ink">{title}</div>
            <p className="truncate text-xs leading-5 text-ink-muted">{summary}</p>
          </div>
        </div>
      </summary>
      <pre className="mt-3 max-h-48 whitespace-pre-wrap break-words rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">{detail}</pre>
    </details>
  )
}

function safePromptText(prompt: string | undefined): string {
  return (prompt || '').slice(0, EXTERNAL_PROMPT_MAX_CHARS)
}

function cloneReferenceDrafts(references: ExternalGenerationReference[]): ExternalGenerationReference[] {
  return references.slice(0, 6).map((ref, index) => ({
    id: ref.id || `ref-${index + 1}`,
    label: ref.label || '',
    role: ref.role || '',
    storageRef: ref.storageRef || '',
    artifactId: ref.artifactId,
    locks: ref.locks,
  }))
}

function newReferenceDraft(index: number): ExternalGenerationReference {
  return {
    id: `manual-ref-${Date.now()}-${index}`,
    label: `新增参考图 ${index}`,
    role: 'reference',
    storageRef: '',
  }
}

function updateReferenceDraft(
  setReferenceDrafts: Dispatch<SetStateAction<ExternalGenerationReference[]>>,
  index: number,
  patch: Partial<ExternalGenerationReference>,
) {
  setReferenceDrafts((current) => current.map((ref, itemIndex) => itemIndex === index ? { ...ref, ...patch } : ref))
}

function regeneratePromptDraft(
  request: ExternalGenerationRequestContent,
  references: ExternalGenerationReference[],
  instruction: string,
  currentPrompt: string,
  mode: ShotWorkspaceMode,
): string {
  const referenceText = references
    .filter((ref) => ref.storageRef.trim())
    .map((ref, index) => `${index + 1}. ${referenceDependencyLabel(ref)}：${ref.label || ref.id || '参考图'}`)
    .join('\n')
  const layerText = request.kind === 'video'
    ? [
      request.aigcPlan?.prompt ? `AIGC 层：${request.aigcPlan.prompt}` : mode === 'aigc_shot' ? 'AIGC 主画面层：生成完整镜头画面，强调角色、场景、运镜、景别、道具和情绪。' : 'AIGC 插入层：只生成无文字背景或局部动态素材。',
      request.textSafeLayout || request.aigcPlan?.textSafeLayout ? `文字留白：${request.textSafeLayout || request.aigcPlan?.textSafeLayout}` : mode === 'aigc_shot' ? '文字策略：AIGC 不负责字幕和中文文字，字幕交给 HyperFrames 层。' : '文字留白：为 HyperFrames 标题、字幕和流程标签预留干净区域。',
      request.hyperframesPlan?.prompt ? `HyperFrames 层：${request.hyperframesPlan.prompt}` : mode === 'aigc_shot' ? 'HyperFrames 层：只做字幕、少量说明和可控文字，不干扰 AIGC 主画面。' : 'HyperFrames 层：中文文字、字幕、UI 文案和关键帧由本地渲染。',
      request.ffmpegFusionPlan?.plan ? `FFmpeg 融合：${request.ffmpegFusionPlan.plan}` : 'FFmpeg 融合：上传素材后叠加 HyperFrames 层，输出完整 shot。',
    ].join('\n')
    : ''
  const parts = [
    `生成类型：${request.kind === 'image' ? '图片' : '视频'}`,
    request.target?.durationSec ? `目标时长：${request.target.durationSec}s` : '',
    request.target?.aspectRatio ? `画幅：${request.target.aspectRatio}` : '',
    referenceText ? `必须参考：\n${referenceText}` : '参考图：无，可仅使用文字提示词。',
    layerText ? `分层创作约束：\n${layerText}` : '',
    instruction.trim() ? `用户修改要求：${instruction.trim()}` : '',
    `基于当前提示词优化：${currentPrompt || request.prompt}`,
    mode === 'aigc_shot'
      ? '保持跨 shot 的主体、场景、道具、物理状态和风格一致；提示词要有文学性的画面描写、镜头移动、景别变化和情绪细节，但不要生成错误文字、字幕、水印或 Logo。'
      : '保持口播内容意图一致；AIGC 只服务本段口播，不承担跨 shot 一致性；明确插入时机、画面变化和文字安全区，避免乱码、错误文字、主体漂移和过度重写。',
  ].filter(Boolean)
  return safePromptText(parts.join('\n\n'))
}

function buildPartialRevisionInstruction(selectedText: string, instruction: string, mode: ShotWorkspaceMode, sourceLabel: string): string {
  const cleanInstruction = instruction.trim() || '请提升这段内容的可执行性，但保持原意。'
  const modeContract = mode === 'aigc_shot'
    ? '这是影视 / AIGC shot 视频：只改选中片段，重点补强 AIGC 主画面描述、角色/场景/道具一致性、景别、镜头移动、情绪和物理连续性。HyperFrames 只负责字幕和少量说明。'
    : '这是口播 / 知识类视频：只改选中片段，重点服务口播稿，明确 HyperFrames 可控层的时间线变化、AIGC 插入位置和文字安全区。AIGC 不需要跨 shot 一致性。'
  return [
    `请只局部返工「${sourceLabel}」中的选中片段，不要重写整个产物。`,
    modeContract,
    `用户要求：${cleanInstruction}`,
    '选中片段：',
    selectedText,
    '输出要求：保持原 JSON / Markdown 结构，未选中的内容保持不变；如果这是 external_generation_request，只更新相关 prompt / plan 字段。',
  ].join('\n\n')
}

function requestDependencySummary(request: ExternalGenerationRequestContent, slot: DirectorShotAssetSlot): string {
  if (!request.references.length) return request.kind === 'video' ? '无参考图，可先生成故事板或直接用提示词' : '无参考图，可直接用提示词'
  const labels = request.references
    .map((ref) => referenceDependencyLabel(ref))
    .filter(Boolean)
  const uniqueLabels = [...new Set(labels)]
  return `依赖 ${uniqueLabels.slice(0, 3).join('、') || slot.label} ${request.references.length} 个`
}

function requestReferenceRows(request: ExternalGenerationRequestContent): string[] {
  return request.references.slice(0, 4).map((ref, index) => {
    const label = ref.label || ref.id || `参考 ${index + 1}`
    return `${index + 1}. ${referenceDependencyLabel(ref)}：${label}`
  })
}

function referenceDependencyLabel(reference: ExternalGenerationReference): string {
  const raw = `${reference.role || ''} ${reference.label || ''} ${reference.id || ''}`.toLowerCase()
  if (raw.includes('storyboard') || raw.includes('keyframe') || raw.includes('首帧') || raw.includes('故事板') || raw.includes('关键帧')) return '故事板 / 首帧'
  if (raw.includes('character') || raw.includes('人物') || raw.includes('角色')) return '角色参考图'
  if (raw.includes('scene') || raw.includes('场景')) return '场景参考图'
  if (raw.includes('prop') || raw.includes('道具')) return '道具参考图'
  if (raw.includes('style') || raw.includes('风格')) return '风格参考图'
  return '参考图'
}

function ShotAssetSlotCard({
  shotId,
  slot,
  durationSec,
  projectId,
  hideMaterializedArtifacts = false,
  shotReferences,
  promptPreviews,
  projectReady,
  uploadingKey,
  mode,
  onRequestRevised,
  onUpload,
}: {
  shotId: string
  slot: DirectorShotAssetSlot
  durationSec?: number
  projectId?: string
  hideMaterializedArtifacts?: boolean
  shotReferences: ExternalGenerationReference[]
  promptPreviews: Record<string, ShotPromptPreview>
  projectReady: boolean
  uploadingKey: string | null
  mode: ShotWorkspaceMode
  onRequestRevised?: (artifactId: string, request: ExternalGenerationRequestContent) => Promise<void> | void
  onUpload: (event: ChangeEvent<HTMLInputElement>, shotId: string, slot: DirectorShotAssetSlot, request?: ExternalGenerationRequestContent) => void
}) {
  const requestArtifacts = externalGenerationRequestArtifactsForSlot(slot)
  const requestArtifactIds = new Set(requestArtifacts.map((artifact) => artifact.id))
  const materializedArtifacts = slot.artifacts.filter((artifact) => !requestArtifactIds.has(artifact.id))
  const requests = requestArtifacts
    .map((artifact) => {
      const request = promptPreviews[artifact.id]?.request
      return request ? {
        artifact,
        request: mergeExternalGenerationTaskReferences(request, shotReferences) as ExternalGenerationRequestContent,
      } : null
    })
    .filter((entry): entry is { artifact: DirectorArtifactRecord; request: ExternalGenerationRequestContent } => Boolean(entry))
  const copyValue = requests.length ? '' : slot.artifacts.map(artifactToCopyText).join('\n\n')
  const canUpload = Boolean(slot.uploadKind)
  const accept = slot.uploadKind === 'video' ? 'video/*' : 'image/*'
  const manualUploadKey = `${shotId}-${slot.kind}-manual`
  const slotStatusLabel = slot.kind === 'prompt' ? '可复制' : slot.status === 'review' && canUpload ? '可回填' : undefined

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
        {slot.status === 'valid' && !slotStatusLabel ? null : <StatusBadge status={slot.status} label={slotStatusLabel} />}
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
        {slot.kind === 'prompt' && requests.length > 0 ? (
          <p className="rounded-lg bg-green-50 p-3 text-xs leading-5 text-green-700 ring-1 ring-green-100">
            已整理该 shot 的生成任务。展开下方素材卡，查看参考图，必要时编辑提示词后再复制使用。
          </p>
        ) : null}
        {requests.map((entry, index) => (
          <ShotRequestSummaryCard
            key={`${entry.artifact.id}-${entry.request.requestId}`}
            shotId={shotId}
            slot={slot}
            request={entry.request}
            sequence={index + 1}
            artifactId={entry.artifact.id}
            mode={mode}
            projectId={projectId}
            projectReady={projectReady}
            allowUpload={slot.kind !== 'prompt'}
            uploading={uploadingKey === `${shotId}-${slot.kind}-${entry.request.requestId}`}
            onRequestRevised={onRequestRevised}
            onUpload={onUpload}
          />
        ))}
        {requestArtifacts.map((artifact) => {
          const preview = promptPreviews[artifact.id]
          if (!preview || preview.request) return null
          return (
            <div key={artifact.id} className="rounded-lg bg-background-card p-3 text-xs text-ink-muted ring-1 ring-line">
              {preview.loading ? '正在读取外部生成请求...' : preview.error || '生成请求暂不可读'}
            </div>
          )
        })}
        {hideMaterializedArtifacts && materializedArtifacts.length > 0 ? (
          <p className="rounded-lg bg-green-50 p-3 text-xs leading-5 text-green-700 ring-1 ring-green-100">已生成的 HyperFrames / 字幕产物已在本步骤上方展示，可直接播放或按时间轴校对。</p>
        ) : slot.kind === 'overlay' && materializedArtifacts.length > 0 ? (
          <HyperFramesOutputPreview artifacts={materializedArtifacts} mode={mode} projectId={projectId} durationSec={durationSec} />
        ) : materializedArtifacts.map((artifact) => (
          <ShotArtifactPreview key={artifact.id} artifact={artifact} mode={mode} projectId={projectId} />
        ))}
        {slot.kind !== 'prompt' && !slot.artifacts.length ? (
          <p className="rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">这里还没有素材。可以先在本地或外部网页生成，再用上方按钮上传到这个 shot。</p>
        ) : null}
      </div>
    </div>
  )
}

function ExternalReferenceCard({ reference, index, mode, projectId }: { reference: ExternalGenerationReference; index: number; mode: ShotWorkspaceMode; projectId?: string }) {
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)

  useEffect(() => {
    let cancelled = false
    let objectUrl: string | null = null
    setPreviewError(null)
    setPreviewLoading(false)

    const directHref = directMediaPreviewUrl(reference.storageRef)
    if (directHref) {
      setPreviewUrl(directHref)
      return undefined
    }

    setPreviewUrl(null)
    const localArtifactId = localArtifactIdFromStorageRef(reference.storageRef)
    if (!projectId || !localArtifactId) return undefined

    setPreviewLoading(true)
    fetchLocalArtifactFile({ projectId, id: localArtifactId })
      .then((localArtifact) => {
        if (cancelled) return
        const blob = localArtifactFileToBlob(localArtifact)
        objectUrl = URL.createObjectURL(blob)
        setPreviewUrl(objectUrl)
      })
      .catch((err) => {
        if (!cancelled) setPreviewError(normalizeDirectorErrorMessage(err))
      })
      .finally(() => {
        if (!cancelled) setPreviewLoading(false)
      })

    return () => {
      cancelled = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [projectId, reference.storageRef])

  const locks = stringListField(reference.locks) || []
  const referenceCopy = referenceCopyTextForUser(reference, index)
  const title = reference.label || reference.id || `参考图 ${index}`
  return (
    <div className="min-w-0 rounded-lg bg-white p-3 ring-1 ring-line">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-xs font-black text-ink">{title}</div>
          <div className="mt-1 text-[11px] text-ink-muted">{reference.role || 'reference'}</div>
        </div>
        <CopyButton value={referenceCopy} label="复制参考信息" />
      </div>
      <div className="mt-3 overflow-hidden rounded-lg bg-background-card ring-1 ring-line">
        {previewUrl ? (
          <button
            type="button"
            onClick={() => setDialogOpen(true)}
            className="group relative block w-full cursor-zoom-in bg-white text-left focus:outline-none focus:ring-2 focus:ring-primary"
            aria-label={`放大预览 ${title}`}
          >
            <img
              src={previewUrl}
              alt={title}
              className="h-36 w-full object-contain"
              onError={() => {
                setPreviewError('当前无法预览这张参考图。')
                setPreviewUrl(null)
              }}
            />
            <span className="pointer-events-none absolute bottom-2 right-2 inline-flex items-center gap-1.5 rounded-lg bg-ink/80 px-2.5 py-1.5 text-[11px] font-black text-white opacity-0 transition group-hover:opacity-100 group-focus:opacity-100">
              <FiSearch /> 点击放大
            </span>
          </button>
        ) : (
          <div className="flex h-28 items-center justify-center px-3 text-center text-xs leading-5 text-ink-muted">
            {previewLoading ? '正在读取参考图...' : previewError ? '当前无法预览这张参考图。' : '选择或上传参考图后会显示预览。'}
          </div>
        )}
      </div>
      {locks.length ? (
        <div className="mt-2 line-clamp-2 text-[11px] text-ink-muted">锁定：{locks.join('、')}</div>
      ) : null}
      {dialogOpen && previewUrl ? (
        <ImagePreviewDialog
          imageUrl={previewUrl}
          title={title}
          sourceLabel={reference.role || referenceDependencyLabel(reference)}
          mode={mode}
          locks={locks}
          onClose={() => setDialogOpen(false)}
        />
      ) : null}
    </div>
  )
}

function ImagePreviewDialog({
  imageUrl,
  title,
  sourceLabel,
  mode,
  locks = [],
  onClose,
}: {
  imageUrl: string
  title: string
  sourceLabel: string
  mode: ShotWorkspaceMode
  locks?: string[]
  onClose: () => void
}) {
  const [instruction, setInstruction] = useState('')
  const [draft, setDraft] = useState('')
  const instructionFieldId = useMemo(() => `image-revision-${safeLocalUploadId(title).slice(0, 48)}`, [title])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  const buildDraft = () => {
    setDraft(buildImageRegenerationInstruction({
      mode,
      title,
      sourceLabel,
      userInstruction: instruction,
      locks,
    }))
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink/72 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label={`${title} 图片预览和对话修改`}
      onMouseDown={onClose}
    >
      <div
        className="grid max-h-[92vh] w-[min(1180px,calc(100vw-24px))] overflow-hidden rounded-lg bg-background p-3 shadow-card ring-1 ring-white/20 lg:grid-cols-[minmax(0,1fr)_360px]"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex min-h-[360px] items-center justify-center overflow-hidden rounded-lg bg-black/90">
          <img src={imageUrl} alt={title} className="max-h-[86vh] w-full object-contain" />
        </div>
        <aside className="flex min-h-0 flex-col gap-3 overflow-y-auto p-3">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="text-xs font-black text-primary-dark">图片对话修改</div>
              <h3 className="mt-1 truncate text-lg font-black text-ink" title={title}>{title}</h3>
              <p className="mt-1 text-xs leading-5 text-ink-muted">{sourceLabel}</p>
            </div>
            <button type="button" onClick={onClose} className="rounded-lg p-2 text-ink-soft hover:bg-white hover:text-primary-dark" aria-label="关闭图片预览">
              <FiX />
            </button>
          </div>
          {locks.length ? (
            <div className="rounded-lg bg-white p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">
              <div className="font-black text-ink">保持不变</div>
              <div className="mt-1">{locks.slice(0, 6).join('、')}</div>
            </div>
          ) : null}
          <div className="rounded-lg bg-white p-3 ring-1 ring-line">
            <label className="text-xs font-black text-ink" htmlFor={instructionFieldId}>你想怎么改</label>
            <textarea
              id={instructionFieldId}
              value={instruction}
              onChange={(event) => setInstruction(event.target.value)}
              placeholder={mode === 'aigc_shot' ? '例如：范进更瘦弱，眼神更怯懦，堂屋光线更压抑，但服装和场景不要变。' : '例如：右侧留出字幕区，动作更轻松，画面不要出现文字。'}
              className="mt-2 h-24 w-full resize-none rounded-lg border border-line bg-background-card p-3 text-xs leading-5 text-ink outline-none focus:border-primary"
            />
            <div className="mt-3 flex flex-wrap gap-2">
              <button type="button" onClick={() => setInstruction(mode === 'aigc_shot' ? '保持人物、场景和道具一致，只增强镜头情绪和画面细节。' : '保持服务口播，画面更轻松，保留字幕安全区。')} className="rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft">
                保持一致
              </button>
              <button type="button" onClick={() => setInstruction(mode === 'aigc_shot' ? '重新安排构图和景别，但不要改变角色身份、服装和道具位置。' : '调整构图，右侧或下方留出 30% 干净空间。')} className="rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft">
                调整构图
              </button>
              <button type="button" onClick={() => setInstruction(mode === 'aigc_shot' ? '加强情绪细节、光线和空气质感，让画面更有电影感。' : '增强反差动作和轻松情绪，但不要出现可读文字。')} className="rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft">
                增强情绪
              </button>
            </div>
            <button type="button" onClick={buildDraft} className="mt-3 inline-flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-3 py-2 text-sm font-black text-white shadow-sm">
              <FiRefreshCw /> 按要求生成返工提示词
            </button>
          </div>
          <div className="rounded-lg bg-white p-3 ring-1 ring-line">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-xs font-black text-ink">返工提示词</div>
                <p className="mt-1 text-[11px] leading-5 text-ink-muted">复制到图片 / 视频生成工具，生成后在当前步骤上传回填。</p>
              </div>
              {draft ? <CopyButton value={draft} label="复制" /> : null}
            </div>
            <textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder="点击上方按钮后，这里会生成可编辑的返工提示词。"
              className="mt-3 min-h-40 w-full resize-y rounded-lg border border-line bg-background-card p-3 text-xs leading-5 text-ink outline-none focus:border-primary"
            />
          </div>
        </aside>
      </div>
    </div>
  )
}

function artifactImageLocks(artifact: DirectorArtifactRecord): string[] {
  const metadata = artifact.metadata || {}
  const fields = [
    metadata.locks,
    metadata.lockedDimensions,
    metadata.invariants,
    metadata.mustPreserve,
    metadata.preserve,
    metadata.referenceLocks,
  ]
  for (const field of fields) {
    const values = stringListField(field)
    if (values?.length) return values
  }
  return []
}

function VideoPreviewDialog({
  videoUrl,
  title,
  onClose,
}: {
  videoUrl: string
  title: string
  onClose: () => void
}) {
  const videoRef = useRef<HTMLVideoElement | null>(null)

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink/82 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label={`${title} 放大播放`}
      onMouseDown={onClose}
    >
      <div
        className="w-[min(1280px,calc(100vw-24px))] overflow-hidden rounded-lg bg-black shadow-card ring-1 ring-white/20"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between gap-3 bg-background px-4 py-3">
          <div className="min-w-0">
            <div className="text-[11px] font-black text-primary-dark">放大播放</div>
            <h3 className="truncate text-sm font-black text-ink" title={title}>{title}</h3>
          </div>
          <button type="button" onClick={onClose} className="rounded-lg p-2 text-ink-soft hover:bg-white hover:text-primary-dark" aria-label="关闭视频播放">
            <FiX />
          </button>
        </div>
        <video
          ref={videoRef}
          src={videoUrl}
          controls
          autoPlay
          className="max-h-[82vh] w-full bg-black object-contain"
        />
      </div>
    </div>
  )
}

function ShotArtifactPreview({
  artifact,
  mode = 'aigc_shot',
  projectId,
  subtitleFallbackCues = [],
}: {
  artifact: DirectorArtifactRecord
  mode?: ShotWorkspaceMode
  projectId?: string
  subtitleFallbackCues?: SubtitleCue[]
}) {
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const previewObjectUrlRef = useRef<string | null>(null)
  const mediaKind = artifactPreviewKind(artifact)
  const readableKind = readableArtifactKind(artifact)
  const textPreview = useArtifactTextPreview(artifact, projectId, Boolean(readableKind))
  const subtitleCues = useMemo(() => readableKind === 'subtitle' ? parseSubtitleCues(textPreview.text || '') : [], [readableKind, textPreview.text])
  const assetSummary = artifactAssetSummary(mediaKind, readableKind, {
    previewReady: Boolean(previewUrl),
    previewLoading,
    previewUnavailable: Boolean(previewError),
  })
  const revokePreviewObjectUrl = useCallback(() => {
    if (!previewObjectUrlRef.current) return
    URL.revokeObjectURL(previewObjectUrlRef.current)
    previewObjectUrlRef.current = null
  }, [])
  useEffect(() => () => revokePreviewObjectUrl(), [revokePreviewObjectUrl])

  useEffect(() => {
    let cancelled = false
    revokePreviewObjectUrl()
    setPreviewError(null)
    setPreviewLoading(false)

    if (!mediaKind) {
      setPreviewUrl(null)
      return undefined
    }

    const directHref = directMediaPreviewUrl(artifact.storageRef || '')
    if (directHref) {
      setPreviewUrl(directHref)
      return undefined
    }

    setPreviewUrl(null)
    const localArtifactId = localArtifactIdFromStorageRef(artifact.storageRef)
    if (!projectId || !localArtifactId) return undefined

    if (mediaKind === 'video') {
      setPreviewUrl(localArtifactRawUrl({ projectId, id: localArtifactId }))
      return undefined
    }

    setPreviewLoading(true)
    fetchLocalArtifactFile({ projectId, id: localArtifactId })
      .then((localArtifact) => {
        if (cancelled) return
        const blob = localArtifactFileToBlob(localArtifact, artifact.metadata)
        const objectUrl = URL.createObjectURL(blob)
        previewObjectUrlRef.current = objectUrl
        setPreviewUrl(objectUrl)
      })
      .catch((err) => {
        if (!cancelled) setPreviewError(normalizeDirectorErrorMessage(err))
      })
      .finally(() => {
        if (!cancelled) setPreviewLoading(false)
      })

    return () => {
      cancelled = true
      revokePreviewObjectUrl()
    }
  }, [artifact.id, artifact.metadata, artifact.storageRef, mediaKind, projectId, revokePreviewObjectUrl])

  const loadVideoBlobFallback = useCallback(async () => {
    const localArtifactId = localArtifactIdFromStorageRef(artifact.storageRef)
    if (!projectId || !localArtifactId) {
      setPreviewUrl(null)
      setPreviewError('当前无法播放这个视频。')
      return
    }
    setPreviewLoading(true)
    setPreviewError(null)
    try {
      const localArtifact = await fetchLocalArtifactFile({ projectId, id: localArtifactId })
      const blob = localArtifactFileToBlob(localArtifact, artifact.metadata)
      const objectUrl = URL.createObjectURL(blob)
      revokePreviewObjectUrl()
      previewObjectUrlRef.current = objectUrl
      setPreviewUrl(objectUrl)
    } catch (err) {
      setPreviewUrl(null)
      setPreviewError(normalizeDirectorErrorMessage(err) || '当前无法播放这个视频。')
    } finally {
      setPreviewLoading(false)
    }
  }, [artifact.metadata, artifact.storageRef, projectId, revokePreviewObjectUrl])

  return (
    <div className="min-w-0 rounded-lg bg-background-card p-3 ring-1 ring-line">
      <div className="flex items-center justify-between gap-2">
        <div className="truncate text-xs font-black text-ink" title={artifact.name}>{artifact.name}</div>
        {artifact.status === 'valid' ? null : <StatusBadge status={artifact.status} />}
      </div>
      {mediaKind ? (
        <div className="mt-3 overflow-hidden rounded-lg bg-white ring-1 ring-line">
          {previewUrl && mediaKind === 'image' ? (
            <button
              type="button"
              onClick={() => setDialogOpen(true)}
              className="group relative block w-full cursor-zoom-in bg-white text-left focus:outline-none focus:ring-2 focus:ring-primary"
              aria-label={`放大预览 ${artifact.name}`}
            >
              <img src={previewUrl} alt={artifact.name} className="h-40 w-full object-contain" />
              <span className="pointer-events-none absolute bottom-2 right-2 inline-flex items-center gap-1.5 rounded-lg bg-ink/80 px-2.5 py-1.5 text-[11px] font-black text-white opacity-0 transition group-hover:opacity-100 group-focus:opacity-100">
                <FiSearch /> 点击放大
              </span>
            </button>
          ) : previewUrl && mediaKind === 'video' ? (
            <div className="relative bg-black">
              <video
                src={previewUrl}
                controls
                className="h-44 w-full bg-black object-contain"
                onLoadedMetadata={() => setPreviewError(null)}
                onError={() => {
                  if (previewUrl && !previewUrl.startsWith('blob:')) {
                    void loadVideoBlobFallback()
                    return
                  }
                  setPreviewUrl(null)
                  setPreviewError('当前无法播放这个视频。可以刷新后重试，或在导出页查看本地文件。')
                }}
              />
              <button
                type="button"
                onClick={() => setDialogOpen(true)}
                className="absolute bottom-2 right-2 inline-flex items-center gap-1.5 rounded-lg bg-ink/80 px-2.5 py-1.5 text-[11px] font-black text-white shadow-sm ring-1 ring-white/15 hover:bg-primary focus:outline-none focus:ring-2 focus:ring-white"
                aria-label={`放大播放 ${artifact.name}`}
              >
                <FiPlayCircle /> 放大播放
              </button>
            </div>
          ) : (
            <MediaPreviewPlaceholder mediaKind={mediaKind} loading={previewLoading} unavailable={Boolean(previewError)} />
          )}
        </div>
      ) : null}
      {readableKind === 'subtitle' ? (
        <SubtitleCuePanel
          cues={subtitleCues}
          fallbackCues={subtitleFallbackCues}
          rawText={textPreview.text}
          loading={textPreview.loading}
          error={textPreview.error}
        />
      ) : readableKind ? (
        <ReadableArtifactTextPanel
          text={textPreview.text}
          loading={textPreview.loading}
          error={textPreview.error}
        />
      ) : null}
      {assetSummary ? (
        <div className="mt-2 rounded-lg bg-white px-3 py-2 text-[11px] font-semibold text-ink-muted ring-1 ring-line">
          {assetSummary}
        </div>
      ) : null}
      {dialogOpen && previewUrl && mediaKind === 'image' ? (
        <ImagePreviewDialog
          imageUrl={previewUrl}
          title={artifact.name || displayNameForArtifact(artifact.kind)}
          sourceLabel={displayNameForArtifact(artifact.kind)}
          mode={mode}
          locks={artifactImageLocks(artifact)}
          onClose={() => setDialogOpen(false)}
        />
      ) : null}
      {dialogOpen && previewUrl && mediaKind === 'video' ? (
        <VideoPreviewDialog
          videoUrl={previewUrl}
          title={artifact.name || displayNameForArtifact(artifact.kind)}
          onClose={() => setDialogOpen(false)}
        />
      ) : null}
    </div>
  )
}

function MediaPreviewPlaceholder({
  mediaKind,
  loading,
  unavailable,
}: {
  mediaKind: 'image' | 'video'
  loading: boolean
  unavailable: boolean
}) {
  const Icon = mediaKind === 'video' ? FiVideo : FiFileText
  const loadingText = mediaKind === 'video' ? '正在准备视频预览...' : '正在准备图片预览...'
  const title = mediaKind === 'video' ? '视频预览暂不可用' : '图片预览暂不可用'
  const detail = mediaKind === 'video'
    ? '当前页面还没有拿到可播放预览。可以刷新，或在最终预览 / 导出阶段查看。'
    : '当前页面还没有拿到可显示预览。可以刷新或重新上传。'
  if (loading) {
    return (
      <div className="flex h-32 items-center justify-center px-3 text-center text-xs leading-5 text-ink-muted">
        {loadingText}
      </div>
    )
  }
  return (
    <div className="flex h-32 items-center justify-center px-4 text-center">
      <div>
        <div className="mx-auto grid h-9 w-9 place-items-center rounded-lg bg-background-card text-primary-dark ring-1 ring-line">
          <Icon />
        </div>
        <div className="mt-2 text-xs font-black text-ink">{unavailable ? title : '等待预览'}</div>
        <p className="mt-1 max-w-sm text-xs leading-5 text-ink-muted">
          {unavailable ? detail : '页面拿到可预览内容后会直接展示。'}
        </p>
      </div>
    </div>
  )
}

interface ArtifactTextPreviewState {
  text: string
  loading: boolean
  error: string | null
}

interface SubtitleCue {
  start: string
  end: string
  text: string
}

function useArtifactTextPreview(artifact: DirectorArtifactRecord, projectId: string | undefined, enabled: boolean): ArtifactTextPreviewState {
  const [state, setState] = useState<ArtifactTextPreviewState>({ text: '', loading: false, error: null })
  const inlineText = useMemo(() => inlineArtifactTextFromFields(artifact.inlineJson, artifact.metadata), [artifact.inlineJson, artifact.metadata])
  const localArtifactId = useMemo(() => localArtifactIdFromStorageRef(artifact.storageRef), [artifact.storageRef])

  useEffect(() => {
    let cancelled = false
    if (!enabled) {
      setState({ text: '', loading: false, error: null })
      return undefined
    }

    if (inlineText) {
      setState({ text: inlineText, loading: false, error: null })
      return undefined
    }

    setState({ text: '', loading: true, error: null })

    const load = localArtifactId && projectId
      ? fetchLocalArtifactFile({ projectId, id: localArtifactId }).then((localArtifact) => localArtifactFileToText(localArtifact))
      : fetchArtifactContent(artifact.id).then((response) => artifactContentText(response.content))

    load
      .then((text) => {
        if (!cancelled) setState({ text, loading: false, error: null })
      })
      .catch((err) => {
        if (!cancelled) setState({ text: '', loading: false, error: normalizeDirectorErrorMessage(err) })
      })

    return () => {
      cancelled = true
    }
  }, [artifact.id, enabled, inlineText, localArtifactId, projectId])

  return state
}

function SubtitleCuePanel({
  cues,
  fallbackCues,
  rawText,
  loading,
  error,
}: {
  cues: SubtitleCue[]
  fallbackCues?: SubtitleCue[]
  rawText: string
  loading: boolean
  error: string | null
}) {
  const displayedCues = cues.length ? cues : (error && fallbackCues?.length ? fallbackCues : [])
  const copyText = displayedCues.length ? displayedCues.map((cue) => `${cue.start} - ${cue.end}\n${cue.text}`).join('\n\n') : rawText
  return (
    <div className="mt-3 rounded-lg bg-white p-3 ring-1 ring-line">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-xs font-black text-primary-dark">字幕时间轴</div>
          <div className="mt-1 text-[11px] text-ink-muted">{displayedCues.length ? `${displayedCues.length} 条字幕，可直接校对时间和文字。` : '读取字幕文件后会在这里按时间展示。'}</div>
        </div>
        {copyText ? <CopyButton value={copyText} label="复制字幕" /> : null}
      </div>
      {loading ? (
        <p className="mt-3 rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">正在读取字幕内容...</p>
      ) : error && !displayedCues.length ? (
        <p className="mt-3 rounded-lg bg-amber-50 p-3 text-xs leading-5 text-amber-800 ring-1 ring-amber-100">字幕正文暂不可预览。可以先校对口播、时间线和 HyperFrames 层。</p>
      ) : error && displayedCues.length ? (
        <div className="mt-3 rounded-lg bg-amber-50 p-3 text-xs leading-5 text-amber-800 ring-1 ring-amber-100">
          当前先显示口播稿校对视图，便于核对字幕节奏；读取到字幕正文后会自动替换。
        </div>
      ) : null}
      {!loading && displayedCues.length ? (
        <div className="mt-3 max-h-72 space-y-2 overflow-auto pr-1">
          {displayedCues.map((cue, index) => (
            <div key={`${cue.start}-${index}`} className="grid gap-3 rounded-lg bg-background-card p-3 ring-1 ring-line md:grid-cols-[132px_1fr]">
              <div className="font-mono text-[11px] font-black text-primary-dark">{cue.start} - {cue.end}</div>
              <div className="whitespace-pre-wrap text-xs leading-5 text-ink">{cue.text}</div>
            </div>
          ))}
        </div>
      ) : !loading && !error && rawText ? (
        <pre className="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-background-card p-3 text-xs leading-5 text-ink ring-1 ring-line">{rawText}</pre>
      ) : !loading && !error ? (
        <p className="mt-3 rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">当前没有读取到可展示字幕。</p>
      ) : null}
    </div>
  )
}

function ReadableArtifactTextPanel({
  text,
  loading,
  error,
}: {
  text: string
  loading: boolean
  error: string | null
}) {
  return (
    <div className="mt-3 rounded-lg bg-white p-3 ring-1 ring-line">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-xs font-black text-primary-dark">文本内容</div>
          <div className="mt-1 text-[11px] text-ink-muted">可直接阅读、复制和校对。</div>
        </div>
        {text ? <CopyButton value={text} label="复制正文" /> : null}
      </div>
      {loading ? (
        <p className="mt-3 rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">正在读取文本内容...</p>
      ) : error ? (
        <p className="mt-3 rounded-lg bg-red-50 p-3 text-xs font-semibold leading-5 text-red-600 ring-1 ring-red-100">{error}</p>
      ) : text ? (
        <pre className="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-background-card p-3 text-xs leading-5 text-ink ring-1 ring-line">{text}</pre>
      ) : (
        <p className="mt-3 rounded-lg bg-background-card p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">当前没有读取到可展示正文。</p>
      )}
    </div>
  )
}

function artifactPreviewKind(artifact: DirectorArtifactRecord): 'image' | 'video' | '' {
  const metadata = artifact.metadata || {}
  const mimeType = stringField(metadata.mimeType) || stringField(metadata.mime_type)
  const searchable = [
    artifact.kind,
    artifact.name,
    artifact.storageRef,
    stringField(metadata.artifactType),
    stringField(metadata.artifact_kind),
    stringField(metadata.generationKind),
    stringField(metadata.assetType),
    mimeType,
  ].join(' ').toLowerCase()
  if (mimeType.startsWith('image/') || /\.(png|jpe?g|webp|gif)(?:\s|$)/u.test(searchable) || searchable.includes(' image') || searchable.includes('shot_keyframe')) return 'image'
  if (
    artifact.kind === 'HYPERFRAMES_SHOT' ||
    mimeType.startsWith('video/') ||
    /\.(mp4|mov|webm|m4v)(?:\s|$)/u.test(searchable) ||
    searchable.includes(' video') ||
    searchable.includes('shot_video') ||
    searchable.includes('hyperframes_shot')
  ) return 'video'
  return ''
}

function readableArtifactKind(artifact: DirectorArtifactRecord): 'subtitle' | 'text' | '' {
  if (isSubtitleArtifact(artifact)) return 'subtitle'
  const metadata = artifact.metadata || {}
  const mimeType = stringField(metadata.mimeType) || stringField(metadata.mime_type)
  const searchable = [
    artifact.kind,
    artifact.name,
    artifact.storageRef,
    stringField(metadata.artifactType),
    stringField(metadata.artifact_kind),
    mimeType,
  ].join(' ').toLowerCase()
  if (mimeType.startsWith('text/') || /\.(txt|md|json|html|css)$/u.test(searchable) || searchable.includes('text_overlay') || searchable.includes('html_overlay')) return 'text'
  return ''
}

function isSubtitleArtifact(artifact: DirectorArtifactRecord): boolean {
  const metadata = artifact.metadata || {}
  const searchable = [
    artifact.kind,
    artifact.name,
    artifact.storageRef,
    stringField(metadata.artifactType),
    stringField(metadata.artifact_kind),
    stringField(metadata.mimeType),
    stringField(metadata.mime_type),
  ].join(' ').toLowerCase()
  return artifact.kind === 'SHOT_SUBTITLE' ||
    searchable.includes('shot_subtitle') ||
    /\.(srt|vtt)$/u.test(searchable) ||
    searchable.includes('subtitle')
}

function isHyperFramesArtifact(artifact: DirectorArtifactRecord): boolean {
  const metadata = artifact.metadata || {}
  const searchable = [
    artifact.kind,
    artifact.name,
    artifact.storageRef,
    stringField(metadata.artifactType),
    stringField(metadata.artifact_kind),
    stringField(metadata.generationKind),
    stringField(metadata.assetType),
    stringField(metadata.description),
  ].join(' ').toLowerCase()
  return artifact.kind === 'HYPERFRAMES_SHOT' ||
    searchable.includes('hyperframes') ||
    searchable.includes('html_overlay') ||
    searchable.includes('exact_text_overlay') ||
    searchable.includes('text_overlay')
}

function inlineArtifactTextFromFields(inlineJsonValue: string | undefined, metadataValue: Record<string, unknown> | undefined): string {
  const metadata = metadataValue || {}
  const inlineJson = inlineJsonValue ? artifactContentText(parseMaybeJSON(inlineJsonValue)) : ''
  if (inlineJson) return inlineJson
  const inlineContent = metadata.inlineContent ? artifactContentText(metadata.inlineContent) : ''
  if (inlineContent) return inlineContent
  const content = metadata.content ? artifactContentText(metadata.content) : ''
  if (content) return content
  return ''
}

function parseSubtitleCues(text: string): SubtitleCue[] {
  const normalized = text.replace(/\r\n?/gu, '\n').replace(/^\uFEFF/u, '').trim()
  if (!normalized) return []
  const body = normalized.replace(/^WEBVTT[^\n]*\n+/iu, '')
  return body
    .split(/\n{2,}/u)
    .map((block) => {
      const lines = block.split('\n').map((line) => line.trim()).filter(Boolean)
      if (lines[0] && /^\d+$/u.test(lines[0])) lines.shift()
      const timingIndex = lines.findIndex((line) => line.includes('-->'))
      if (timingIndex < 0) return null
      const [startRaw, endRaw = ''] = lines[timingIndex].split('-->')
      const textLines = lines.slice(timingIndex + 1).filter((line) => !/^(NOTE|STYLE|REGION)\b/iu.test(line))
      const cueText = textLines.join('\n').trim()
      if (!cueText) return null
      return {
        start: compactSubtitleTime(startRaw),
        end: compactSubtitleTime(endRaw),
        text: cueText,
      }
    })
    .filter((cue): cue is SubtitleCue => Boolean(cue))
}

function compactSubtitleTime(value: string): string {
  const clean = value.trim().split(/\s+/u)[0]?.replace(',', '.') || ''
  const match = clean.match(/^(?:(\d{2}):)?(\d{2}):(\d{2})(?:\.(\d{1,3}))?$/u)
  if (!match) return clean
  const [, hours, minutes, seconds, ms] = match
  const prefix = hours && hours !== '00' ? `${Number(hours)}:` : ''
  const suffix = ms ? `.${ms.padEnd(3, '0').slice(0, 3)}` : ''
  return `${prefix}${minutes}:${seconds}${suffix}`
}

function artifactAssetSummary(
  mediaKind: 'image' | 'video' | '',
  readableKind: 'subtitle' | 'text' | '' = '',
  previewState?: { previewReady: boolean; previewLoading: boolean; previewUnavailable: boolean },
): string {
  if (mediaKind === 'image') {
    if (previewState?.previewLoading) return '正在准备图片预览。'
    if (previewState?.previewUnavailable) return '当前无法预览图片。'
    return ''
  }
  if (mediaKind === 'video') {
    if (previewState?.previewLoading) return '正在准备视频预览。'
    if (previewState?.previewUnavailable) return '当前无法播放视频。'
    return ''
  }
  if (readableKind === 'subtitle' || readableKind === 'text') return ''
  return ''
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
  const videoPreviewObjectUrlRef = useRef<string | null>(null)
  const [videoPreviewUrl, setVideoPreviewUrl] = useState<string | null>(null)
  const [videoPreviewLoading, setVideoPreviewLoading] = useState(false)
  const [videoPreviewError, setVideoPreviewError] = useState<string | null>(null)
  const [videoLocalPath, setVideoLocalPath] = useState<string | null>(null)
  const [publishContent, setPublishContent] = useState<unknown>(null)
  const [publishLoading, setPublishLoading] = useState(false)
  const [publishError, setPublishError] = useState<string | null>(null)
  const [diagnosticsPath, setDiagnosticsPath] = useState<string | null>(null)
  const [diagnosticsLoading, setDiagnosticsLoading] = useState(false)
  const [diagnosticsError, setDiagnosticsError] = useState<string | null>(null)
  const videoReady = video?.status === 'valid' && Boolean(video.storageRef)
  const packageReady = packageArtifact?.status === 'valid' && Boolean(packageArtifact.storageRef)
  const videoStorageRef = video?.storageRef || ''
  const revokeVideoPreviewObjectUrl = useCallback(() => {
    if (!videoPreviewObjectUrlRef.current) return
    URL.revokeObjectURL(videoPreviewObjectUrlRef.current)
    videoPreviewObjectUrlRef.current = null
  }, [])
  useEffect(() => () => revokeVideoPreviewObjectUrl(), [revokeVideoPreviewObjectUrl])
  useEffect(() => {
    let cancelled = false
    revokeVideoPreviewObjectUrl()
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
      setVideoPreviewError('最终视频暂时无法读取。')
      return undefined
    }
    if (!projectId) {
      setVideoPreviewError('缺少项目 ID，无法读取本地视频文件。')
      return undefined
    }

    setVideoPreviewUrl(localArtifactRawUrl({ projectId, id: localArtifactId }))
    fetchLocalArtifactFile({ projectId, id: localArtifactId })
      .then((localArtifact) => {
        if (!cancelled) setVideoLocalPath(localArtifact.path || null)
      })
      .catch(() => {
        if (!cancelled) setVideoLocalPath(null)
      })

    return () => {
      cancelled = true
      revokeVideoPreviewObjectUrl()
    }
  }, [projectId, revokeVideoPreviewObjectUrl, video?.id, video?.metadata, videoReady, videoStorageRef])

  const loadFinalVideoBlobFallback = useCallback(async () => {
    const localArtifactId = localArtifactIdFromStorageRef(videoStorageRef)
    if (!projectId || !localArtifactId) {
      setVideoPreviewUrl(null)
      setVideoPreviewError('最终视频已生成，但当前页面还没有拿到可播放文件。')
      return
    }
    setVideoPreviewLoading(true)
    setVideoPreviewError(null)
    try {
      const localArtifact = await fetchLocalArtifactFile({ projectId, id: localArtifactId })
      const blob = localArtifactFileToBlob(localArtifact, video?.metadata)
      const objectUrl = URL.createObjectURL(blob)
      revokeVideoPreviewObjectUrl()
      videoPreviewObjectUrlRef.current = objectUrl
      setVideoLocalPath(localArtifact.path || null)
      setVideoPreviewUrl(objectUrl)
    } catch (err) {
      setVideoPreviewUrl(null)
      setVideoPreviewError(normalizeDirectorErrorMessage(err) || '最终视频已生成，但当前页面暂时无法播放。')
    } finally {
      setVideoPreviewLoading(false)
    }
  }, [projectId, revokeVideoPreviewObjectUrl, video?.metadata, videoStorageRef])
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
      setVideoPreviewError('当前环境无法直接打开本地文件夹，请在桌面端使用“打开文件夹”。')
      return
    }
    const opened = await openLocalPath(videoLocalPath)
    if (!opened) setVideoPreviewError('当前浏览器环境不能打开本地文件夹，请在桌面端使用该功能。')
  }
  const exportDiagnostics = async () => {
    setDiagnosticsLoading(true)
    setDiagnosticsError(null)
    try {
      const result = await createLocalDiagnostics(`closed-beta-export:${projectId || 'no-project'}`)
      setDiagnosticsPath(result.path)
    } catch (err) {
      setDiagnosticsError(normalizeDirectorErrorMessage(err))
    } finally {
      setDiagnosticsLoading(false)
    }
  }
  const openDiagnostics = async () => {
    if (!diagnosticsPath) return
    const opened = await openLocalPath(diagnosticsPath)
    if (!opened) setDiagnosticsError('当前浏览器环境不能打开诊断包，请在桌面端使用该功能。')
  }
  return (
    <div className="space-y-5">
      <div className="grid grid-cols-1 gap-5 xl:grid-cols-12">
      <section className="card p-6 xl:col-span-7">
        <div className="flex items-center justify-between"><div><p className="text-sm font-bold text-primary-dark">最终预览 / 导出</p><h2 className="mt-2 text-2xl font-black text-ink">最终视频预览</h2></div><StatusBadge status={videoReady ? (videoPreviewError ? 'review' : 'valid') : 'pending'} label={videoReady ? (videoPreviewError ? '待读取' : 'final.mp4 已生成') : '等待渲染'} /></div>
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
              onLoadedMetadata={() => setVideoPreviewError(null)}
              onError={() => {
                if (videoPreviewUrl && !videoPreviewUrl.startsWith('blob:')) {
                  void loadFinalVideoBlobFallback()
                  return
                }
                setVideoPreviewUrl(null)
                setVideoPreviewError('最终视频已生成，但当前页面暂时无法播放。可以打开文件夹查看本地视频。')
              }}
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
          <h3 className="text-lg font-black text-ink">诊断包</h3>
          <p className="mt-2 text-sm leading-6 text-ink-muted">导出 beta-diagnostics.zip，包含脱敏环境、日志、MCP 状态、artifact manifest 和 QA 报告索引。</p>
          {diagnosticsError ? <div className="mt-3 rounded-lg bg-red-50 p-3 text-xs font-semibold text-red-700 ring-1 ring-red-200">{diagnosticsError}</div> : null}
          {diagnosticsPath ? <div className="mt-3 truncate rounded-lg bg-background-card p-3 font-mono text-[11px] text-ink-soft ring-1 ring-line" title={diagnosticsPath}>{diagnosticsPath}</div> : null}
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <button disabled={diagnosticsLoading} onClick={() => { void exportDiagnostics() }} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"><FiArchive /> {diagnosticsLoading ? '导出中...' : '导出诊断包'}</button>
            <button disabled={!diagnosticsPath} onClick={() => { void openDiagnostics() }} className="flex items-center justify-center gap-2 rounded-lg bg-white px-4 py-3 text-sm font-black text-primary-dark ring-1 ring-line disabled:cursor-not-allowed disabled:opacity-45"><FiFolder /> 打开诊断包</button>
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
          {item.storageRef ? <div className="mt-1 rounded bg-white px-2 py-1 text-[11px] font-semibold text-ink-muted ring-1 ring-line">{userFacingStorageStatus(item.storageRef, '本地产物')}</div> : null}
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

function localArtifactFileToText(localArtifact: LocalArtifactFileResponse): string {
  if (typeof localArtifact.content === 'string') return localArtifact.content
  if (localArtifact.contentBase64) {
    const binary = window.atob(localArtifact.contentBase64)
    const bytes = new Uint8Array(binary.length)
    for (let index = 0; index < binary.length; index += 1) {
      bytes[index] = binary.charCodeAt(index)
    }
    return new TextDecoder('utf-8').decode(bytes)
  }
  throw new Error('本地文本文件没有可读取内容')
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

function ArtifactTable({ artifacts, compact = false, projectId, mode = 'voice_visual', onArtifactsChanged }: { artifacts: DirectorArtifactRecord[]; compact?: boolean; projectId?: string; mode?: ShotWorkspaceMode; onArtifactsChanged?: () => Promise<void> | void }) {
  const [selectedId, setSelectedId] = useState<string | undefined>()
  const [content, setContent] = useState<unknown>(null)
  const [history, setHistory] = useState<Artifact[]>([])
  const [viewerLoading, setViewerLoading] = useState(false)
  const [viewerError, setViewerError] = useState<string | null>(null)
  const [revisionMessage, setRevisionMessage] = useState('')
  const [revisionLoading, setRevisionLoading] = useState(false)
  const [externalUploading, setExternalUploading] = useState(false)
  const [externalUploadMessage, setExternalUploadMessage] = useState<string | null>(null)
  const [contentSelection, setContentSelection] = useState<TextSelectionDraft | null>(null)
  const [selectionRevisionLoading, setSelectionRevisionLoading] = useState(false)
  const [selectionRevisionError, setSelectionRevisionError] = useState<string | null>(null)
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
    setContentSelection(null)
    setSelectionRevisionError(null)
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

  const submitSelectionRevision = async (instruction: string) => {
    if (!selected || !contentSelection || !isInspectableArtifact(selected)) return
    setSelectionRevisionLoading(true)
    setSelectionRevisionError(null)
    setViewerError(null)
    try {
      const clientModelProviders = await buildClientModelProvidersForRun()
      const revised = await reviseArtifact(
        selected.id,
        buildPartialRevisionInstruction(contentSelection.text, instruction, mode, selected.name || selected.kind),
        clientModelProviders as Record<string, unknown> | undefined,
      )
      setContent(revised.content)
      const nextHistory = await fetchArtifactHistory(selected.id)
      setHistory(nextHistory.history || [])
      setContentSelection(null)
      window.getSelection()?.removeAllRanges()
      await onArtifactsChanged?.()
    } catch (err) {
      setSelectionRevisionError(normalizeDirectorErrorMessage(err))
    } finally {
      setSelectionRevisionLoading(false)
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
      setExternalUploadMessage(`已上传 ${registered.artifact?.name || registered.artifact?.id || '外部生成结果'}`)
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
            const provenance = artifactProvenanceSummary(artifact)
            return (
              <Fragment key={artifact.id}>
                <tr>
                  <td className="whitespace-nowrap px-4 py-3 font-mono text-xs font-bold">{artifact.id}</td>
                  <td className="whitespace-nowrap px-4 py-3">
                    <div className="font-semibold text-ink">{artifact.name}</div>
                    {provenance ? <div className={clsx('mt-1 text-[11px] font-bold', provenance.isFallback ? 'text-amber-700' : 'text-ink-soft')}>{provenance.label}</div> : null}
                  </td>
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
                              <div className="mt-1 text-[11px] font-semibold text-ink-soft">{artifactDetailStatus(artifact)}</div>
                              {provenance ? (
                                <div className={clsx('mt-2 rounded-lg px-3 py-2 text-xs font-semibold ring-1', provenance.isFallback ? 'bg-amber-50 text-amber-800 ring-amber-200' : 'bg-background-card text-ink-muted ring-line')}>
                                  {provenance.detail}
                                </div>
                              ) : null}
                            </div>
                            {content !== null && content !== undefined ? <CopyButton value={artifactContentText(content)} label="复制正文" /> : null}
                          </div>
                          <div
                            className="relative mt-4 max-h-[420px] overflow-auto rounded-lg border border-line bg-background-card p-4 selection:bg-primary-soft"
                            onMouseUp={(event) => {
                              const nextSelection = textSelectionFromDocument(event.currentTarget, artifact.name || artifact.kind)
                              if (nextSelection) setContentSelection(nextSelection)
                            }}
                          >
                            {viewerLoading ? (
                              <p className="text-sm text-ink-muted">正在加载产物正文...</p>
                            ) : viewerError ? (
                              <p className="text-sm font-semibold text-red-600">{viewerError}</p>
                            ) : content !== null && content !== undefined ? (
                              <ReviewContent text={artifactContentText(content)} />
                            ) : (
                              <p className="text-sm text-ink-muted">暂无可展示正文。</p>
                            )}
                            {contentSelection ? (
                              <SelectionFloatingAssistant
                                selection={contentSelection}
                                mode={mode}
                                loading={selectionRevisionLoading}
                                error={selectionRevisionError}
                                actionLabel="AI 局部返工"
                                onClose={() => setContentSelection(null)}
                                onApply={submitSelectionRevision}
                              />
                            ) : null}
                          </div>
                          {externalRequest ? (
                            <ExternalGenerationRequestPanel
                              request={externalRequest}
                              projectId={projectId}
                              artifactId={selected?.id}
                              mode={mode}
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
                                  <div className="mt-1 truncate text-xs text-ink-muted">{formatArtifactHistoryLabel(item)}</div>
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
  locks?: unknown
}

interface ExternalGenerationLayerPlan {
  role?: string
  prompt?: string
  plan?: string
  textSafeLayout?: string
  mode?: string
  keyframeStrategy?: string
  textRenderer?: string
  locks?: string[]
  inputArtifacts?: string[]
  outputArtifactKind?: string
  avoidGeneratedText?: boolean
  requiresBlankArea?: boolean
}

interface ExternalGenerationRequestContent {
  requestId: string
  kind: 'image' | 'video'
  shotId?: string
  narrationText?: string
  sourceScriptSegment?: string
  visualText?: string
  prompt: string
  overallShotPrompt?: string
  negativePrompt?: string
  references: ExternalGenerationReference[]
  aigcPlan?: ExternalGenerationLayerPlan
  hyperframesPlan?: ExternalGenerationLayerPlan
  ffmpegFusionPlan?: ExternalGenerationLayerPlan
  textSafeLayout?: string
  target?: {
    aspectRatio?: string
    durationSec?: number
    resolution?: string
  }
  promptCharLimit?: number
  referenceImageLimit?: number
}

const EXTERNAL_PROMPT_MAX_CHARS = 2000

function ExternalGenerationRequestPanel({
  request,
  projectId,
  artifactId,
  mode,
  disabled,
  uploading,
  message,
  onUpload,
}: {
  request: ExternalGenerationRequestContent
  projectId?: string
  artifactId?: string
  mode: ShotWorkspaceMode
  disabled: boolean
  uploading: boolean
  message: string | null
  onUpload: (event: ChangeEvent<HTMLInputElement>, request: ExternalGenerationRequestContent) => void
}) {
  const accept = request.kind === 'image' ? 'image/*' : 'video/*'
  const [promptDraft, setPromptDraft] = useState(() => safePromptText(request.prompt))
  const [promptInstruction, setPromptInstruction] = useState('')
  const [referenceDrafts, setReferenceDrafts] = useState<ExternalGenerationReference[]>(() => cloneReferenceDrafts(request.references))
  const [approved, setApproved] = useState(false)
  const [promptSelection, setPromptSelection] = useState<TextSelectionDraft | null>(null)
  const [partialRevisionLoading, setPartialRevisionLoading] = useState(false)
  const [partialRevisionError, setPartialRevisionError] = useState<string | null>(null)

  useEffect(() => {
    setPromptDraft(safePromptText(request.prompt))
    setPromptInstruction('')
    setReferenceDrafts(cloneReferenceDrafts(request.references))
    setApproved(false)
    setPromptSelection(null)
    setPartialRevisionError(null)
  }, [request.references, request.requestId, request.prompt])

  const targetText = [
    request.target?.aspectRatio,
    request.target?.resolution,
    request.target?.durationSec ? `${request.target.durationSec}s` : '',
  ].filter(Boolean).join(' / ')
  const usableReferences = referenceDrafts.filter((ref) => ref.storageRef.trim())
  const editableRequest: ExternalGenerationRequestContent = { ...request, references: usableReferences }
  const safePrompt = promptDraft
  const referenceRows = requestReferenceRows(editableRequest)
  const promptSourceLabel = mode === 'aigc_shot' ? 'AIGC 镜头提示词' : 'AIGC 插入素材提示词'

  const applyPromptSelectionRewrite = async (instruction: string) => {
    if (!promptSelection) return
    const fallbackReplacement = buildSelectionRewriteText(promptSelection.text, instruction, mode)
    if (!artifactId) {
      if (promptSelection.start !== undefined && promptSelection.end !== undefined) {
        setPromptDraft((current) => replaceRange(current, promptSelection.start || 0, promptSelection.end || 0, fallbackReplacement))
      }
      setPromptSelection(null)
      return
    }
    setPartialRevisionLoading(true)
    setPartialRevisionError(null)
    try {
      const clientModelProviders = await buildClientModelProvidersForRun()
      const revised = await reviseArtifact(
        artifactId,
        buildPartialRevisionInstruction(promptSelection.text, instruction, mode, promptSourceLabel),
        clientModelProviders as Record<string, unknown> | undefined,
      )
      const nextRequest = externalGenerationRequestFromContent(revised.content)
      if (nextRequest) {
        setPromptDraft(safePromptText(nextRequest.prompt))
        setReferenceDrafts(cloneReferenceDrafts(nextRequest.references))
      } else if (promptSelection.start !== undefined && promptSelection.end !== undefined) {
        setPromptDraft((current) => replaceRange(current, promptSelection.start || 0, promptSelection.end || 0, fallbackReplacement))
      }
      setPromptSelection(null)
    } catch (err) {
      setPartialRevisionError(normalizeDirectorErrorMessage(err))
    } finally {
      setPartialRevisionLoading(false)
    }
  }

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
        当前依赖点等待用户提供素材。先确认提示词和参考图，生成后上传回填，系统会把结果关联到对应 shot。
      </p>
      {request.kind === 'video' ? <ShotLayerPlanPanel request={editableRequest} mode={mode} /> : null}
      <div className="mt-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div className="text-xs font-black text-ink-soft">素材操作 · {mode === 'aigc_shot' ? '镜头生成' : '插入生成'}</div>
            <div className="mt-1 text-[11px] text-ink-muted">划词局部修改提示词和参考图，复制到外部工具生成；确认后上传结果。</div>
          </div>
          <div className="flex flex-wrap gap-2">
            <CopyButton value={safePrompt} label="复制提示词" />
            <button
              type="button"
              onClick={() => setApproved(true)}
              disabled={approved}
              className={clsx(
                'inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-black ring-1',
                approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
              )}
            >
              <FiCheck /> {approved ? '已通过，内容已锁定' : '通过并锁定'}
            </button>
          </div>
        </div>
        <div className="mt-3 rounded-lg bg-white p-3 ring-1 ring-line">
          <div className="text-xs font-black text-ink-soft">优化要求</div>
          <textarea
            value={promptInstruction}
            disabled={disabled || approved}
            onChange={(event) => setPromptInstruction(event.target.value)}
            placeholder={mode === 'aigc_shot' ? '例如：加强镜头从中景推到近景，主角表情更压抑，保持道具位置。' : '例如：改成 2 秒 b-roll，背景更现代，右侧留给字幕。'}
            className="mt-2 min-h-20 w-full resize-y rounded-lg border border-line bg-background-card p-3 text-xs leading-5 text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
            aria-label={`${request.requestId} 提示词生成要求`}
          />
          <button
            type="button"
            disabled={disabled || approved}
            onClick={() => setPromptDraft(regeneratePromptDraft(request, referenceDrafts, promptInstruction, promptDraft, mode))}
            className={clsx(
              'mt-2 inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-black ring-1',
              disabled || approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
            )}
          >
            <FiRefreshCw /> 按要求重新生成提示词
          </button>
        </div>
        <div className="mt-3 text-xs font-black text-ink-soft">{promptSourceLabel}</div>
        <div className="relative">
          <textarea
            value={safePrompt}
            maxLength={EXTERNAL_PROMPT_MAX_CHARS}
            disabled={disabled || approved}
            onChange={(event) => setPromptDraft(safePromptText(event.target.value))}
            onSelect={(event) => setPromptSelection(textareaSelectionDraft(event.currentTarget, promptSourceLabel))}
            onMouseUp={(event) => setPromptSelection(textareaSelectionDraft(event.currentTarget, promptSourceLabel))}
            className="mt-3 min-h-36 w-full resize-y rounded-lg border border-line bg-white p-3 text-xs leading-5 text-ink outline-none selection:bg-primary-soft focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
            aria-label={`${request.requestId} 可编辑提示词`}
          />
          {promptSelection ? (
            <SelectionFloatingAssistant
              selection={promptSelection}
              mode={mode}
              loading={partialRevisionLoading}
              error={partialRevisionError}
              actionLabel={artifactId ? 'AI 局部返工' : '替换选中段'}
              onClose={() => setPromptSelection(null)}
              onApply={applyPromptSelectionRewrite}
            />
          ) : null}
        </div>
      </div>
      {request.negativePrompt ? (
        <details className="mt-3 rounded-lg border border-line bg-background-card p-3">
          <summary className="cursor-pointer text-xs font-black text-ink-soft">可选负面提示词</summary>
          <pre className="mt-2 max-h-32 whitespace-pre-wrap break-words text-xs leading-5 text-ink">{request.negativePrompt}</pre>
        </details>
      ) : null}
      <div className="mt-3 rounded-lg bg-background-card p-3 ring-1 ring-line">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div className="text-xs font-black text-ink-soft">参考图</div>
            <div className="mt-1 text-[11px] text-ink-muted">可修改名称、类型和路径；有路径时可在下方预览。</div>
          </div>
          <button
            type="button"
            disabled={disabled || approved}
            onClick={() => setReferenceDrafts((current) => [...current, newReferenceDraft(current.length + 1)])}
            className={clsx(
              'inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-black ring-1',
              disabled || approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
            )}
          >
            <FiUpload /> 新增参考图
          </button>
        </div>
        {referenceDrafts.length ? (
          <div className="mt-3 space-y-3">
            {referenceDrafts.map((ref, index) => (
              <div key={ref.id || `${request.requestId}-panel-ref-${index}`} className="rounded-lg bg-white p-3 ring-1 ring-line">
                <div className="grid gap-2 md:grid-cols-[1fr_0.8fr_1.4fr_auto]">
                  <input
                    value={ref.label || ''}
                    disabled={disabled || approved}
                    onChange={(event) => updateReferenceDraft(setReferenceDrafts, index, { label: event.target.value })}
                    placeholder="参考图名称"
                    className="rounded-lg border border-line bg-background-card px-3 py-2 text-xs text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
                    aria-label={`参考图 ${index + 1} 名称`}
                  />
                  <input
                    value={ref.role || ''}
                    disabled={disabled || approved}
                    onChange={(event) => updateReferenceDraft(setReferenceDrafts, index, { role: event.target.value })}
                    placeholder="类型，如 storyboard"
                    className="rounded-lg border border-line bg-background-card px-3 py-2 text-xs text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
                    aria-label={`参考图 ${index + 1} 类型`}
                  />
                  {isInternalStorageRef(ref.storageRef) ? (
                    <div className="rounded-lg border border-line bg-green-50 px-3 py-2 text-xs font-semibold text-green-700">
                      已选择参考图
                    </div>
                  ) : (
                    <input
                      value={ref.storageRef}
                      disabled={disabled || approved}
                      onChange={(event) => updateReferenceDraft(setReferenceDrafts, index, { storageRef: event.target.value })}
                      placeholder="图片 URL 或上传后自动登记"
                      className="rounded-lg border border-line bg-background-card px-3 py-2 text-xs text-ink outline-none focus:border-primary disabled:cursor-not-allowed disabled:bg-green-50"
                      aria-label={`参考图 ${index + 1} 来源`}
                    />
                  )}
                  <button
                    type="button"
                    disabled={disabled || approved}
                    onClick={() => setReferenceDrafts((current) => current.filter((_, itemIndex) => itemIndex !== index))}
                    className={clsx(
                      'rounded-lg px-3 py-2 text-xs font-black ring-1',
                      disabled || approved ? 'cursor-not-allowed bg-green-50 text-green-700 ring-green-100' : 'bg-white text-primary-dark ring-line hover:bg-primary-soft',
                    )}
                  >
                    移除
                  </button>
                </div>
                {ref.storageRef.trim() ? (
                  <div className="mt-3">
                    <ExternalReferenceCard reference={ref} index={index + 1} mode={mode} projectId={projectId} />
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        ) : (
          <p className="mt-3 rounded-lg bg-white p-3 text-xs leading-5 text-ink-muted ring-1 ring-line">本素材没有图片依赖，可以直接使用提示词生成，也可以新增参考图。</p>
        )}
        {referenceRows.length ? (
          <div className="mt-3 space-y-2">
            {referenceRows.map((row) => (
              <div key={row} className="rounded-lg bg-white px-3 py-2 text-xs leading-5 text-ink-muted ring-1 ring-line">{row}</div>
            ))}
          </div>
        ) : null}
      </div>
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
    narrationText: stringField(data.narrationText) || stringField(data.scriptText) || stringField(data.sourceScriptSegment) || undefined,
    sourceScriptSegment: stringField(data.sourceScriptSegment) || undefined,
    visualText: stringField(data.visual) || stringField(data.visualText) || stringField(data.visualChange) || undefined,
    prompt,
    overallShotPrompt: stringField(data.overallShotPrompt) || undefined,
    negativePrompt: stringField(data.negativePrompt) || undefined,
    references,
    aigcPlan: layerPlanFromUnknown(data.aigcPlan),
    hyperframesPlan: layerPlanFromUnknown(data.hyperframesPlan),
    ffmpegFusionPlan: layerPlanFromUnknown(data.ffmpegFusionPlan),
    textSafeLayout: stringField(data.textSafeLayout) || undefined,
    target: target ? {
      aspectRatio: stringField(target.aspectRatio) || undefined,
      durationSec: numberField(target.durationSec),
      resolution: stringField(target.resolution) || undefined,
    } : undefined,
    promptCharLimit: numberField(data.promptCharLimit),
    referenceImageLimit: numberField(data.referenceImageLimit),
  }
}

function layerPlanFromUnknown(value: unknown): ExternalGenerationLayerPlan | undefined {
  const item = objectField(value)
  if (!item) return undefined
  const plan: ExternalGenerationLayerPlan = {
    role: stringField(item.role) || undefined,
    prompt: stringField(item.prompt) || undefined,
    plan: stringField(item.plan) || undefined,
    textSafeLayout: stringField(item.textSafeLayout) || undefined,
    mode: stringField(item.mode) || undefined,
    keyframeStrategy: stringField(item.keyframeStrategy) || undefined,
    textRenderer: stringField(item.textRenderer) || undefined,
    locks: stringListField(item.locks),
    inputArtifacts: stringListField(item.inputArtifacts),
    outputArtifactKind: stringField(item.outputArtifactKind) || undefined,
    avoidGeneratedText: typeof item.avoidGeneratedText === 'boolean' ? item.avoidGeneratedText : undefined,
    requiresBlankArea: typeof item.requiresBlankArea === 'boolean' ? item.requiresBlankArea : undefined,
  }
  return Object.values(plan).some((field) => field !== undefined && field !== '') ? plan : undefined
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
    locks: stringListField(item.locks),
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

function stringListField(value: unknown): string[] | undefined {
  if (Array.isArray(value)) {
    const items = value.map((item) => stringField(item)).filter(Boolean)
    return items.length ? items : undefined
  }
  if (typeof value === 'string') {
    const items = value.split(/[,\n，、]+/u).map((item) => item.trim()).filter(Boolean)
    return items.length ? items : undefined
  }
  return undefined
}

function safeLocalUploadId(value: string): string {
  const cleaned = value.trim().replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^[._-]+|[._-]+$/g, '')
  return cleaned || 'external-result'
}

function artifactContentText(content: unknown): string {
  const safeContent = sanitizeLocalStorageRefs(content)
  if (typeof safeContent === 'string') return safeContent
  if (safeContent == null) return ''
  try {
    return JSON.stringify(safeContent, null, 2)
  } catch {
    return String(safeContent)
  }
}

function sanitizeLocalStorageRefs(value: unknown): unknown {
  if (typeof value === 'string') return sanitizeStorageRefText(value)
  if (Array.isArray(value)) return value.map((item) => sanitizeLocalStorageRefs(item))
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(([key, item]) => {
        if (isStorageRefKey(key) && typeof item === 'string') return [key, userFacingStorageStatus(item, '本地文件')]
        return [key, sanitizeLocalStorageRefs(item)]
      }),
    )
  }
  return value
}

function sanitizeStorageRefText(value: string): string {
  const refList = value.split(/\s*,\s*/u).filter(Boolean)
  if (refList.length > 0 && refList.every((item) => item.trim().startsWith('local://'))) {
    return refList.length === 1 ? '本地产物文件' : `${refList.length} 个本地产物文件`
  }
  return value.replace(/local:\/\/[^\s"',)\]}<>]+/gu, '本地文件')
}

function isStorageRefKey(key: string): boolean {
  return ['storageref', 'storage_ref', 'path', 'url'].includes(key.toLowerCase())
}

function isInternalStorageRef(value: string | undefined): boolean {
  return Boolean(value?.trim().startsWith('local://'))
}

function userFacingStorageStatus(storageRef: string | undefined, fallback: string): string {
  const ref = storageRef?.trim() || ''
  if (!ref) return fallback
  if (isInternalStorageRef(ref)) return fallback
  if (/^https?:\/\//u.test(ref)) return '外部链接'
  if (/^(blob:|data:)/u.test(ref)) return fallback
  return storageRefFileName(ref) || fallback
}

function storageRefFileName(storageRef: string): string {
  const clean = storageRef.split(/[?#]/u)[0] || ''
  const name = clean.split('/').filter(Boolean).pop() || ''
  try {
    return decodeURIComponent(name)
  } catch {
    return name
  }
}

function referenceCopyTextForUser(reference: ExternalGenerationReference, index: number): string {
  const lines = [
    `参考图 ${index}`,
    `名称：${reference.label || reference.id || '未命名参考图'}`,
    reference.role ? `用途：${reference.role}` : '',
    stringListField(reference.locks)?.length ? `锁定：${stringListField(reference.locks)?.join('、')}` : '',
  ].filter(Boolean)
  return lines.join('\n')
}

function artifactDetailStatus(artifact: DirectorArtifactRecord): string {
  if (!artifact.storageRef) return '产物内容待写入'
  const mediaKind = artifactPreviewKind(artifact)
  if (mediaKind === 'video') return '视频文件已生成，可在产物预览中播放'
  if (mediaKind === 'image') return '图片文件已生成，可在产物预览中查看'
  return `${displayNameForArtifact(artifact.kind)} 已生成`
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

function artifactProvenanceSummary(artifact: DirectorArtifactRecord): { label: string; detail: string; isFallback: boolean } | null {
  const metadata = artifact.metadata || {}
  const provenance = metadata.provenance && typeof metadata.provenance === 'object'
    ? metadata.provenance as Record<string, unknown>
    : {}
  const sourceType = stringFromUnknown(provenance.sourceType) || stringFromUnknown(metadata.sourceType)
  const providerName = stringFromUnknown(provenance.providerName) || stringFromUnknown(metadata.providerName) || stringFromUnknown(artifact.metadata?.provider)
  const providerJobId = stringFromUnknown(provenance.providerJobId) || stringFromUnknown(metadata.providerJobId)
  const fallbackReason = stringFromUnknown(provenance.fallbackReason) || stringFromUnknown(metadata.fallbackReason)
  const isFallback = booleanFromUnknown(provenance.isFallback) || booleanFromUnknown(metadata.isFallback) || sourceType.startsWith('fallback_')
  if (!sourceType && !providerName && !fallbackReason) return null
  const sourceLabel = sourceType ? sourceType.replace(/_/gu, ' ') : 'unknown source'
  const label = isFallback ? `fallback: ${sourceLabel}` : sourceLabel
  const parts = [
    `来源: ${sourceLabel}`,
    providerName ? `provider: ${providerName}` : '',
    providerJobId ? `job: ${providerJobId}` : '',
    fallbackReason ? `fallback reason: ${fallbackReason}` : '',
  ].filter(Boolean)
  return { label, detail: parts.join(' · '), isFallback }
}

function stringFromUnknown(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function booleanFromUnknown(value: unknown): boolean {
  return value === true || value === 'true'
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
    fileStatus: userFacingStorageStatus(artifact.storageRef, '本地文件'),
    dependsOn: artifact.dependsOn,
  }, null, 2)
}

function shotQaStatusLabel(status: string): string {
  const normalized = status.trim().toLowerCase()
  if (!normalized) return ''
  if (['valid', 'passed', 'shot_qa_passed', 'accepted_for_assembly', 'accepted'].includes(normalized)) return 'QA 通过'
  if (['failed', 'shot_qa_failed', 'rejected'].includes(normalized)) return 'QA 未通过'
  if (['pending', 'review', 'waiting'].includes(normalized)) return 'QA 待检查'
  return ''
}

function shotSourceTypeLabel(sourceType: string, isFallback: boolean): string {
  const normalized = sourceType.trim().toLowerCase()
  if (!normalized) return ''
  if (isFallback || normalized.startsWith('fallback_')) return '备用素材'
  const labelMap: Record<string, string> = {
    aigc_video: 'AIGC 视频',
    aigc_main_layer_preview: 'AIGC 主画面预览',
    hyperframes_overlay: 'HyperFrames 辅助层',
    hyperframes: 'HyperFrames 辅助层',
    manual_upload: '用户上传视频',
    uploaded_video: '用户上传视频',
    complete_shot: '完整 Shot',
    final_shot: '完整 Shot',
  }
  return labelMap[normalized] || ''
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
