import { useState, useEffect, useCallback } from 'react'
import { FiPlay, FiCheck, FiX, FiEdit2, FiRefreshCw, FiArrowLeft, FiAlertCircle, FiZap } from 'react-icons/fi'
import {
  startAgentRun, getAgentRunReviews, approveAgentReview, rejectAgentReview,
  submitEditedArtifact, regenerateAgentStage, fetchVideoPreflight
} from '../services/api'
import type { PreflightResponse } from '../services/api'
import type { AgentReviewItem } from '../utils/types'

// ---- Types ----

interface StageState {
  key: string
  label: string
  icon: string
  status: 'not_started' | 'running' | 'awaiting_human' | 'approved' | 'completed' | 'failed'
  artifact?: string
  reviewFocus?: string[]
  reviewId?: string // backend review node ID
}

// ---- Constants ----

const STAGES: StageState[] = [
  { key: 'proposal', label: '创作方案', icon: '📋', status: 'not_started', reviewFocus: ['主题是否准确', '目标时长是否合理', '视频结构是否清楚', '是否适合图文视频'] },
  { key: 'script', label: '口播脚本', icon: '📝', status: 'not_started', reviewFocus: ['开头是否有吸引力', '口播是否自然', '内容是否准确', '时长是否匹配'] },
  { key: 'composition', label: '视频结构', icon: '🎬', status: 'not_started', reviewFocus: ['卡片顺序是否合理', '每页文字是否过长', '时间轴是否覆盖全片', '是否适合 HyperFrames'] },
  { key: 'preview', label: '画面预览', icon: '👁️', status: 'not_started', reviewFocus: ['画面是否可读', '文字是否溢出', '卡片顺序是否正确', '是否允许进入最终渲染'] },
  { key: 'render', label: '最终渲染', icon: '🎥', status: 'not_started' },
  { key: 'package', label: '导出项目', icon: '📦', status: 'not_started' },
]

function reviewIdFor(stages: StageState[], stageKey: string): string | undefined {
  const s = stages.find(s => s.key === stageKey)
  return s?.reviewId
}

// ---- Component ----

