import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { previewShotRegeneration, regenerateShot } from '../../../services/creatorApi'
import type { ShotImpact, ShotListItem, ShotLock, ShotRegenerationResult, ShotRegenerationScope, ShotWorkspace } from '../types'
import { canSubmitShotDuration, isCreatorConflict, isShotRetryEligible, isTargetOnlyShotImpact, SHOT_QUEUE_CONFLICT_COPY } from '../logic'

interface ShotImprovePanelProps {
  projectId: string
  workspace: ShotWorkspace
  item?: ShotListItem
  totalShots: number
  onRegenerationStarted: (result: ShotRegenerationResult) => Promise<void> | void
  onConflict: () => Promise<void> | void
}

const SCOPES: readonly { value: ShotRegenerationScope; label: string }[] = [
  { value: 'prompt', label: '画面提示' }, { value: 'reference', label: '参考素材' },
  { value: 'base_media', label: '基础画面' }, { value: 'overlay', label: '叠加元素' },
  { value: 'audio_alignment', label: '旁白对齐' }, { value: 'full_shot', label: '整个 Shot' },
]
const LOCKS: readonly { value: ShotLock; label: string }[] = [
  { value: 'duration', label: '时长' }, { value: 'narration', label: '旁白' }, { value: 'character', label: '人物' },
  { value: 'wardrobe', label: '服装' }, { value: 'scene', label: '场景' }, { value: 'camera', label: '镜头' },
  { value: 'first_frame', label: '首帧' }, { value: 'last_frame', label: '尾帧' },
  { value: 'reference_set', label: '参考集' }, { value: 'accepted_overlay', label: '已确认叠加层' },
]

