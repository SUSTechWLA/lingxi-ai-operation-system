import type { CreatorProcessEvent, CreatorStepId } from '../types'
import { creatorStepLabel } from '../logic'

const eventCopy: Record<CreatorProcessEvent['state'], string> = {
  started: '执行中',
  generated: '已生成',
  needs_review: '待审核',
  confirmed: '已确认',
  failed: '未完成',
  stale: '历史版本',
}

export default function CreatorProcessTimeline({ events, selectedStepId }: { events: readonly CreatorProcessEvent[]; selectedStepId: CreatorStepId }) {
  return (
    <details className="creator-process-timeline" open>
      <summary>
        <span><strong>完整创作过程</strong><small>{events.length} 条可审计记录</small></span>
        <span aria-hidden="true">展开 / 收起</span>
      </summary>
      {events.length === 0 ? <p className="artifact-empty">这个项目还没有可展示的执行记录。</p> : (
        <ol>
          {events.map(event => (
            <li key={event.id} className={`is-${event.state}${event.stepId === selectedStepId ? ' is-selected-step' : ''}`}>
              <span className="creator-process-marker" aria-hidden="true" />
              <div>
                <div className="creator-process-event-heading">
                  <strong>{event.title}</strong>
                  <span>{eventCopy[event.state]}</span>
                </div>
                <p>{creatorStepLabel(event.stepId)} · 第 {Math.max(1, event.attempt)} 轮 · {formatEventTime(event.completedAt || event.startedAt)}</p>
                {event.summary && <p>{event.summary}</p>}
              </div>
            </li>
          ))}
        </ol>
      )}
    </details>
  )
}

function formatEventTime(value?: string | null): string {
  if (!value) return '时间未记录'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未记录'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}
