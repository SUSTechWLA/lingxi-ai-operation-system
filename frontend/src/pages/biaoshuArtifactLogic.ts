import type { AgentReviewItem, AgentRun, AgentStep } from '../utils/types'

export type BiaoshuArtifactStatus = 'valid' | 'review' | 'pending' | 'running' | 'failed' | 'missing'

export interface BiaoshuArtifactRecord {
  id: string
  name: string
  kind: string
  version: string
  status: BiaoshuArtifactStatus
  owner: string
  updatedAt: string
  storageRef: string
  summary: string
  sourceTool: string
  metadata?: Record<string, unknown>
}

interface BiaoshuStageDefinition {
  key: string
  label: string
  tool: string
  kind: string
  owner: string
}

interface TraceNodeLike {
  id?: string
  name?: string
  type?: string
  status?: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  error?: string
  errorMessage?: string
  createdAt?: string
  updatedAt?: string
  completedAt?: string
  tool?: string
}

export interface BiaoshuFallbackRunInput {
  runId: string
  projectName: string
  bidFilePath: string
  status?: 'CREATED' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'UNKNOWN'
  createdAt?: string
  updatedAt?: string
}

export function createBiaoshuFallbackRun(input: BiaoshuFallbackRunInput): AgentRun {
  const now = new Date().toISOString()
  const status =
    input.status === 'CREATED' ||
    input.status === 'RUNNING' ||
    input.status === 'SUCCESS' ||
    input.status === 'FAILED'
      ? input.status
      : 'SUCCESS'

  return {
    id: input.runId,
    domain: 'bid_writing',
    message: `本地恢复标书项目：${input.projectName || '未命名项目'}`,
    status,
    createdAt: input.createdAt || now,
    updatedAt: input.updatedAt || now,
    metadata: {
      projectName: input.projectName,
      project_name: input.projectName,
      filePath: input.bidFilePath,
      file_path: input.bidFilePath,
      bidFilePath: input.bidFilePath,
      localHistoryFallback: true,
    },
    plan: {
      goal: 'Open local Biaoshu historical artifacts',
      domain: 'bid_writing',
      mode: 'local_history_fallback',
      steps: [
        {
          id: 'parse-local-history',
          tool: 'parse_bid_files',
          arguments: { filePath: input.bidFilePath },
          expectedOutput: ['BID_RAW_TEXT'],
        },
        {
          id: 'analysis-local-history',
          tool: 'bid_analysis_report',
          arguments: { filePath: input.bidFilePath },
          expectedOutput: ['BID_ANALYSIS'],
        },
        {
          id: 'context-local-history',
          tool: 'project_context_report',
          arguments: { filePath: input.bidFilePath },
          expectedOutput: ['BID_PROJECT_CONTEXT'],
        },
        {
          id: 'outline-local-history',
          tool: 'outline_generator',
          arguments: { filePath: input.bidFilePath },
          expectedOutput: ['BID_OUTLINE'],
        },
      ],
    },
  }
}

const BIAOSHU_STAGES: BiaoshuStageDefinition[] = [
  { key: 'raw-parse', label: '招标文件原文解析', tool: 'parse_bid_files', kind: 'BID_RAW_TEXT', owner: '文件解析' },
  { key: 'parse', label: '招标文件解析报告', tool: 'bid_analysis_report', kind: 'BID_ANALYSIS', owner: 'AI分析' },
  { key: 'context', label: '项目背景信息确认表', tool: 'project_context_report', kind: 'BID_PROJECT_CONTEXT', owner: '信息确认' },
  { key: 'scoring', label: '评分标准拆解表', tool: 'scoring_breakdown_generator', kind: 'BID_SCORING_BREAKDOWN', owner: '评分拆解' },
  { key: 'outline', label: '技术标大纲', tool: 'outline_generator', kind: 'BID_OUTLINE', owner: '大纲规划' },
  { key: 'chapters', label: '章节初稿', tool: 'chapter_writer', kind: 'BID_CHAPTERS', owner: '章节编写' },
  { key: 'wordcheck', label: '字数检查报告', tool: 'chapter_word_checker', kind: 'WORD_COUNT_REPORT', owner: '质量检查' },
  { key: 'merge', label: '整合成稿', tool: 'merge_chapters', kind: 'MERGED_DRAFT', owner: '成稿整合' },
  { key: 'word', label: '技术标 Word 文档', tool: 'convert_to_word', kind: 'TECHNICAL_BID_DOCX', owner: 'Word 导出' },
]

