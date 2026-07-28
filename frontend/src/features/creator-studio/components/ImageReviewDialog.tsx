import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
  type PointerEvent,
} from 'react'
import { getElectronAPI } from '../../../utils/electron'
import type { ArtifactSelection } from '../types'
import { normalizeRectSelection } from '../logic'

type RectSelection = Extract<ArtifactSelection, { kind: 'rect' }>

interface ImageReviewDialogProps {
  open: boolean
  projectId: string
  artifactId: string
  baseVersion: number
  title: string
  src: string
  alt: string
  selection: RectSelection | null
  instruction: string
  revisionInputsLocked: boolean
  onSelectionChange: (selection: RectSelection | null) => void
  onInstructionChange: (instruction: string) => void
  onPrepareRevision: (instruction: string) => void
  onPrepareReplacement: (file: File) => Promise<void>
  onKeep: () => void
  onReload: () => void
  onClose: () => void
}

const quickActions = [
  { label: '局部重绘', instruction: '只重绘所选区域，保持画面其他部分不变。' },
  { label: '调整构图', instruction: '优化主体位置、视觉层次和画面构图，保持内容主题不变。' },
  { label: '风格与光影', instruction: '优化画面风格、色彩和光影氛围，保持主体与叙事一致。' },
  { label: '整张重生成', instruction: '根据当前创作方向重新生成整张图片。', clearSelection: true },
] as const

function clampZoom(value: number): number {
  return Math.min(300, Math.max(50, value))
}

