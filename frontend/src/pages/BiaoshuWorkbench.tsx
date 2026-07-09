import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FiArchive, FiChevronDown, FiCopy, FiEye, FiFileText, FiPlay, FiRefreshCw, FiCheck, FiX, FiAlertTriangle, FiClock, FiList, FiTool, FiFolder, FiSearch, FiSend, FiZap } from 'react-icons/fi'
import ReactMarkdown from 'react-markdown'
import axios from 'axios'
import { getAgentRun, getAgentRunReviews, getAgentRunTrace, readBiaoshuArtifact, reviseBiaoshuArtifact, startAgentRun, generateBidAnalysisReport, generateProjectContextQuestions, generateProjectContextReport, generateOutline, generateScoringBreakdown, generateChapterTaskBook, generateChapters, type BiaoshuReviseRequest, type ReferenceArtifact } from '../services/api'
import { 
  fetchBiaoshuConversation, 
  saveBiaoshuProject, 
  sendBiaoshuConversationMessage, 
  writeLocalBiaoshuArtifact,
  readLocalBiaoshuArtifact,
  createBiaoshuManagedProject,
  fetchBiaoshuManagedProjects,
  deleteBiaoshuManagedProject,
  fetchBiaoshuHistory,
  registerBiaoshuManagedArtifact,
  type BiaoshuConversationMessage,
  type BiaoshuProjectManifest,
  type BiaoshuProjectStatus,
} from '../services/localAgent'
import type { AgentPlan, AgentReviewItem, AgentRun, AgentStep, TraceData, TraceNode } from '../utils/types'
import {
  biaoshuArtifactToCopyText,
  buildBiaoshuArtifacts,
  createBiaoshuFallbackRun,
  createManualReportArtifact,
  createManualProjectContextArtifact,
  createManualScoringBreakdownArtifact,
  createManualOutlineArtifact,
  createManualChapterTaskBookArtifact,
  createManualChapterArtifact,
  deriveBiaoshuAnalysisReportPath,
  deriveBiaoshuProjectContextPath,
  deriveBiaoshuChapterTaskBookPath,
  deriveChapterOutputDir,
  deriveBiaoshuProjectContextQuestionnairePath,
  deriveBiaoshuOutlinePath,
  deriveBiaoshuScoringBreakdownPath,
  displayNameForBiaoshuArtifact,
  mergeManualBiaoshuArtifacts,
  type BiaoshuArtifactRecord,
  type BiaoshuArtifactStatus,
} from './biaoshuArtifactLogic'
import { BiaoshuProjectContextDialog } from './BiaoshuProjectContextDialog'
import {
  createQuestionnaire,
  validateQuestionnaire,
  type ProjectContextQuestionnaire,
  type ProjectContextQuestionWithAnswer,
} from './biaoshuProjectContextQuestionnaire'
import {
  type BiaoshuProjectHistoryItem,
  biaoshuHistoryProjectToViewItem,
  biaoshuManagedProjectsToHistory,
  biaoshuProjectToArtifacts,
  selectBestBiaoshuHistoryProject,
} from './biaoshuProjectSystem'

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