export function deriveBiaoshuAnalysisReportPath(rawTextPath: string): string {
  return rawTextPath.replace(/原文解析/g, '解析报告')
}

export function deriveBiaoshuProjectContextPath(analysisReportPath: string): string {
  return joinBiaoshuSiblingPath(analysisReportPath, '01_项目背景信息确认表.md')
}

export function deriveBiaoshuProjectContextQuestionnairePath(analysisReportPath: string): string {
  return joinBiaoshuSiblingPath(analysisReportPath, '01_项目背景信息问答记录.json')
}

export function deriveBiaoshuOutlinePath(analysisReportPath: string): string {
  return joinBiaoshuSiblingPath(analysisReportPath, '03_技术标四级大纲.md')
}

export function deriveBiaoshuScoringBreakdownPath(analysisReportPath: string): string {
  return joinBiaoshuSiblingPath(analysisReportPath, '02_评分标准拆解表.md')
}

function joinBiaoshuSiblingPath(basePath: string, fileName: string): string {
  const trimmed = basePath.trim()
  if (!trimmed) return fileName
  const slashIndex = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'))
  if (slashIndex < 0) return fileName
  return `${trimmed.slice(0, slashIndex + 1)}${fileName}`
}

export function deriveBiaoshuChapterTaskBookPath(outlinePath: string): string {
  return joinBiaoshuSiblingPath(outlinePath, '04_章节写作任务书.md')
}

export function deriveChapterOutputDir(outlinePath: string): string {
  // Derive 章节/ directory as sibling to the outline file
  const dir = outlinePath.replace(/[\\/][^\\/]+$/, '') // remove file name
  return `${dir}/章节`
}

// ── Chapter artifact creation ──

export function createManualChapterArtifact(
  artifact: Record<string, unknown> | undefined,
  filePath: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  const chapterNumber = Number(artifact?.chapterNumber || metadata?.chapterNumber || 0)
  const chapterTitle = String(artifact?.chapterTitle || artifact?.name || '')
  return {
    id: String(artifact?.id || artifact?.unitId || `chapter-${chapterNumber}`),
    name: chapterTitle || `第${toChineseNumber(chapterNumber)}章`,
    kind: 'BID_CHAPTERS',
    version: '-',
    status: 'valid',
    owner: 'AI写作',
    updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
    storageRef: String(artifact?.storageRef || filePath),
    summary: `第${toChineseNumber(chapterNumber)}章初稿，${metadata?.targetWords ? `目标${metadata.targetWords}字` : ''}`,
    sourceTool: 'chapter_generation',
    metadata,
  }
}

function toChineseNumber(n: number): string {
  if (n <= 0) return '零'
  const digits = ['', '一', '二', '三', '四', '五', '六', '七', '八', '九']
  const tens = ['', '', '二十', '三十', '四十', '五十', '六十', '七十', '八十', '九十']
  if (n <= 10) return ['', '一', '二', '三', '四', '五', '六', '七', '八', '九', '十'][n]
  const t = Math.floor(n / 10), d = n % 10
  return tens[t] + (d ? digits[d] : '')
}