export default function ImageReviewDialog({
  open,
  title,
  src,
  alt,
  selection,
  instruction,
  revisionInputsLocked,
  onSelectionChange,
  onInstructionChange,
  onPrepareRevision,
  onPrepareReplacement,
  onKeep,
  onReload,
  onClose,
}: ImageReviewDialogProps) {
  const [zoom, setZoom] = useState(100)
  const [selectionMode, setSelectionMode] = useState(false)
  const [imageFailed, setImageFailed] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [replacementWorking, setReplacementWorking] = useState(false)
  const [replacementError, setReplacementError] = useState('')
  const dialogRef = useRef<HTMLElement>(null)
  const headingRef = useRef<HTMLHeadingElement>(null)
  const viewportRef = useRef<HTMLDivElement>(null)
  const imageRef = useRef<HTMLImageElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const triggerRef = useRef<HTMLElement | null>(null)
  const selectionStartRef = useRef<{ x: number; y: number } | null>(null)
  const panStartRef = useRef<{ x: number; y: number; left: number; top: number } | null>(null)
  const busy = revisionInputsLocked || replacementWorking

  useEffect(() => {
    if (!open) return () => undefined
    triggerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setImageFailed(false)
    setReplacementError('')
    window.requestAnimationFrame(() => headingRef.current?.focus())
    const trigger = triggerRef.current
    return () => {
      window.requestAnimationFrame(() => trigger?.focus())
    }
  }, [open])

  useEffect(() => {
    if (!busy) return
    selectionStartRef.current = null
    panStartRef.current = null
  }, [busy])

  if (!open) return null

  const trapFocus = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key !== 'Tab') return
    const controls = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>(
      'button:not([disabled]), input:not([disabled]):not([tabindex="-1"]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ) || [])
    if (controls.length === 0) {
      event.preventDefault()
      headingRef.current?.focus()
      return
    }
    const current = controls.indexOf(document.activeElement as HTMLElement)
    const next = event.shiftKey
      ? (current <= 0 ? controls.length - 1 : current - 1)
      : (current < 0 || current === controls.length - 1 ? 0 : current + 1)
    event.preventDefault()
    controls[next]?.focus()
  }

  const normalizedPoint = (event: PointerEvent<HTMLElement>) => {
    const bounds = imageRef.current?.getBoundingClientRect()
    if (!bounds || bounds.width <= 0 || bounds.height <= 0) return null
    return {
      x: Math.min(bounds.width, Math.max(0, event.clientX - bounds.left)),
      y: Math.min(bounds.height, Math.max(0, event.clientY - bounds.top)),
      bounds,
    }
  }

  const startPointer = (event: PointerEvent<HTMLDivElement>) => {
    if (busy) return
    if (selectionMode) {
      const point = normalizedPoint(event)
      if (!point) return
      selectionStartRef.current = { x: point.x, y: point.y }
      event.currentTarget.setPointerCapture(event.pointerId)
      return
    }
    if (zoom <= 100 || !viewportRef.current) return
    panStartRef.current = {
      x: event.clientX,
      y: event.clientY,
      left: viewportRef.current.scrollLeft,
      top: viewportRef.current.scrollTop,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  const movePointer = (event: PointerEvent<HTMLDivElement>) => {
    if (busy) return
    if (selectionMode && selectionStartRef.current) {
      const point = normalizedPoint(event)
      if (!point) return
      const start = selectionStartRef.current
      const next = normalizeRectSelection({
        x: start.x,
        y: start.y,
        width: point.x - start.x,
        height: point.y - start.y,
      }, { width: point.bounds.width, height: point.bounds.height })
      onSelectionChange(next.width < 0.002 || next.height < 0.002 ? null : next)
      return
    }
    const pan = panStartRef.current
    const viewport = viewportRef.current
    if (!pan || !viewport || zoom <= 100) return
    viewport.scrollLeft = pan.left - (event.clientX - pan.x)
    viewport.scrollTop = pan.top - (event.clientY - pan.y)
  }

  const finishPointer = (event: PointerEvent<HTMLDivElement>) => {
    selectionStartRef.current = null
    panStartRef.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  const chooseQuickAction = (action: (typeof quickActions)[number]) => {
    if (busy) return
    if ('clearSelection' in action && action.clearSelection) onSelectionChange(null)
    onInstructionChange(action.instruction)
    onPrepareRevision(action.instruction)
  }

  const chooseReplacement = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (busy || !file) return
    if (!file.type.startsWith('image/')) {
      setReplacementError('请选择图片文件后重试。')
      return
    }
    setReplacementError('')
    setReplacementWorking(true)
    try {
      await onPrepareReplacement(file)
    } catch {
      setReplacementError('替换图片尚未准备好，请保留当前图片并重试。')
    } finally {
      setReplacementWorking(false)
    }
  }

  const reloadPreview = () => {
    setImageFailed(false)
    setReloadKey(value => value + 1)
    onReload()
  }

  const locateOriginal = () => {
    const api = getElectronAPI()
    if (!api) return
    let target = src
    try {
      target = new URL(src).searchParams.get('path') || src
    } catch {
      // Keep the original source as a best-effort desktop recovery target.
    }
    void api.openPath(target)
  }

  return (
    <div className="image-review-backdrop" role="presentation">
      <section
        ref={dialogRef}
        className="image-review-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="image-review-title"
        onKeyDown={trapFocus}
      >
        <header className="image-review-header">
          <div>
            <p className="creator-eyebrow">图片审阅</p>
            <h2 ref={headingRef} id="image-review-title" tabIndex={-1}>{title}</h2>
          </div>
          <button type="button" className="creator-secondary-button" onClick={onClose} aria-label="关闭图片审阅">关闭</button>
        </header>

        <div className="image-review-toolbar" aria-label="图片查看工具">
          <button type="button" disabled={zoom <= 50} onClick={() => setZoom(value => clampZoom(value - 25))}>缩小</button>
          <output aria-live="polite">{zoom}%</output>
          <button type="button" disabled={zoom >= 300} onClick={() => setZoom(value => clampZoom(value + 25))}>放大</button>
          <button
            type="button"
            aria-pressed={selectionMode}
            disabled={busy}
            className={selectionMode ? 'is-active' : ''}
            onClick={() => setSelectionMode(value => !value)}
          >
            {selectionMode ? '退出框选' : '框选局部'}
          </button>
          {selection && <button type="button" disabled={busy} onClick={() => onSelectionChange(null)}>清除选区</button>}
        </div>

        <div
          ref={viewportRef}
          className={`image-review-viewport${selectionMode ? ' is-selecting' : zoom > 100 ? ' is-pannable' : ''}`}
          onPointerDown={startPointer}
          onPointerMove={movePointer}
          onPointerUp={finishPointer}
          onPointerCancel={finishPointer}
        >
          {imageFailed ? (
            <div className="image-review-recovery" role="alert">
              <strong>预览暂时无法读取</strong>
              <button type="button" onClick={reloadPreview}>重新加载预览</button>
              {getElectronAPI() && <button type="button" onClick={locateOriginal}>定位原文件</button>}
            </div>
          ) : (
            <div className="image-review-image-wrap" style={{ width: `${zoom}%` }}>
              <img
                key={`${src}:${reloadKey}`}
                ref={imageRef}
                src={src}
                alt={alt}
                draggable={false}
                onError={() => setImageFailed(true)}
              />
              {selection && (
                <span
                  className="image-review-selection"
                  aria-label="已选中的图片区域"
                  style={{
                    left: `${selection.x * 100}%`,
                    top: `${selection.y * 100}%`,
                    width: `${selection.width * 100}%`,
                    height: `${selection.height * 100}%`,
                  }}
                />
              )}
            </div>
          )}
        </div>

        <aside className="image-review-conversation" aria-label="图片修改对话">
          <div className="image-review-quick-actions">
            {quickActions.map(action => (
              <button key={action.label} type="button" disabled={busy} onClick={() => chooseQuickAction(action)}>
                {action.label}
              </button>
            ))}
          </div>
          <label>告诉 AI 怎么改
            <textarea
              value={instruction}
              disabled={busy}
              onChange={event => onInstructionChange(event.target.value)}
              placeholder="例如：让主体更突出，背景光线更柔和"
            />
          </label>
          <div className="image-review-conversation-actions">
            <button
              type="button"
              className="creator-secondary-button"
              disabled={busy || !instruction.trim()}
              onClick={() => onPrepareRevision(instruction.trim())}
            >
              预览修改影响
            </button>
            <input ref={fileInputRef} className="creator-visually-hidden" type="file" accept="image/*" tabIndex={-1} disabled={busy} onChange={chooseReplacement} />
            <button type="button" className="creator-secondary-button" disabled={busy} onClick={() => fileInputRef.current?.click()}>
              替换图片
            </button>
            <button type="button" className="creator-primary-button" disabled={busy} onClick={onKeep}>保留这张</button>
          </div>
          {replacementError && <p className="creator-form-error" role="alert">{replacementError}</p>}
        </aside>
      </section>
    </div>
  )
}
