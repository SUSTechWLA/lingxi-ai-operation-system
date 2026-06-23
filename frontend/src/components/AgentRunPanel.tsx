import { useState, useEffect, useCallback, useRef } from 'react'
import {
  startAgentRun,
  getAgentRun,
  getAgentRunReviews,
  approveAgentReview,
  rejectAgentReview,
  getAgentRunTrace,
} from '../services/api'
import type {
  AgentPlan,
  AgentStep,
  AgentRun,
  AgentReviewItem,
  AgentStartRunRequest,
} from '../utils/types'

type Props = {
  brief: string
  aspectRatio?: string
  targetDurationSec?: number
  platform?: string
  onTrace?: (data: unknown) => void
}

export default function AgentRunPanel({ brief, aspectRatio, targetDurationSec, platform, onTrace }: Props) {
  const [run, setRun] = useState<AgentRun | null>(null)
  const [plan, setPlan] = useState<AgentPlan | null>(null)
  const [reviews, setReviews] = useState<AgentReviewItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expandedStep, setExpandedStep] = useState<string | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [])

  const startPolling = useCallback((runId: string) => {
    if (pollRef.current) clearInterval(pollRef.current)
    pollRef.current = setInterval(async () => {
      try {
        const [currentRun, currentReviews] = await Promise.all([
          getAgentRun(runId),
          getAgentRunReviews(runId),
        ])
        setRun(currentRun)
        setReviews(currentReviews.reviews || [])
        if (currentRun.status === 'FAILED') {
          if (pollRef.current) clearInterval(pollRef.current)
        }
        // If all reviews are resolved (no PENDING), keep polling for completion
        const hasPending = (currentReviews.reviews || []).some(r => r.status === 'PENDING')
        if (!hasPending && currentRun.status === 'RUNNING') {
          // Still running but no pending reviews — keep polling
        }
      } catch {
        // Silently ignore poll errors
      }
    }, 3000)
  }, [])

  const handleStart = async () => {
    if (!brief.trim()) return
    setLoading(true)
    setError(null)
    setPlan(null)
    setRun(null)

    try {
      const req: AgentStartRunRequest = {
        message: brief,
        domain: 'video_creation',
        context: {
          platform: platform || '通用平台',
          aspectRatio: aspectRatio || '16:9',
          language: 'zh-CN',
          targetDurationSec: targetDurationSec || 90,
        },
        mode: 'dynamic_agent',
      }

      const result = await startAgentRun(req)
      setPlan(result.plan || null)

      if (result.runId) {
        const currentRun = await getAgentRun(result.runId)
        setRun(currentRun)
        startPolling(result.runId)

        // Fetch trace for display
        if (onTrace) {
          try {
            const trace = await getAgentRunTrace(result.runId)
            onTrace(trace)
          } catch { /* ignore */ }
        }
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Agent run failed'
      setError(msg)
    } finally {
      setLoading(false)
    }
  }

  const handleApprove = async (reviewId: string) => {
    if (!run?.id) return
    try {
      await approveAgentReview(run.id, reviewId)
      // Optimistic update
      setReviews(prev => prev.map(r => r.id === reviewId ? { ...r, status: 'APPROVED' as const } : r))
    } catch {
      // Will be corrected by next poll
    }
  }

  const handleReject = async (reviewId: string, comment?: string) => {
    if (!run?.id) return
    try {
      await rejectAgentReview(run.id, reviewId, comment || '审核未通过')
      setReviews(prev => prev.map(r => r.id === reviewId ? { ...r, status: 'REJECTED' as const } : r))
    } catch {
      // Will be corrected by next poll
    }
  }

  const pendingReviews = reviews.filter(r => r.status === 'PENDING')

  return (
    <div className="space-y-4">
      {/* Start button */}
      {!run && (
        <button
          onClick={handleStart}
          disabled={loading || !brief.trim()}
          className="w-full py-3 px-6 rounded-xl font-semibold text-white
            bg-gradient-to-r from-amber-600 to-orange-600 hover:from-amber-700 hover:to-orange-700
            disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200 shadow-lg"
        >
          {loading ? (
            <span className="flex items-center justify-center gap-2">
              <span className="animate-spin h-4 w-4 border-2 border-white border-t-transparent rounded-full" />
              LLM Planner 正在编排...
            </span>
          ) : (
            '🚀 启动 Dynamic Agent 创作线'
          )}
        </button>
      )}

      {/* Error */}
      {error && (
        <div className="p-3 rounded-lg bg-red-50 border border-red-200 text-red-700 text-sm">
          {error}
        </div>
      )}

      {/* Run status */}
      {run && (
        <div className="flex items-center gap-2 text-sm">
          <span className={`inline-block w-2 h-2 rounded-full ${
            run.status === 'RUNNING' ? 'bg-green-500 animate-pulse' :
            run.status === 'FAILED' ? 'bg-red-500' : 'bg-gray-400'
          }`} />
          <span className="text-gray-600">
            Agent Run: <code className="text-xs bg-gray-100 px-1 rounded">{run.id}</code>
          </span>
          <span className="text-gray-400">|</span>
          <span className="font-medium">{run.status}</span>
        </div>
      )}

      {/* Pending reviews */}
      {pendingReviews.length > 0 && (
        <div className="space-y-2">
          <h4 className="text-sm font-semibold text-amber-800">
            ⏳ 待审核 ({pendingReviews.length})
          </h4>
          {pendingReviews.map(review => (
            <div key={review.id} className="p-3 rounded-lg border border-amber-200 bg-amber-50">
              <div className="flex items-start justify-between gap-2">
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-gray-800">
                    {review.stepId || review.tool || '审核节点'}
                  </div>
                  {review.reviewReason && (
                    <div className="text-xs text-gray-500 mt-1">{review.reviewReason}</div>
                  )}
                  <div className="text-xs text-gray-400 mt-1">
                    Phase: {review.reviewPhase || 'after_artifact'}
                    {review.blocksDownstream && ' · 阻断下游'}
                  </div>
                </div>
                <div className="flex gap-2 flex-shrink-0">
                  <button
                    onClick={() => handleApprove(review.id)}
                    className="px-3 py-1 text-xs rounded-md bg-green-600 text-white hover:bg-green-700 transition-colors"
                  >
                    ✓ 通过
                  </button>
                  <button
                    onClick={() => handleReject(review.id)}
                    className="px-3 py-1 text-xs rounded-md bg-red-100 text-red-700 hover:bg-red-200 transition-colors"
                  >
                    ✗ 驳回
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Agent Plan Preview */}
      {plan && (
        <div className="rounded-xl border border-gray-200 bg-white overflow-hidden">
          <div className="px-4 py-3 bg-gray-50 border-b border-gray-200 flex items-center justify-between">
            <div>
              <h4 className="text-sm font-semibold text-gray-800">📋 AgentPlan</h4>
              <p className="text-xs text-gray-500 mt-0.5">{plan.goal}</p>
            </div>
            <span className="text-xs px-2 py-0.5 rounded-full bg-blue-100 text-blue-700">
              {plan.steps.length} steps
            </span>
          </div>
          <div className="divide-y divide-gray-100">
            {plan.steps.map((step, idx) => (
              <PlanStepItem
                key={step.id}
                step={step}
                index={idx}
                review={reviews.find(r => r.stepId === step.id)}
                expanded={expandedStep === step.id}
                onToggle={() => setExpandedStep(expandedStep === step.id ? null : step.id)}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function PlanStepItem({
  step,
  index,
  review,
  expanded,
  onToggle,
}: {
  step: AgentStep
  index: number
  review?: AgentReviewItem
  expanded: boolean
  onToggle: () => void
}) {
  const reviewStatus = review?.status
  const statusBadge = reviewStatus === 'APPROVED' ? '✅' :
    reviewStatus === 'REJECTED' ? '❌' :
    reviewStatus === 'PENDING' ? '⏳' : '⬜'

  return (
    <div className="px-4 py-2 hover:bg-gray-50 transition-colors">
      <button onClick={onToggle} className="w-full text-left flex items-center gap-3">
        <span className="text-xs text-gray-400 font-mono w-6">{index + 1}</span>
        <span className="text-sm">{statusBadge}</span>
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium text-gray-700 truncate">{step.tool}</div>
          <div className="text-xs text-gray-400 truncate">{step.intent}</div>
        </div>
        {step.dependsOn && step.dependsOn.length > 0 && (
          <span className="text-xs text-gray-400">
            ← {step.dependsOn.length} dep{step.dependsOn.length > 1 ? 's' : ''}
          </span>
        )}
      </button>
      {expanded && (
        <div className="mt-2 ml-9 p-2 rounded bg-gray-50 text-xs font-mono text-gray-600 space-y-1">
          {step.dependsOn && step.dependsOn.length > 0 && (
            <div>dependsOn: [{step.dependsOn.join(', ')}]</div>
          )}
          {step.expectedOutput && step.expectedOutput.length > 0 && (
            <div>expectedOutput: [{step.expectedOutput.join(', ')}]</div>
          )}
          {step.produceArtifact && <div>produceArtifact: true</div>}
          <div className="text-gray-400 mt-1">
            arguments: {JSON.stringify(step.arguments, null, 2)}
          </div>
        </div>
      )}
    </div>
  )
}