export function buildBiaoshuArtifacts(
  run: AgentRun | null | undefined,
  trace: unknown = undefined,
  reviews: AgentReviewItem[] = [],
): BiaoshuArtifactRecord[] {
  const steps = run?.plan?.steps || []
  const traceNodes = extractTraceNodes(trace)
  const plannedStages = BIAOSHU_STAGES.filter((stage) =>
    steps.length === 0 || steps.some((step) => stepMatchesStage(step, stage)),
  )
  const stages = plannedStages.length ? plannedStages : BIAOSHU_STAGES

  return stages.map((stage, index) => {
    const step = steps.find((item) => stepMatchesStage(item, stage))
    const node = traceNodes.find((item) => nodeMatchesStage(item, stage))
    const artifact = findOutputArtifact(node?.output, stage.kind)
    const review = reviews.find((item) => itemMatchesStage(item, stage))
    const storageRef = storageRefFor(node?.output, artifact, stage.kind)
    const status = statusFor(step, node, review, storageRef)
    const updatedAt = formatTime(
      stringValue(artifact?.updatedAt) ||
      stringValue(artifact?.createdAt) ||
      node?.completedAt ||
      node?.updatedAt ||
      node?.createdAt ||
      run?.updatedAt,
    )

    return {
      id: String(artifact?.id || artifact?.artifactId || `B${String(index + 1).padStart(2, '0')}`),
      name: String(artifact?.name || stage.label),
      kind: String(artifact?.kind || stage.kind),
      version: versionLabel(artifact),
      status,
      owner: stage.owner,
      updatedAt,
      storageRef,
      summary: summaryFor(node?.output, artifact, review),
      sourceTool: stage.tool,
      metadata: objectValue(artifact?.metadata) || objectValue(node?.output),
    }
  })
}

export function displayNameForBiaoshuArtifact(kind: string): string {
  const labels: Record<string, string> = {
    BID_RAW_TEXT: '原文解析',
    BID_ANALYSIS: '招标解析',
    BID_PROJECT_CONTEXT: '项目背景',
    BID_SCORING_BREAKDOWN: '评分拆解',
    BID_OUTLINE: '标书大纲',
    BID_CHAPTER_TASK_BOOK: '写作任务书',
    BID_CHAPTERS: '章节稿件',
    WORD_COUNT_REPORT: '字数检查',
    MERGED_DRAFT: '整合成稿',
    TECHNICAL_BID_DOCX: 'Word 文档',
  }
  return labels[kind] || kind
}

export function biaoshuArtifactToCopyText(artifact: BiaoshuArtifactRecord): string {
  return JSON.stringify({
    id: artifact.id,
    name: artifact.name,
    kind: artifact.kind,
    version: artifact.version,
    status: artifact.status,
    owner: artifact.owner,
    updatedAt: artifact.updatedAt,
    storageRef: artifact.storageRef,
    summary: artifact.summary,
    sourceTool: artifact.sourceTool,
  }, null, 2)
}

export function mergeManualBiaoshuArtifacts(
  artifacts: BiaoshuArtifactRecord[],
  manualArtifacts: BiaoshuArtifactRecord[],
): BiaoshuArtifactRecord[] {
  if (manualArtifacts.length === 0) return artifacts

  return manualArtifacts.reduce((current, manualArtifact) => {
    return mergeOneManualArtifact(current, manualArtifact)
  }, artifacts)
}

export function mergeManualReportArtifact(
  artifacts: BiaoshuArtifactRecord[],
  manualReportArtifact: BiaoshuArtifactRecord | null,
): BiaoshuArtifactRecord[] {
  return mergeManualBiaoshuArtifacts(
    artifacts,
    manualReportArtifact ? [manualReportArtifact] : [],
  )
}

function mergeOneManualArtifact(
  artifacts: BiaoshuArtifactRecord[],
  manualArtifact: BiaoshuArtifactRecord,
): BiaoshuArtifactRecord[] {
  let replaced = false
  const merged = artifacts.map((artifact) => {
    if (artifact.kind !== manualArtifact.kind) return artifact
    replaced = true
    return manualArtifact
  })
  if (replaced) return merged

  const previousKind = previousBiaoshuStageKind(manualArtifact.kind)
  const previousIndex = merged.findIndex((artifact) => artifact.kind === previousKind)
  if (previousIndex >= 0) {
    return [
      ...merged.slice(0, previousIndex + 1),
      manualArtifact,
      ...merged.slice(previousIndex + 1),
    ]
  }
  return [...merged, manualArtifact]
}

