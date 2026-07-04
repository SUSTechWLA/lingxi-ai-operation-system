import { useCallback, useEffect, useMemo, useState } from 'react'
import { FiArchive, FiCopy, FiEye, FiFileText, FiPlay, FiRefreshCw, FiCheck, FiX, FiAlertTriangle, FiClock, FiList, FiTool, FiFolder, FiSearch, FiSend, FiZap } from 'react-icons/fi'
import ReactMarkdown from 'react-markdown'
import { getAgentRun, getAgentRunReviews, getAgentRunTrace, readBiaoshuArtifact, reviseBiaoshuArtifact, startAgentRun, generateBidAnalysisReport, type BiaoshuReviseRequest } from '../services/api'
import { 
  fetchBiaoshuConversation, 
  fetchBiaoshuProjects, 
  saveBiaoshuProject, 
  sendBiaoshuConversationMessage, 
  writeLocalBiaoshuArtifact,
  type BiaoshuConversationMessage,
  type BiaoshuProjectStatus, 
  type LocalBiaoshuProject,
} from '../services/localAgent'
import type { AgentPlan, AgentReviewItem, AgentRun, AgentStep } from '../utils/types'
import {
  biaoshuArtifactToCopyText,
  buildBiaoshuArtifacts,
  createManualReportArtifact,
  displayNameForBiaoshuArtifact,
  mergeManualReportArtifact,
  type BiaoshuArtifactRecord,
  type BiaoshuArtifactStatus,
} from './biaoshuArtifactLogic'

const BID_WORKFLOW_STAGES = [
  { key: 'parse', label: '解析招标文件', icon: '📄', tool: 'parse_bid_files' },
  { key: 'outline', label: '生成大纲', icon: '📋', tool: 'outline_generator' },
  { key: 'chapters', label: '编写章节', icon: '✍️', tool: 'chapter_writer' },
  { key: 'wordcheck', label: '字数检查', icon: '🔍', tool: 'chapter_word_checker' },
  { key: 'merge', label: '整合成稿', icon: '📦', tool: 'merge_chapters' },
  { key: 'word', label: '导出Word', icon: '📝', tool: 'convert_to_word' },
]

const SUPPORTED_BID_EXTENSIONS = ['.txt', '.docx', '.pdf', '.xlsx', '.xls']
const DEFAULT_PROJECT_NAME = '广惠高速改扩建'
const DEFAULT_BID_FILE_PATH = 'e:\\lingxi\\tangying-ai-operation-system\\biaoshu-tools\\test_bid.txt'

type BiaoshuView = 'workbench' | 'artifacts' | 'history'
type BiaoshuProjectHistoryItem = LocalBiaoshuProject

const fileExtension = (path: string) => {
  const normalized = path.trim().toLowerCase()
  const dot = normalized.lastIndexOf('.')
  return dot >= 0 ? normalized.slice(dot) : ''
}

const statusText = (status: BiaoshuProjectStatus) => {
  if (status === 'CREATED') return '已创建'
  if (status === 'RUNNING') return '运行中'
  if (status === 'FAILED') return '失败'
  if (status === 'SUCCESS') return '完成'
  if (status === 'UNKNOWN') return '未知'
  return status
}

const extractProjectNameFromRun = (run: AgentRun | null, fallback: string) => {
  const source = run as (AgentRun & { context?: Record<string, unknown> }) | null
  const context = source?.context || source?.metadata
  const contextName = context?.projectName || context?.project_name || context?.output_dir
  return typeof contextName === 'string' && contextName.trim() ? contextName : fallback
}

const extractBidFilePathFromRun = (run: AgentRun | null, fallback: string) => {
  const source = run as (AgentRun & { context?: Record<string, unknown> }) | null
  const context = source?.context || source?.metadata
  const contextPath = context?.filePath || context?.file_path || context?.bidFilePath
  return typeof contextPath === 'string' && contextPath.trim() ? contextPath : fallback
}

