import { useEffect, useState } from 'react'
import { approveAgentReview, regenerateAgentStage } from '../../../services/api'
import type { AgentReviewItem } from '../../../utils/types'
import {
  creatorAgentReviewApprovalBlocked,
  creatorAgentReviewCanRegenerate,
  creatorAgentReviewContent,
  creatorAgentReviewRegenerationHint,
  creatorStepForAgentReview,
  creatorStepLabel,
} from '../logic'
import { parseArtifactJson } from '../artifactPresentation'
import JsonArtifactViewer from './JsonArtifactViewer'
import MarkdownArtifactViewer from './MarkdownArtifactViewer'

interface AgentReviewGatePanelProps {
  runId: string
  review: AgentReviewItem
  onApproved: () => Promise<void>
}

export default function AgentReviewGatePanel({ runId, review, onApproved }: AgentReviewGatePanelProps) {
  const [working, setWorking] = useState(false)
  const [error, setError] = useState('')
  const [regenerationHint, setRegenerationHint] = useState(() => creatorAgentReviewRegenerationHint(review))
  const stepId = creatorStepForAgentReview(review)
  const content = creatorAgentReviewContent(review)
  const canRegenerate = creatorAgentReviewCanRegenerate(review)
  const approvalBlocked = creatorAgentReviewApprovalBlocked(review)
  const qualityReview = review.reviewPhase === 'quality_gate'
  const parsedContent = parseArtifactJson(content)
  const structuredContent = review.reviewOutput && Object.keys(review.reviewOutput).length > 0
    ? review.reviewOutput
    : parsedContent.ok && parsedContent.value && typeof parsedContent.value === 'object'
      ? parsedContent.value
      : undefined
  const jsonReviewContent = structuredContent ?? (looksLikeStructuredContent(content) ? content : undefined)

  useEffect(() => {
    setRegenerationHint(creatorAgentReviewRegenerationHint(review))
    setError('')
  }, [review])

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

  const regenerate = async () => {
    if (working || !canRegenerate) return
    setWorking(true)
    setError('')
    try {
      await regenerateAgentStage(runId, review.id, regenerationHint.trim() || undefined)
      await onApproved()
    } catch {
      setError('暂时无法重新生成，请稍后重试。')
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
      <p className="creator-review-guidance">{review.reviewReason || '确认当前结果后，系统会继续执行下一步。'}</p>
      {qualityReview && content
        ? <div className="creator-quality-summary"><p>{content}</p></div>
        : jsonReviewContent !== undefined
          ? <JsonArtifactViewer content={jsonReviewContent} />
          : content && <MarkdownArtifactViewer content={content} name={`${stepId}-review.md`} />}
      {canRegenerate && <label className="artifact-editor-label">修改要求
        <textarea
          value={regenerationHint}
          onChange={event => setRegenerationHint(event.target.value)}
          placeholder="例如：把口播扩充到 55 秒，并保留开场钩子"
          disabled={working}
        />
      </label>}
      {approvalBlocked && <p className="creator-form-error">质量未通过时不要直接放行，请先按建议重新生成。</p>}
      <div className="artifact-actions">
        {canRegenerate && <button type="button" className="creator-secondary-button" disabled={working} onClick={() => void regenerate()}>
          {working ? '正在处理…' : '按要求重新生成'}
        </button>}
        <button type="button" className="creator-primary-button" disabled={working || approvalBlocked} onClick={() => void approve()}>
          {working ? '正在继续…' : '审核并继续'}
        </button>
      </div>
      {error && <p className="creator-form-error" role="alert">{error}</p>}
    </section>
  )
}

function looksLikeStructuredContent(content: string): boolean {
  return /^\s*[{[]/.test(content)
}
