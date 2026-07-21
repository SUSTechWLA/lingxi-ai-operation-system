import { useState } from 'react'
import { approveAgentReview } from '../../../services/api'
import type { AgentReviewItem } from '../../../utils/types'
import { creatorStepForAgentReview, creatorStepLabel } from '../logic'

interface AgentReviewGatePanelProps {
  runId: string
  review: AgentReviewItem
  onApproved: () => Promise<void>
}

export default function AgentReviewGatePanel({ runId, review, onApproved }: AgentReviewGatePanelProps) {
  const [working, setWorking] = useState(false)
  const [error, setError] = useState('')
  const stepId = creatorStepForAgentReview(review)
  const content = review.reviewContent?.trim() || reviewOutputText(review.reviewOutput)

  const approve = async () => {
    if (working) return
    setWorking(true)
    setError('')
    try {
      await approveAgentReview(runId, review.id, '已在创作者工作台审核通过')
      await onApproved()
    } catch {
      setError('暂时无法通过这一步，请稍后重试。')
    } finally {
      setWorking(false)
    }
  }

  return (
    <section className="artifact-review-panel creator-agent-review" aria-labelledby="creator-agent-review-title">
      <div className="artifact-review-heading">
        <div>
          <p className="creator-eyebrow">等待你的确认</p>
          <h2 id="creator-agent-review-title">审核{creatorStepLabel(stepId)}</h2>
        </div>
        <span className="artifact-state is-needs_review">待审核</span>
      </div>
      <p className="artifact-selection-help">确认当前结果后，系统会继续执行下一步。</p>
      {content && <pre className="creator-agent-review-content">{content}</pre>}
      <div className="artifact-actions">
        <button type="button" className="creator-primary-button" disabled={working} onClick={() => void approve()}>
          {working ? '正在继续…' : '审核并继续'}
        </button>
      </div>
      {error && <p className="creator-form-error" role="alert">{error}</p>}
    </section>
  )
}

function reviewOutputText(output: Record<string, unknown> | undefined): string {
  if (!output || Object.keys(output).length === 0) return ''
  const preferred = output.script ?? output.summary ?? output.content ?? output.result
  if (typeof preferred === 'string') return preferred
  try {
    return JSON.stringify(output, null, 2)
  } catch {
    return ''
  }
}
