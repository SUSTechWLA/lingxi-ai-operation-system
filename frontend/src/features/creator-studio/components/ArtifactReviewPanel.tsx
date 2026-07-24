import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import type { ArtifactContentResponse } from '../../../utils/types'
import {
  confirmStep,
  previewStepRevision,
  restoreStepVersion,
  reviseStep,
} from '../../../services/creatorApi'
import { buildClientModelProvidersForRun } from '../../../services/localAgent'
import type {
  ArtifactSelection,
  CreationView,
  CreatorArtifactVersion,
  CreatorStep,
  StepImpact,
  StepRevisionMutationRequest,
} from '../types'
import {
  CREATOR_CONFLICT_COPY,
  canConfirmCreatorStep,
  createTimeSelection,
  creatorMutationIdempotencyKey,
  creatorStepLabel,
  formatStepImpact,
  isCreatorConflict,
  normalizeRectSelection,
} from '../logic'
import { cycleFocusIndex } from '../focusCycle'
import { classifyArtifactPresentation } from '../artifactPresentation'
import type { CreatorReviewArtifact } from '../creatorReviewArtifacts'
import ArtifactProofingCanvas from './ArtifactProofingCanvas'

interface ArtifactReviewPanelProps {
  projectId: string
  step: CreatorStep
  artifact?: CreatorReviewArtifact
  content: ArtifactContentResponse | null
  versions: readonly CreatorArtifactVersion[]
  viewingHistorical?: boolean
  onViewChanged: (view: CreationView, navigateToActiveStep?: boolean) => void
  onConflict: (signal: AbortSignal) => Promise<unknown>
}

type PendingAction =
  | { kind: 'revision'; request: StepRevisionMutationRequest; impact: StepImpact }
  | { kind: 'restore'; version: number; impact: StepImpact }