const GuidedVideoStudio: React.FC = () => {
  const [topic, setTopic] = useState('')
  const [durationSec, setDurationSec] = useState(45)
  const [active, setActive] = useState(false)
  const [runId, setRunId] = useState<string | null>(null)
  const [stages, setStages] = useState<StageState[]>(STAGES)
  const [currentStageIdx, setCurrentStageIdx] = useState(-1)
  const [preflight, setPreflight] = useState<PreflightResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  // Preflight check
  useEffect(() => {
    fetchVideoPreflight('wf-guided-image-text-video')
      .then(setPreflight)
      .catch(() => {})
  }, [])

  // Poll reviews while active
  useEffect(() => {
    if (!active || !runId) return
    const timer = setInterval(() => {
      getAgentRunReviews(runId).then(data => {
        if (!data?.reviews) return
        syncReviewsToStages(data.reviews)
      }).catch(() => {})
    }, 2000)
    return () => clearInterval(timer)
  }, [active, runId])

  // Sync backend review states into local stage state
  const syncReviewsToStages = useCallback((reviews: AgentReviewItem[]) => {
    setStages(prev => {
      let changed = false
      const next = prev.map(s => {
        // Match reviews to stages by stepId or stage name
        const review = reviews.find(r => {
          const stepId = r.stepId ?? ''
          const tool = r.tool ?? ''
          return stepId.includes(s.key) || tool.includes(s.key)
        })
        if (!review) {
          if (s.status === 'awaiting_human') {
            changed = true
            return { ...s, reviewId: undefined }
          }
          return s
        }
        const newStatus = review.status === 'PENDING' ? 'awaiting_human'
          : review.status === 'APPROVED' ? 'approved'
          : review.status === 'REJECTED' ? 'awaiting_human' // allow re-do
          : s.status
        if (newStatus !== s.status || s.reviewId !== review.nodeId) {
          changed = true
          return { ...s, status: newStatus, reviewId: review.nodeId }
        }
        return s
      })
      return changed ? next : prev
    })
  }, [])

  const start = async () => {
    if (!topic.trim()) return
    setError(null)
    setActive(true)
    setStages(STAGES.map(s => ({ ...s, status: 'not_started' as const })))
    setCurrentStageIdx(0)
    updateStage(0, { status: 'running' })

    try {
      const result = await startAgentRun({
        message: `创作一个${durationSec}秒的图文视频：${topic.trim()}`,
        domain: 'video_creation',
        mode: 'dynamic_agent',
        context: { topic: topic.trim(), durationSec }
      })
      setRunId(result.runId)
      updateStage(0, { status: 'awaiting_human' })
    } catch (err: any) {
      setError(err?.message ?? '启动失败')
      updateStage(0, { status: 'failed' })
    }
  }

  const approveStage = async (idx: number) => {
    const stage = stages[idx]
    const reviewId = reviewIdFor(stages, stage.key)
    if (!reviewId || !runId) return

    try {
      await approveAgentReview(runId, reviewId)
      updateStage(idx, { status: 'approved' })
      // Advance to next stage
      const nextIdx = idx + 1
      if (nextIdx < STAGES.length) {
        if (STAGES[nextIdx].key === 'render') {
          // Render needs explicit user trigger; don't auto-advance
          updateStage(nextIdx, { status: 'not_started' })
          setCurrentStageIdx(idx)
        } else {
          setCurrentStageIdx(nextIdx)
          updateStage(nextIdx, { status: 'running' })
        }
      }
    } catch (err: any) {
      setError(err?.message ?? '审批失败')
    }
  }

  const rejectStage = async (idx: number, reason?: string) => {
    const stage = stages[idx]
    const reviewId = reviewIdFor(stages, stage.key)
    if (!reviewId || !runId) return

    try {
      await rejectAgentReview(runId, reviewId, reason)
      updateStage(idx, { status: 'awaiting_human' })
      setError(reason ?? '已驳回，请修改后重新提交')
    } catch (err: any) {
      setError(err?.message ?? '驳回失败')
    }
  }

  const handleSubmitEdited = async (idx: number, content: unknown) => {
    const stage = stages[idx]
    const reviewId = reviewIdFor(stages, stage.key)
    if (!reviewId || !runId) return
    try {
      await submitEditedArtifact(runId, reviewId, content)
      updateStage(idx, { status: 'approved' })
    } catch (err: any) {
      setError(err?.message ?? '提交编辑失败')
    }
  }

  const handleRegenerate = async (idx: number) => {
    const stage = stages[idx]
    const reviewId = reviewIdFor(stages, stage.key)
    if (!reviewId || !runId) return
    try {
      await regenerateAgentStage(runId, reviewId)
      updateStage(idx, { status: 'running' })
      setError(null)
    } catch (err: any) {
      setError(err?.message ?? '重新生成失败')
    }
  }

  const startRender = () => {
    const renderIdx = STAGES.findIndex(s => s.key === 'render')
    if (renderIdx >= 0) {
      setCurrentStageIdx(renderIdx)
      updateStage(renderIdx, { status: 'running' })
    }
  }

  const updateStage = (idx: number, partial: Partial<StageState>) => {
    setStages(prev => prev.map((s, i) => i === idx ? { ...s, ...partial } : s))
  }

  const goBack = (idx: number) => {
    const prevIdx = idx - 1
    if (prevIdx >= 0) setCurrentStageIdx(prevIdx)
  }

  const statusBadge = (status: string) => {
    switch (status) {
      case 'running': return <span className="px-2 py-0.5 rounded-full text-xs bg-yellow-500/15 text-yellow-400">执行中</span>
      case 'awaiting_human': return <span className="px-2 py-0.5 rounded-full text-xs bg-blue-500/15 text-blue-400">待确认</span>
      case 'approved': return <FiCheck className="text-green-400" />
      case 'completed': return <FiCheck className="text-green-400" />
      case 'failed': return <FiAlertCircle className="text-red-400" />
      default: return <div className="w-3 h-3 rounded-full border-2 border-gray-500" />
    }
  }

  const canStartPipeline = preflight?.canStart !== false

  return (
    <div className="flex flex-col h-screen bg-[#0f0f1a] text-white">
      <header className="flex items-center justify-between px-8 py-4 border-b border-white/10">
        <div className="flex items-center gap-3">
          <FiZap className="w-6 h-6 text-[#FFCF4A]" />
          <h1 className="text-xl font-semibold">Guided Video Studio</h1>
          <span className="text-xs px-2 py-0.5 rounded bg-[#FFCF4A]/20 text-[#FFCF4A]">引导式视频创作</span>
        </div>
        <div className="flex items-center gap-4 text-sm">
          <PreflightStatus result={preflight} />
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        <main className="flex-1 flex flex-col items-center justify-center px-8 py-12">
          {!active && (
            <div className="w-full max-w-2xl space-y-8">
              <div className="text-center space-y-3">
                <h2 className="text-3xl font-bold">一句话启动视频创作</h2>
                <p className="text-white/50">分阶段确认：方案 → 脚本 → 结构 → 预览 → 渲染 → 导出</p>
              </div>

              <div className="space-y-4">
                <textarea value={topic} onChange={e => setTopic(e.target.value)}
                  placeholder="例如：做一个 45 秒的视频，讲 AI Agent 为什么改变的是工作流..."
                  className="w-full h-32 px-5 py-4 bg-white/5 border border-white/10 rounded-xl text-white placeholder-white/30 resize-none focus:outline-none focus:border-[#FFCF4A]/50" />
                <div className="flex items-center gap-4">
                  <label className="text-sm text-white/50">目标时长</label>
                  <select value={durationSec} onChange={e => setDurationSec(Number(e.target.value))}
                    className="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white">
                    {[30, 45, 60, 90, 120].map(d => <option key={d} value={d}>{d} 秒</option>)}
                  </select>
                </div>
                <button onClick={start} disabled={!canStartPipeline || !topic.trim()}
                  className="w-full py-3.5 rounded-xl font-semibold text-base flex items-center justify-center gap-2 transition-all
                    bg-[#FFCF4A] text-black hover:bg-[#e0b635] disabled:opacity-30 disabled:cursor-not-allowed">
                  <FiZap className="w-5 h-5" /> 启动项目
                </button>
                {!canStartPipeline && preflight && (
                  <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm">
                    <PreflightBlockers result={preflight} />
                  </div>
                )}
              </div>
            </div>
          )}

          {active && (
            <div className="w-full max-w-3xl space-y-6">
              {stages.map((stage, idx) => (
                <div key={stage.key}
                  className={`flex items-center gap-4 p-4 rounded-xl border transition-all
                    ${idx === currentStageIdx ? 'border-[#FFCF4A]/50 bg-[#FFCF4A]/5' : 'border-white/5 bg-white/[0.02]'}
                    ${stage.status === 'approved' ? 'border-green-500/20 bg-green-500/5' : ''}`}>
                  <div className="w-8 h-8 flex items-center justify-center text-lg">{stage.icon}</div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{stage.label}</span>
                      {statusBadge(stage.status)}
                    </div>
                    {stage.status === 'awaiting_human' && stage.reviewFocus && (
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        {stage.reviewFocus.map((f, i) => (
                          <span key={i} className="px-2 py-0.5 rounded text-xs bg-white/5 text-white/50">{f}</span>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    {stage.status === 'awaiting_human' && idx === currentStageIdx && (
                      <>
                        <button onClick={() => approveStage(idx)}
                          className="p-2 rounded-lg bg-green-500/15 text-green-400 hover:bg-green-500/25" title="确认通过">
                          <FiCheck className="w-4 h-4" />
                        </button>
                        <button onClick={() => rejectStage(idx)}
                          className="p-2 rounded-lg bg-red-500/15 text-red-400 hover:bg-red-500/25" title="驳回修改">
                          <FiX className="w-4 h-4" />
                        </button>
                        <button onClick={() => handleSubmitEdited(idx, topic)}
                          className="p-2 rounded-lg bg-blue-500/15 text-blue-400 hover:bg-blue-500/25" title="编辑后提交">
                          <FiEdit2 className="w-4 h-4" />
                        </button>
                        <button onClick={() => handleRegenerate(idx)}
                          className="p-2 rounded-lg bg-purple-500/15 text-purple-400 hover:bg-purple-500/25" title="重新生成">
                          <FiRefreshCw className="w-4 h-4" />
                        </button>
                      </>
                    )}
                    {stage.status === 'approved' && idx > 0 && (
                      <button onClick={() => goBack(idx)}
                        className="p-2 rounded-lg bg-white/5 text-white/40 hover:bg-white/10" title="返回">
                        <FiArrowLeft className="w-4 h-4" />
                      </button>
                    )}
                  </div>
                </div>
              ))}

              {/* Explicit Render trigger */}
              {(() => {
                const previewStage = stages.find(s => s.key === 'preview')
                const renderStage = stages.find(s => s.key === 'render')
                if (previewStage?.status === 'approved' && renderStage?.status === 'not_started') {
                  return (
                    <div className="flex justify-center pt-4">
                      <button onClick={startRender}
                        className="px-8 py-3.5 rounded-xl font-semibold text-base flex items-center gap-2
                          bg-gradient-to-r from-[#FFCF4A] to-[#FFA726] text-black hover:opacity-90 transition-all shadow-lg shadow-[#FFCF4A]/25">
                        <FiPlay className="w-5 h-5" /> 开始最终渲染
                      </button>
                      <p className="text-xs text-white/30 mt-2 text-center">
                        最终渲染会调用本地 HyperFrames，可能耗时数分钟。
                      </p>
                    </div>
                  )
                }
                return null
              })()}

              {error && (
                <div className="flex items-center gap-2 p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm">
                  <FiAlertCircle className="w-4 h-4 shrink-0" />
                  <span>{error}</span>
                </div>
              )}
            </div>
          )}
        </main>
      </div>
    </div>
  )
}

// ---- Sub Components ----

const PreflightStatus: React.FC<{ result: PreflightResponse | null }> = ({ result }) => {
  if (!result) return <span className="text-white/30">检测中…</span>
  if (result.status === 'passed') {
    return <span className="flex items-center gap-1 text-green-400"><FiCheck className="w-3 h-3" /> 环境就绪</span>
  }
  return <span className="flex items-center gap-1 text-red-400"><FiAlertCircle className="w-3 h-3" /> 环境未就绪</span>
}

const PreflightBlockers: React.FC<{ result: PreflightResponse }> = ({ result }) => {
  if (!result.blockers?.length) return null
  return (
    <div className="space-y-1">
      {result.blockers.map((b, i) => (
        <div key={i} className="flex items-center gap-2">
          <FiAlertCircle className="w-3 h-3 shrink-0" />
          <span>{b.message}</span>
        </div>
      ))}
    </div>
  )
}

export default GuidedVideoStudio
