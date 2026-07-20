import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ArtifactContentResponse } from '../../utils/types'
import { getCreationView, getCreatorArtifactContent, getShotSummary, getShotWorkspace, getStepVersions, listShots } from '../../services/creatorApi'
import type { CreationView, CreatorArtifactVersion, CreatorStep, CreatorStepId, ShotListFilters, ShotListItem, ShotSummary, ShotUnit, ShotWorkspace } from './types'
import {
  creatorPollDelay,
  creatorStepLabel,
  didSelectedShotTaskChange,
  isCurrentWorkspaceArtifact,
  isLatestWorkspaceRequest,
  DEFAULT_SHOT_QUEUE_FILTER,
  selectedShotAfterAppend,
  type KeyedWorkspaceArtifact,
  type WorkspaceArtifactSelection,
  workspaceArtifactKey,
} from './logic'
import CreationStrip from './components/CreationStrip'
import ArtifactReviewPanel from './components/ArtifactReviewPanel'
import TaskRecoveryBanner from './components/TaskRecoveryBanner'
import ShotReviewQueue from './components/ShotReviewQueue'
import ShotInspector from './components/ShotInspector'

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
  const activeTasksRef = useRef<CreationView['activeTasks']>([])
  const viewRequestTokenRef = useRef(0)
  const artifactRequestTokenRef = useRef(0)
  const shotListRequestTokenRef = useRef(0)
  const shotWorkspaceRequestTokenRef = useRef(0)
  const selectedShotIdRef = useRef<string | undefined>(selectedShotId)
  const shotItemsRef = useRef<ShotListItem[]>([])
  const shotNextCursorRef = useRef('')
  const shotQueueLoadingRef = useRef(false)
  const currentStep = view?.steps.find(step => step.id === stepId)
  const isShotsStep = stepId === 'shots'
  const activeShotId = selectedShotId ?? selectedQueueShotId

  useEffect(() => { shotItemsRef.current = shotItems }, [shotItems])
  const currentArtifactId = currentStep?.currentArtifactId
  const currentVersion = currentStep?.currentVersion
  const artifactSelection = useMemo<WorkspaceArtifactSelection | null>(() => (
    currentArtifactId && currentVersion ? { stepId, artifactId: currentArtifactId, version: currentVersion } : null
  ), [currentArtifactId, currentVersion, stepId])

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
      if (reset) setSelectedQueueShotId(selectedShotAfterAppend(undefined, [], merged))
      else setSelectedQueueShotId(selected => selectedShotAfterAppend(selected, current, page.shots))
    } catch {
      if (!signal?.aborted) setError('暂时无法读取 Shot 队列，请稍后重试。')
    } finally {
      if (!signal?.aborted && isLatestWorkspaceRequest(requestToken, shotListRequestTokenRef.current)) {
        shotQueueLoadingRef.current = false
        setShotQueueLoading(false)
      }
    }
  }, [isShotsStep, projectId, shotFilters])

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
      if (!controller.signal.aborted && isLatestWorkspaceRequest(requestToken, artifactRequestTokenRef.current)) setError('暂时无法读取当前内容，请稍后重试。')
    })
    return () => controller.abort()
  }, [artifactSelection, projectId, stepId])

  useEffect(() => {
    if (!view?.activeTasks.length) return
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
  }, [refreshView, view?.activeTasks.length])

  if (loading) return <section className="creator-library-state" aria-live="polite">正在打开创作内容…</section>
  if (error && !view) return <section className="creator-library-state" role="alert">{error}</section>
  if (!view) return null

  const selectedStep = view.steps.find(step => step.id === stepId) ?? fallbackStep(stepId)
  const currentArtifactResult = isCurrentWorkspaceArtifact(artifactResult, artifactSelection) && artifactResult?.value.content.artifact.id === currentArtifactId
    ? artifactResult
    : null
  const content = currentArtifactResult?.value.content ?? null
  const versions = currentArtifactResult?.value.versions ?? []
  const navigateToStep = (nextStepId: CreatorStepId) => onNavigate(`#/videos/${encodeURIComponent(projectId)}/steps/${nextStepId}`)
  return (
    <section className="creator-workspace" aria-labelledby="creator-page-title">
      <header className="creator-workspace-heading">
        <p className="creator-eyebrow">创作工作台</p>
        <h1 id="creator-page-title">{view.project.name || '正在创作的视频'}</h1>
        <p>从需求到交付，每一步都可以回看和继续调整。</p>
      </header>
      <CreationStrip steps={view.steps} currentStepId={stepId} onSelect={navigateToStep} />
      <TaskRecoveryBanner tasks={view.activeTasks} />
      {isShotsStep ? <div className="shot-review-workspace">
        <ShotReviewQueue
          items={shotItems}
          total={filteredShotTotal}
          selectedShotId={activeShotId}
          filters={shotFilters}
          loading={shotQueueLoading}
          hasMore={Boolean(shotNextCursor)}
          onFiltersChange={setShotFilters}
          onSelect={setSelectedQueueShotId}
          onLoadMore={() => { void loadShotPage(false) }}
        />
        {shotWorkspaceLoading || shotWorkspace?.shot.id !== activeShotId ? <section className="shot-inspector artifact-review-panel" aria-live="polite">正在打开 Shot…</section> : <ShotInspector
          projectId={projectId}
          workspace={shotWorkspace}
          item={shotItems.find(item => item.id === activeShotId)}
          previous={shotItems[shotItems.findIndex(item => item.id === activeShotId) - 1]}
          next={shotItems[shotItems.findIndex(item => item.id === activeShotId) + 1]}
          totalShots={shotSummary?.total ?? filteredShotTotal}
          onShotChanged={async (shot: ShotUnit) => {
            setShotItems(current => current.map(item => item.id === shot.id ? { ...item, version: shot.version, reviewStatus: shot.reviewStatus as ShotListItem['reviewStatus'], acceptedCandidateId: shot.acceptedCandidateId, qaStatus: shot.qaStatus as ShotListItem['qaStatus'] } : item))
            void loadShotPage(true)
            void getShotSummary(projectId).then(setShotSummary).catch(() => undefined)
            await reloadShotWorkspace(shot.id)
          }}
          onReload={async () => { await reloadShotWorkspace(activeShotId); void loadShotPage(true); void getShotSummary(projectId).then(setShotSummary).catch(() => undefined) }}
        />}
      </div> : <ArtifactReviewPanel
        projectId={projectId}
        step={selectedStep}
        content={content}
        versions={versions}
        onConflict={async signal => { await refreshView(signal) }}
        onViewChanged={(nextView, navigateToActiveStep) => {
          viewRequestTokenRef.current += 1
          activeTasksRef.current = nextView.activeTasks
          setView(nextView)
          if (navigateToActiveStep) navigateToStep(nextView.activeStep)
        }}
      />}
      {error && <p className="creator-form-error" role="alert">{error}</p>}
    </section>
  )
}


function fallbackStep(stepId: CreatorStepId): CreatorStep {
  return { id: stepId, label: creatorStepLabel(stepId), state: 'not_started', allowedActions: [] }
}
