import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import type { ArtifactContentResponse } from '../../../utils/types'
import {
  confirmStep,
  previewStepRevision,
  registerProjectMaterial,
  restoreStepVersion,
  reviseStep,
} from '../../../services/creatorApi'
import { buildClientModelProvidersForRun, uploadLocalArtifactFile } from '../../../services/localAgent'
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
  buildProjectMaterialStorageRef,
  creatorMutationIdempotencyKey,
  creatorStepLabel,
  formatStepImpact,
  isCreatorConflict,
  normalizeRectSelection,
} from '../logic'
import { cycleFocusIndex } from '../focusCycle'
import {
  classifyArtifactPresentation,
  creatorDirectEditText,
  reconcileCreatorEditMode,
} from '../artifactPresentation'
import type { CreatorReviewArtifact } from '../creatorReviewArtifacts'
import type { TextSelectionDraft } from '../textSelection'
import ArtifactProofingCanvas from './ArtifactProofingCanvas'
import ImageReviewDialog from './ImageReviewDialog'
import TextSelectionAssistant from './TextSelectionAssistant'

const TEXT_SELECTION_CONFLICT_COPY = '内容已更新，请重新选择需要修改的文字。'

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
  const [directContent, setDirectContent] = useState('')
  const [mode, setMode] = useState<'instruction' | 'direct'>('instruction')
  const [selection, setSelection] = useState<ArtifactSelection | null>(null)
  const [textSelectionDraft, setTextSelectionDraft] = useState<TextSelectionDraft | null>(null)
  const [pending, setPending] = useState<PendingAction | null>(null)
  const [imageDialogSrc, setImageDialogSrc] = useState('')
  const [imageDialogOpen, setImageDialogOpen] = useState(false)
  const [error, setError] = useState('')
  const [working, setWorking] = useState(false)
  const imageRef = useRef<HTMLImageElement>(null)
  const textSurfaceRef = useRef<HTMLPreElement>(null)
  const instructionRef = useRef<HTMLTextAreaElement>(null)
  const selectionStart = useRef<{ x: number; y: number } | null>(null)
  const mountedRef = useRef(true)
  const operationControllerRef = useRef<AbortController | null>(null)
  const contentLoadedRef = useRef<string | null>(null)
  const impactTriggerRef = useRef<HTMLButtonElement | null>(null)
  const preserveConflictErrorRef = useRef(false)

  const artifactId = artifact?.artifactId
  const baseVersion = artifact?.version
  const presentation = classifyArtifactPresentation({
    mimeType: content?.artifact.mimeType,
    kind: content?.artifact.kind,
    name: content?.artifact.name,
  })
  const isImage = presentation === 'image'
  const isVideo = presentation === 'video'
  const directEditText = creatorDirectEditText(presentation, content?.reviewText)
  const canDirectEdit = directEditText !== undefined
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
    setTextSelectionDraft(null)
    setPending(null)
    setImageDialogOpen(false)
    setImageDialogSrc('')
    if (!preserveConflictErrorRef.current) setError('')
    contentLoadedRef.current = null
  }, [artifactId, baseVersion])

  useEffect(() => {
    const key = `${artifactId || 'none'}:${baseVersion || 0}`
    if (!content || content.artifact.id !== artifactId || contentLoadedRef.current === key) return
    contentLoadedRef.current = key
    setDirectContent(directEditText ?? '')
    setMode(current => reconcileCreatorEditMode(current, directEditText))
  }, [artifactId, baseVersion, content, directEditText])

  const withErrorHandling = async (operation: (signal: AbortSignal, isCurrent: () => boolean) => Promise<void>) => {
    operationControllerRef.current?.abort()
    const controller = new AbortController()
    operationControllerRef.current = controller
    const isCurrent = () => mountedRef.current && !controller.signal.aborted && operationControllerRef.current === controller
    preserveConflictErrorRef.current = false
    setError('')
    setWorking(true)
    try {
      await operation(controller.signal, isCurrent)
    } catch (caught) {
      if (!isCurrent()) return
      if (isCreatorConflict(caught)) {
        const textSelectionConflict = selection?.kind === 'text'
        preserveConflictErrorRef.current = true
        setError(textSelectionConflict ? TEXT_SELECTION_CONFLICT_COPY : CREATOR_CONFLICT_COPY)
        if (textSelectionConflict) {
          setSelection(null)
          setTextSelectionDraft(null)
          window.getSelection()?.removeAllRanges()
        }
        try {
          await onConflict(controller.signal)
        } catch {
          if (isCurrent()) setError('暂时无法刷新最新内容，请稍后重试。')
        } finally {
          preserveConflictErrorRef.current = false
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

  const previewRevision = (preparedInstruction?: string) => {
    if (!artifactId || !baseVersion) return
    setTextSelectionDraft(null)
    window.getSelection()?.removeAllRanges()
    const effectiveMode = preparedInstruction === undefined ? mode : 'instruction'
    if (effectiveMode === 'direct' && !canDirectEdit) {
      setDirectContent('')
      setMode('instruction')
      setError('当前内容请使用修改说明整体优化。')
      return
    }
    const trimmedInstruction = (preparedInstruction ?? instruction).trim()
    const trimmedContent = directContent.trim()
    if (effectiveMode === 'instruction' && !trimmedInstruction) {
      setError('请先告诉 AI 需要怎样修改。')
      return
    }
    if (effectiveMode === 'direct' && !trimmedContent) {
      setError('直接编辑的内容不能为空。')
      return
    }
    const request: StepRevisionMutationRequest = effectiveMode === 'instruction'
      ? {
          artifactId, baseVersion, mode: 'instruction', instruction: trimmedInstruction,
          reviewId: step.reviewId, runId: step.runId, selection, confirmedAffectedShotIds: [],
        }
      : {
          artifactId, baseVersion, mode: 'direct', directContent: trimmedContent,
          reviewId: step.reviewId, runId: step.runId, selection, confirmedAffectedShotIds: [],
        }
    void withErrorHandling(async (signal, isCurrent) => {
      const impact = await previewStepRevision(projectId, step.id, { artifactId, baseVersion }, signal)
      if (isCurrent()) setPending({ kind: 'revision', request, impact })
    })
  }

  const prepareImageRevision = (nextInstruction: string) => {
    setMode('instruction')
    setInstruction(nextInstruction)
    setImageDialogOpen(false)
    previewRevision(nextInstruction)
  }

  const prepareImageReplacement = async (file: File) => {
    if (!artifactId || !baseVersion || !canRevise || !file.type.startsWith('image/')) {
      throw new Error('invalid image replacement')
    }
    operationControllerRef.current?.abort()
    const controller = new AbortController()
    operationControllerRef.current = controller
    const isCurrent = () => mountedRef.current && !controller.signal.aborted && operationControllerRef.current === controller
    setError('')
    setWorking(true)
    try {
      const replacementId = `creator-replacement-${crypto.randomUUID()}`
      const upload = await uploadLocalArtifactFile({
        projectId,
        id: replacementId,
        storageRef: buildProjectMaterialStorageRef(projectId, replacementId),
        file,
        mimeType: file.type,
        signal: controller.signal,
        metadata: {
          artifactType: 'project_source_material',
          source: 'creator_image_replacement',
          localOnly: true,
        },
      })
      if (!isCurrent() || !upload.storageRef || !upload.contentHash) throw new Error('replacement upload is incomplete')
      const registered = await registerProjectMaterial(projectId, {
        name: file.name,
        kind: 'image',
        storageRef: upload.storageRef,
        mimeType: upload.mimeType || file.type,
        sizeBytes: upload.sizeBytes ?? file.size,
        contentHash: upload.contentHash,
      }, controller.signal)
      if (!isCurrent() || registered.material.kind !== 'image') throw new Error('replacement registration is incomplete')
      const request: StepRevisionMutationRequest = {
        artifactId,
        baseVersion,
        mode: 'replace',
        replacementMaterial: {
          contentHash: registered.material.contentHash,
          storageRef: registered.material.storageRef,
          mimeType: registered.material.mimeType,
          sizeBytes: registered.material.sizeBytes,
        },
        reviewId: step.reviewId,
        runId: step.runId,
        selection: selection?.kind === 'rect' ? selection : null,
        confirmedAffectedShotIds: [],
      }
      const impact = await previewStepRevision(projectId, step.id, { artifactId, baseVersion }, controller.signal)
      if (!isCurrent()) throw new Error('replacement preview was cancelled')
      setPending({ kind: 'revision', request, impact })
      setImageDialogOpen(false)
    } catch (caught) {
      if (isCurrent()) setError('替换图片尚未准备好，请保留当前图片并重试。')
      throw caught
    } finally {
      if (isCurrent()) setWorking(false)
    }
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
    setTextSelectionDraft(null)
    selectionStart.current = null
  }

  const handleTextSelectionChange = (draft: TextSelectionDraft | null) => {
    setTextSelectionDraft(draft)
    setSelection(current => draft?.selection ?? (current?.kind === 'text' ? null : current))
  }

  const chooseTextQuickAction = (nextInstruction: string) => {
    if (!textSelectionDraft) return
    setSelection(textSelectionDraft.selection)
    setMode('instruction')
    setInstruction(nextInstruction)
    setTextSelectionDraft(null)
    window.getSelection()?.removeAllRanges()
  }

  const chooseCustomTextInstruction = () => {
    if (!textSelectionDraft) return
    setSelection(textSelectionDraft.selection)
    setMode('instruction')
    setTextSelectionDraft(null)
    window.getSelection()?.removeAllRanges()
    window.requestAnimationFrame(() => instructionRef.current?.focus())
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
            onOpenImage={src => {
              setImageDialogSrc(src)
              setImageDialogOpen(true)
            }}
            onImagePointerDown={startRectangle}
            onImagePointerUp={finishRectangle}
            onImagePointerCancel={() => { selectionStart.current = null }}
            textSurfaceRef={textSurfaceRef}
            onTextSelectionChange={handleTextSelectionChange}
          />
          {textSelectionDraft && <TextSelectionAssistant
            draft={textSelectionDraft}
            surfaceRef={textSurfaceRef}
            onQuickAction={chooseTextQuickAction}
            onCustomInstruction={chooseCustomTextInstruction}
            onClear={() => handleTextSelectionChange(null)}
          />}
          {isImage && <p className="artifact-selection-help">在图片上拖拽框选需要调整的区域。</p>}
          {isVideo && <TimeSelection selection={selection} onChange={nextSelection => {
            setTextSelectionDraft(null)
            setSelection(nextSelection)
          }} />}
          {selection && <p className="artifact-selection-help">已保留本次选择范围，修改时会一并发送。</p>}

          <div className="artifact-actions">
            <button type="button" className="creator-primary-button" disabled={!canConfirm || working} onClick={handleConfirm}>确认并继续</button>
            <button type="button" className="creator-secondary-button" disabled={!canRevise || working} onClick={() => setMode('instruction')}>告诉 AI 怎么改</button>
            {canDirectEdit && <button type="button" className="creator-secondary-button" disabled={!canRevise || working} onClick={() => setMode('direct')}>直接编辑</button>}
          </div>

          {mode === 'instruction' && (
            <label className="artifact-editor-label">修改说明
              <textarea ref={instructionRef} value={instruction} onChange={event => setInstruction(event.target.value)} placeholder="例如：把开头改得更有悬念" />
            </label>
          )}
          {mode === 'direct' && canDirectEdit && (
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

      {pending && <ImpactConfirmation pending={pending} working={working} error={error} onCancel={closeImpact} onConfirm={pending.kind === 'revision' ? confirmRevision : confirmRestore} />}
      {isImage && artifactId && baseVersion && imageDialogSrc && <ImageReviewDialog
        open={imageDialogOpen}
        projectId={projectId}
        artifactId={artifactId}
        baseVersion={baseVersion}
        title={artifact?.reviewLabel || content?.artifact.name || '当前图片'}
        src={imageDialogSrc}
        alt={content?.artifact.name || artifact?.reviewLabel || '当前图片内容'}
        selection={selection?.kind === 'rect' ? selection : null}
        instruction={instruction}
        working={working}
        onSelectionChange={setSelection}
        onInstructionChange={setInstruction}
        onPrepareRevision={prepareImageRevision}
        onPrepareReplacement={prepareImageReplacement}
        onKeep={() => {
          setImageDialogOpen(false)
          handleConfirm()
        }}
        onReload={() => setError('')}
        onClose={() => setImageDialogOpen(false)}
      />}
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

function ImpactConfirmation({ pending, working, error, onCancel, onConfirm }: { pending: PendingAction; working: boolean; error: string; onCancel: () => void; onConfirm: () => void }) {
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
        {error && <p className="creator-form-error" role="alert">{error}</p>}
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
