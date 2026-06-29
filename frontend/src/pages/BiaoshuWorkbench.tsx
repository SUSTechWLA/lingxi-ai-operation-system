import { useCallback, useEffect, useMemo, useState } from 'react'
import { FiFileText, FiPlay, FiRefreshCw, FiCheck, FiX, FiAlertTriangle, FiClock, FiList, FiTool } from 'react-icons/fi'
import { getAgentRun, startAgentRun } from '../services/api'
import type { AgentPlan, AgentRun, AgentStep } from '../utils/types'

const BID_WORKFLOW_STAGES = [
  { key: 'parse', label: '解析招标文件', icon: '📄', tool: 'parse_bid_files' },
  { key: 'outline', label: '生成大纲', icon: '📋', tool: 'outline_generator' },
  { key: 'chapters', label: '编写章节', icon: '✍️', tool: 'chapter_writer' },
  { key: 'wordcheck', label: '字数检查', icon: '🔍', tool: 'chapter_word_checker' },
  { key: 'merge', label: '整合成稿', icon: '📦', tool: 'merge_chapters' },
  { key: 'word', label: '导出Word', icon: '📝', tool: 'convert_to_word' },
]

const SUPPORTED_BID_EXTENSIONS = ['.txt', '.docx', '.pdf', '.xlsx', '.xls']

const fileExtension = (path: string) => {
  const normalized = path.trim().toLowerCase()
  const dot = normalized.lastIndexOf('.')
  return dot >= 0 ? normalized.slice(dot) : ''
}