function previousBiaoshuStageKind(kind: string): string {
  if (kind === 'BID_PROJECT_CONTEXT') return 'BID_ANALYSIS'
  if (kind === 'BID_SCORING_BREAKDOWN') return 'BID_PROJECT_CONTEXT'
  if (kind === 'BID_OUTLINE') return 'BID_SCORING_BREAKDOWN'
  if (kind === 'BID_CHAPTERS') return 'BID_OUTLINE'
  if (kind === 'WORD_COUNT_REPORT') return 'BID_CHAPTERS'
  if (kind === 'MERGED_DRAFT') return 'WORD_COUNT_REPORT'
  if (kind === 'TECHNICAL_BID_DOCX') return 'MERGED_DRAFT'
  return 'BID_RAW_TEXT'
}

export function createManualReportArtifact(
  artifact: Record<string, unknown> | undefined,
  reportPath: string,
  sourceFile: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  if (sourceFile) metadata.sourceFile = sourceFile
  metadata.manualGenerated = true

  return {
    id: String(artifact?.id || artifact?.artifactId || 'manual-bid-analysis'),
    name: String(artifact?.name || '招标文件解析报告'),
    kind: 'BID_ANALYSIS',
    version: '-',
    status: 'valid',
    owner: 'AI分析',
    updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
    storageRef: String(artifact?.storageRef || artifact?.storage_ref || reportPath),
    summary: String(artifact?.summary || '手动生成的招标文件解析报告'),
    sourceTool: 'bid_analysis_report',
    metadata,
  }
}

export function createManualProjectContextArtifact(
  artifact: Record<string, unknown> | undefined,
  reportPath: string,
  sourceFile: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  if (sourceFile) metadata.sourceFile = sourceFile
  metadata.manualGenerated = true

  return {
    id: String(artifact?.id || artifact?.artifactId || 'manual-project-context'),
    name: String(artifact?.name || '项目背景信息确认表'),
    kind: 'BID_PROJECT_CONTEXT',
    version: '-',
    status: 'valid',
    owner: '信息确认',
    updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
    storageRef: String(artifact?.storageRef || artifact?.storage_ref || reportPath),
    summary: String(artifact?.summary || '手动生成的项目背景信息确认表'),
    sourceTool: 'project_context_report',
    metadata,
  }
}

export function createManualScoringBreakdownArtifact(
  artifact: Record<string, unknown> | undefined,
  reportPath: string,
  sourceFile: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  if (sourceFile) metadata.sourceFile = sourceFile
  metadata.manualGenerated = true

  return {
    id: String(artifact?.id || artifact?.artifactId || 'manual-scoring-breakdown'),
    name: String(artifact?.name || '评分标准拆解表'),
    kind: 'BID_SCORING_BREAKDOWN',
    version: '-',
    status: 'valid',
    owner: '评分拆解',
    updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
    storageRef: String(artifact?.storageRef || artifact?.storage_ref || reportPath),
    summary: String(artifact?.summary || '手动生成的评分标准拆解表'),
    sourceTool: 'scoring_breakdown_generator',
    metadata,
  }
}

export function createManualOutlineArtifact(
  artifact: Record<string, unknown> | undefined,
  outlinePath: string,
  sourceFile: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  if (sourceFile) metadata.sourceFile = sourceFile
  metadata.manualGenerated = true

  return {
    id: String(artifact?.id || artifact?.artifactId || 'manual-outline'),
    name: String(artifact?.name || '技术标四级大纲'),
    kind: 'BID_OUTLINE',
    version: '-',
    status: 'valid',
    owner: '大纲规划',
    updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
    storageRef: String(artifact?.storageRef || artifact?.storage_ref || outlinePath),
    summary: String(artifact?.summary || '手动生成的技术标四级大纲'),
    sourceTool: 'outline_generator',
    metadata,
  }
}

