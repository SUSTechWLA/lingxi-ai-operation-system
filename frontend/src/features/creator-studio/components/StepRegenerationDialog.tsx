import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { previewStepRegeneration, regenerateStep } from '../../../services/creatorApi'
import type { CreationView, CreatorStep, StepImpact } from '../types'
import { creatorStepLabel, creatorStepRegenerationIdempotencyKey, isCreatorConflict } from '../logic'
import { cycleFocusIndex } from '../focusCycle'

export default function StepRegenerationDialog({
  projectId,
  step,
  onViewChanged,
}: {
  projectId: string
  step: CreatorStep
  onViewChanged: (view: CreationView) => void
}) {
  const [open, setOpen] = useState(false)
  const [impact, setImpact] = useState<StepImpact | null>(null)
  const [instruction, setInstruction] = useState('')
  const [working, setWorking] = useState(false)
  const [notice, setNotice] = useState('')
  const dialogRef = useRef<HTMLElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const controllerRef = useRef<AbortController | null>(null)
  const idempotencyKeyRef = useRef('')
  const canRegenerate = step.hasHistory && step.allowedActions.includes('regenerate') && step.state !== 'generating'

  useEffect(() => () => controllerRef.current?.abort(), [])
  const restoreFocus = () => {
    const trigger = triggerRef.current
    window.requestAnimationFrame(() => trigger?.focus())
  }

  const begin = async () => {
    if (!canRegenerate || working) return
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    setWorking(true)
    setNotice('')
    try {
      const nextImpact = await previewStepRegeneration(projectId, step.id, controller.signal)
      if (controller.signal.aborted) return
      setImpact(nextImpact)
      setOpen(true)
      window.requestAnimationFrame(() => cancelRef.current?.focus())
    } catch {
      if (!controller.signal.aborted) setNotice('暂时无法核对重新生成的影响，请稍后重试。')
    } finally {
      if (!controller.signal.aborted) setWorking(false)
    }
  }

  const confirm = async () => {
    if (!impact || working) return
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    const key = idempotencyKeyRef.current || creatorStepRegenerationIdempotencyKey(projectId, step.id, crypto.randomUUID())
    idempotencyKeyRef.current = key
    setWorking(true)
    setNotice('')
    try {
      const result = await regenerateStep(projectId, step.id, {
        ...(step.currentArtifactId && step.currentVersion ? { baseArtifactId: step.currentArtifactId, baseVersion: step.currentVersion } : {}),
        instruction: instruction.trim() || undefined,
        runId: step.runId,
        reviewId: step.reviewId,
        confirmedAffectedStepIds: impact.affectedStepIds,
      }, key, controller.signal)
      if (controller.signal.aborted) return
      onViewChanged(result.view)
      setOpen(false)
      restoreFocus()
      setImpact(null)
      setInstruction('')
      idempotencyKeyRef.current = ''
      setNotice('已开始重新生成，旧版本仍然保留。')
    } catch (caught) {
      if (controller.signal.aborted) return
      if (isCreatorConflict(caught)) {
        idempotencyKeyRef.current = ''
        setNotice('内容已经更新，请刷新后重新确认影响。')
      } else if (typeof caught === 'object' && caught !== null && 'response' in caught) {
        idempotencyKeyRef.current = ''
        setNotice('这次重新生成未能开始，请检查当前步骤后重试。')
      } else {
        setNotice('网络状态不确定；再次确认会沿用同一次请求，避免重复生成。')
      }
    } finally {
      if (!controller.signal.aborted) setWorking(false)
    }
  }

  const close = () => {
    if (working) return
    setOpen(false)
    restoreFocus()
    setImpact(null)
    idempotencyKeyRef.current = ''
  }

  const trapFocus = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      close()
      return
    }
    if (event.key !== 'Tab') return
    const controls = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]), textarea:not([disabled])') || [])
    const next = cycleFocusIndex(controls.indexOf(document.activeElement as HTMLElement), controls.length, event.shiftKey)
    if (next >= 0) {
      event.preventDefault()
      controls[next]?.focus()
    }
  }

  return (
    <div className="step-regeneration-control">
      {canRegenerate && <button ref={triggerRef} type="button" className="creator-secondary-button" disabled={working} onClick={() => void begin()}>{working && !open ? '正在核对影响…' : '从此步骤重新生成'}</button>}
      {notice && <p role="status">{notice}</p>}
      {open && impact && (
        <div className="artifact-impact-backdrop" role="presentation" onPointerDown={event => { if (event.currentTarget === event.target) close() }}>
          <aside ref={dialogRef} className="artifact-impact-confirmation step-regeneration-dialog" role="dialog" aria-modal="true" aria-labelledby="step-regeneration-title" tabIndex={-1} onKeyDown={trapFocus}>
            <p className="creator-eyebrow">保留历史</p>
            <h2 id="step-regeneration-title">重新生成{creatorStepLabel(step.id)}</h2>
            <p>将创建新的内容版本，不会覆盖当前结果。</p>
            {impact.affectedStepIds.length > 0 ? <div className="step-regeneration-impact"><strong>以下下游步骤会标记为待更新</strong><ul>{impact.affectedStepIds.map(id => <li key={id}>{creatorStepLabel(id)}</li>)}</ul></div> : <p>这是最终步骤，不会影响其他步骤。</p>}
            <label className="artifact-editor-label">本轮修改要求（可选）
              <textarea value={instruction} onChange={event => setInstruction(event.target.value)} placeholder="例如：保留结构，把开头改得更有吸引力" disabled={working} />
            </label>
            <div>
              <button ref={cancelRef} type="button" className="creator-secondary-button" disabled={working} onClick={close}>取消</button>
              <button type="button" className="creator-primary-button" disabled={working} onClick={() => void confirm()}>{working ? '正在创建新一轮…' : '确认并重新生成'}</button>
            </div>
          </aside>
        </div>
      )}
    </div>
  )
}
