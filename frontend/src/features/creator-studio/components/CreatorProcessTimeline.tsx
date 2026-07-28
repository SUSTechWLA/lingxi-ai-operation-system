import type { CreatorStep, CreatorStepId } from '../types'
import { creatorProgressSteps, creatorStepLabel } from '../logic'

const stepCopy: Record<CreatorStep['state'], { state: string; action: string }> = {
  not_started: { state: '未开始', action: '完成前一步后，会继续到这里。' },
  generating: { state: '生成中', action: '正在准备内容，完成后即可审阅。' },
  needs_review: { state: '等待审阅', action: '内容已经准备好，请打开查看并确认。' },
  confirmed: { state: '已完成', action: '内容已确认；需要调整时，可以打开后重新生成。' },
  needs_attention: { state: '需要处理', action: '前面的内容有更新，请打开检查并重新生成。' },
  failed: { state: '需要处理', action: '这一步尚未完成，请打开后重试或调整。' },
}

export default function CreatorProcessTimeline({
  steps,
  selectedStepId,
  onSelect,
}: {
  steps: readonly CreatorStep[]
  selectedStepId: CreatorStepId
  onSelect: (stepId: CreatorStepId) => void
}) {
  const visibleSteps = creatorProgressSteps(steps)

  return (
    <details className="creator-process-timeline" open>
      <summary>
        <span><strong>创作进度</strong><small>每一步都可以回看和继续调整</small></span>
        <span aria-hidden="true">展开 / 收起</span>
      </summary>
      {visibleSteps.length === 0 ? <p className="artifact-empty">创作开始后，进度会显示在这里。</p> : (
        <ol aria-live="polite" aria-relevant="additions text">
          {visibleSteps.map(step => {
            const copy = step.isStale ? stepCopy.needs_attention : stepCopy[step.state]
            return <li key={step.id} className={`is-${step.state}${step.isStale ? ' is-stale' : ''}${step.id === selectedStepId ? ' is-selected-step' : ''}`}>
              <span className="creator-process-marker" aria-hidden="true" />
              <button type="button" aria-current={step.id === selectedStepId ? 'step' : undefined} onClick={() => onSelect(step.id)}>
                <div className="creator-process-event-heading">
                  <strong>{creatorStepLabel(step.id)}</strong>
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