export default function ArtifactReviewPanel({ projectId, step, artifact, content, versions, viewingHistorical = false, onViewChanged, onConflict }: ArtifactReviewPanelProps) {
  const [instruction, setInstruction] = useState('')
  const [directContent, setDirectContent] = useState(contentText(content?.content))
  const [mode, setMode] = useState<'instruction' | 'direct'>('instruction')
  const [selection, setSelection] = useState<ArtifactSelection | null>(null)
  const [pending, setPending] = useState<PendingAction | null>(null)
  const [error, setError] = useState('')
  const [working, setWorking] = useState(false)
  const imageRef = useRef<HTMLImageElement>(null)
  const selectionStart = useRef<{ x: number; y: number } | null>(null)
  const mountedRef = useRef(true)
  const operationControllerRef = useRef<AbortController | null>(null)
  const contentLoadedRef = useRef<string | null>(null)
  const impactTriggerRef = useRef<HTMLButtonElement | null>(null)

  const artifactId = artifact?.artifactId
  const baseVersion = artifact?.version
  const presentation = classifyArtifactPresentation({
    mimeType: content?.artifact.mimeType,
    kind: content?.artifact.kind,
    name: content?.artifact.name,
  })
  const isImage = presentation === 'image'
  const isVideo = presentation === 'video'
  const isText = ['json', 'markdown', 'text'].includes(presentation)
  const canConfirm = !viewingHistorical && canConfirmCreatorStep(step)
  const canRevise = !viewingHistorical && Boolean(artifactId && baseVersion && step.allowedActions.includes('revise'))
  const versionList = useMemo(() => [...versions].sort((left, right) => right.version - left.version), [versions])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      operationControllerRef.current?.abort()
    }
  }, [])

  useEffect(() => {
    operationControllerRef.current?.abort()
    setDirectContent('')
    setMode('instruction')
    setSelection(null)
    setPending(null)
    setError('')
    contentLoadedRef.current = null
  }, [artifactId, baseVersion])

  useEffect(() => {
    const key = `${artifactId || 'none'}:${baseVersion || 0}`
    if (!content || content.artifact.id !== artifactId || contentLoadedRef.current === key) return
    contentLoadedRef.current = key
    setDirectContent(contentText(content.content))
  }, [artifactId, baseVersion, content])

  const withErrorHandling = async (operation: (signal: AbortSignal, isCurrent: () => boolean) => Promise<void>) => {
    operationControllerRef.current?.abort()
    const controller = new AbortController()
    operationControllerRef.current = controller
    const isCurrent = () => mountedRef.current && !controller.signal.aborted && operationControllerRef.current === controller
    setError('')
    setWorking(true)
    try {
      await operation(controller.signal, isCurrent)
    } catch (caught) {
      if (!isCurrent()) return
      if (isCreatorConflict(caught)) {
        setError(CREATOR_CONFLICT_COPY)
        try {
          await onConflict(controller.signal)
        } catch {
          if (isCurrent()) setError('暂时无法刷新最新内容，请稍后重试。')
        }
      } else {
        setError('暂时无法保存，请稍后重试。')
      }
    } finally {
      if (isCurrent()) setWorking(false)
    }
  }

  const closeImpact = () => {
    setPending(null)
    window.requestAnimationFrame(() => impactTriggerRef.current?.focus())
  }

  const previewRevision = () => {
    if (!artifactId || !baseVersion) return
    const trimmedInstruction = instruction.trim()
    const trimmedContent = directContent.trim()
    if (mode === 'instruction' && !trimmedInstruction) {
      setError('请先告诉 AI 需要怎样修改。')
      return
    }
    if (mode === 'direct' && !trimmedContent) {
      setError('直接编辑的内容不能为空。')
      return
    }
    const request: StepRevisionMutationRequest = mode === 'instruction'
      ? {
          artifactId, baseVersion, mode, instruction: trimmedInstruction,
          reviewId: step.reviewId, runId: step.runId, selection, confirmedAffectedShotIds: [],
        }
      : {
          artifactId, baseVersion, mode, directContent: trimmedContent,
          reviewId: step.reviewId, runId: step.runId, selection, confirmedAffectedShotIds: [],
        }
    void withErrorHandling(async (signal, isCurrent) => {
      const impact = await previewStepRevision(projectId, step.id, { artifactId, baseVersion }, signal)
      if (isCurrent()) setPending({ kind: 'revision', request, impact })
    })
  }

  const confirmRevision = () => {
    if (!pending || pending.kind !== 'revision' || !canRevise) return
    const confirmedAffectedShotIds = pending.impact.affectedShotIds ?? []
    const request = { ...pending.request, confirmedAffectedShotIds }
    void withErrorHandling(async (signal, isCurrent) => {
      const modelProviders = request.mode === 'instruction'
        ? await buildClientModelProvidersForRun()
        : undefined
      if (!isCurrent()) return
      const result = await reviseStep(
        projectId,
        step.id,
        request.mode === 'instruction' && modelProviders ? { ...request, modelProviders } : request,
        creatorMutationIdempotencyKey(projectId, step.id, request as unknown as Record<string, unknown>), signal,
      )
      if (isCurrent()) {
        closeImpact()
        onViewChanged(result.view)
      }
    })
  }

  const previewRestore = (version: number) => {
    if (!artifactId || !baseVersion || !canRevise) return
    void withErrorHandling(async (signal, isCurrent) => {
      const impact = await previewStepRevision(projectId, step.id, { artifactId, baseVersion }, signal)
      if (isCurrent()) setPending({ kind: 'restore', version, impact })
    })
  }

  const confirmRestore = () => {
    if (!pending || pending.kind !== 'restore' || !baseVersion || !canRevise) return
    const request = {
      baseVersion,
      reviewId: step.reviewId,
      runId: step.runId,
      confirmedAffectedShotIds: pending.impact.affectedShotIds ?? [],
    }
    void withErrorHandling(async (signal, isCurrent) => {
      const result = await restoreStepVersion(
        projectId,
        step.id,
        pending.version,
        request,
        creatorMutationIdempotencyKey(projectId, step.id, { ...request, mode: 'restore', version: pending.version }), signal,
      )
      if (isCurrent()) {
        closeImpact()
        onViewChanged(result.view)
      }
    })
  }

  const handleConfirm = () => {
    if (!artifactId || !canConfirm) return
    void withErrorHandling(async (signal, isCurrent) => {
      const view = await confirmStep(projectId, step.id, {
        artifactId,
        reviewId: step.reviewId,
        runId: step.runId,
      }, signal)
      if (isCurrent()) onViewChanged(view, true)
    })
  }

  const startRectangle = (event: PointerEvent<HTMLImageElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    selectionStart.current = { x: event.clientX - bounds.left, y: event.clientY - bounds.top }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  const finishRectangle = (event: PointerEvent<HTMLImageElement>) => {
    const start = selectionStart.current
    const image = imageRef.current
    if (!start || !image) return
    const bounds = image.getBoundingClientRect()
    const nextSelection = normalizeRectSelection({
      x: start.x,
      y: start.y,
      width: event.clientX - bounds.left - start.x,
      height: event.clientY - bounds.top - start.y,
    }, { width: bounds.width, height: bounds.height })
    setSelection(nextSelection.width < 0.002 || nextSelection.height < 0.002 ? null : nextSelection)
    selectionStart.current = null
  }

  return (
    <section className="artifact-review-panel" aria-labelledby="artifact-review-title">
      <div className="artifact-review-heading">
        <div>
          <p className="creator-eyebrow">当前内容</p>
          <h2 id="artifact-review-title">{artifact?.reviewLabel || creatorStepLabel(step.id)}</h2>
          {versionList.length > 1 && <small>版本 {baseVersion} · 共 {versionList.length} 版</small>}
        </div>
        <span className={`artifact-state is-${step.state}`}>{stateCopy(step.state)}</span>
      </div>

      {!artifactId ? <p className="artifact-empty">这一步还没有可查看的内容。</p> : (
        <>
          {viewingHistorical && <div className="artifact-history-notice" role="status"><strong>正在查看历史产物</strong><span>当前版本不会被覆盖；需要时可从版本列表恢复。</span></div>}
          <ArtifactProofingCanvas
            content={content}
            selection={selection}
            imageRef={imageRef}
            onImagePointerDown={startRectangle}
            onImagePointerUp={finishRectangle}
            onImagePointerCancel={() => { selectionStart.current = null }}
          />
          {isImage && <p className="artifact-selection-help">在图片上拖拽框选需要调整的区域。</p>}
          {isVideo && <TimeSelection selection={selection} onChange={setSelection} />}
          {selection && <p className="artifact-selection-help">已保留本次选择范围，修改时会一并发送。</p>}

          <div className="artifact-actions">
            <button type="button" className="creator-primary-button" disabled={!canConfirm || working} onClick={handleConfirm}>确认并继续</button>
            <button type="button" className="creator-secondary-button" disabled={!canRevise || working} onClick={() => setMode('instruction')}>告诉 AI 怎么改</button>
            {isText && <button type="button" className="creator-secondary-button" disabled={!canRevise || working} onClick={() => setMode('direct')}>直接编辑</button>}
          </div>

          {mode === 'instruction' && (
            <label className="artifact-editor-label">修改说明
              <textarea value={instruction} onChange={event => setInstruction(event.target.value)} placeholder="例如：把开头改得更有悬念" />
            </label>
          )}
          {mode === 'direct' && isText && (
            <label className="artifact-editor-label">直接编辑内容
              <textarea value={directContent} onChange={event => setDirectContent(event.target.value)} />
              <button type="button" className="creator-secondary-button" disabled={!canRevise || working} onClick={event => { impactTriggerRef.current = event.currentTarget; previewRevision() }}>预览修改影响</button>
            </label>
          )}
          {mode === 'instruction' && <button type="button" className="creator-secondary-button artifact-preview-button" disabled={!canRevise || working} onClick={event => { impactTriggerRef.current = event.currentTarget; previewRevision() }}>预览修改影响</button>}

          {versionList.length > 1 && <details className="artifact-history">
            <summary>查看版本</summary>
            <ul>
              {versionList.map(version => (
                <li key={`${version.artifactId}-${version.version}`}>
                  <span>版本 {version.version}{version.isCurrent ? '（当前）' : ''}</span>
                  {!version.isCurrent && <button type="button" className="creator-text-button" disabled={!canRevise || working} onClick={event => { impactTriggerRef.current = event.currentTarget; previewRestore(version.version) }}>恢复这一版</button>}
                </li>
              ))}
            </ul>
          </details>}
        </>
      )}

      {pending && <ImpactConfirmation pending={pending} working={working} onCancel={closeImpact} onConfirm={pending.kind === 'revision' ? confirmRevision : confirmRestore} />}
      {error && <p className="creator-form-error" role="alert">{error}</p>}
    </section>
  )
}

