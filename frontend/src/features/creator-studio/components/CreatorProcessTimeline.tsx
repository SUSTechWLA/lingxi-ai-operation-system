import type { CreatorProcessEvent, CreatorStepId } from '../types'
import { creatorStepLabel } from '../logic'

const eventCopy: Record<CreatorProcessEvent['state'], { state: string; action: string }> = {
  started: { state: '生成中', action: '正在准备内容，完成后即可审阅。' },
  generated: { state: '等待审阅', action: '内容已经准备好，请打开查看。' },
  needs_review: { state: '等待审阅', action: '内容已经准备好，请打开查看并确认。' },
  confirmed: { state: '已完成', action: '内容已确认；需要调整时，可以打开后重新生成。' },
  failed: { state: '需要处理', action: '这一步尚未完成，请打开后重试或调整。' },
  stale: { state: '需要处理', action: '前面的内容有更新，请打开检查并重新生成。' },
}

export default function CreatorProcessTimeline({
  events,
  selectedStepId,
  onSelect,
}: {
  events: readonly CreatorProcessEvent[]
  selectedStepId: CreatorStepId
  onSelect: (stepId: CreatorStepId) => void
}) {
  return (
    <details className="creator-process-timeline" open>
      <summary>
        <span><strong>创作进度</strong><small>每一步都可以回看和继续调整</small></span>
        <span aria-hidden="true">展开 / 收起</span>
      </summary>
      {events.length === 0 ? <p className="artifact-empty">创作开始后，进度会显示在这里。</p> : (
        <ol aria-live="polite" aria-relevant="additions text">
          {events.map(event => {
            const copy = eventCopy[event.state]
            return <li key={event.id} className={`is-${event.state}${event.stepId === selectedStepId ? ' is-selected-step' : ''}`}>
              <span className="creator-process-marker" aria-hidden="true" />
              <button type="button" aria-current={event.stepId === selectedStepId ? 'step' : undefined} onClick={() => onSelect(event.stepId)}>
                <div className="creator-process-event-heading">
                  <strong>{creatorStepLabel(event.stepId)}</strong>
                  <span>{copy.state}</span>
                </div>
                <p>{copy.action}</p>
              </button>
            </li>
          })}
        </ol>
      )}
    </details>
  )
}
