import { useCallback, useEffect, type RefObject } from 'react'
import type { TextSelectionDraft } from '../textSelection'

interface TextSelectionAssistantProps {
  draft: TextSelectionDraft
  surfaceRef: RefObject<HTMLElement>
  onQuickAction: (instruction: string) => void
  onCustomInstruction: () => void
  onClear: () => void
}

const QUICK_ACTIONS = [
  ['更精炼', '只修改所选内容，使表达更精炼；保持上下文含义和未选内容不变。'],
  ['增强画面感', '只修改所选内容，增强画面感和具体细节；保持上下文含义和未选内容不变。'],
  ['优化节奏', '只修改所选内容，优化表达节奏；保持上下文含义和未选内容不变。'],
] as const

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