export function createManualChapterTaskBookArtifact(
  artifact: Record<string, unknown> | undefined,
  taskBookPath: string,
  sourceFile: string,
): BiaoshuArtifactRecord {
  const metadata = objectRecord(artifact?.metadata)
  if (sourceFile) metadata.sourceFile = sourceFile
  metadata.manualGenerated = true

  return {
    id: String(artifact?.id || artifact?.artifactId || 'manual-chapter-task-book'),
    name: String(artifact?.name || '04_章节写作任务书.md'),
    kind: 'BID_CHAPTER_TASK_BOOK',
    version: '-',
    status: 'valid',
    owner: '任务规划',
    updatedAt: new Date().toLocaleString('zh-CN', { hour12: false }),
    storageRef: String(artifact?.storageRef || artifact?.storage_ref || taskBookPath),
    summary: String(artifact?.summary || '章节写作任务书'),
    sourceTool: 'chapter_task_book_generator',
    metadata,
  }
}

function stepMatchesStage(step: AgentStep, stage: BiaoshuStageDefinition): boolean {
  const tool = step.tool || ''
  const expected = step.expectedOutput || []
  return tool.includes(stage.tool) ||
    tool.includes(stage.key) ||
    expected.includes(stage.kind) ||
    Object.values(step.arguments || {}).some((value) => String(value).includes(stage.tool))
}

function nodeMatchesStage(node: TraceNodeLike, stage: BiaoshuStageDefinition): boolean {
  const input = node.input || {}
  const output = node.output || {}
  const candidates = [
    node.tool,
    node.name,
    stringValue(input.tool),
    stringValue(input.capabilityTool),
    stringValue(input.capability_tool),
    stringValue(output.tool),
    stringValue(output.capabilityTool),
  ].filter(Boolean).join(' ')

  return candidates.includes(stage.tool) || candidates.includes(stage.key)
}

function itemMatchesStage(review: AgentReviewItem, stage: BiaoshuStageDefinition): boolean {
  return Boolean(
    review.tool?.includes(stage.tool) ||
    review.tool?.includes(stage.key) ||
    review.stage?.includes(stage.key) ||
    review.requiredOutputs?.includes(stage.kind) ||
    review.reviewArtifactKinds?.includes(stage.kind),
  )
}

function statusFor(
  step: AgentStep | undefined,
  node: TraceNodeLike | undefined,
  review: AgentReviewItem | undefined,
  storageRef: string,
): BiaoshuArtifactStatus {
  if (review?.status === 'PENDING') return 'review'
  if (!step && !node) return 'pending'
  const raw = String(node?.status || '').toUpperCase()
  if (['FAILED', 'ERROR', 'CANCELED', 'CANCELLED'].includes(raw)) return 'failed'
  if (['RUNNING', 'PROCESSING', 'IN_PROGRESS', 'READY'].includes(raw)) return 'running'
  if (['SUCCESS', 'SUCCEEDED', 'COMPLETED', 'DONE'].includes(raw)) return storageRef ? 'valid' : 'missing'
  return step ? 'pending' : 'missing'
}

function findOutputArtifact(output: Record<string, unknown> | undefined, kind: string): Record<string, unknown> | undefined {
  if (!output) return undefined
  const artifacts = Array.isArray(output.artifacts) ? output.artifacts : []
  for (const item of artifacts) {
    const artifact = objectValue(item)
    if (!artifact) continue
    const artifactKind = stringValue(artifact.kind) || stringValue(artifact.artifactKind)
    if (!artifactKind || artifactKind === kind) return artifact
  }
  return undefined
}

