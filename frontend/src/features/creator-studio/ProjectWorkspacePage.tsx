import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ArtifactContentResponse } from '../../utils/types'
import { getCreationView, getCreatorArtifactContent, getStepVersions } from '../../services/creatorApi'
import type { CreationView, CreatorArtifactVersion, CreatorStep, CreatorStepId } from './types'
import {
  creatorPollDelay,
  creatorStepLabel,
  didSelectedShotTaskChange,
  isCurrentWorkspaceArtifact,
  isLatestWorkspaceRequest,
  type KeyedWorkspaceArtifact,
  type WorkspaceArtifactSelection,
  workspaceArtifactKey,
} from './logic'
import CreationStrip from './components/CreationStrip'
import ArtifactReviewPanel from './components/ArtifactReviewPanel'
import TaskRecoveryBanner from './components/TaskRecoveryBanner'

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
  const activeTasksRef = useRef<CreationView['activeTasks']>([])
  const viewRequestTokenRef = useRef(0)
  const artifactRequestTokenRef = useRef(0)
  const currentStep = view?.steps.find(step => step.id === stepId)
  const currentArtifactId = currentStep?.currentArtifactId
  const currentVersion = currentStep?.currentVersion
  const artifactSelection = useMemo<WorkspaceArtifactSelection | null>(() => (
    currentArtifactId && currentVersion ? { stepId, artifactId: currentArtifactId, version: currentVersion } : null
  ), [currentArtifactId, currentVersion, stepId])

  const refreshView = useCallback(async (signal?: AbortSignal) => {
    const requestToken = ++viewRequestTokenRef.current
    const nextView = await getCreationView(projectId, signal)
    if (signal?.aborted || !isLatestWorkspaceRequest(requestToken, viewRequestTokenRef.current)) return undefined
    const previousTasks = activeTasksRef.current
    if (didSelectedShotTaskChange(previousTasks, nextView.activeTasks, selectedShotId) && selectedShotId) {
      await onSelectedShotTaskChanged?.(selectedShotId)
    }
    if (signal?.aborted || !isLatestWorkspaceRequest(requestToken, viewRequestTokenRef.current)) return undefined
    activeTasksRef.current = nextView.activeTasks
    setView(nextView)
    return nextView
  }, [onSelectedShotTaskChanged, projectId, selectedShotId])

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
    let attempt = 0
    let timer: number | undefined
    const controller = new AbortController()
    const schedule = () => {
      if (stopped || document.visibilityState !== 'visible') return
      timer = window.setTimeout(() => void poll(), creatorPollDelay(attempt))
    }
    const poll = async () => {
      if (stopped || document.visibilityState !== 'visible') return
      try {
        await refreshView(controller.signal)
      } catch {
        if (!stopped && !controller.signal.aborted) setError('正在后台继续获取最新进度。')
      } finally {
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
      <ArtifactReviewPanel
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
      />
      {error && <p className="creator-form-error" role="alert">{error}</p>}
    </section>
  )
}

function fallbackStep(stepId: CreatorStepId): CreatorStep {
  return { id: stepId, label: creatorStepLabel(stepId), state: 'not_started', allowedActions: [] }
}
