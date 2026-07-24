import { useCallback, useEffect, type RefObject } from 'react'
import { buildTextSelection, type TextSelectionDraft } from '../textSelection'

interface CanonicalTextSelectionSurfaceProps {
  source: string
  surfaceRef: RefObject<HTMLPreElement>
  onSelectionChange: (draft: TextSelectionDraft | null) => void
}

interface TextSelectionAssistantProps {
  draft: TextSelectionDraft
  surfaceRef: RefObject<HTMLPreElement>
  onQuickAction: (instruction: string) => void
  onCustomInstruction: () => void
  onClear: () => void
}

const QUICK_ACTIONS = [
  ['更精炼', '只修改所选内容，使表达更精炼；保持上下文含义和未选内容不变。'],
  ['增强画面感', '只修改所选内容，增强画面感和具体细节；保持上下文含义和未选内容不变。'],
  ['优化节奏', '只修改所选内容，优化表达节奏；保持上下文含义和未选内容不变。'],
] as const

export function CanonicalTextSelectionSurface({
  source,
  surfaceRef,
  onSelectionChange,
}: CanonicalTextSelectionSurfaceProps) {
  const captureSelection = () => {
    const surface = surfaceRef.current
    const browserSelection = window.getSelection()
    if (!surface || !browserSelection || browserSelection.rangeCount !== 1) {
      onSelectionChange(null)
      return
    }
    const range = browserSelection.getRangeAt(0)
    const textNode = surface.firstChild
    if (!textNode || range.startContainer !== textNode || range.endContainer !== textNode) {
      onSelectionChange(null)
      return
    }
    const selection = buildTextSelection(source, range.startOffset, range.endOffset)
    if (!selection) {
      onSelectionChange(null)
      return
    }
    const rangeBounds = range.getBoundingClientRect()
    const surfaceBounds = surface.getBoundingClientRect()
    onSelectionChange({
      selection,
      anchor: {
        left: rangeBounds.width > 0 ? rangeBounds.left + rangeBounds.width / 2 : surfaceBounds.left + 16,
        top: rangeBounds.height > 0 ? rangeBounds.bottom + 8 : surfaceBounds.top + 16,
      },
    })
  }

  return (
    <pre
      ref={surfaceRef}
      className="artifact-canonical-manuscript"
      tabIndex={0}
      role="document"
      aria-label="可划选的原稿"
      onMouseUp={captureSelection}
      onKeyUp={captureSelection}
    >{source}</pre>
  )
}

export default function TextSelectionAssistant({
  draft,
  surfaceRef,
  onQuickAction,
  onCustomInstruction,
  onClear,
}: TextSelectionAssistantProps) {
  const clearAndRestoreFocus = useCallback(() => {
    window.getSelection()?.removeAllRanges()
    onClear()
    window.requestAnimationFrame(() => surfaceRef.current?.focus())
  }, [onClear, surfaceRef])

  useEffect(() => {
    const handleEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        clearAndRestoreFocus()
      }
    }
    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  }, [clearAndRestoreFocus])

  const viewportWidth = typeof window === 'undefined' ? 1024 : window.innerWidth
  const viewportHeight = typeof window === 'undefined' ? 768 : window.innerHeight
  const left = Math.max(12, Math.min(draft.anchor.left - 130, viewportWidth - 292))
  const top = Math.max(12, Math.min(draft.anchor.top, viewportHeight - 176))

  return (
    <aside
      className="text-selection-assistant"
      role="toolbar"
      aria-label="优化所选文字"
      style={{ left, top }}
    >
      <strong>优化所选文字</strong>
      <div>
        {QUICK_ACTIONS.map(([label, instruction]) => (
          <button key={label} type="button" onClick={() => onQuickAction(instruction)}>{label}</button>
        ))}
        <button type="button" onClick={onCustomInstruction}>自定义修改</button>
      </div>
      <small>按 Esc 取消划选</small>
    </aside>
  )
}