function storageRefFor(
  output: Record<string, unknown> | undefined,
  artifact: Record<string, unknown> | undefined,
  kind: string,
): string {
  const fromArtifact = artifact && (
    stringValue(artifact.storageRef) ||
    stringValue(artifact.storage_ref) ||
    stringValue(artifact.url) ||
    stringValue(artifact.path)
  )
  if (fromArtifact) return fromArtifact
  if (!output) return ''

  // 顶层字段
  const topLevel = stringValue(output.storageRef) ||
    stringValue(output.storage_ref) ||
    stringValue(output.output_file) ||
    stringValue(output.outputFile) ||
    stringValue(output.output_path) ||
    stringValue(output.outputPath) ||
    stringValue(output.file) ||
    stringValue(output.path)
  if (topLevel) return topLevel

  // 解析 stdout JSON 字符串，提取嵌套的路径字段（如 data.report_path）
  const stdout = stringValue(output.stdout)
  if (stdout) {
    try {
      const parsed = JSON.parse(stdout) as Record<string, unknown>
      const data = objectValue(parsed.data)
      const preferred = preferredPathForKind(kind, parsed, data)
      if (preferred) return preferred
      return stringValue(parsed.output_path) ||
        stringValue(parsed.report_path) ||
        stringValue(parsed.file) ||
        stringValue(parsed.path) ||
        (data && (
          stringValue(data.output_path) ||
          stringValue(data.report_path) ||
          stringValue(data.file) ||
          stringValue(data.path)
        )) ||
        ''
    } catch {
      // stdout 不是 JSON，忽略
    }
  }

  return ''
}

function preferredPathForKind(
  kind: string,
  parsed: Record<string, unknown>,
  data: Record<string, unknown> | undefined,
): string {
  if (kind === 'BID_RAW_TEXT') {
    return stringValue(parsed.raw_text_path) ||
      stringValue(parsed.rawTextPath) ||
      (data && (
        stringValue(data.raw_text_path) ||
        stringValue(data.rawTextPath)
      )) ||
      ''
  }
  if (kind === 'BID_ANALYSIS') {
    return stringValue(parsed.report_path) ||
      stringValue(parsed.reportPath) ||
      (data && (
        stringValue(data.report_path) ||
        stringValue(data.reportPath)
      )) ||
      ''
  }
  return ''
}

function summaryFor(
  output: Record<string, unknown> | undefined,
  artifact: Record<string, unknown> | undefined,
  review: AgentReviewItem | undefined,
): string {
  return stringValue(artifact?.summary) ||
    stringValue(output?.summary) ||
    stringValue(output?.message) ||
    stringValue(output?.content) ||
    stringValue(review?.reviewContent) ||
    stringValue(review?.reviewReason) ||
    '-'
}

function extractTraceNodes(trace: unknown): TraceNodeLike[] {
  if (Array.isArray(trace)) return toTraceNodes(trace)
  const root = objectValue(trace)
  if (!root) return []
  const direct = arrayValue(root.nodes) || arrayValue(root.traceNodes)
  if (direct) return toTraceNodes(direct)
  const data = objectValue(root.data)
  const task = objectValue(data?.task)
  const nested = arrayValue(task?.nodes) || arrayValue(data?.nodes)
  if (nested) return toTraceNodes(nested)
  return []
}

function toTraceNodes(values: unknown[]): TraceNodeLike[] {
  return values
    .map(objectValue)
    .filter((item): item is Record<string, unknown> => Boolean(item))
    .map((item) => item as TraceNodeLike)
}

function versionLabel(artifact: Record<string, unknown> | undefined): string {
  const version = artifact?.version
  if (typeof version === 'number' && Number.isFinite(version)) return `第 ${version} 版`
  if (typeof version === 'string' && version.trim()) return version.startsWith('第') ? version : `第 ${version} 版`
  return '-'
}

function formatTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function objectValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}

function objectRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? { ...(value as Record<string, unknown>) }
    : {}
}

function arrayValue(value: unknown): unknown[] | undefined {
  return Array.isArray(value) ? value : undefined
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined
}
