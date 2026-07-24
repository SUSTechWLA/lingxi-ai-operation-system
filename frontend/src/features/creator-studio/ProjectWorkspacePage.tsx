import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { AgentReviewItem, AgentRun, ArtifactContentResponse } from '../../utils/types'
import { getAgentRun, getAgentRunReviews, retryLatestFailedAgentNode } from '../../services/api'
import { getCreationView, getCreatorArtifactContent, getShotSummary, getShotWorkspace, getStepVersions, listShots } from '../../services/creatorApi'
import type { CreationView, CreatorArtifactVersion, CreatorStep, CreatorStepId, CreatorTask, ShotListFilters, ShotListItem, ShotRegenerationResult, ShotSummary, ShotUnit, ShotWorkspace } from './types'
import {
  creatorPollDelay,
  creatorArtifactLoadState,
  creatorStepLabel,
  creatorStepForAgentReview,
  didSelectedShotTaskChange,
  isCurrentWorkspaceArtifact,
  isLatestWorkspaceRequest,
  DEFAULT_SHOT_QUEUE_FILTER,
  adoptCreatorShotTask,
  selectedShotAfterAppend,
  selectedShotAfterReplacement,
  nextPendingCreatorReview,
  SHOT_QUEUE_CONFLICT_COPY,
  type KeyedWorkspaceArtifact,
  type WorkspaceArtifactSelection,
  workspaceArtifactKey,
  withCreatorProjectRecord,
} from './logic'
import CreationStrip from './components/CreationStrip'
import ArtifactReviewPanel from './components/ArtifactReviewPanel'
import TaskRecoveryBanner from './components/TaskRecoveryBanner'
import ShotReviewQueue from './components/ShotReviewQueue'
import ShotInspector from './components/ShotInspector'
import PreviewDeliveryPanel from './components/PreviewDeliveryPanel'
import AgentReviewGatePanel from './components/AgentReviewGatePanel'
import CreatorProcessTimeline from './components/CreatorProcessTimeline'
import CreatorContentLibrary from './components/CreatorContentLibrary'
import StepRegenerationDialog from './components/StepRegenerationDialog'
import ProjectBriefPanel from './components/ProjectBriefPanel'
import {
  projectCreatorReviewArtifacts,
  selectCreatorReviewArtifact,
  type CreatorReviewArtifact,
} from './creatorReviewArtifacts'

interface ProjectWorkspacePageProps {
  projectId: string
  stepId: CreatorStepId
  onNavigate: (hash: string) => void
  selectedShotId?: string
  onSelectedShotTaskChanged?: (shotId: string) => Promise<void> | void
}

type LoadedWorkspaceArtifact = KeyedWorkspaceArtifact<{
  content: ArtifactContentResponse
  versions: CreatorArtifactVersion[]
}>

