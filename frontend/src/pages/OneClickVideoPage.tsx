import { useState, useEffect, useCallback } from 'react'
import { FiPlay, FiCheck, FiX, FiEdit2, FiRefreshCw, FiArrowLeft, FiAlertCircle, FiZap } from 'react-icons/fi'
import { fetchWorkflowTemplates, instantiateWorkflow } from '../services/api'

// ---- Types ----

interface StageState {
  key: string
  label: string
  icon: string
  status: 'not_started' | 'running' | 'awaiting_human' | 'approved' | 'completed' | 'failed'
  artifact?: string
  reviewFocus?: string[]
}

interface PreflightResult {
  pipeline: string
  status: string
  canStart: boolean
  capabilityMenu: {
    localRunner: { available: boolean; reason?: string }
    compositionRuntime: { hyperframes: { available: boolean; reason?: string } }
    localTools: { command: string; available: boolean }[]
    warnings: string[]
  }
  blockers?: { code: string; message: string }[]
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

// ---- Component ----

const GuidedVideoStudio: React.FC = () => {
  const [topic, setTopic] = useState('')
  const [durationSec, setDurationSec] = useState(45)
  const [active, setActive] = useState(false)
  const [_taskId, setTaskId] = useState<string | null>(null)
  const [stages, setStages] = useState<StageState[]>(STAGES)
  const [currentStageIdx, setCurrentStageIdx] = useState(-1)
  const [preflight, setPreflight] = useState<PreflightResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [workflowId, setWorkflowId] = useState<string | null>(null)

  // Load guided workflow template
  useEffect(() => {
    fetchWorkflowTemplates()
      .then(templates => {
        const wf = templates.find(w => w.id === 'wf-guided-image-text-video')
        if (wf) setWorkflowId(wf.id)
      })
      .catch(() => {})
  }, [])

  // Preflight check
  const runPreflight = useCallback(async () => {
    try {
      const resp = await fetch('/api/video/preflight')
      if (resp.ok) {
        const data: PreflightResult = await resp.json()
        setPreflight(data)
        return data.canStart
      }
    } catch { /* preflight unavailable — proceed anyway */ }
    return true
  }, [])

  useEffect(() => { runPreflight() }, [runPreflight])

  const start = async () => {
    if (!topic.trim() || !workflowId) return
    const canStart = await runPreflight()
    if (!canStart) {
      setError('本地执行器未就绪，请启动桌面端并确保 HyperFrames 和 FFmpeg 可用。')
      return
    }
    setError(null)
    setActive(true)
    setStages(STAGES.map(s => ({ ...s, status: 'not_started' as const })))
    setCurrentStageIdx(0)

    // Update first stage to running
    updateStage(0, { status: 'running' })

    try {
      const result = await instantiateWorkflow(workflowId, { topic: topic.trim(), durationSec })
      setTaskId(result.taskId)
      // Stage 0 (proposal) is now awaiting review
      updateStage(0, { status: 'awaiting_human', artifact: 'video_proposal' })
    } catch (err: any) {
      setError(err?.message ?? '启动失败')
      updateStage(0, { status: 'failed' })
    }
  }

  const approveStage = (idx: number) => {
    updateStage(idx, { status: 'approved' })
    const nextIdx = idx + 1
    if (nextIdx < STAGES.length) {
      setCurrentStageIdx(nextIdx)
      updateStage(nextIdx, { status: 'running' })
      // For the render stage, show a special confirmation step
      if (STAGES[nextIdx].key === 'render') {
        // Stay in preview approved until user explicitly clicks render
        updateStage(nextIdx, { status: 'not_started' })
        setCurrentStageIdx(idx) // stay at preview
      }
    }
  }

  const rejectStage = (idx: number, reason?: string) => {
    updateStage(idx, { status: 'awaiting_human', artifact: undefined })
    setError(reason ?? '已驳回，请修改后重新提交')
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
    if (prevIdx >= 0) {
      setCurrentStageIdx(prevIdx)
    }
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
                  placeholder="例如：AI Agent 如何改变现代工作流"
                  rows={3}
                  className="w-full bg-white/5 border border-white/10 rounded-xl px-5 py-4 text-white placeholder-white/30 resize-none focus:outline-none focus:border-[#FFCF4A] focus:ring-1 focus:ring-[#FFCF4A]/30"
                  onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); start() } }}
                />
                <div className="flex items-center gap-4">
                  <select value={durationSec} onChange={e => setDurationSec(Number(e.target.value))}
                    className="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white">
                    <option value={30}>30 秒</option><option value={45}>45 秒</option><option value={60}>60 秒</option>
                  </select>
                  <button onClick={start} disabled={!topic.trim() || !workflowId || !canStartPipeline}
                    className="flex-1 flex items-center justify-center gap-2 bg-[#FFCF4A] text-[#2B1708] font-semibold py-3 px-8 rounded-xl hover:bg-[#e6ba3d] disabled:opacity-40 disabled:cursor-not-allowed transition-colors">
                    <FiPlay className="w-5 h-5" />开始创作
                  </button>
                </div>
              </div>
              {!canStartPipeline && preflight?.blockers && (
                <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/30 space-y-2">
                  <p className="text-sm font-semibold text-red-300">本地执行器未就绪</p>
                  {preflight.blockers.map(b => <p key={b.code} className="text-xs text-red-400">{b.message}</p>)}
                </div>
              )}
            </div>
          )}

          {active && (
            <div className="w-full max-w-3xl space-y-6">
              <h3 className="text-lg font-semibold">创作进度</h3>
              <div className="space-y-2">
                {stages.map((stage, idx) => {
                  const isCurrent = idx === currentStageIdx
                  const isPast = stage.status === 'approved' || stage.status === 'completed'
                  const isAwaiting = stage.status === 'awaiting_human'
                  return (
                    <div key={stage.key}
                      className={`p-4 rounded-xl border transition-colors ${
                        isCurrent ? 'border-[#FFCF4A]/50 bg-[#FFCF4A]/5' :
                        isPast ? 'border-green-500/20 bg-green-500/3' :
                        isAwaiting ? 'border-blue-500/30 bg-blue-500/5' :
                        'border-white/5 bg-white/[0.02]'
                      }`}>
                      <div className="flex items-center gap-3">
                        <span className="text-lg">{stage.icon}</span>
                        <div className="flex-1">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium">{stage.label}</span>
                            {statusBadge(stage.status)}
                          </div>
                          {stage.artifact && <p className="text-xs text-white/30 mt-0.5">已生成 {stage.artifact}</p>}
                        </div>
                        {isAwaiting && (
                          <div className="flex items-center gap-2">
                            <button onClick={() => approveStage(idx)}
                              className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-green-500/15 text-green-400 text-xs font-medium hover:bg-green-500/25 transition-colors">
                              <FiCheck className="w-3 h-3" />确认继续
                            </button>
                            <button onClick={() => rejectStage(idx)}
                              className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-red-500/15 text-red-400 text-xs font-medium hover:bg-red-500/25 transition-colors">
                              <FiX className="w-3 h-3" />驳回
                            </button>
                            <button onClick={() => {}}
                              className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-white/5 border border-white/10 text-white/60 text-xs hover:bg-white/10 transition-colors">
                              <FiEdit2 className="w-3 h-3" />编辑
                            </button>
                            <button onClick={() => {}}
                              className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-white/5 border border-white/10 text-white/60 text-xs hover:bg-white/10 transition-colors">
                              <FiRefreshCw className="w-3 h-3" />重新生成
                            </button>
                          </div>
                        )}
                        {isPast && (
                          <button onClick={() => goBack(idx)}
                            className="flex items-center gap-1 px-2 py-1 text-xs text-white/30 hover:text-white/60 transition-colors">
                            <FiArrowLeft className="w-3 h-3" />返回
                          </button>
                        )}
                      </div>
                      {/* Review focus hints */}
                      {isAwaiting && stage.reviewFocus && (
                        <div className="mt-3 pl-9 space-y-1">
                          {stage.reviewFocus.map(f => (
                            <p key={f} className="text-xs text-white/40">• {f}</p>
                          ))}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>

              {/* Render button — separate and prominent */}
              {stages.find(s => s.key === 'preview')?.status === 'approved' &&
               stages.find(s => s.key === 'render')?.status === 'not_started' && (
                <div className="p-6 rounded-xl border border-[#FFCF4A]/30 bg-[#FFCF4A]/5 text-center space-y-3">
                  <p className="text-sm text-white/60">所有创作阶段已确认，可以开始最终渲染</p>
                  <p className="text-xs text-white/30">最终渲染会调用本地 HyperFrames，可能耗时数分钟</p>
                  <button onClick={startRender}
                    className="inline-flex items-center gap-2 bg-[#FFCF4A] text-[#2B1708] font-semibold py-3 px-10 rounded-xl hover:bg-[#e6ba3d] transition-colors">
                    <FiPlay className="w-5 h-5" />开始最终渲染
                  </button>
                </div>
              )}

              {error && (
                <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/30 text-red-300 text-sm">{error}</div>
              )}
            </div>
          )}
        </main>
      </div>
    </div>
  )
}

const PreflightStatus: React.FC<{ result: PreflightResult | null }> = ({ result }) => {
  if (!result) return <span className="text-xs text-white/20">检测中...</span>
  const runner = result.capabilityMenu.localRunner.available
  const hf = result.capabilityMenu.compositionRuntime.hyperframes.available
  return (
    <div className="flex items-center gap-3">
      <span className={`flex items-center gap-1 text-xs ${runner ? 'text-green-400' : 'text-red-400'}`}>
        <div className={`w-1.5 h-1.5 rounded-full ${runner ? 'bg-green-400' : 'bg-red-400'}`} />Runner
      </span>
      <span className={`flex items-center gap-1 text-xs ${hf ? 'text-green-400' : 'text-red-400'}`}>
        <div className={`w-1.5 h-1.5 rounded-full ${hf ? 'bg-green-400' : 'bg-red-400'}`} />HyperFrames
      </span>
    </div>
  )
}

export default GuidedVideoStudio