export default function ShotImprovePanel({ projectId, workspace, item, totalShots, onRegenerationStarted, onConflict }: ShotImprovePanelProps) {
  const [scope, setScope] = useState<ShotRegenerationScope>('prompt')
  const [locks, setLocks] = useState<ShotLock[]>([])
  const [instruction, setInstruction] = useState('')
  const [impact, setImpact] = useState<ShotImpact | null>(null)
  const [error, setError] = useState('')
  const [working, setWorking] = useState(false)
  const operationKeyRef = useRef<string | null>(null)
  const controllerRef = useRef<AbortController | null>(null)
  const confirmButtonRef = useRef<HTMLButtonElement | null>(null)
  const triggerRef = useRef<HTMLButtonElement | null>(null)
  const currentShotKeyRef = useRef('')
  const shot = workspace.shot
  const generationRunning = ['queued', 'dispatching', 'running', 'GENERATING', 'SHOT_QA_RUNNING'].includes(item?.generationStatus ?? '')
  const durationValid = canSubmitShotDuration(shot.durationSec)
  const sequence = item?.sequenceIndex || shot.sequenceIndex || shot.id
  const locksKey = locks.join('|')

  useEffect(() => { currentShotKeyRef.current = `${shot.id}:${shot.version}` }, [shot.id, shot.version])

  useEffect(() => () => controllerRef.current?.abort(), [])
  useEffect(() => { if (impact) window.requestAnimationFrame(() => confirmButtonRef.current?.focus()) }, [impact])
  useEffect(() => {
    controllerRef.current?.abort()
    setImpact(null)
    setError('')
    operationKeyRef.current = null
  }, [shot.id, shot.version, scope, instruction, locksKey])

  const preview = () => {
    if (!durationValid) {
      setError('这个 Shot 时长无效（必须小于 15 秒），不能重新生成。')
      return
    }
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    setWorking(true)
    setError('')
    void previewShotRegeneration(projectId, shot.id, controller.signal).then(nextImpact => {
      if (controller.signal.aborted) return
      if (!isTargetOnlyShotImpact(nextImpact, shot.id)) {
        setError('影响范围不是仅当前 Shot，已停止重新生成。')
        return
      }
      setImpact(nextImpact)
    }).catch(() => {
      if (!controller.signal.aborted) setError('暂时无法确认影响范围，请稍后重试。')
    }).finally(() => {
      if (!controller.signal.aborted) setWorking(false)
    })
  }

  const confirm = () => {
    if (!impact || !durationValid || !isTargetOnlyShotImpact(impact, shot.id)) return
    const idempotencyKey = operationKeyRef.current ?? crypto.randomUUID()
    operationKeyRef.current = idempotencyKey
    const currentShotKey = `${shot.id}:${shot.version}`
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    setWorking(true)
    setError('')
    void regenerateShot(projectId, shot.id, {
      baseVersion: shot.version, scope, locks, instruction: instruction.trim() || undefined,
    }, idempotencyKey, controller.signal).then(async result => {
      if (controller.signal.aborted) return
      await onRegenerationStarted(result)
      if (controller.signal.aborted || currentShotKeyRef.current !== currentShotKey) return
      closeImpact()
    }).catch(async caught => {
      if (controller.signal.aborted) return
      if (isCreatorConflict(caught)) {
        setError(SHOT_QUEUE_CONFLICT_COPY)
        await onConflict()
        if (controller.signal.aborted || currentShotKeyRef.current !== currentShotKey) return
      } else {
        setError('暂时无法重新生成这个 Shot，请稍后重试。')
      }
    }).finally(() => {
      if (!controller.signal.aborted) setWorking(false)
    })
  }

  const closeImpact = () => {
    setImpact(null)
    window.requestAnimationFrame(() => triggerRef.current?.focus())
  }

  return (
    <section className="shot-improve-panel" aria-labelledby="shot-improve-title">
      <div><p className="creator-eyebrow">局部改进</p><h3 id="shot-improve-title">只调整这个 Shot</h3></div>
      {!durationValid && <p className="creator-form-error" role="alert">这个 Shot 时长无效（必须小于 15 秒），不能接受或重新生成。</p>}
      <label>调整范围
        <select value={scope} disabled={working || generationRunning || !durationValid} onChange={event => setScope(event.target.value as ShotRegenerationScope)}>
          {SCOPES.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
      </label>
      <fieldset disabled={working || generationRunning || !durationValid}><legend>保持不变</legend>
        <div className="shot-lock-grid">{LOCKS.map(lock => <label key={lock.value}><input type="checkbox" checked={locks.includes(lock.value)} onChange={() => setLocks(current => current.includes(lock.value) ? current.filter(value => value !== lock.value) : [...current, lock.value])} />{lock.label}</label>)}</div>
      </fieldset>
      <label>修改说明<textarea value={instruction} disabled={working || generationRunning || !durationValid} onChange={event => setInstruction(event.target.value)} placeholder="例如：让人物转身更自然" /></label>
      {generationRunning && <p className="artifact-selection-help">这个 Shot 正在生成，队列中的其他 Shot 仍可继续审核。</p>}
      {item?.generationStatus === 'failed' || item?.generationStatus === 'cancelled' || isShotRetryEligible(shot) ? <button ref={triggerRef} type="button" className="creator-secondary-button" disabled={working || !durationValid} onClick={preview}>重试这个 Shot</button> : <button ref={triggerRef} type="button" className="creator-secondary-button" disabled={working || generationRunning || !durationValid} onClick={preview}>预览重新生成影响</button>}
      {error && <p className="creator-form-error" role="alert">{error}</p>}
      {impact && <div className="artifact-impact-backdrop" role="presentation" onPointerDown={closeImpact}><section className="artifact-impact-confirmation" role="dialog" aria-modal="true" aria-labelledby="shot-impact-title" onPointerDown={event => event.stopPropagation()} onKeyDown={(event: KeyboardEvent<HTMLElement>) => { if (event.key === 'Escape') closeImpact(); if (event.key === 'Tab') { const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')); if (buttons.length) { event.preventDefault(); const current = buttons.indexOf(document.activeElement as HTMLButtonElement); const next = current < 0 ? (event.shiftKey ? buttons.length - 1 : 0) : (current + (event.shiftKey ? buttons.length - 1 : 1)) % buttons.length; buttons[next].focus() } } }}><h3 id="shot-impact-title">确认重新生成</h3><p>只会新增 Shot {sequence} 的候选，不影响其他 {Math.max(0, totalShots - 1)} 个 Shot</p><p>当前候选不会被自动接受。</p><div><button ref={confirmButtonRef} type="button" className="creator-primary-button" disabled={working} onClick={confirm}>确认新增候选</button><button type="button" className="creator-secondary-button" disabled={working} onClick={closeImpact}>取消</button></div></section></div>}
    </section>
  )
}