function TimeSelection({ selection, onChange }: { selection: ArtifactSelection | null; onChange: (selection: ArtifactSelection) => void }) {
  const current = selection?.kind === 'time' ? selection : { startMs: 0, endMs: 1000 }
  return (
    <div className="artifact-time-selection" aria-label="视频评论时间范围">
      <label>开始时间（秒）<input type="number" min="0" step="0.1" value={current.startMs / 1000} onChange={event => onChange(createTimeSelection(Math.round(Number(event.target.value) * 1000), current.endMs))} /></label>
      <label>结束时间（秒）<input type="number" min="0" step="0.1" value={current.endMs / 1000} onChange={event => onChange(createTimeSelection(current.startMs, Math.round(Number(event.target.value) * 1000)))} /></label>
    </div>
  )
}

function ImpactConfirmation({ pending, working, onCancel, onConfirm }: { pending: PendingAction; working: boolean; onCancel: () => void; onConfirm: () => void }) {
  const shotIds = pending.impact.affectedShotIds ?? []
  const dialogRef = useRef<HTMLElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    window.requestAnimationFrame(() => cancelRef.current?.focus())
  }, [])

  const trapFocus = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      if (!working) onCancel()
      return
    }
    if (event.key !== 'Tab') return
    const controls = Array.from(dialogRef.current?.querySelectorAll<HTMLButtonElement>('button:not([disabled])') || [])
    if (controls.length === 0) {
      event.preventDefault()
      dialogRef.current?.focus()
      return
    }
    const nextIndex = cycleFocusIndex(controls.indexOf(document.activeElement as HTMLButtonElement), controls.length, event.shiftKey)
    if (nextIndex >= 0) {
      event.preventDefault()
      controls[nextIndex]?.focus()
    }
  }

  return (
    <div className="artifact-impact-backdrop" role="presentation" onPointerDown={event => { if (!working && event.currentTarget === event.target) onCancel() }}>
      <aside className="artifact-impact-confirmation" ref={dialogRef} role="dialog" aria-modal="true" aria-label="确认修改影响" tabIndex={-1} onPointerDown={event => event.stopPropagation()} onKeyDown={trapFocus}>
        <strong>确认这次修改</strong>
        <p>{formatStepImpact(pending.impact)}</p>
        {shotIds.length > 0 && <p>受影响镜头：{shotIds.join('、')}</p>}
        <div>
          <button ref={cancelRef} type="button" className="creator-secondary-button" disabled={working} onClick={onCancel}>取消</button>
          <button type="button" className="creator-primary-button" disabled={working} onClick={onConfirm}>{pending.kind === 'restore' ? '确认恢复' : '确认修改'}</button>
        </div>
      </aside>
    </div>
  )
}

function stateCopy(state: CreatorStep['state']): string {
  if (state === 'confirmed') return '已确认'
  if (state === 'needs_review') return '等待审阅'
  if (state === 'needs_attention') return '等待处理'
  if (state === 'generating') return '生成中'
  if (state === 'failed') return '生成失败'
  return '未开始'
}

function contentText(content: unknown): string {
  if (typeof content === 'string') return content
  if (content === undefined || content === null) return ''
  return JSON.stringify(content, null, 2)
}