export default function BiaoshuWorkbench() {
  const [projectName, setProjectName] = useState(DEFAULT_PROJECT_NAME)
  const [bidFilePath, setBidFilePath] = useState(DEFAULT_BID_FILE_PATH)
  const [customMessage, setCustomMessage] = useState('')
  const [run, setRun] = useState<AgentRun | null>(null)
  const [trace, setTrace] = useState<unknown>(null)
  const [reviews, setReviews] = useState<AgentReviewItem[]>([])
  const [manualReportArtifact, setManualReportArtifact] = useState<BiaoshuArtifactRecord | null>(null)
  const [activeView, setActiveView] = useState<BiaoshuView>('workbench')
  const [projectHistory, setProjectHistory] = useState<BiaoshuProjectHistoryItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [runLog, setRunLog] = useState<string[]>([])

  const plan = run?.plan as AgentPlan | undefined
  const steps = plan?.steps || []
  const isTerminalStatus = run?.status === 'SUCCESS' || run?.status === 'FAILED'
  const baseArtifacts = useMemo(() => buildBiaoshuArtifacts(run, trace, reviews), [run, trace, reviews])
  const artifacts = useMemo(
    () => mergeManualReportArtifact(baseArtifacts, manualReportArtifact),
    [baseArtifacts, manualReportArtifact],
  )

  const addLog = useCallback((msg: string) => {
    setRunLog(prev => [...prev, `[${new Date().toLocaleTimeString()}] ${msg}`])
  }, [])

  const handleReportGenerated = useCallback((artifact: Record<string, unknown> | undefined, reportPath: string, sourceFile: string) => {
    setManualReportArtifact(createManualReportArtifact(artifact, reportPath, sourceFile))
    addLog(`解析报告已生成: ${reportPath}`)
  }, [addLog])

  const refreshRunData = useCallback(async (runId: string) => {
    const [nextRun, nextTrace, nextReviews] = await Promise.all([
      getAgentRun(runId),
      getAgentRunTrace(runId).catch(() => null),
      getAgentRunReviews(runId).catch(() => ({ runId, reviews: [] })),
    ])
    const reviewItems = nextReviews.reviews || []
    setRun(nextRun)
    setTrace(nextTrace)
    setReviews(reviewItems)
    return { run: nextRun, trace: nextTrace, reviews: reviewItems }
  }, [])

  const saveHistoryFromRun = useCallback(async (nextRun: AgentRun, options?: { projectName?: string; bidFilePath?: string; trace?: unknown; reviews?: AgentReviewItem[] }) => {
    const nextArtifacts = buildBiaoshuArtifacts(nextRun, options?.trace ?? trace, options?.reviews ?? reviews)
    const historyItem: BiaoshuProjectHistoryItem = {
      runId: nextRun.id,
      projectName: options?.projectName || extractProjectNameFromRun(nextRun, projectName || '未命名项目'),
      bidFilePath: options?.bidFilePath || extractBidFilePathFromRun(nextRun, bidFilePath),
      status: (nextRun.status || 'UNKNOWN') as BiaoshuProjectStatus,
      createdAt: nextRun.createdAt || new Date().toISOString(),
      updatedAt: nextRun.updatedAt || new Date().toISOString(),
      artifactCount: nextArtifacts.length,
      validArtifactCount: nextArtifacts.filter((artifact) => artifact.status === 'valid').length,
    }
    const response = await saveBiaoshuProject(historyItem)
    setProjectHistory(response.projects)
  }, [bidFilePath, projectName, reviews, trace])

  // 页面加载时从本地后端恢复历史项目
  useEffect(() => {
    fetchBiaoshuProjects().then(async (response) => {
      setProjectHistory(response.projects)
      const latest = response.projects[0]
      if (!latest) return
      setProjectName(latest.projectName || DEFAULT_PROJECT_NAME)
      setBidFilePath(latest.bidFilePath || DEFAULT_BID_FILE_PATH)
      addLog(`恢复最近标书项目: ${latest.projectName}`)
      try {
        const snapshot = await refreshRunData(latest.runId)
        await saveHistoryFromRun(snapshot.run, {
          projectName: latest.projectName,
          bidFilePath: latest.bidFilePath,
          trace: snapshot.trace,
          reviews: snapshot.reviews,
        })
        if (snapshot.run.status === 'SUCCESS' || snapshot.run.status === 'FAILED') setActiveView('artifacts')
      } catch {
        addLog('最近标书项目的云端任务暂不可读取，可从历史项目列表稍后重试')
      }
    }).catch((err: unknown) => {
      addLog(`本地标书项目库读取失败: ${err instanceof Error ? err.message : String(err)}`)
    })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!run?.id || isTerminalStatus) return
    const timer = setInterval(async () => {
      try {
        const snapshot = await refreshRunData(run.id)
        await saveHistoryFromRun(snapshot.run, { trace: snapshot.trace, reviews: snapshot.reviews })
        if (snapshot.run.status === 'SUCCESS') addLog('运行完成')
        if (snapshot.run.status === 'FAILED') addLog('运行失败')
      } catch { /* polling */ }
    }, 2000)
    return () => clearInterval(timer)
  }, [run?.id, isTerminalStatus, addLog, refreshRunData])

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
    setTrace(null)
    setReviews([])
    setManualReportArtifact(null)

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
      const snapshot = await refreshRunData(result.runId)
      await saveHistoryFromRun(snapshot.run, {
        projectName: projectName || '未命名项目',
        bidFilePath,
        trace: snapshot.trace,
        reviews: snapshot.reviews,
      })
      setActiveView('artifacts')
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

  const handleOpenHistoryItem = async (item: BiaoshuProjectHistoryItem) => {
    setLoading(true)
    setError(null)
    try {
      setProjectName(item.projectName)
      setBidFilePath(item.bidFilePath)
      setManualReportArtifact(null)
      const snapshot = await refreshRunData(item.runId)
      await saveHistoryFromRun(snapshot.run, {
        projectName: item.projectName,
        bidFilePath: item.bidFilePath,
        trace: snapshot.trace,
        reviews: snapshot.reviews,
      })
      addLog(`打开历史项目: ${item.projectName}`)
      setActiveView('artifacts')
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      setError(msg)
      addLog(`历史项目读取失败: ${msg}`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex-1 p-8 overflow-y-auto">
      <div className="max-w-6xl mx-auto">
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

        <div className="mb-6 flex rounded-xl border border-amber-100 bg-white/80 p-1 shadow-sm">
          <button
            onClick={() => setActiveView('workbench')}
            className={`flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold transition-colors ${activeView === 'workbench' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-500 hover:bg-amber-50 hover:text-amber-700'}`}
          >
            <FiTool /> 工作台
          </button>
          <button
            onClick={() => setActiveView('artifacts')}
            className={`flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold transition-colors ${activeView === 'artifacts' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-500 hover:bg-amber-50 hover:text-amber-700'}`}
          >
            <FiArchive /> 产物
          </button>
          <button
            onClick={() => setActiveView('history')}
            className={`flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold transition-colors ${activeView === 'history' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-500 hover:bg-amber-50 hover:text-amber-700'}`}
          >
            <FiFolder /> 历史项目
          </button>
        </div>

        {activeView === 'artifacts' ? (
          <BiaoshuArtifactsPage
            artifacts={artifacts}
            run={run}
            onGoWorkbench={() => setActiveView('workbench')}
            onReportGenerated={handleReportGenerated}
          />
        ) : activeView === 'history' ? (
          <BiaoshuProjectHistoryPage
            currentRunId={run?.id}
            history={projectHistory}
            loading={loading}
            onOpenProject={handleOpenHistoryItem}
            onGoWorkbench={() => setActiveView('workbench')}
          />
        ) : (
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
        )}
      </div>
    </div>
  )
}

function BiaoshuProjectHistoryPage({
  currentRunId,
  history,
  loading,
  onOpenProject,
  onGoWorkbench,
}: {
  currentRunId?: string
  history: BiaoshuProjectHistoryItem[]
  loading: boolean
  onOpenProject: (item: BiaoshuProjectHistoryItem) => void
  onGoWorkbench: () => void
}) {
  const [keyword, setKeyword] = useState('')
  const normalizedKeyword = keyword.trim().toLowerCase()
  const filteredHistory = normalizedKeyword
    ? history.filter((item) =>
      [item.projectName, item.bidFilePath, item.runId, item.status]
        .join(' ')
        .toLowerCase()
        .includes(normalizedKeyword),
    )
    : history
  const completedCount = history.filter((item) => item.status === 'SUCCESS').length
  const runningCount = history.filter((item) => item.status === 'RUNNING' || item.status === 'CREATED').length

  return (
    <div className="space-y-5">
      <section className="card p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <p className="text-sm font-bold text-primary-dark">历史项目</p>
            <h2 className="mt-2 text-3xl font-black text-ink">以往标书项目</h2>
            <p className="mt-2 text-sm leading-6 text-ink-muted">
              本机保存的标书任务索引，包含项目名称、招标文件路径、运行状态和产物生成进度。
            </p>
          </div>
          <button
            onClick={onGoWorkbench}
            className="shrink-0 rounded-lg bg-white px-4 py-2 text-sm font-bold text-primary-dark ring-1 ring-line hover:bg-primary-soft"
          >
            新建标书任务
          </button>
        </div>
        <div className="mt-5 grid grid-cols-1 gap-3 md:grid-cols-4">
          <BiaoshuMetric label="项目总数" value={String(history.length)} />
          <BiaoshuMetric label="已完成" value={String(completedCount)} />
          <BiaoshuMetric label="进行中" value={String(runningCount)} />
          <BiaoshuMetric label="当前任务" value={currentRunId ? currentRunId.slice(0, 8) : '-'} />
        </div>
      </section>

      <div className="flex items-center gap-3 rounded-lg bg-white/80 px-4 py-3 ring-1 ring-line">
        <FiSearch className="shrink-0 text-ink-soft" />
        <input
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-ink-soft"
          placeholder="搜索项目名、文件路径、Run ID"
        />
      </div>

      {history.length === 0 ? (
        <div className="rounded-lg bg-amber-50 p-5 text-sm font-semibold text-primary-dark ring-1 ring-amber-200">
          暂无历史项目。启动一次标书任务后，这里会自动记录。
        </div>
      ) : filteredHistory.length === 0 ? (
        <div className="rounded-lg bg-white/75 p-5 text-sm font-semibold text-ink-muted ring-1 ring-line">
          没有匹配的历史项目。
        </div>
      ) : (
        <section className="card overflow-hidden p-0">
          <table className="w-full text-left text-sm">
            <thead className="bg-background-mist text-xs text-ink-soft">
              <tr>
                <th className="whitespace-nowrap px-4 py-3">项目</th>
                <th className="whitespace-nowrap px-4 py-3">状态</th>
                <th className="whitespace-nowrap px-4 py-3">产物</th>
                <th className="whitespace-nowrap px-4 py-3">最近更新</th>
                <th className="whitespace-nowrap px-4 py-3">Run ID</th>
                <th className="whitespace-nowrap px-4 py-3">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line bg-white/70">
              {filteredHistory.map((item) => (
                <tr key={item.runId} className={item.runId === currentRunId ? 'bg-amber-50/70' : undefined}>
                  <td className="min-w-[260px] px-4 py-3">
                    <div className="font-semibold text-ink">{item.projectName || '未命名项目'}</div>
                    <div className="mt-1 max-w-md truncate font-mono text-xs text-ink-soft" title={item.bidFilePath}>
                      {item.bidFilePath || '-'}
                    </div>
                  </td>
                  <td className="whitespace-nowrap px-4 py-3">
                    <BiaoshuRunStatusBadge status={item.status} />
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-ink-muted">
                    {typeof item.validArtifactCount === 'number' && typeof item.artifactCount === 'number'
                      ? `${item.validArtifactCount}/${item.artifactCount}`
                      : '-'}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-ink-muted">{formatHistoryDate(item.updatedAt)}</td>
                  <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-ink-muted">{item.runId.slice(0, 12)}</td>
                  <td className="whitespace-nowrap px-4 py-3">
                    <button
                      onClick={() => onOpenProject(item)}
                      disabled={loading}
                      className="inline-flex items-center gap-1.5 rounded-lg bg-primary-soft px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-primary-200 hover:bg-primary-100 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {loading ? <FiRefreshCw className="animate-spin" /> : <FiEye />}
                      查看产物
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </div>
  )
}

function BiaoshuRunStatusBadge({ status }: { status: BiaoshuProjectStatus }) {
  const tone = status === 'SUCCESS'
    ? 'bg-green-50 text-green-700 ring-green-200'
    : status === 'RUNNING' || status === 'CREATED'
      ? 'bg-blue-50 text-blue-700 ring-blue-200'
      : status === 'FAILED'
        ? 'bg-red-50 text-red-700 ring-red-200'
        : 'bg-stone-50 text-stone-600 ring-stone-200'
  return <span className={`inline-flex items-center rounded-full px-2.5 py-1 text-xs font-bold ring-1 ${tone}`}>{statusText(status)}</span>
}

function formatHistoryDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function BiaoshuArtifactsPage({
  artifacts,
  run,
  onGoWorkbench,
  onReportGenerated,
}: {
  artifacts: BiaoshuArtifactRecord[]
  run: AgentRun | null
  onGoWorkbench: () => void
  onReportGenerated: (artifact: Record<string, unknown> | undefined, reportPath: string, sourceFile: string) => void
}) {
  const validCount = artifacts.filter((artifact) => artifact.status === 'valid').length
  const activeCount = artifacts.filter((artifact) => artifact.status === 'running' || artifact.status === 'review').length
  const blockedCount = artifacts.filter((artifact) => artifact.status === 'failed' || artifact.status === 'missing').length

  const [viewingArtifact, setViewingArtifact] = useState<BiaoshuArtifactRecord | null>(null)
  const [viewContent, setViewContent] = useState<string>('')
  const [viewFormat, setViewFormat] = useState<string>('')
  const [viewLoading, setViewLoading] = useState(false)
  const [viewError, setViewError] = useState<string | null>(null)

  // Bid analysis report generation state
  const [generatingReport, setGeneratingReport] = useState(false)
  const [generateError, setGenerateError] = useState<string | null>(null)

  const rawTextArtifact = artifacts.find((a) => a.kind === 'BID_RAW_TEXT' && a.status === 'valid')
  const analysisArtifact = artifacts.find((a) => a.kind === 'BID_ANALYSIS')

  const deriveReportPath = (rawTextPath: string): string => {
    return rawTextPath.replace(/原文解析/g, '解析报告')
  }

  const handleGenerateReport = async () => {
    if (!rawTextArtifact?.storageRef) return
    setGeneratingReport(true)
    setGenerateError(null)
    try {
      const sourceFile = typeof rawTextArtifact.metadata?.sourceFile === 'string'
        ? rawTextArtifact.metadata.sourceFile
        : ''
      const reportPath = deriveReportPath(rawTextArtifact.storageRef)
      const result = await generateBidAnalysisReport({
        rawTextPath: rawTextArtifact.storageRef,
        reportPath,
        sourceFile,
        projectId: run?.id,
        runId: run?.id,
      })
      if (!result.success) {
        setGenerateError(result.error || '生成失败')
        return
      }
      if (result.data) {
        onReportGenerated(result.data.artifact, result.data.reportPath, sourceFile)
      }
    } catch (e: unknown) {
      setGenerateError(e instanceof Error ? e.message : '生成解析报告失败')
    } finally {
      setGeneratingReport(false)
    }
  }

  const handleView = async (artifact: BiaoshuArtifactRecord) => {
    if (!artifact.storageRef) return
    setViewingArtifact(artifact)
    setViewLoading(true)
    setViewError(null)
    try {
      const data = await readBiaoshuArtifact(artifact.storageRef)
      setViewContent(data.content)
      setViewFormat(data.format)
    } catch (e: unknown) {
      setViewError(e instanceof Error ? e.message : '读取失败')
    } finally {
      setViewLoading(false)
    }
  }

  const handleCloseViewer = () => {
    setViewingArtifact(null)
    setViewContent('')
    setViewFormat('')
    setViewError(null)
  }

  return (
    <div className="space-y-5">
      <section className="card p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-bold text-primary-dark">标书产物库</p>
            <h2 className="mt-2 text-3xl font-black text-ink">技术标产物索引</h2>
            <p className="mt-2 text-sm leading-6 text-ink-muted">
              汇总招标解析、大纲、章节、字数检查、整合成稿和 Word 文档，便于追踪每个阶段的状态与本地输出路径。
            </p>
          </div>
          <button
            onClick={onGoWorkbench}
            className="shrink-0 rounded-lg bg-white px-4 py-2 text-sm font-bold text-primary-dark ring-1 ring-line hover:bg-primary-soft"
          >
            返回工作台
          </button>
        </div>
        <div className="mt-5 grid grid-cols-1 gap-3 md:grid-cols-4">
          <BiaoshuMetric label="任务状态" value={run ? run.status : '未启动'} />
          <BiaoshuMetric label="已生成" value={`${validCount}/${artifacts.length}`} />
          <BiaoshuMetric label="进行中" value={String(activeCount)} />
          <BiaoshuMetric label="需处理" value={String(blockedCount)} />
        </div>
      </section>

      {!run && (
        <div className="rounded-lg bg-amber-50 p-4 text-sm font-semibold text-primary-dark ring-1 ring-amber-200">
          尚未启动标书任务。启动后这里会自动展示本次运行的产物状态。
        </div>
      )}

      {generateError && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 flex items-start gap-2">
          <FiAlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
          <span>{generateError}</span>
        </div>
      )}

      {rawTextArtifact && (
        <div className="rounded-lg bg-amber-50 p-4 ring-1 ring-amber-200">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-bold text-primary-dark">
                {analysisArtifact?.status === 'valid' ? '已有解析报告，可重新生成' : '原文解析已完成，可生成解析报告'}
              </p>
              <p className="mt-1 text-xs text-ink-muted">
                基于原文解析调用大模型，生成结构化招标文件解析报告
              </p>
            </div>
            <button
              onClick={handleGenerateReport}
              disabled={generatingReport}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-bold text-white hover:bg-primary-dark disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {generatingReport ? (
                <><FiRefreshCw className="animate-spin" /> 生成中...</>
              ) : analysisArtifact?.status === 'valid' ? (
                <><FiZap /> 重新生成解析报告</>
              ) : (
                <><FiZap /> 生成解析报告</>
              )}
            </button>
          </div>
        </div>
      )}

      <BiaoshuArtifactTable artifacts={artifacts} onView={handleView} />

      {viewingArtifact && (
        <BiaoshuArtifactViewer
          artifact={viewingArtifact}
          content={viewContent}
          format={viewFormat}
          loading={viewLoading}
          error={viewError}
          run={run}
          onClose={handleCloseViewer}
          onContentUpdate={(newContent: string) => {
            // Refresh the viewed content after write-back
            setViewContent(newContent)
          }}
        />
      )}
    </div>
  )
}

function BiaoshuMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-background-card px-4 py-3 ring-1 ring-line">
      <div className="text-xs font-bold text-ink-soft">{label}</div>
      <div className="mt-1 text-lg font-black text-ink">{value}</div>
    </div>
  )
}

function BiaoshuArtifactTable({ artifacts, onView }: { artifacts: BiaoshuArtifactRecord[]; onView: (a: BiaoshuArtifactRecord) => void }) {
  const headers = ['ID', '名称', '类型', '状态', '负责人', '路径', '操作']
  return (
    <section className="card overflow-hidden p-0">
      <table className="w-full text-left text-sm">
        <thead className="bg-background-mist text-xs text-ink-soft">
          <tr>
            {headers.map((header) => (
              <th className="whitespace-nowrap px-4 py-3" key={header}>{header}</th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-line bg-white/70">
          {artifacts.map((artifact) => (
            <tr key={artifact.id}>
              <td className="whitespace-nowrap px-4 py-3 font-mono text-xs font-bold">{artifact.id}</td>
              <td className="whitespace-nowrap px-4 py-3 font-semibold text-ink">
                <div>{artifact.name}</div>
                <div className="mt-1 max-w-xs truncate text-xs font-normal text-ink-soft" title={artifact.summary}>{artifact.summary}</div>
              </td>
              <td className="whitespace-nowrap px-4 py-3 text-ink-muted">{displayNameForBiaoshuArtifact(artifact.kind)}</td>
              <td className="whitespace-nowrap px-4 py-3"><BiaoshuStatusBadge status={artifact.status} /></td>
              <td className="whitespace-nowrap px-4 py-3 text-ink-muted">{artifact.owner}</td>
              <td className="max-w-sm truncate px-4 py-3 font-mono text-xs text-ink-muted" title={artifact.storageRef}>{artifact.storageRef || '-'}</td>
              <td className="whitespace-nowrap px-4 py-3">
                <div className="flex items-center gap-1.5">
                  <BiaoshuCopyButton value={biaoshuArtifactToCopyText(artifact)} label="复制" />
                  {artifact.status === 'valid' && artifact.storageRef && (
                    <button
                      onClick={() => onView(artifact)}
                      className="inline-flex items-center gap-1.5 rounded-lg bg-primary-soft px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-primary-200 hover:bg-primary-100"
                    >
                      <FiEye /> 查看
                    </button>
                  )}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}

function BiaoshuStatusBadge({ status }: { status: BiaoshuArtifactStatus }) {
  const labelMap: Record<BiaoshuArtifactStatus, string> = {
    valid: '有效',
    review: '待审核',
    pending: '待生成',
    running: '生成中',
    failed: '失败',
    missing: '缺少文件',
  }
  const toneMap: Record<BiaoshuArtifactStatus, string> = {
    valid: 'bg-green-50 text-green-700 ring-green-200',
    review: 'bg-amber-50 text-primary-dark ring-amber-200',
    pending: 'bg-stone-50 text-stone-600 ring-stone-200',
    running: 'bg-blue-50 text-blue-700 ring-blue-200',
    failed: 'bg-red-50 text-red-700 ring-red-200',
    missing: 'bg-red-50 text-red-700 ring-red-200',
  }
  return <span className={`inline-flex items-center rounded-full px-2.5 py-1 text-xs font-bold ring-1 ${toneMap[status]}`}>{labelMap[status]}</span>
}

function BiaoshuCopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = async () => {
    await copyText(value)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1200)
  }
  return (
    <button onClick={handleCopy} className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft">
      <FiCopy /> {copied ? '已复制' : label}
    </button>
  )
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

function BiaoshuArtifactViewer({
  artifact, content, format, loading, error, run, onClose, onContentUpdate,
}: {
  artifact: BiaoshuArtifactRecord
  content: string
  format: string
  loading: boolean
  error: string | null
  run: AgentRun | null
  onClose: () => void
  onContentUpdate?: (newContent: string) => void
}) {
  // AI chat state
  const [chatMessages, setChatMessages] = useState<BiaoshuConversationMessage[]>([])
  const [inputValue, setInputValue] = useState('')
  const [aiLoading, setAiLoading] = useState(false)
  const [aiError, setAiError] = useState<string | null>(null)
  // Preview state
  const [revisedContent, setRevisedContent] = useState<string | null>(null)
  const [reviseSummary, setReviseSummary] = useState<string | null>(null)
  const [applying, setApplying] = useState(false)
  // Track whether we've loaded conversation history
  const [historyLoaded, setHistoryLoaded] = useState(false)

  const artifactPath = artifact.storageRef || ''
  const runId = run?.id || ''

  // Load conversation history when viewer opens
  useEffect(() => {
    if (!runId || !artifactPath || historyLoaded) return
    let cancelled = false
    const load = async () => {
      try {
        const res = await fetchBiaoshuConversation(runId, artifactPath)
        if (!cancelled) {
          setChatMessages(res.messages)
          setHistoryLoaded(true)
        }
      } catch {
        // If conversation doesn't exist yet, that's fine — start fresh
        if (!cancelled) setHistoryLoaded(true)
      }
    }
    load()
    return () => { cancelled = true }
  }, [runId, artifactPath, historyLoaded])

  const handleSend = async () => {
    const instruction = inputValue.trim()
    if (!instruction || aiLoading) return
    setInputValue('')
    setAiError(null)
    setRevisedContent(null)
    setReviseSummary(null)
    setAiLoading(true)

    try {
      // 1. Save user message locally
      const userMsg: BiaoshuConversationMessage = {
        id: `msg-${Date.now()}`,
        role: 'user',
        content: instruction,
        createdAt: new Date().toISOString(),
      }
      if (runId && artifactPath) {
        await sendBiaoshuConversationMessage({
          runId,
          artifactPath,
          artifactKind: artifact.kind,
          role: 'user',
          content: instruction,
        })
      }
      setChatMessages(prev => [...prev, userMsg])

      // 2. Call cloud revise endpoint
      const contextMessages = chatMessages.slice(-10).map(m => ({
        role: m.role,
        content: m.content,
      }))
      const revisePayload: BiaoshuReviseRequest = {
        runId: runId || undefined,
        artifactKind: artifact.kind,
        artifactName: artifact.name,
        artifactContent: content,
        userInstruction: instruction,
        contextMessages,
      }
      const result = await reviseBiaoshuArtifact(revisePayload)

      // 3. Save assistant response locally
      const assistantMsg: BiaoshuConversationMessage = {
        id: `msg-${Date.now()}-assistant`,
        role: 'assistant',
        content: `修改完成：${result.summary}`,
        createdAt: new Date().toISOString(),
        metadata: { revisedContent: result.revisedContent, summary: result.summary, model: result.model },
      }
      if (runId && artifactPath) {
        await sendBiaoshuConversationMessage({
          runId,
          artifactPath,
          artifactKind: artifact.kind,
          role: 'assistant',
          content: assistantMsg.content,
          metadata: assistantMsg.metadata,
        })
      }
      setChatMessages(prev => [...prev, assistantMsg])

      // 4. Show preview
      setRevisedContent(result.revisedContent)
      setReviseSummary(result.summary)
    } catch (err) {
      setAiError(err instanceof Error ? err.message : 'AI修改请求失败')
    } finally {
      setAiLoading(false)
    }
  }

  const handleApplyChanges = async () => {
    if (!revisedContent || !artifactPath) return
    setApplying(true)
    setAiError(null)
    try {
      await writeLocalBiaoshuArtifact({
        filePath: artifactPath,
        content: revisedContent,
        expectedPreviousContent: content,
      })
      onContentUpdate?.(revisedContent)
      // Clear preview after apply
      setRevisedContent(null)
      setReviseSummary(null)
    } catch (err) {
      setAiError(err instanceof Error ? err.message : '应用修改失败')
    } finally {
      setApplying(false)
    }
  }

  const handleDiscardPreview = () => {
    setRevisedContent(null)
    setReviseSummary(null)
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const isTextFormat = format === 'md' || format === 'txt' || format === 'json'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div
        className="relative mx-4 max-h-[90vh] w-full max-w-7xl overflow-hidden rounded-2xl bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-line px-6 py-4">
          <div>
            <h3 className="text-lg font-black text-ink">{artifact.name}</h3>
            <p className="mt-0.5 text-xs text-ink-muted">
              {displayNameForBiaoshuArtifact(artifact.kind)} · {format.toUpperCase()}
              {artifact.storageRef && ` · ${artifact.storageRef}`}
            </p>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-2 text-ink-muted hover:bg-background-mist hover:text-ink"
          >
            <FiX size={20} />
          </button>
        </div>

        {/* Two-column body */}
        <div className="grid grid-cols-1 lg:grid-cols-2 divide-y lg:divide-y-0 lg:divide-x divide-line" style={{ height: 'calc(90vh - 77px)' }}>
          {/* Left: Artifact content */}
          <div className="overflow-y-auto px-6 py-5">
            {loading && (
              <div className="flex items-center justify-center py-20 text-ink-muted">
                <FiRefreshCw className="mr-2 animate-spin" /> 加载中...
              </div>
            )}
            {error && (
              <div className="rounded-lg bg-red-50 p-4 text-sm text-red-700 ring-1 ring-red-200">
                <FiAlertTriangle className="mr-2 inline" />{error}
              </div>
            )}
            {!loading && !error && format === 'md' && (
              <div className="prose prose-sm max-w-none">
                <ReactMarkdown>{content}</ReactMarkdown>
              </div>
            )}
            {!loading && !error && format !== 'md' && content && (
              <pre className="whitespace-pre-wrap break-words text-sm leading-relaxed text-ink">
                {content}
              </pre>
            )}
            {!loading && !error && !content && (
              <p className="py-10 text-center text-sm text-ink-muted">暂无内容</p>
            )}
          </div>

          {/* Right: AI Chat panel */}
          <div className="flex flex-col">
            {/* Chat messages */}
            <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
              {!isTextFormat && (
                <div className="rounded-lg bg-amber-50 p-3 text-xs text-amber-700 ring-1 ring-amber-200">
                  此格式暂不支持 AI 修改
                </div>
              )}
              
              {isTextFormat && chatMessages.length === 0 && !aiLoading && (
                <div className="py-8 text-center text-sm text-ink-muted">
                  <p>在下方输入修改指令，AI 将帮你修订此产物</p>
                  <p className="mt-1 text-xs">例如：把第一章标题改为"项目概述"</p>
                </div>
              )}

              {chatMessages.map((msg) => (
                <div
                  key={msg.id}
                  className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
                >
                  <div
                    className={`max-w-[85%] rounded-xl px-4 py-2 text-sm whitespace-pre-wrap break-words ${
                      msg.role === 'user'
                        ? 'bg-primary text-white'
                        : 'bg-background-mist text-ink'
                    }`}
                  >
                    <p>{msg.content}</p>
                    <p className={`mt-1 text-xs ${msg.role === 'user' ? 'text-white/60' : 'text-ink-muted'}`}>
                      {new Date(msg.createdAt).toLocaleTimeString()}
                    </p>
                  </div>
                </div>
              ))}

              {aiLoading && (
                <div className="flex justify-start">
                  <div className="max-w-[85%] rounded-xl bg-background-mist px-4 py-3 text-sm text-ink-muted">
                    <FiRefreshCw className="mr-2 inline animate-spin" />
                    正在分析修改...
                  </div>
                </div>
              )}

              {aiError && (
                <div className="rounded-lg bg-red-50 p-3 text-xs text-red-700 ring-1 ring-red-200">
                  <FiAlertTriangle className="mr-2 inline" />{aiError}
                </div>
              )}

              {/* Preview panel */}
              {revisedContent && (
                <div className="rounded-xl border border-primary/30 bg-primary/5 p-4">
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="text-sm font-bold text-primary">AI 修订预览</h4>
                    <div className="flex gap-2">
                      <button
                        onClick={handleDiscardPreview}
                        className="rounded-md px-2 py-1 text-xs font-medium text-ink-muted hover:bg-white hover:text-ink"
                      >
                        放弃预览
                      </button>
                      <button
                        onClick={handleApplyChanges}
                        disabled={applying}
                        className="rounded-md bg-primary px-3 py-1 text-xs font-bold text-white hover:bg-primary-dark disabled:opacity-50"
                      >
                        {applying ? '应用中...' : '应用修改'}
                      </button>
                    </div>
                  </div>
                  {reviseSummary && (
                    <p className="mb-2 text-xs text-primary/70">{reviseSummary}</p>
                  )}
                  <div className="max-h-60 overflow-y-auto rounded-lg bg-white p-3 text-xs leading-relaxed whitespace-pre-wrap border border-line">
                    {revisedContent.slice(0, 3000)}
                    {revisedContent.length > 3000 && (
                      <p className="mt-2 text-ink-muted">... 内容已截断，应用修改可查看完整内容</p>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Chat input */}
            {isTextFormat && (
              <div className="border-t border-line px-4 py-3">
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={inputValue}
                    onChange={(e) => setInputValue(e.target.value)}
                    onKeyDown={handleKeyDown}
                    placeholder="输入修改指令，例如：把第一章标题改为'项目概述'"
                    disabled={aiLoading}
                    className="flex-1 rounded-lg border border-line bg-white px-3 py-2 text-sm text-ink placeholder-ink-muted focus:outline-none focus:ring-2 focus:ring-primary/30 disabled:opacity-50"
                  />
                  <button
                    onClick={handleSend}
                    disabled={!inputValue.trim() || aiLoading}
                    className="rounded-lg bg-primary p-2 text-white hover:bg-primary-dark disabled:opacity-40"
                  >
                    <FiSend size={18} />
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