export default function ProjectWorkspacePage({ projectId, stepId, onNavigate, selectedShotId, onSelectedShotTaskChanged }: ProjectWorkspacePageProps) {
  const [view, setView] = useState<CreationView | null>(null)
  const [artifactResult, setArtifactResult] = useState<LoadedWorkspaceArtifact | null>(null)
  const [artifactLoadError, setArtifactLoadError] = useState('')
  const [artifactReloadNonce, setArtifactReloadNonce] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [shotItems, setShotItems] = useState<ShotListItem[]>([])
  const [filteredShotTotal, setFilteredShotTotal] = useState(0)
  const [shotSummary, setShotSummary] = useState<ShotSummary | null>(null)
  const [shotNextCursor, setShotNextCursor] = useState('')
  const [shotFilters, setShotFilters] = useState<ShotListFilters>(DEFAULT_SHOT_QUEUE_FILTER)
  const [shotQueueLoading, setShotQueueLoading] = useState(false)
  const [selectedQueueShotId, setSelectedQueueShotId] = useState<string | undefined>(selectedShotId)
  const [shotWorkspace, setShotWorkspace] = useState<ShotWorkspace | null>(null)
  const [shotWorkspaceLoading, setShotWorkspaceLoading] = useState(false)
  const [shotNotice, setShotNotice] = useState('')
  const [agentRun, setAgentRun] = useState<AgentRun | null>(null)
  const [agentReviews, setAgentReviews] = useState<AgentReviewItem[]>([])
  const [recoveringAgentRun, setRecoveringAgentRun] = useState(false)
  const [selectedArtifactIds, setSelectedArtifactIds] = useState<Partial<Record<CreatorStepId, string>>>({})
  const activeTasksRef = useRef<CreationView['activeTasks']>([])
  const viewRequestTokenRef = useRef(0)
  const artifactRequestTokenRef = useRef(0)
  const shotListRequestTokenRef = useRef(0)
  const shotWorkspaceRequestTokenRef = useRef(0)
  const selectedShotIdRef = useRef<string | undefined>(selectedShotId)
  const selectedQueueShotIdRef = useRef<string | undefined>(selectedShotId)
  const shotItemsRef = useRef<ShotListItem[]>([])
  const shotNextCursorRef = useRef('')
  const shotQueueLoadingRef = useRef(false)
  const previousActiveTaskSignatureRef = useRef<string | null>(null)
  const previousVisibleArtifactsRef = useRef<Partial<Record<CreatorStepId, readonly CreatorReviewArtifact[]>>>({})
  const isShotsStep = stepId === 'shots'
  const isPreviewDeliveryStep = stepId === 'preview' || stepId === 'delivery'
  const activeShotId = selectedShotId && (shotItems.length === 0 || shotItems.some(item => item.id === selectedShotId)) ? selectedShotId : selectedQueueShotId
  const activeTaskSignature = useMemo(() => (view?.activeTasks ?? []).map(task => `${task.id}:${task.shotId ?? ''}:${task.status}`).sort().join('|'), [view?.activeTasks])
  const pollingSignature = activeTaskSignature || (view?.steps.some(step => step.state === 'generating') ? 'creator-step-generating' : '')
  const currentRunId = view?.project.currentRunId
  const pendingAgentReview = nextPendingCreatorReview(agentReviews)
  const displaySteps = useMemo(() => {
    if (!view) return []
    const reviewStepId = pendingAgentReview ? creatorStepForAgentReview(pendingAgentReview) : undefined
    const reviewed = reviewStepId
      ? view.steps.map(step => step.id === reviewStepId
        ? { ...step, state: 'needs_review' as const, allowedActions: ['confirm'] }
        : step)
      : view.steps
    return withCreatorProjectRecord(reviewed, view.project)
  }, [pendingAgentReview, view])

  useEffect(() => { shotItemsRef.current = shotItems }, [shotItems])
  useEffect(() => { selectedQueueShotIdRef.current = selectedQueueShotId }, [selectedQueueShotId])
  const visibleArtifacts = useMemo(
    () => projectCreatorReviewArtifacts(view?.stepArtifacts?.[stepId] ?? []),
    [stepId, view?.stepArtifacts],
  )
  const explicitArtifactId = selectedArtifactIds[stepId]
  const explicitArtifact = explicitArtifactId
    ? visibleArtifacts.find(artifact => artifact.artifactId === explicitArtifactId)
    : undefined
  const previousExplicitArtifact = explicitArtifactId
    ? previousVisibleArtifactsRef.current[stepId]?.find(artifact => artifact.artifactId === explicitArtifactId)
    : undefined
  const selectedBecameStale = Boolean(explicitArtifact?.isStale && previousExplicitArtifact && !previousExplicitArtifact.isStale)
  const selectedArtifact = selectCreatorReviewArtifact(
    visibleArtifacts,
    selectedBecameStale ? undefined : explicitArtifactId,
  )
  const currentArtifactId = selectedArtifact?.artifactId
  const currentVersion = selectedArtifact?.version
  const artifactSelection = useMemo<WorkspaceArtifactSelection | null>(() => (
    currentArtifactId && currentVersion ? { stepId, artifactId: currentArtifactId, version: currentVersion } : null
  ), [currentArtifactId, currentVersion, stepId])

  useEffect(() => {
    const selectedBecameHidden = Boolean(explicitArtifactId && !explicitArtifact)
    if (selectedBecameHidden || selectedBecameStale) {
      setSelectedArtifactIds(current => {
        if (current[stepId] !== explicitArtifactId) return current
        return { ...current, [stepId]: selectedArtifact?.artifactId }
      })
    }
    previousVisibleArtifactsRef.current[stepId] = visibleArtifacts
  }, [
    explicitArtifact,
    explicitArtifactId,
    selectedArtifact?.artifactId,
    selectedBecameStale,
    stepId,
    visibleArtifacts,
  ])

  const reloadShotWorkspace = useCallback(async (shotId = selectedShotIdRef.current, signal?: AbortSignal) => {
    if (!shotId) return
    const requestToken = ++shotWorkspaceRequestTokenRef.current
    setShotWorkspace(null)
    setShotWorkspaceLoading(true)
    try {
      const nextWorkspace = await getShotWorkspace(projectId, shotId, signal)
      if (!signal?.aborted && isLatestWorkspaceRequest(requestToken, shotWorkspaceRequestTokenRef.current)) setShotWorkspace(nextWorkspace)
    } finally {
      if (!signal?.aborted && isLatestWorkspaceRequest(requestToken, shotWorkspaceRequestTokenRef.current)) setShotWorkspaceLoading(false)
    }
  }, [projectId])

  const refreshView = useCallback(async (signal?: AbortSignal) => {
    const requestToken = ++viewRequestTokenRef.current
    const nextView = await getCreationView(projectId, signal)
    if (signal?.aborted || !isLatestWorkspaceRequest(requestToken, viewRequestTokenRef.current)) return undefined
    const previousTasks = activeTasksRef.current
    const currentSelectedShotId = selectedShotId ?? selectedShotIdRef.current
    if (didSelectedShotTaskChange(previousTasks, nextView.activeTasks, currentSelectedShotId) && currentSelectedShotId) {
      await reloadShotWorkspace(currentSelectedShotId, signal)
      await onSelectedShotTaskChanged?.(currentSelectedShotId)
    }
    if (signal?.aborted || !isLatestWorkspaceRequest(requestToken, viewRequestTokenRef.current)) return undefined
    activeTasksRef.current = nextView.activeTasks
    setView(nextView)
    return nextView
  }, [onSelectedShotTaskChanged, projectId, reloadShotWorkspace, selectedShotId])

  const loadShotPage = useCallback(async (reset: boolean, signal?: AbortSignal) => {
    if (!isShotsStep || (!reset && (!shotNextCursorRef.current || shotQueueLoadingRef.current))) return
    const requestToken = ++shotListRequestTokenRef.current
    shotQueueLoadingRef.current = true
    setShotQueueLoading(true)
    try {
      const page = await listShots(projectId, { ...shotFilters, ...(reset ? {} : { cursor: shotNextCursorRef.current }), limit: 24 }, signal)
      if (signal?.aborted || !isLatestWorkspaceRequest(requestToken, shotListRequestTokenRef.current)) return
      setFilteredShotTotal(page.total)
      shotNextCursorRef.current = page.nextCursor
      setShotNextCursor(page.nextCursor)
      const current = reset ? [] : shotItemsRef.current
      const merged = reset ? page.shots : [...current, ...page.shots.filter(next => !current.some(item => item.id === next.id))]
      shotItemsRef.current = merged
      setShotItems(merged)
      if (reset) setSelectedQueueShotId(selectedShotAfterReplacement(selectedShotId ?? selectedQueueShotIdRef.current, merged))
      else setSelectedQueueShotId(selected => selectedShotAfterAppend(selected, current, page.shots))
    } catch {
      if (!signal?.aborted) setError('暂时无法读取 Shot 队列，请稍后重试。')
    } finally {
      if (!signal?.aborted && isLatestWorkspaceRequest(requestToken, shotListRequestTokenRef.current)) {
        shotQueueLoadingRef.current = false
        setShotQueueLoading(false)
      }
    }
  }, [isShotsStep, projectId, selectedShotId, shotFilters])

  useEffect(() => {
    const controller = new AbortController()
    let mounted = true
    const load = async () => {
      setLoading(true)
      setError('')
      try {
        await refreshView(controller.signal)
      } catch {
        if (mounted) setError('暂时无法读取创作进度，请稍后重试。')
      } finally {
        if (mounted) setLoading(false)
      }
    }
    void load()
    return () => {
      mounted = false
      controller.abort()
    }
  }, [refreshView])

  const refreshAgentBridge = useCallback(async (runId: string, signal?: AbortSignal) => {
    const [nextRun, nextReviews] = await Promise.all([
      getAgentRun(runId),
      getAgentRunReviews(runId),
    ])
    if (signal?.aborted) return
    setAgentRun(nextRun)
    setAgentReviews(nextReviews.reviews ?? [])
  }, [])

  const retryFailedAgentRun = useCallback(async () => {
    if (!agentRun?.taskId || !currentRunId || recoveringAgentRun) return
    setRecoveringAgentRun(true)
    setError('')
    try {
      await retryLatestFailedAgentNode(agentRun.taskId)
      await Promise.all([refreshAgentBridge(currentRunId), refreshView()])
    } catch (retryError) {
      setError(retryError instanceof Error ? retryError.message : '失败步骤暂时无法重试，请稍后再试。')
    } finally {
      setRecoveringAgentRun(false)
    }
  }, [agentRun?.taskId, currentRunId, recoveringAgentRun, refreshAgentBridge, refreshView])

  useEffect(() => {
    if (!currentRunId) {
      setAgentRun(null)
      setAgentReviews([])
      return
    }
    const controller = new AbortController()
    const refresh = () => void refreshAgentBridge(currentRunId, controller.signal).catch(() => undefined)
    refresh()
    const timer = window.setInterval(refresh, 2500)
    return () => {
      controller.abort()
      window.clearInterval(timer)
    }
  }, [currentRunId, refreshAgentBridge])

  useEffect(() => {
    if (!isShotsStep) return
    const controller = new AbortController()
    shotNextCursorRef.current = ''
    void loadShotPage(true, controller.signal)
    void getShotSummary(projectId, controller.signal).then(summary => {
      if (!controller.signal.aborted) setShotSummary(summary)
    }).catch(() => undefined)
    return () => controller.abort()
  }, [isShotsStep, loadShotPage, projectId])

  useEffect(() => {
    selectedShotIdRef.current = activeShotId
    setShotNotice('')
    if (!isShotsStep || !activeShotId) return
    const controller = new AbortController()
    void reloadShotWorkspace(activeShotId, controller.signal).catch(() => {
      if (!controller.signal.aborted) setError('暂时无法读取这个 Shot，请稍后重试。')
    })
    return () => controller.abort()
  }, [activeShotId, isShotsStep, reloadShotWorkspace])

  useEffect(() => {
    const requestToken = ++artifactRequestTokenRef.current
    const requestSelection = artifactSelection
    setArtifactLoadError('')
    if (!requestSelection) return () => undefined
    const controller = new AbortController()
    void Promise.all([
      getCreatorArtifactContent(requestSelection.artifactId, controller.signal),
      getStepVersions(projectId, stepId, controller.signal),
    ]).then(([nextContent, nextVersions]) => {
      if (
        controller.signal.aborted ||
        !isLatestWorkspaceRequest(requestToken, artifactRequestTokenRef.current) ||
        nextContent.artifact.id !== requestSelection.artifactId
      ) return
      setArtifactResult({ key: workspaceArtifactKey(requestSelection), value: { content: nextContent, versions: nextVersions } })
    }).catch(() => {
      if (!controller.signal.aborted && isLatestWorkspaceRequest(requestToken, artifactRequestTokenRef.current)) {
        setArtifactLoadError('暂时无法读取当前内容，请稍后重试。')
      }
    })
    return () => controller.abort()
  }, [artifactReloadNonce, artifactSelection, projectId, stepId])

  useEffect(() => {
    if (!pollingSignature) return
    let stopped = false
    let polling = false
    let attempt = 0
    let timer: number | undefined
    const controller = new AbortController()
    const schedule = () => {
      if (stopped || document.visibilityState !== 'visible') return
      timer = window.setTimeout(() => void poll(), creatorPollDelay(attempt))
    }
    const poll = async () => {
      if (stopped || polling || document.visibilityState !== 'visible') return
      polling = true
      try {
        await refreshView(controller.signal)
      } catch {
        if (!stopped && !controller.signal.aborted) setError('正在后台继续获取最新进度。')
      } finally {
        polling = false
        attempt += 1
        schedule()
      }
    }
    const resume = () => {
      if (document.visibilityState !== 'visible' || stopped) return
      attempt = 0
      if (timer !== undefined) window.clearTimeout(timer)
      void poll()
    }
    document.addEventListener('visibilitychange', resume)
    schedule()
    return () => {
      stopped = true
      if (timer !== undefined) window.clearTimeout(timer)
      controller.abort()
      document.removeEventListener('visibilitychange', resume)
    }
  }, [pollingSignature, refreshView])

  useEffect(() => {
    if (!isShotsStep) return
    if (previousActiveTaskSignatureRef.current === null) {
      previousActiveTaskSignatureRef.current = activeTaskSignature
      return
    }
    if (previousActiveTaskSignatureRef.current === activeTaskSignature) return
    previousActiveTaskSignatureRef.current = activeTaskSignature
    void loadShotPage(true)
    void getShotSummary(projectId).then(setShotSummary).catch(() => undefined)
  }, [activeTaskSignature, isShotsStep, loadShotPage, projectId])

  if (loading) return <section className="creator-library-state" aria-live="polite">正在打开创作内容…</section>
  if (error && !view) return <section className="creator-library-state" role="alert">{error}</section>
  if (!view) return null

  const selectedStep = displaySteps.find(step => step.id === stepId) ?? fallbackStep(stepId)
  const currentArtifactResult = isCurrentWorkspaceArtifact(artifactResult, artifactSelection) && artifactResult?.value.content.artifact.id === currentArtifactId
    ? artifactResult
    : null
  const content = currentArtifactResult?.value.content ?? null
  const visibleArtifactIds = new Set(visibleArtifacts.map(artifact => artifact.artifactId))
  const versions = (currentArtifactResult?.value.versions ?? []).filter(version => visibleArtifactIds.has(version.artifactId))
  const artifactLoadState = creatorArtifactLoadState(artifactSelection, currentArtifactResult, artifactLoadError)
  const isRequirementsRecord = stepId === 'requirements' && visibleArtifacts.length === 0
  const viewingHistorical = Boolean(selectedArtifact && !selectedArtifact.isCurrent)
  const navigateToStep = (nextStepId: CreatorStepId) => onNavigate(`#/videos/${encodeURIComponent(projectId)}/steps/${nextStepId}`)
  return (
    <section className="creator-workspace" aria-labelledby="creator-page-title">
      <header className="creator-workspace-heading">
        <p className="creator-eyebrow">创作工作台</p>
        <h1 id="creator-page-title">{view.project.name || '正在创作的视频'}</h1>
        <p>从需求到交付，每一步都可以回看和继续调整。</p>
      </header>
      <CreationStrip steps={displaySteps} currentStepId={stepId} onSelect={navigateToStep} />
      <TaskRecoveryBanner tasks={view.activeTasks} />
      <CreatorProcessTimeline events={view.processTimeline ?? []} selectedStepId={stepId} />
      <div className="creator-step-commandbar">
        <div>
          <strong>{creatorStepLabel(stepId)}</strong>
          <span>{visibleArtifacts.length > 0 ? `${visibleArtifacts.length} 项可审阅内容` : '尚无可审阅内容'}{selectedStep.isStale ? ' · 下游已过期' : ''}</span>
        </div>
        <StepRegenerationDialog
          projectId={projectId}
          step={selectedStep}
          onViewChanged={nextView => {
            viewRequestTokenRef.current += 1
            activeTasksRef.current = nextView.activeTasks
            setView(nextView)
          }}
        />
      </div>
      <div className="creator-content-workspace">
        <CreatorContentLibrary
          projectId={projectId}
          artifacts={visibleArtifacts}
          selectedArtifactId={selectedArtifact?.artifactId}
          onSelect={artifact => setSelectedArtifactIds(current => ({ ...current, [stepId]: artifact.artifactId }))}
        />
        <div className="creator-proofing-canvas">
      {pendingAgentReview && currentRunId ? <AgentReviewGatePanel
        runId={currentRunId}
        review={pendingAgentReview}
        onApproved={async () => {
          await refreshAgentBridge(currentRunId)
          await refreshView()
        }}
      /> : isRequirementsRecord ? <ProjectBriefPanel project={view.project} /> : isShotsStep ? <div className="shot-review-workspace">
        <ShotReviewQueue
          items={shotItems}
          total={filteredShotTotal}
          selectedShotId={activeShotId}
          filters={shotFilters}
          loading={shotQueueLoading}
          hasMore={Boolean(shotNextCursor)}
          onFiltersChange={setShotFilters}
          onSelect={shotId => { setShotNotice(''); setSelectedQueueShotId(shotId) }}
          onLoadMore={() => { void loadShotPage(false) }}
        />
        <div className="shot-review-detail">
        {shotWorkspaceLoading || shotWorkspace?.shot.id !== activeShotId ? <section className="shot-inspector artifact-review-panel" aria-live="polite">正在打开 Shot…</section> : <ShotInspector
          projectId={projectId}
          workspace={shotWorkspace}
          item={shotItems.find(item => item.id === activeShotId)}
          previous={shotItems[shotItems.findIndex(item => item.id === activeShotId) - 1]}
          next={shotItems[shotItems.findIndex(item => item.id === activeShotId) + 1]}
          totalShots={shotSummary?.total ?? filteredShotTotal}
          onShotChanged={async (shot: ShotUnit) => {
            setShotNotice('')
            setShotItems(current => current.map(item => item.id === shot.id ? { ...item, version: shot.version, reviewStatus: shot.reviewStatus as ShotListItem['reviewStatus'], acceptedCandidateId: shot.acceptedCandidateId, qaStatus: shot.qaStatus as ShotListItem['qaStatus'] } : item))
            void loadShotPage(true)
            void getShotSummary(projectId).then(setShotSummary).catch(() => undefined)
            await reloadShotWorkspace(shot.id)
          }}
          onReload={async () => { setShotNotice(SHOT_QUEUE_CONFLICT_COPY); await reloadShotWorkspace(activeShotId); void loadShotPage(true); void getShotSummary(projectId).then(setShotSummary).catch(() => undefined) }}
          onRegenerationStarted={async (result: ShotRegenerationResult) => {
            setShotNotice('')
            viewRequestTokenRef.current += 1
            setShotItems(current => current.map(item => item.id === result.shot.id ? { ...item, version: result.shot.version, generationStatus: result.task.status as ShotListItem['generationStatus'], reviewStatus: result.shot.reviewStatus as ShotListItem['reviewStatus'], qaStatus: result.shot.qaStatus as ShotListItem['qaStatus'] } : item))
            const task: CreatorTask = { id: result.task.taskId, scope: 'shots', shotId: result.task.shotId, status: result.task.status as CreatorTask['status'], label: '正在重新生成镜头' }
            activeTasksRef.current = adoptCreatorShotTask(activeTasksRef.current, task)
            setView(current => current ? { ...current, activeTasks: adoptCreatorShotTask(current.activeTasks, task) } : current)
          }}
        />}
        {shotNotice && <p className="creator-form-error" role="alert">{shotNotice}</p>}
        </div>
      </div> : isPreviewDeliveryStep ? <PreviewDeliveryPanel
        projectId={projectId}
        step={selectedStep}
        content={content}
        assemblyDirty={view.assemblyDirty}
        viewingHistorical={viewingHistorical}
        onAssemblyUpdated={async () => { await refreshView() }}
      /> : artifactLoadState === 'error' ? <section className="artifact-review-panel artifact-load-error" role="alert">
        <p className="creator-eyebrow">当前内容</p>
        <h2>暂时无法打开这项产物</h2>
        <p>{artifactLoadError}</p>
        <button type="button" className="creator-secondary-button" onClick={() => setArtifactReloadNonce(value => value + 1)}>重新读取当前内容</button>
      </section> : <ArtifactReviewPanel
        projectId={projectId}
        step={selectedStep}
        artifact={selectedArtifact}
        content={content}
        versions={versions}
        viewingHistorical={viewingHistorical}
        onConflict={async signal => { await refreshView(signal) }}
        onViewChanged={(nextView, navigateToActiveStep) => {
          viewRequestTokenRef.current += 1
          activeTasksRef.current = nextView.activeTasks
          setView(nextView)
          if (navigateToActiveStep) navigateToStep(nextView.activeStep)
        }}
      />}
        </div>
      </div>
      {error && <p className="creator-form-error" role="alert">{error}</p>}
      {agentRun?.status === 'FAILED' && <div className="creator-form-error" role="alert">
        <span>创作任务遇到失败步骤，可在当前页面恢复。</span>
        <button type="button" disabled={recoveringAgentRun || !agentRun.taskId} onClick={() => { void retryFailedAgentRun() }}>
          {recoveringAgentRun ? '正在恢复…' : '重试失败步骤'}
        </button>
      </div>}
    </section>
  )
}


function fallbackStep(stepId: CreatorStepId): CreatorStep {
  return {
    id: stepId,
    label: creatorStepLabel(stepId),
    state: 'not_started',
    hasHistory: false,
    attemptCount: 0,
    artifactCount: 0,
    isStale: false,
    allowedActions: [],
  }
}