export default function BiaoshuWorkbench() {
  const [projectName, setProjectName] = useState('广惠高速改扩建')
  const [bidFilePath, setBidFilePath] = useState('e:\\lingxi\\tangying-ai-operation-system\\biaoshu-tools\\test_bid.txt')
  const [customMessage, setCustomMessage] = useState('')
  const [run, setRun] = useState<AgentRun | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [runLog, setRunLog] = useState<string[]>([])

  const plan = run?.plan as AgentPlan | undefined
  const steps = plan?.steps || []
  const isTerminalStatus = run?.status === 'SUCCESS' || run?.status === 'FAILED'

  const addLog = useCallback((msg: string) => {
    setRunLog(prev => [...prev, `[${new Date().toLocaleTimeString()}] ${msg}`])
  }, [])

  useEffect(() => {
    if (!run?.id || isTerminalStatus) return
    const timer = setInterval(async () => {
      try {
        const r = await getAgentRun(run.id)
        setRun(r)
        if (r.status === 'SUCCESS') addLog('运行完成')
        if (r.status === 'FAILED') addLog('运行失败')
      } catch { /* polling */ }
    }, 2000)
    return () => clearInterval(timer)
  }, [run?.id, isTerminalStatus, addLog])

  const handleStart = async () => {
    if (!bidFilePath.trim()) return
    const ext = fileExtension(bidFilePath)
    if (!SUPPORTED_BID_EXTENSIONS.includes(ext)) {
      const msg = `不支持的招标文件格式：${ext || '无扩展名'}。支持格式：${SUPPORTED_BID_EXTENSIONS.join(', ')}`
      setError(msg)
      addLog(`错误: ${msg}`)
      return
    }
    setLoading(true)
    setError(null)
    setRunLog([])

    const message = customMessage.trim()
      || `请解析招标文件并生成技术标文档。文件路径：${bidFilePath}，项目名称：${projectName || '未命名项目'}`

    addLog(`启动任务: ${message}`)

    try {
      const result = await startAgentRun({
        message,
        domain: 'bid_writing',
        mode: 'dynamic_agent',
        context: {
          filePath: bidFilePath,
          file_path: bidFilePath,
          projectName: projectName || '未命名项目',
          project_name: projectName || '未命名项目',
          output_dir: projectName || '未命名项目',
        },
      })
      addLog(`任务已创建: ${result.runId}`)
      const r = await getAgentRun(result.runId)
      setRun(r)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      setError(msg)
      addLog(`错误: ${msg}`)
    } finally {
      setLoading(false)
    }
  }

  const statusIcon = useMemo(() => {
    if (!run) return null
    switch (run.status) {
      case 'RUNNING': return <FiRefreshCw className="animate-spin text-blue-500" />
      case 'FAILED': return <FiX className="text-red-500" />
      case 'SUCCESS': return <FiCheck className="text-green-500" />
      default: return <FiCheck className="text-green-500" />
    }
  }, [run])

  const matchedStage = (step: AgentStep) => {
    return BID_WORKFLOW_STAGES.find(s => step.tool.includes(s.key) || s.tool.includes(step.tool))
  }

  return (
    <div className="flex-1 p-8 overflow-y-auto">
      <div className="max-w-5xl mx-auto">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-10 h-10 bg-amber-100 rounded-xl flex items-center justify-center text-lg">
              <FiFileText className="w-5 h-5 text-amber-600" />
            </div>
            <div>
              <h2 className="text-2xl font-bold text-gray-800">标书工作台</h2>
              <p className="text-sm text-gray-500">智能解析招标文件，自动生成技术标文档</p>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Left: Input Panel */}
          <div className="lg:col-span-1 space-y-4">
            <div className="bg-white rounded-2xl border border-gray-100 p-5">
              <h3 className="text-sm font-semibold text-gray-800 mb-4 flex items-center gap-2">
                <FiTool className="w-4 h-4 text-gray-500" />
                任务配置
              </h3>

              <div className="space-y-4">
                <label className="block">
                  <span className="text-xs font-semibold text-gray-600">项目名称</span>
                  <input
                    value={projectName}
                    onChange={e => setProjectName(e.target.value)}
                    className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm text-gray-700 outline-none focus:border-amber-400 focus:ring-1 focus:ring-amber-200"
                    placeholder="项目名称"
                  />
                </label>

                <label className="block">
                  <span className="text-xs font-semibold text-gray-600">招标文件路径</span>
                  <input
                    value={bidFilePath}
                    onChange={e => setBidFilePath(e.target.value)}
                    className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm font-mono text-gray-700 outline-none focus:border-amber-400 focus:ring-1 focus:ring-amber-200"
                    placeholder="e:\path\to\bid-file.pdf"
                  />
                </label>

                <label className="block">
                  <span className="text-xs font-semibold text-gray-600">自定义指令（可选）</span>
                  <textarea
                    value={customMessage}
                    onChange={e => setCustomMessage(e.target.value)}
                    rows={3}
                    className="mt-1.5 w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 outline-none focus:border-amber-400 focus:ring-1 focus:ring-amber-200 resize-none"
                    placeholder="留空使用默认解析流程"
                  />
                </label>

                <button
                  onClick={handleStart}
                  disabled={loading || !bidFilePath.trim()}
                  className="w-full h-11 rounded-xl bg-amber-500 text-white text-sm font-semibold hover:bg-amber-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2 transition-colors"
                >
                  {loading ? (
                    <><FiRefreshCw className="animate-spin" /> 任务执行中...</>
                  ) : (
                    <><FiPlay /> 开始解析</>
                  )}
                </button>
              </div>
            </div>

            {/* Workflow Stages */}
            <div className="bg-white rounded-2xl border border-gray-100 p-5">
              <h3 className="text-sm font-semibold text-gray-800 mb-3 flex items-center gap-2">
                <FiList className="w-4 h-4 text-gray-500" />
                执行流程
              </h3>
              <div className="space-y-1">
                {BID_WORKFLOW_STAGES.map((stage) => {
                  const step = steps.find(s => {
                    const m = matchedStage(s)
                    return m?.key === stage.key
                  })
                  return (
                    <div
                      key={stage.key}
                      className={`flex items-center gap-3 px-3 py-2 rounded-lg text-xs transition-colors ${
                        step ? 'bg-amber-50 text-amber-800' : 'text-gray-400'
                      }`}
                    >
                      <span className="w-5 text-center">{stage.icon}</span>
                      <span className="flex-1 font-medium">{stage.label}</span>
                      {step && <FiCheck className="w-3.5 h-3.5 text-green-500" />}
                    </div>
                  )
                })}
              </div>
            </div>
          </div>

          {/* Right: Output Panel */}
          <div className="lg:col-span-2 space-y-4">
            {/* Status */}
            <div className="bg-white rounded-2xl border border-gray-100 p-5">
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-sm font-semibold text-gray-800 flex items-center gap-2">
                  <FiClock className="w-4 h-4 text-gray-500" />
                  任务状态
                </h3>
                {run && (
                  <div className="flex items-center gap-2 text-xs text-gray-500">
                    {statusIcon}
                    <span className="font-medium">
                      {run.status === 'RUNNING' ? '运行中' : run.status === 'FAILED' ? '失败' : run.status === 'SUCCESS' ? '完成' : run.status}
                    </span>
                  </div>
                )}
              </div>

              {error && (
                <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 mb-4 flex items-start gap-2">
                  <FiAlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              {!run && !error && (
                <p className="text-sm text-gray-400 py-8 text-center">
                  输入招标文件路径，点击「开始解析」启动任务
                </p>
              )}

              {run && (
                <div className="space-y-2 text-xs">
                  <div className="flex justify-between py-1.5 border-b border-gray-50">
                    <span className="text-gray-500">任务ID</span>
                    <span className="font-mono text-gray-700">{run.id}</span>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-gray-50">
                    <span className="text-gray-500">领域</span>
                    <span className="text-gray-700">{run.domain || '-'}</span>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-gray-50">
                    <span className="text-gray-500">创建时间</span>
                    <span className="text-gray-700">{run.createdAt ? new Date(run.createdAt).toLocaleString() : '-'}</span>
                  </div>
                  {plan && (
                    <>
                      <div className="flex justify-between py-1.5 border-b border-gray-50">
                        <span className="text-gray-500">计划目标</span>
                        <span className="text-gray-700 text-right max-w-[60%]">{plan.goal}</span>
                      </div>
                      <div className="flex justify-between py-1.5">
                        <span className="text-gray-500">步骤数</span>
                        <span className="text-gray-700">{steps.length}</span>
                      </div>
                    </>
                  )}
                </div>
              )}
            </div>

            {/* Plan Steps */}
            {steps.length > 0 && (
              <div className="bg-white rounded-2xl border border-gray-100 p-5">
                <h3 className="text-sm font-semibold text-gray-800 mb-4 flex items-center gap-2">
                  <FiList className="w-4 h-4 text-gray-500" />
                  执行步骤
                </h3>
                <div className="space-y-3">
                  {steps.map((step, idx) => {
                    const stage = matchedStage(step)
                    return (
                      <div
                        key={step.id || idx}
                        className="rounded-xl border border-gray-100 bg-gray-50/70 p-4"
                      >
                        <div className="flex items-center gap-2 mb-2">
                          <span className="w-6 h-6 rounded-md bg-amber-100 text-amber-700 text-xs font-bold flex items-center justify-center">
                            {idx + 1}
                          </span>
                          <span className="text-sm font-semibold text-gray-800">
                            {stage?.label || step.tool}
                          </span>
                        </div>
                        <div className="space-y-1.5 ml-8">
                          {step.intent && (
                            <p className="text-xs text-gray-500">{step.intent}</p>
                          )}
                          <div className="flex flex-wrap gap-1">
                            <span className="inline-block px-2 py-0.5 rounded-md bg-gray-100 text-xs font-mono text-gray-600">
                              {step.tool}
                            </span>
                          </div>
                          {Object.keys(step.arguments).length > 0 && (
                            <div className="mt-2 text-xs text-gray-500 bg-white rounded-lg p-2 border border-gray-100 font-mono overflow-x-auto">
                              {Object.entries(step.arguments).map(([k, v]) => {
                                const val = typeof v === 'string' && v.length > 60
                                  ? v.slice(0, 60) + '...'
                                  : JSON.stringify(v)
                                return (
                                  <div key={k} className="flex gap-3">
                                    <span className="text-gray-400">{k}:</span>
                                    <span className="text-gray-600">{val}</span>
                                  </div>
                                )
                              })}
                            </div>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Run Log */}
            {runLog.length > 0 && (
              <div className="bg-white rounded-2xl border border-gray-100 p-5">
                <h3 className="text-sm font-semibold text-gray-800 mb-3">运行日志</h3>
                <div className="bg-gray-900 rounded-xl p-4 max-h-64 overflow-y-auto font-mono text-xs text-green-400 space-y-1">
                  {runLog.map((line, i) => (
                    <div key={i}>{line}</div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
