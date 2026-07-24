import { useEffect, useState } from 'react'
import {
  clearMediaRange,
  formatMediaTimecode,
  updateMediaRangeBoundary,
  type MediaRangeDraft,
  type TimeSelection,
} from '../mediaRange'

interface MediaRangeControlsProps {
  currentTimeMs: number
  selection: TimeSelection | null
  resetKey?: string | number
  disabled?: boolean
  onSelectionChange: (selection: TimeSelection | null) => void
  onPendingChange?: (pending: boolean) => void
}

export default function MediaRangeControls({
  currentTimeMs,
  selection,
  resetKey,
  disabled = false,
  onSelectionChange,
  onPendingChange,
}: MediaRangeControlsProps) {
  const [draft, setDraft] = useState<MediaRangeDraft>({})

  useEffect(() => {
    setDraft({})
    onPendingChange?.(false)
    return () => onPendingChange?.(false)
  }, [resetKey, onPendingChange])

  const applyBoundary = (boundary: 'start' | 'end') => {
    const next = updateMediaRangeBoundary(selection, draft, boundary, currentTimeMs)
    setDraft(next.draft)
    onSelectionChange(next.selection)
    onPendingChange?.(next.selection === null)
  }

  const clear = () => {
    const next = clearMediaRange()
    setDraft(next.draft)
    onSelectionChange(next.selection)
    onPendingChange?.(false)
  }

  const hasPendingBoundary = draft.startMs !== undefined || draft.endMs !== undefined
  const hasRange = selection !== null

  return (
    <div className="media-range-controls" role="group" aria-label="播放范围">
      <div className="media-range-summary" aria-live="polite">
        {hasRange ? (
          <><span>已选范围</span><strong>{formatMediaTimecode(selection.startMs)} – {formatMediaTimecode(selection.endMs)}</strong></>
        ) : draft.startMs !== undefined ? (
          <><span>开始于 {formatMediaTimecode(draft.startMs)}</span><strong>请移动播放头，再设置结束位置</strong></>
        ) : draft.endMs !== undefined ? (
          <><span>结束于 {formatMediaTimecode(draft.endMs)}</span><strong>请移动播放头，再设置开始位置</strong></>
        ) : (
          <><span>播放头</span><strong>{formatMediaTimecode(currentTimeMs)}</strong></>
        )}
      </div>
      <div className="media-range-actions">
        <button type="button" className="creator-secondary-button" disabled={disabled} onClick={() => applyBoundary('start')}>从这里开始</button>
        <button type="button" className="creator-secondary-button" disabled={disabled} onClick={() => applyBoundary('end')}>到这里结束</button>
        <button type="button" className="creator-text-button" disabled={disabled || (!hasRange && !hasPendingBoundary)} onClick={clear}>清除范围</button>
      </div>
    </div>
  )
}