const getDirName = (path?: string) => (path ? path.replace(/[/\\]+$/, '').split(/[/\\]/).pop() : undefined)

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
  const [manualArtifacts, setManualArtifacts] = useState<BiaoshuArtifactRecord[]>([])
  const [activeProject, setActiveProject] = useState<BiaoshuProjectManifest | null>(null)
  const [managedProjects, setManagedProjects] = useState<BiaoshuProjectManifest[]>([])
  const [activeView, setActiveView] = useState<BiaoshuView>('workbench')
  const [projectHistory, setProjectHistory] = useState<BiaoshuProjectHistoryItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [runLog, setRunLog] = useState<string[]>([])
  const registeredKinds = useRef(new Set<string>())
  const pollCount = useRef(0)
  const shownErrors = useRef(new Set<string>())

  const plan = run?.plan as AgentPlan | undefined
  const steps = plan?.steps || []
  const isTerminalStatus = run?.status === 'SUCCESS' || run?.status === 'FAILED' || run?.status === 'CANCELLED' || run?.status === 'ERROR' || run?.status === 'TIMEOUT'
  const baseArtifacts = useMemo(() => buildBiaoshuArtifacts(run, trace, reviews), [run, trace, reviews])
  const manifestArtifacts = useMemo(
    () => activeProject ? biaoshuProjectToArtifacts(activeProject) : [],
    [activeProject],
  )
  const artifacts = useMemo(
    () => activeProject
      ? mergeManualBiaoshuArtifacts(manifestArtifacts, manualArtifacts)
      : mergeManualBiaoshuArtifacts(baseArtifacts, manualArtifacts),
    [activeProject, baseArtifacts, manifestArtifacts, manualArtifacts],
  )

  const addLog = useCallback((msg: string) => {
    setRunLog(prev => [...prev, `[${new Date().toLocaleTimeString()}] ${msg}`])
  }, [])

  const stringValue = (value: unknown): string =>
    typeof value === 'string' && value.trim() ? value : ''

  const upsertManualArtifact = useCallback((artifact: BiaoshuArtifactRecord) => {
    setManualArtifacts((prev) => {
      const next = prev.filter((item) => item.id !== artifact.id)
      return [...next, artifact]
    })
  }, [])

  const handleReportGenerated = useCallback((artifact: Record<string, unknown> | undefined, filePath: string, sourceFile: string) => {
    const kind = stringValue(artifact?.kind) || ''
    if (kind === 'BID_PROJECT_CONTEXT') {
      upsertManualArtifact(createManualProjectContextArtifact(artifact, filePath, sourceFile))
      addLog(`项目背景确认表已生成: ${filePath}`)
    } else if (kind === 'BID_SCORING_BREAKDOWN') {
      upsertManualArtifact(createManualScoringBreakdownArtifact(artifact, filePath, sourceFile))
      addLog(`评分标准拆解表已生成: ${filePath}`)
    } else if (kind === 'BID_OUTLINE') {
      upsertManualArtifact(createManualOutlineArtifact(artifact, filePath, sourceFile))
      addLog(`技术标大纲已生成: ${filePath}`)
    } else if (kind === 'BID_CHAPTER_TASK_BOOK') {
      upsertManualArtifact(createManualChapterTaskBookArtifact(artifact, filePath, sourceFile))
      addLog(`章节写作任务书已生成: ${filePath}`)
    } else if (kind === 'BID_CHAPTERS') {
      upsertManualArtifact(createManualChapterArtifact(artifact, filePath))
      addLog(`章节初稿已生成: ${filePath}`)
    } else {
      upsertManualArtifact(createManualReportArtifact(artifact, filePath, sourceFile))
      addLog(`解析报告已生成: ${filePath}`)
    }
    if (activeProject?.projectId && filePath) {
      registerBiaoshuManagedArtifact(activeProject.projectId, {
        kind: kind || 'BID_ANALYSIS',
        name: String(artifact?.name || filePath.split(/[\\/]/).pop() || kind || '标书产物'),
        storageRef: filePath,
        mimeType: 'text/markdown',
        status: 'valid',
        metadata: {
          ...(typeof artifact?.metadata === 'object' && artifact.metadata ? artifact.metadata : {}),
          sourceFile,
        },
      }).then((response) => {
        setActiveProject(response.project)
        setManagedProjects((prev) => {
          const next = prev.filter((project) => project.projectId !== response.project.projectId)
          return [response.project, ...next]
        })
        setProjectHistory((prev) => {
          const managedHistoryItem = biaoshuManagedProjectsToHistory([response.project])[0]
          return [
            managedHistoryItem,
            ...prev.filter((item) => item.projectId !== response.project.projectId),
          ]
        })
        addLog(`产物已登记: ${filePath}`)
      }).catch((err: unknown) => {
        addLog(`产物登记失败: ${err instanceof Error ? err.message : String(err)}`)
      })
    }
  }, [addLog, activeProject, upsertManualArtifact])

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
    const status = (nextRun.status || 'UNKNOWN') as BiaoshuProjectStatus
    const response = await saveBiaoshuProject({
      runId: nextRun.id,
      projectName: options?.projectName || extractProjectNameFromRun(nextRun, projectName || '未命名项目'),
      bidFilePath: options?.bidFilePath || extractBidFilePathFromRun(nextRun, bidFilePath),
      status,
      createdAt: nextRun.createdAt || new Date().toISOString(),
      updatedAt: nextRun.updatedAt || new Date().toISOString(),
      artifactCount: nextArtifacts.length,
      validArtifactCount: nextArtifacts.filter((artifact) => artifact.status === 'valid').length,
    })
    setProjectHistory(response.projects.map((p) => ({
      runId: p.runId,
      projectId: '',
      projectName: p.projectName,
      bidFilePath: p.bidFilePath,
      status: p.status,
      currentStage: '',
      createdAt: p.createdAt,
      updatedAt: p.updatedAt,
      artifactCount: p.artifactCount ?? 0,
      validArtifactCount: p.validArtifactCount ?? 0,
      managedProject: false,
    })))
  }, [bidFilePath, projectName, reviews, trace])

  // 页面加载时从后端统一历史接口恢复所有项目
  useEffect(() => {
    fetchBiaoshuHistory().then((response) => {
      if (response.warnings?.length) {
        response.warnings.forEach((w) => addLog(`[历史诊断] ${w}`))
      }
      addLog(`历史项目来源: 当前清单 ${response.sources.currentManaged}, 旧临时清单 ${response.sources.legacyTempManaged}, 旧运行记录 ${response.sources.legacyRuns}`)

      const viewItems = response.projects.map(biaoshuHistoryProjectToViewItem)
      setProjectHistory(viewItems)

      const best = selectBestBiaoshuHistoryProject(response.projects)
      if (!best) return
      if (best.hasManagedManifest && best.projectId) {
        fetchBiaoshuManagedProjects().then((mp) => {
          const match = mp.projects.find((p) => p.projectId === best.projectId)
          if (match) {
            setActiveProject(match)
            setManagedProjects(mp.projects)
          }
        }).catch(() => {})
      }
      setProjectName(best.projectName || DEFAULT_PROJECT_NAME)
      setBidFilePath(best.bidFilePath || DEFAULT_BID_FILE_PATH)
      setActiveView('artifacts')
      addLog(`恢复标书项目: ${best.projectName}`)
    }).catch((err: unknown) => {
      addLog(`标书历史接口读取失败: ${err instanceof Error ? err.message : String(err)}`)
    })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!run?.id || isTerminalStatus) return
    const MAX_POLLS = 150 // 150 times × 2s = 5 minutes max
    pollCount.current = 0
    const timer = setInterval(async () => {
      try {
        pollCount.current++
        if (pollCount.current > MAX_POLLS) {
          clearInterval(timer)
          addLog(`轮询超时（已轮询 ${MAX_POLLS} 次，约 5 分钟），请检查服务状态后重试`)
          setError(`轮询超时，任务可能卡住。请检查 biaoshu-tools (:9001)、cloud-backend (:8080) 是否正常运行`)
          return
        }
        const snapshot = await refreshRunData(run.id)
        await saveHistoryFromRun(snapshot.run, { trace: snapshot.trace, reviews: snapshot.reviews })

        // Extract error details from failed trace nodes
        const traceData = snapshot.trace as TraceData | null
        const failedNodes = traceData?.task?.nodes?.filter(
          (n: TraceNode) => n.status === 'FAILED' && !!n.errorMessage && !shownErrors.current.has(n.id),
        ) || []
        for (const node of failedNodes) {
          shownErrors.current.add(node.id)
          const msg = node.errorMessage!
          const shortMsg = msg.length > 200 ? msg.slice(0, 200) + '…' : msg
          addLog(`[${node.name}] 节点失败: ${shortMsg}`)
          if (node.retryCount > 0) {
            addLog(`[${node.name}] 已重试 ${node.retryCount} 次`)
          }
        }

        if (snapshot.run.status === 'SUCCESS') addLog('运行完成')
        if (snapshot.run.status === 'FAILED') {
          const errors = traceData?.task?.nodes
            ?.filter((n: TraceNode) => n.status === 'FAILED' && n.errorMessage)
            ?.map((n: TraceNode) => `${n.name}: ${n.errorMessage}`)
          const detail = errors?.join('; ') || '未知错误'
          addLog(`运行失败 — ${detail}`)
          setError(`任务失败: ${detail.length > 300 ? detail.slice(0, 300) + '…' : detail}`)
        }
      } catch { /* polling */ }
    }, 2000)
    return () => { clearInterval(timer); pollCount.current = 0 }
  }, [run?.id, isTerminalStatus, addLog, refreshRunData])

  // Sync baseArtifacts (from cloud trace) into managed project manifest
  useEffect(() => {
    if (!activeProject?.projectId || baseArtifacts.length === 0) return
    const newArtifacts = baseArtifacts.filter(
      (a) => a.status === 'valid' && a.storageRef && !registeredKinds.current.has(a.kind),
    )
    if (newArtifacts.length === 0) return
    ;(async () => {
      for (const artifact of newArtifacts) {
        try {
          const response = await registerBiaoshuManagedArtifact(activeProject.projectId, {
            kind: artifact.kind,
            name: artifact.name,
            storageRef: artifact.storageRef,
            mimeType: 'text/markdown',
            status: 'valid',
            metadata: { sourceTool: artifact.sourceTool },
          })
          registeredKinds.current.add(artifact.kind)
          setActiveProject(response.project)
          setManagedProjects((prev) => {
            const next = prev.filter((p) => p.projectId !== response.project.projectId)
            return [response.project, ...next]
          })
          setProjectHistory((prev) => {
            const managed = biaoshuManagedProjectsToHistory([response.project])[0]
            return [managed, ...prev.filter((item) => item.projectId !== response.project.projectId)]
          })
          addLog(`产物已登记: ${artifact.name}`)
        } catch (err: unknown) {
          addLog(`产物登记失败: ${err instanceof Error ? err.message : String(err)}`)
        }
      }
    })()
  }, [baseArtifacts, activeProject?.projectId, addLog])

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
    setManualArtifacts([])
    registeredKinds.current.clear()
    pollCount.current = 0
    shownErrors.current.clear()

    const message = customMessage.trim()
      || `请解析招标文件并生成技术标文档。文件路径：${bidFilePath}，项目名称：${projectName || '未命名项目'}`

    addLog(`启动任务: ${message}`)

    try {
      const managed = await createBiaoshuManagedProject({
        projectName: projectName || '未命名项目',
        bidFilePath,
      })
      setActiveProject(managed.project)
      addLog(`本地项目已创建: ${managed.project.projectId}`)

      const result = await startAgentRun({
        message,
        domain: 'bid_writing',
        mode: 'dynamic_agent',
        context: {
          filePath: bidFilePath,
          file_path: bidFilePath,
          projectName: projectName || '未命名项目',
          project_name: projectName || '未命名项目',
          output_dir: getDirName(managed.project.outputDir) || managed.project.outputDirName || projectName || '未命名项目',
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

  const isAgentRunNotFound = (err: unknown): boolean => {
    if (!axios.isAxiosError(err)) return false
    const status = err.response?.status
    const message =
      typeof err.response?.data === 'object' && err.response?.data
        ? String((err.response.data as { message?: unknown }).message || '')
        : ''
    return status === 404 && message.toLowerCase().includes('agent run not found')
  }

  const openHistoryItemFromLocalRecord = useCallback((item: BiaoshuProjectHistoryItem) => {
    const fallbackRun = createBiaoshuFallbackRun({
      runId: item.runId,
      projectName: item.projectName,
      bidFilePath: item.bidFilePath,
      status: item.status,
      createdAt: item.createdAt,
      updatedAt: item.updatedAt,
    })
    setProjectName(item.projectName)
    setBidFilePath(item.bidFilePath)
    setManualArtifacts([])
    setRun(fallbackRun)
    setTrace(null)
    setReviews([])
    addLog(`云端任务不存在，已从本地历史恢复项目: ${item.projectName}`)
    setActiveView('artifacts')
  }, [addLog])

  const findManagedProjectForHistoryItem = useCallback((item: BiaoshuProjectHistoryItem): BiaoshuProjectManifest | null => {
    if (!item.projectId) return null
    return managedProjects.find((project) => project.projectId === item.projectId) || null
  }, [managedProjects])

  const handleOpenHistoryItem = async (item: BiaoshuProjectHistoryItem) => {
    setLoading(true)
    setError(null)
    try {
      const managedProject = findManagedProjectForHistoryItem(item)
      if (managedProject) {
        setProjectName(managedProject.projectName || item.projectName)
        setBidFilePath(managedProject.sourceFiles[0]?.path || item.bidFilePath)
        setManualArtifacts([])
        setActiveProject(managedProject)
        setRun(null)
        setTrace(null)
        setReviews([])
        addLog(`打开本地托管标书项目: ${managedProject.projectName}`)
        setActiveView('artifacts')
        setLoading(false)
        return
      }

      setProjectName(item.projectName)
      setBidFilePath(item.bidFilePath)
      setManualArtifacts([])
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
      if (isAgentRunNotFound(err)) {
        openHistoryItemFromLocalRecord(item)
      } else {
        const msg = err instanceof Error ? err.message : String(err)
        setError(msg)
        addLog(`历史项目读取失败: ${msg}`)
      }
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
            bidFilePath={bidFilePath}
            projectName={projectName}
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
            onDeleteProject={async (item: BiaoshuProjectHistoryItem) => {
              if (item.managedProject && item.projectId) {
                if (!window.confirm(`确认删除项目"${item.projectName}"及其所有本地产物？`)) return
                try {
                  const result = await deleteBiaoshuManagedProject(item.projectId)
                  addLog(`项目已删除: ${result.projectName} (${result.deletedPaths.length} 个路径)`)
                  setProjectHistory((prev) => prev.filter((p) => p.projectId !== item.projectId))
                  if (activeProject?.projectId === item.projectId) {
                    setActiveProject(null)
                    setManualArtifacts([])
                  }
                } catch (err: unknown) {
                  const msg = err instanceof Error ? err.message : String(err)
                  addLog(`删除失败: ${msg}`)
                }
              }
            }}
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
  onDeleteProject,
}: {
  currentRunId?: string
  history: BiaoshuProjectHistoryItem[]
  loading: boolean
  onOpenProject: (item: BiaoshuProjectHistoryItem) => void
  onGoWorkbench: () => void
  onDeleteProject?: (item: BiaoshuProjectHistoryItem) => void
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
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => onOpenProject(item)}
                        disabled={loading}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-primary-soft px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-primary-200 hover:bg-primary-100 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        {loading ? <FiRefreshCw className="animate-spin" /> : <FiEye />}
                        查看产物
                      </button>
                      {item.managedProject && onDeleteProject && (
                        <button
                          onClick={() => onDeleteProject(item)}
                          disabled={loading}
                          className="inline-flex items-center gap-1.5 rounded-lg bg-red-50 px-3 py-2 text-xs font-black text-red-700 ring-1 ring-red-200 hover:bg-red-100 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                          删除
                        </button>
                      )}
                    </div>
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
  bidFilePath,
  projectName,
  onGoWorkbench,
  onReportGenerated,
}: {
  artifacts: BiaoshuArtifactRecord[]
  run: AgentRun | null
  bidFilePath: string
  projectName: string
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
  const contextArtifact = artifacts.find((a) => a.kind === 'BID_PROJECT_CONTEXT')
  const scoringArtifact = artifacts.find((a) => a.kind === 'BID_SCORING_BREAKDOWN')
  const outlineArtifact = artifacts.find((a) => a.kind === 'BID_OUTLINE')

  const [contextQuestionnaire, setContextQuestionnaire] = useState<ProjectContextQuestionnaire | null>(null)
  const [contextDialogOpen, setContextDialogOpen] = useState(false)
  const [contextQuestionIndex, setContextQuestionIndex] = useState(0)
  const [generatingQuestions, setGeneratingQuestions] = useState(false)
  const [generatingContextReport, setGeneratingContextReport] = useState(false)
  const [generatingOutline, setGeneratingOutline] = useState(false)
  const [generatingTaskBook, setGeneratingTaskBook] = useState(false)
  const [generatingChapters, setGeneratingChapters] = useState(false)
  const [savingDraft, setSavingDraft] = useState(false)
  const [savedRecently, setSavedRecently] = useState(false)
  const [contextError, setContextError] = useState<string | null>(null)
  const [scannedAnalysisPath, setScannedAnalysisPath] = useState('')

  const deriveReportPath = deriveBiaoshuAnalysisReportPath
  const deriveProjectContextPath = deriveBiaoshuProjectContextPath
  const deriveProjectContextQuestionnairePath = deriveBiaoshuProjectContextQuestionnairePath
  const deriveOutlinePath = deriveBiaoshuOutlinePath
  const deriveScoringBreakdownPath = deriveBiaoshuScoringBreakdownPath
  const deriveChapterTaskBookPath = deriveBiaoshuChapterTaskBookPath

  // ── Recovery scan: check for already-generated local files ──
  const resolveCandidatePath = (probePath: string, alreadyScanned: string): boolean =>
    !!probePath && probePath !== alreadyScanned

  useEffect(() => {
    if (!bidFilePath) return

    // biaoshu-tools OUTPUT_DIR is a fixed path under the workspace root, not
    // derived from bidFilePath (bid file may reside on a different drive).
    // We anchor to the root that holds the known-good tools directory.
    const toolsRoot = DEFAULT_BID_FILE_PATH.replace(/\\/g, '/').replace(/\/[^/]+$/, '')

    // Try multiple candidate project dirs (bid basename, project name, etc.)
    // because the recovery scan doesn't know which subdirectory holds the files.
    const candidateDirNames: string[] = []
    const bidBase = bidFilePath.replace(/\\/g, '/').split('/').pop() || ''
    const bidDirName = bidBase.replace(/\.[^.]+$/, '')
    if (bidDirName && !candidateDirNames.includes(bidDirName)) candidateDirNames.push(bidDirName)
    const projectDirFromName = (projectName || DEFAULT_PROJECT_NAME).replace(/[<>:"/\\|?*]/g, '_').trim()
    if (projectDirFromName && !candidateDirNames.includes(projectDirFromName)) candidateDirNames.push(projectDirFromName)

    // Known file names in the output directory
    const rawTextFileName = '00_招标文件原文解析.md'
    const analysisFileName = '00_招标文件解析报告.md'
    const contextFileName = '01_项目背景信息确认表.md'
    const scoringFileName = '02_评分标准拆解表.md'
    const outlineFileName = '03_技术标四级大纲.md'

    // Determine which project directory to scan by trying each candidate
    // in sequence until we find the analysis report file.
    let cancelled = false
    const scan = async () => {
      let analysisDir = ''
      let analysisFilePath = ''

      // Try each candidate dir to locate the analysis report
      for (const dirName of candidateDirNames) {
        if (cancelled) return
        const probePath = `${toolsRoot}/output/${dirName}/${analysisFileName}`
        try {
          const result = await readLocalBiaoshuArtifact(probePath)
          if (!cancelled && result.content.trim()) {
            analysisDir = `${toolsRoot}/output/${dirName}`
            analysisFilePath = result.filePath || probePath
            break
          }
        } catch { /* try next candidate */ }
      }

      if (cancelled || !analysisDir) return

      // Prevent re-scanning the same analysis path
      if (!resolveCandidatePath(analysisFilePath, scannedAnalysisPath)) return

      const sourceFile = bidFilePath

      // Recover all known output artifacts in one batch
      const candidates = [
        {
          kind: 'BID_RAW_TEXT',
          path: `${analysisDir}/${rawTextFileName}`,
          createArtifact: (filePath: string) => ({
            id: 'recovered-raw-text',
            name: '招标文件原文解析',
            kind: 'BID_RAW_TEXT',
            version: '-',
            status: 'valid' as const,
            owner: '文件解析',
            updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
            storageRef: filePath,
            summary: '从本地已生成文件恢复的招标文件原文解析',
            sourceTool: 'parse_bid_files',
            metadata: { recovered: true, sourceFile },
          }),
        },
        {
          kind: 'BID_ANALYSIS',
          path: analysisFilePath,
          createArtifact: (filePath: string) => createManualReportArtifact({
            id: 'recovered-analysis',
            kind: 'BID_ANALYSIS',
            name: '招标文件解析报告',
            storageRef: filePath,
            summary: '从本地文件恢复的解析报告',
            metadata: { recovered: true, sourceFile },
          }, filePath, sourceFile),
        },
        {
          kind: 'BID_PROJECT_CONTEXT',
          path: `${analysisDir}/${contextFileName}`,
          createArtifact: (filePath: string) => createManualProjectContextArtifact({
            id: 'recovered-project-context',
            kind: 'BID_PROJECT_CONTEXT',
            name: '项目背景信息确认表',
            storageRef: filePath,
            summary: '从本地已生成文件恢复的项目背景信息确认表',
            metadata: { recovered: true, sourceFile },
          }, filePath, sourceFile),
        },
        {
          kind: 'BID_SCORING_BREAKDOWN',
          path: `${analysisDir}/${scoringFileName}`,
          createArtifact: (filePath: string) => createManualScoringBreakdownArtifact({
            id: 'recovered-scoring-breakdown',
            kind: 'BID_SCORING_BREAKDOWN',
            name: '评分标准拆解表',
            storageRef: filePath,
            summary: '从本地已生成文件恢复的评分标准拆解表',
            metadata: { recovered: true, sourceFile },
          }, filePath, sourceFile),
        },
        {
          kind: 'BID_OUTLINE',
          path: `${analysisDir}/${outlineFileName}`,
          createArtifact: (filePath: string) => createManualOutlineArtifact({
            id: 'recovered-outline',
            kind: 'BID_OUTLINE',
            name: '技术标四级大纲',
            storageRef: filePath,
            summary: '从本地已生成文件恢复的技术标四级大纲',
            metadata: { recovered: true, sourceFile },
          }, filePath, sourceFile),
        },
      ]

      for (const candidate of candidates) {
        if (cancelled) return
        const alreadyValid = artifacts.some(
          (artifact) =>
            artifact.kind === candidate.kind &&
            artifact.status === 'valid' &&
            artifact.storageRef,
        )
        if (alreadyValid) continue

        try {
          const result = await readLocalBiaoshuArtifact(candidate.path)
          if (cancelled || !result.content.trim()) continue
          onReportGenerated(candidate.createArtifact(result.filePath) as unknown as Record<string, unknown>, result.filePath, sourceFile)
        } catch {
          // Missing files are expected when the user has not reached this step yet.
        }
      }

      if (!cancelled) setScannedAnalysisPath(analysisFilePath)
    }

    scan()
    return () => {
      cancelled = true
    }
  }, [
    artifacts,
    bidFilePath,
    onReportGenerated,
    projectName,
    scannedAnalysisPath,
  ])

  // ── Questionnaire draft persistence (localStorage primary, file backup) ──

  const questionnaireStorageKey = useMemo(() => {
    const runId = run?.id || analysisArtifact?.storageRef || 'unknown'
    return `biaoshu:project-context:${runId}`
  }, [run?.id, analysisArtifact?.storageRef])

  const loadQuestionnaireDraft = useCallback(async (): Promise<ProjectContextQuestionnaire | null> => {
    // 1. Try localStorage first (instant, no path issues)
    try {
      const stored = localStorage.getItem(questionnaireStorageKey)
      if (stored) {
        const parsed = JSON.parse(stored) as ProjectContextQuestionnaire
        if (parsed.schemaVersion === 'biaoshu.project_context_answers.v1' && parsed.questions?.length > 0) {
          return parsed
        }
      }
    } catch { /* ignore corrupted localStorage */ }

    // 2. Try file (for cross-session persistence)
    if (analysisArtifact?.storageRef) {
      try {
        const filePath = deriveProjectContextQuestionnairePath(analysisArtifact.storageRef)
        const result = await readLocalBiaoshuArtifact(filePath)
        if (result?.content) {
          const parsed = JSON.parse(result.content) as ProjectContextQuestionnaire
          if (parsed.schemaVersion === 'biaoshu.project_context_answers.v1' && parsed.questions?.length > 0) {
            // Sync to localStorage
            localStorage.setItem(questionnaireStorageKey, JSON.stringify(parsed))
            return parsed
          }
        }
      } catch { /* file not found or path error */ }
    }
    return null
  }, [questionnaireStorageKey, analysisArtifact?.storageRef])

  const saveQuestionnaireDraft = useCallback(async (questionnaire: ProjectContextQuestionnaire) => {
    // 1. Always save to localStorage (instant, reliable)
    const json = JSON.stringify(questionnaire, null, 2)
    try {
      localStorage.setItem(questionnaireStorageKey, json)
    } catch { /* storage full — ignore */ }

    // 2. Try file backup (best effort)
    try {
      await writeLocalBiaoshuArtifact({
        filePath: questionnaire.questionnairePath,
        content: json,
      })
    } catch { /* file write failed — localStorage already saved */ }
  }, [questionnaireStorageKey])

  // ── End persistence helpers ──

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

  const handleGenerateQuestions = async () => {
    if (!analysisArtifact?.storageRef) return
    setGeneratingQuestions(true)
    setContextError(null)
    try {
      // 1. Try to load existing draft
      const existing = await loadQuestionnaireDraft()
      if (existing) {
        setContextQuestionnaire(existing)
        setContextQuestionIndex(0)
        setContextDialogOpen(true)
        return
      }

      // 2. No draft — generate new questions
      const sourceFile = typeof analysisArtifact.metadata?.sourceFile === 'string'
        ? analysisArtifact.metadata.sourceFile
        : ''
      const result = await generateProjectContextQuestions({
        analysisReportPath: analysisArtifact.storageRef,
        sourceFile,
      })
      if (!result.success) {
        setContextError(result.error || '生成问题失败')
        return
      }
      if (result.data?.questions) {
        const questionnaire = createQuestionnaire({
          questions: result.data.questions,
          analysisReportPath: analysisArtifact.storageRef,
          questionnairePath: deriveProjectContextQuestionnairePath(analysisArtifact.storageRef),
          contextReportPath: deriveProjectContextPath(analysisArtifact.storageRef),
          runId: run?.id,
          projectName: typeof run?.metadata?.projectName === 'string' ? run.metadata.projectName : undefined,
          sourceFile,
        })
        setContextQuestionnaire(questionnaire)
        setContextQuestionIndex(0)
        setContextDialogOpen(true)
      }
    } catch (e: unknown) {
      setContextError(e instanceof Error ? e.message : '生成问题清单失败')
    } finally {
      setGeneratingQuestions(false)
    }
  }

  const handleGenerateContextReport = async () => {
    if (!analysisArtifact?.storageRef || !contextQuestionnaire) return

    // Validate required fields
    const validation = validateQuestionnaire(contextQuestionnaire)
    if (!validation.valid) {
      setContextError(validation.message)
      setContextQuestionIndex(validation.firstInvalidIndex)
      setContextDialogOpen(true)
      return
    }

    setGeneratingContextReport(true)
    setContextError(null)
    try {
      // Save final draft
      await saveQuestionnaireDraft(contextQuestionnaire)

      const sourceFile = typeof analysisArtifact.metadata?.sourceFile === 'string'
        ? analysisArtifact.metadata.sourceFile
        : ''
      const result = await generateProjectContextReport({
        analysisReportPath: analysisArtifact.storageRef,
        contextAnswers: JSON.stringify({ ...contextQuestionnaire, status: 'submitted', submittedAt: new Date().toISOString() }, null, 2),
        contextReportPath: contextQuestionnaire.contextReportPath,
        sourceFile,
      })
      if (!result.success) {
        setContextError(result.error || '生成失败')
        return
      }
      if (result.data) {
        onReportGenerated(result.data.artifact, result.data.contextReportPath, sourceFile)
        // Close dialog after 1 second so user sees feedback
        setTimeout(() => {
          setContextDialogOpen(false)
          setContextQuestionnaire(null)
        }, 1000)
      }
    } catch (e: unknown) {
      setContextError(e instanceof Error ? e.message : '生成背景确认表失败')
    } finally {
      setGeneratingContextReport(false)
    }
  }

  const handleGenerateScoringBreakdown = async () => {
    if (!analysisArtifact?.storageRef) return
    setGeneratingOutline(true)
    setContextError(null)
    try {
      const sourceFile = typeof analysisArtifact.metadata?.sourceFile === 'string'
        ? analysisArtifact.metadata.sourceFile
        : ''
      const scoringReportPath = deriveScoringBreakdownPath(analysisArtifact.storageRef)
      const result = await generateScoringBreakdown({
        analysisReportPath: analysisArtifact.storageRef,
        scoringReportPath,
        sourceFile,
        projectId: run?.id,
        runId: run?.id,
      })
      if (!result.success) {
        setContextError(result.error || '生成评分标准拆解表失败')
        return
      }
      if (result.data) {
        onReportGenerated(result.data.artifact, result.data.scoringReportPath, sourceFile)
      }
    } catch (e: unknown) {
      setContextError(e instanceof Error ? e.message : '生成评分标准拆解表失败')
    } finally {
      setGeneratingOutline(false)
    }
  }

  const handleGenerateOutline = async () => {
    if (!analysisArtifact?.storageRef || !contextArtifact?.storageRef) return
    setGeneratingOutline(true)
    setContextError(null)
    try {
      const sourceFile = typeof analysisArtifact.metadata?.sourceFile === 'string'
        ? analysisArtifact.metadata.sourceFile
        : ''
      const outlinePath = deriveOutlinePath(analysisArtifact.storageRef)
      const result = await generateOutline({
        analysisReportPath: analysisArtifact.storageRef,
        contextReportPath: contextArtifact.storageRef,
        scoringReportPath: scoringArtifact?.storageRef || deriveScoringBreakdownPath(analysisArtifact.storageRef),
        outlinePath,
        sourceFile,
      })
      if (!result.success) {
        setContextError(result.error || '生成大纲失败')
        return
      }
      if (result.data) {
        onReportGenerated(result.data.artifact, result.data.outlinePath, sourceFile)
      }
    } catch (e: unknown) {
      setContextError(e instanceof Error ? e.message : '生成大纲失败')
    } finally {
      setGeneratingOutline(false)
    }
  }

  const handleGenerateChapterTaskBook = async () => {
    if (!outlineArtifact?.storageRef) return
    const existing = artifacts.find(a => a.kind === 'BID_CHAPTER_TASK_BOOK')
    if (existing && !window.confirm('当前已存在章节写作任务书，重新生成将覆盖原产物。是否继续？')) {
      return
    }
    setGeneratingTaskBook(true)
    setContextError(null)
    try {
      const taskBookPath = deriveChapterTaskBookPath(outlineArtifact.storageRef)
      const result = await generateChapterTaskBook({
        outlinePath: outlineArtifact.storageRef,
        scoringReportPath: scoringArtifact?.storageRef || deriveScoringBreakdownPath(analysisArtifact?.storageRef || ''),
        analysisReportPath: analysisArtifact?.storageRef || '',
        contextReportPath: contextArtifact?.storageRef,
        taskBookPath,
        mode: 'strict',
      })
      if (result.artifact) {
        onReportGenerated(result.artifact, result.taskBookPath, '')
      }
    } catch (e: unknown) {
      setContextError(e instanceof Error ? e.message : '生成章节写作任务书失败')
    } finally {
      setGeneratingTaskBook(false)
    }
  }

  const handleGenerateChapters = async () => {
    const taskBookArtifact = artifacts.find(a => a.kind === 'BID_CHAPTER_TASK_BOOK')
    if (!taskBookArtifact?.storageRef || !outlineArtifact?.storageRef) return

    const existingChapters = artifacts.filter(a => a.kind === 'BID_CHAPTERS')
    if (existingChapters.length > 0 && !window.confirm(
      `当前已存在 ${existingChapters.length} 个章节初稿，重新生成将覆盖。是否继续？`
    )) return

    setGeneratingChapters(true)
    setContextError(null)
    try {
      const outputDir = deriveChapterOutputDir(outlineArtifact.storageRef)
      const result = await generateChapters({
        taskBookPath: taskBookArtifact.storageRef,
        outlinePath: outlineArtifact.storageRef,
        scoringReportPath: scoringArtifact?.storageRef || '',
        analysisReportPath: analysisArtifact?.storageRef || '',
        contextReportPath: contextArtifact?.storageRef,
        outputDir,
      })

      for (const ch of result.chapters) {
        if (!ch.error) {
          onReportGenerated(ch.artifact, ch.filePath, '')
        }
      }

      if (result.failed > 0) {
        setContextError(`${result.failed}/${result.chapters.length} 个章节生成失败`)
      }
      setContextError(`${result.success}/${result.chapters.length}章撰写完成，总字数${result.totalWordCount}字`)
    } catch (e: unknown) {
      setContextError(e instanceof Error ? e.message : '分章撰写失败')
    } finally {
      setGeneratingChapters(false)
    }
  }

  const handleQuestionAnswerChange = (questionId: string, answer: ProjectContextQuestionWithAnswer['answer']) => {
    setContextQuestionnaire((prev) => {
      if (!prev) return prev
      const now = new Date().toISOString()
      const updated: ProjectContextQuestionnaire = {
        ...prev,
        updatedAt: now,
        questions: prev.questions.map((question) =>
          question.id === questionId ? { ...question, answer } : question,
        ),
        audit: [...prev.audit, { type: 'answer_changed', questionId, at: now }],
      }
      // Auto-save to localStorage on every answer change
      try {
        localStorage.setItem(questionnaireStorageKey, JSON.stringify(updated))
      } catch { /* ignore */ }
      return updated
    })
  }

  const handleSaveQuestionnaireDraft = async () => {
    if (!contextQuestionnaire) return
    setSavingDraft(true)
    setContextError(null)
    await saveQuestionnaireDraft(contextQuestionnaire)
    setSavingDraft(false)
    // Show "已保存 ✓" for 2 seconds
    setSavedRecently(true)
    setTimeout(() => setSavedRecently(false), 2000)
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
        {run?.metadata?.localHistoryFallback === true && (
          <div className="mt-3 rounded-lg bg-amber-50 p-3 text-xs font-semibold text-primary-dark ring-1 ring-amber-200">
            云端任务记录已不可用，当前页面基于本地历史和本地产物文件恢复。
          </div>
        )}
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

      {contextError && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 flex items-start gap-2">
          <FiAlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
          <span>{contextError}</span>
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

      {/* Project context flow */}
      {analysisArtifact?.status === 'valid' && contextArtifact?.status !== 'valid' && (
        <div className="rounded-lg bg-blue-50 p-4 ring-1 ring-blue-200">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-bold text-blue-800">
                {contextQuestionnaire ? '项目背景问卷' : '解析报告已完成，请补充项目背景信息'}
              </p>
              <p className="mt-1 text-xs text-blue-600">
                {contextQuestionnaire
                  ? `已完成 ${contextQuestionnaire.questions.filter((q) => q.answer.selected.length > 0 || q.answer.text.trim() || q.answer.extraText.trim()).length} / ${contextQuestionnaire.questions.length} 项`
                  : '按照标书撰写流程，下一步需要补充项目背景信息，确认后才能生成技术标大纲。'}
              </p>
            </div>
            {!contextQuestionnaire ? (
              <button
                onClick={handleGenerateQuestions}
                disabled={generatingQuestions}
                className="shrink-0 inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-bold text-white hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                {generatingQuestions ? <><FiRefreshCw className="animate-spin" /> 生成中...</> : <><FiSend /> 开始补充项目信息</>}
              </button>
            ) : (
              <button
                onClick={() => setContextDialogOpen(true)}
                className="shrink-0 inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-bold text-white hover:bg-blue-700 transition-colors"
              >
                继续逐项回答
              </button>
            )}
          </div>
        </div>
      )}
      {/* Scoring breakdown generation */}
      {analysisArtifact?.status === 'valid' && contextArtifact?.status === 'valid' && (
        <div className="rounded-lg bg-amber-50 p-4 ring-1 ring-amber-200">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-bold text-amber-800">
                {scoringArtifact?.status === 'valid' ? '评分拆解表已生成，可重新生成' : '请生成评分标准拆解表'}
              </p>
              <p className="mt-1 text-xs text-amber-600">
                基于解析报告提取评分办法，生成评分标准拆解表，为大纲生成提供准确的得分点依据
              </p>
            </div>
            <button
              onClick={handleGenerateScoringBreakdown}
              disabled={generatingOutline}
              className="inline-flex items-center gap-2 rounded-lg bg-amber-500 px-4 py-2 text-sm font-black text-white shadow-sm hover:bg-amber-600 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {generatingOutline ? (
                <><FiRefreshCw className="animate-spin" /> 生成中...</>
              ) : scoringArtifact?.status === 'valid' ? (
                <><FiRefreshCw /> 重新生成评分拆解表</>
              ) : (
                <><FiZap /> 生成评分拆解表</>
              )}
            </button>
          </div>
        </div>
      )}

      {/* Outline generation */}
      {analysisArtifact?.status === 'valid' && contextArtifact?.status === 'valid' && (
        <div className="rounded-lg bg-green-50 p-4 ring-1 ring-green-200">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-bold text-green-800">
                {outlineArtifact?.status === 'valid' ? '大纲已生成，可重新生成' : '项目背景信息已确认，可生成技术标大纲'}
              </p>
              <p className="mt-1 text-xs text-green-600">
                基于解析报告和项目背景确认表，生成四层级技术标大纲
              </p>
            </div>
            <button
              onClick={handleGenerateOutline}
              disabled={generatingOutline}
              className="inline-flex items-center gap-2 rounded-lg bg-green-600 px-4 py-2 text-sm font-bold text-white hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {generatingOutline ? (
                <><FiRefreshCw className="animate-spin" /> 生成中...</>
              ) : outlineArtifact?.status === 'valid' ? (
                <><FiRefreshCw /> 重新生成大纲</>
              ) : (
                <><FiZap /> 生成技术标大纲</>
              )}
            </button>
          </div>
        </div>
      )}

      {/* If no context report but analysis exists, show gate notice for outline */}
      {analysisArtifact?.status === 'valid' && contextArtifact?.status !== 'valid' && (
        <div className="rounded-lg border border-dashed border-ink-muted bg-white p-3 text-xs text-ink-muted">
          请先完成项目背景信息确认表，再生成技术标大纲。
        </div>
      )}

      <BiaoshuArtifactTable artifacts={artifacts} onView={handleView} onGenerateTaskBook={handleGenerateChapterTaskBook} generatingTaskBook={generatingTaskBook} onGenerateChapters={handleGenerateChapters} generatingChapters={generatingChapters} />

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
          peerArtifacts={artifacts.filter(a => a.id !== viewingArtifact.id && a.status === 'valid')}
        />
      )}

      {/* Project context questionnaire dialog */}
      {contextQuestionnaire && (
        <BiaoshuProjectContextDialog
          open={contextDialogOpen}
          questionnaire={contextQuestionnaire}
          currentIndex={contextQuestionIndex}
          saving={savingDraft}
          savedRecently={savedRecently}
          submitting={generatingContextReport}
          error={contextError}
          onIndexChange={(index) => { setContextQuestionIndex(index); setContextError(null) }}
          onAnswerChange={handleQuestionAnswerChange}
          onSaveDraft={handleSaveQuestionnaireDraft}
          onSubmit={handleGenerateContextReport}
          onClose={() => setContextDialogOpen(false)}
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

function BiaoshuArtifactTable({ artifacts, onView, onGenerateTaskBook, generatingTaskBook, onGenerateChapters, generatingChapters }: { artifacts: BiaoshuArtifactRecord[]; onView: (a: BiaoshuArtifactRecord) => void; onGenerateTaskBook?: () => void; generatingTaskBook?: boolean; onGenerateChapters?: () => void; generatingChapters?: boolean }) {
  const headers = ['ID', '名称', '类型', '状态', '负责人', '路径', '操作']
  return (
    <section className="card overflow-hidden p-0">
      <div className="overflow-x-auto">
        <table className="min-w-[1120px] w-full table-fixed text-left text-sm">
          <colgroup>
            <col className="w-[19%]" />
            <col className="w-[19%]" />
            <col className="w-[8%]" />
            <col className="w-[8%]" />
            <col className="w-[8%]" />
            <col className="w-[27%]" />
            <col className="w-[160px]" />
          </colgroup>
          <thead className="bg-background-mist text-xs text-ink-soft">
            <tr>
              {headers.map((header) => (
                <th className={`whitespace-nowrap px-4 py-3 ${header === '操作' ? 'sticky right-0 z-20 bg-background-mist' : ''}`} key={header}>{header}</th>
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
                <td className="px-4 py-3 font-mono text-xs text-ink-muted" title={artifact.storageRef}>
                  <div className="truncate">{artifact.storageRef || '-'}</div>
                </td>
                <td className="sticky right-0 z-10 w-[160px] whitespace-nowrap bg-white/95 px-4 py-3 shadow-[-10px_0_18px_-18px_rgba(0,0,0,0.35)]">
                  <div className="flex flex-wrap items-center justify-end gap-1.5">
                    <BiaoshuCopyButton value={biaoshuArtifactToCopyText(artifact)} label="复制" />
                    {artifact.status === 'valid' && artifact.storageRef && (
                      <button
                        onClick={() => onView(artifact)}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-primary-soft px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-primary-200 hover:bg-primary-100"
                      >
                        <FiEye /> 查看/修改
                      </button>
                    )}
                    {artifact.kind === 'BID_OUTLINE' && artifact.status === 'valid' && onGenerateTaskBook && (
                      <button
                        onClick={onGenerateTaskBook}
                        disabled={generatingTaskBook}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-50 px-2.5 py-1.5 text-xs font-black text-emerald-700 ring-1 ring-emerald-200 hover:bg-emerald-100 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {generatingTaskBook ? '生成中...' : '生成任务书'}
                      </button>
                    )}
                    {artifact.kind === 'BID_CHAPTER_TASK_BOOK' && artifact.status === 'valid' && onGenerateChapters && (
                      <button
                        onClick={onGenerateChapters}
                        disabled={generatingChapters}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-green-50 px-2.5 py-1.5 text-xs font-black text-green-700 ring-1 ring-green-200 hover:bg-green-100 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {generatingChapters ? '撰写中...' : '分章撰写'}
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
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
  peerArtifacts = [],
}: {
  artifact: BiaoshuArtifactRecord
  content: string
  format: string
  loading: boolean
  error: string | null
  run: AgentRun | null
  onClose: () => void
  onContentUpdate?: (newContent: string) => void
  peerArtifacts?: BiaoshuArtifactRecord[]
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
  // Reference artifact selections: kind → checked
  const [referenceSelections, setReferenceSelections] = useState<Record<string, boolean>>({})
  const [referencePanelOpen, setReferencePanelOpen] = useState(false)

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

  // Initialize reference selections: all valid peer artifacts default checked
  useEffect(() => {
    if (peerArtifacts.length > 0 && Object.keys(referenceSelections).length === 0) {
      const initial: Record<string, boolean> = {}
      for (const a of peerArtifacts) {
        if (a.status === 'valid' && a.storageRef && a.kind !== artifact.kind) {
          initial[a.kind] = true
        }
      }
      setReferenceSelections(initial)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [peerArtifacts, artifact.kind])

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

      // 2. Load checked reference artifacts
      const loadedRefs: ReferenceArtifact[] = []
      const selectedKinds = Object.entries(referenceSelections)
        .filter(([, v]) => v)
        .map(([k]) => k)
      if (selectedKinds.length > 0) {
        for (const peer of peerArtifacts) {
          if (selectedKinds.includes(peer.kind) && peer.storageRef) {
            try {
              const data = await readBiaoshuArtifact(peer.storageRef)
              loadedRefs.push({ kind: peer.kind, name: peer.name, content: data.content })
            } catch {
              // Silently skip a reference artifact if reading fails
            }
          }
        }
      }

      // 3. Call cloud revise endpoint
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
        referenceArtifacts: loadedRefs.length > 0 ? loadedRefs : undefined,
      }
      const result = await reviseBiaoshuArtifact(revisePayload)

      // Check for JSON wrapper format
      if (looksLikeWrappedRevisionJSON(result.revisedContent)) {
        setAiError('AI 返回了异常 JSON 外壳，未生成可应用的 Markdown 正文。请重新生成或缩小修改范围。')
        setAiLoading(false)
        return
      }

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

    // Safety: block JSON wrapper from being written back
    if (looksLikeWrappedRevisionJSON(revisedContent)) {
      setAiError('当前预览内容不是有效 Markdown，已阻止写回文件。')
      return
    }

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

  const looksLikeWrappedRevisionJSON = (value: string): boolean => {
    const trimmed = value.trim()
    return trimmed.startsWith('{') && trimmed.includes('"revisedContent"')
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
        <div
          className="grid min-h-0 grid-cols-1 divide-y divide-line lg:grid-cols-2 lg:divide-x lg:divide-y-0"
          style={{ height: 'calc(90vh - 77px)' }}
        >
          {/* Left: Artifact content */}
          <div className="min-h-0 overflow-y-auto px-6 py-5">
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
          <div className="flex h-full min-h-0 flex-col overflow-hidden">
            {/* Reference artifact selector */}
            {isTextFormat && peerArtifacts.length > 0 && (
              <div className="border-b border-line px-4 py-2">
                <button
                  onClick={() => setReferencePanelOpen(v => !v)}
                  className="flex w-full items-center justify-between text-xs font-bold text-ink-soft hover:text-ink"
                >
                  <span>参考产物</span>
                  <span className="flex items-center gap-1">
                    <span className="text-ink-muted">
                      {Object.values(referenceSelections).filter(Boolean).length}/{peerArtifacts.length}
                    </span>
                    <FiChevronDown className={`transform transition-transform ${referencePanelOpen ? 'rotate-180' : ''}`} size={14} />
                  </span>
                </button>
                {referencePanelOpen && (
                  <div className="mt-2 max-h-[30vh] space-y-1 overflow-y-auto overscroll-contain">
                    {peerArtifacts.map((peer) => (
                      <label
                        key={peer.kind}
                        className="flex items-center gap-2 rounded px-2 py-1.5 text-xs hover:bg-white/60 cursor-pointer"
                      >
                        <input
                          type="checkbox"
                          checked={referenceSelections[peer.kind] || false}
                          onChange={() =>
                            setReferenceSelections(prev => ({ ...prev, [peer.kind]: !prev[peer.kind] }))
                          }
                          className="h-3.5 w-3.5 rounded accent-primary"
                        />
                        <span className="text-ink">{peer.name}</span>
                        <span className="text-ink-muted font-mono text-[10px]">({peer.kind})</span>
                      </label>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Chat messages */}
            <div className="min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain px-4 py-3">
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
                <div className="min-w-0 rounded-xl border border-primary/30 bg-primary/5 p-4">
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
                  <div className="max-h-[42vh] overflow-y-auto overscroll-contain rounded-lg border border-line bg-white p-4 text-xs leading-relaxed">
                    <ReactMarkdown>{revisedContent}</ReactMarkdown>
                  </div>
                </div>
              )}
            </div>

            {/* Chat input */}
            {isTextFormat && (
              <div className="shrink-0 border-t border-line bg-white px-4 py-3">
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
