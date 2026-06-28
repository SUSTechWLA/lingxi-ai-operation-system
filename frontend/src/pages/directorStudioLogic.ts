import type { AgentReviewItem, VideoRoleAgent } from '../utils/types'

export type DirectorNavKey = 'overview' | 'review' | 'trace' | 'assets' | 'roles' | 'export' | 'system'
export type DirectorStageStatus = 'done' | 'active' | 'review' | 'blocked' | 'pending' | 'running' | 'failed'
export type DirectorArtifactStatus = 'valid' | 'review' | 'stale' | 'pending' | 'running' | 'failed' | 'blocked'

export interface DirectorStage {
  id: string
  name: string
  displayName: string
  stage: string
  goal: string
  status: DirectorStageStatus
  progress: number
  allowedTools: string[]
  forbiddenTools: string[]
  requiredInputs: string[]
  requiredOutputs: string[]
  reviewFocus: string[]
  reviewId?: string
}

export interface DirectorArtifactRecord {
  id: string
  name: string
  kind: string
  version: string
  status: DirectorArtifactStatus
  owner: string
  updatedAt: string
  humanApproved: boolean
  storageRef: string
  dependsOn?: string[]
  metadata?: Record<string, unknown>
}

export interface DirectorTraceNode {
  id: string
  role: string
  stage: string
  status: DirectorStageStatus
  tool: string          // readable tool name
  rawName: string       // original node name for debugging
  rawType: string       // node type (TOOL / REVIEW_GATE / CONTROL)
  plane: 'cloud' | 'local'
  input: string
  output: string
  error: string
  duration: string
  createdAt: string
  review: boolean
}

export interface DirectorNextAction {
  stageId: string
  kind: 'review' | 'running' | 'blocked' | 'start'
  label: string
  description: string
}

export interface RenderReadiness {
  allowed: boolean
  missing: string[]
  message?: string
}

export type PublishPlatform = 'xiaohongshu' | 'bilibili'

export interface PublishCopy {
  platform: PublishPlatform
  platformName: string
  title: string
  description: string
  tags: string[]
  coverText: string
  publishTips: string[]
}

interface TraceNodeLike {
  id?: string
  name?: string
  type?: string
  status?: string
  error?: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  errorMessage?: string
  createdAt?: string
  startedAt?: string
  updatedAt?: string
  completedAt?: string
  durationMs?: number
  // Direct node fields (used by some executor implementations)
  tool?: string
  intent?: string
  dependsOn?: string[]
}

export function buildDirectorStages(
  roleAgents: VideoRoleAgent[],
  reviews: AgentReviewItem[] = [],
  trace: unknown = undefined,
): DirectorStage[] {
  const traceNodes = extractTraceNodes(trace)

  return roleAgents.map((role) => {
    const review = findReviewForRole(role, reviews)
    const node = findTraceNodeForRole(role, traceNodes)
    const status = stageStatusFor(role, review, node)

    return {
      id: role.id,
      name: role.name,
      displayName: role.displayName || role.name,
      stage: role.stage,
      goal: role.goal,
      status,
      progress: progressForStatus(status),
      allowedTools: role.allowedTools || [],
      forbiddenTools: role.forbiddenTools || [],
      requiredInputs: role.requiredInputs || [],
      requiredOutputs: role.requiredOutputs || [],
      reviewFocus: review?.humanReview?.reviewFocus || role.humanReview?.reviewFocus || [],
      reviewId: review?.id,
    }
  })
}

export function stageActionLabel(stage: string): string {
  const labels: Record<string, string> = {
    proposal: '定方向',
    script: '写脚本',
    storyboard: '拆画面',
    composition: '排时间轴',
    reference: '定素材',
    continuity: '查一致',
    preview: '看预览',
    render: '出成片',
    quality: '做体检',
    package: '打包',
  }
  return labels[stage] || stage
}

export function downstreamStaleArtifacts(changedKind: string): string[] {
  const labels: Record<string, string> = {
    CARD_PLAN: '卡片分镜',
    VIDEO_COMPOSITION_SPEC: '视频结构',
    REFERENCE_ASSET_PLAN: '素材策略',
    CONTINUITY_REPORT: '一致性报告',
    HYPERFRAMES_PROJECT: '视频项目',
    PREVIEW_SNAPSHOTS: '预览图',
    VIDEO: '最终视频',
    FINAL_REVIEW: '质量报告',
    PROJECT_PACKAGE: '交付包',
  }
  return downstreamKindsFor(changedKind)
    .map((kind) => labels[kind])
    .filter((label): label is string => Boolean(label))
}

export function canStartFinalRender(artifacts: DirectorArtifactRecord[], localRunnerReady: boolean): RenderReadiness {
  const missing: string[] = []
  const byKind = new Map(artifacts.map((artifact) => [artifact.kind, artifact]))
  const composition = byKind.get('VIDEO_COMPOSITION_SPEC')
  const project = byKind.get('HYPERFRAMES_PROJECT')
  const preview = byKind.get('PREVIEW_SNAPSHOTS')

  if (!composition || composition.status !== 'valid' || !composition.humanApproved) {
    missing.push('视频结构尚未确认')
  }
  if (!project || project.status !== 'valid') {
    missing.push('视频项目尚未生成')
  }
  if (!preview || preview.status !== 'valid' || !preview.humanApproved) {
    missing.push('预览图尚未确认')
  }
  if (!localRunnerReady) {
    missing.push('本地执行器未就绪')
  }

  return {
    allowed: missing.length === 0,
    missing,
    message: missing.length ? `暂不能开始最终渲染：${missing.join('、')}` : undefined,
  }
}

export function buildPublishCopies(topic: string, durationSec: number, artifacts: DirectorArtifactRecord[]): PublishCopy[] {
  const normalizedTopic = normalizePublishTopic(topic)
  const hasVideo = artifacts.some((artifact) => artifact.kind === 'VIDEO' && artifact.status === 'valid')
  const hasPrompt = artifacts.some((artifact) => ['CARD_PLAN', 'SHOT_LIST', 'KEYFRAME_PROMPTS'].includes(artifact.kind))
  const baseTags = publishTagsFor(normalizedTopic, hasPrompt)

  return [
    {
      platform: 'xiaohongshu',
      platformName: '小红书',
      title: titleWithin(`${normalizedTopic}：${durationSec}秒讲清楚`, 20),
      description: [
        `这条视频用${durationSec}秒讲「${normalizedTopic}」。`,
        '核心不是堆更多工具，而是把想法、执行、审核和返工放进同一条可追踪工作流。',
        hasVideo ? '成片已生成，可直接手动发布。' : '当前已整理脚本、分镜和 Prompt，可先手动打磨后发布。',
      ].join('\n'),
      tags: baseTags.slice(0, 6),
      coverText: titleWithin(normalizedTopic, 12),
      publishTips: ['封面保留一个核心判断', '正文前两行直接给结论', '发布前检查字幕是否完整'],
    },
    {
      platform: 'bilibili',
      platformName: 'B站',
      title: titleWithin(`${normalizedTopic}｜工作流视角`, 40),
      description: [
        `本视频围绕「${normalizedTopic}」展开，目标时长约 ${durationSec} 秒。`,
        '内容结构：开场观点、关键解释、案例化展开、结尾总结。',
        hasVideo ? '视频文件已进入产物库，请结合最终审核报告检查后手动投稿。' : '当前版本适合作为短片前期稿件，建议确认镜头和 Prompt 后再进入渲染。',
      ].join('\n'),
      tags: uniqueStrings([...baseTags, '知识分享', 'AI工具']).slice(0, 10),
      coverText: titleWithin(`${normalizedTopic}\n工作流视角`, 18),
      publishTips: ['标题保留关键词和明确角度', '简介写清楚视频结构', '选择知识/科技相关分区'],
    },
  ]
}

export function publishCopiesToMarkdown(copies: PublishCopy[]): string {
  return copies.map((copy) => [
    `## ${copy.platformName}`,
    '',
    `**标题**：${copy.title}`,
    '',
    '**正文**：',
    copy.description,
    '',
    `**标签**：${copy.tags.map((tag) => `#${tag}`).join(' ')}`,
    '',
    `**封面文案**：${copy.coverText}`,
    '',
    '**发布建议**：',
    ...copy.publishTips.map((tip) => `- ${tip}`),
  ].join('\n')).join('\n\n')
}

export function publishCopiesToJSON(copies: PublishCopy[]): string {
  return JSON.stringify(copies.map(({ platform, title, description, tags, coverText, publishTips }) => ({
    platform,
    title,
    description,
    tags,
    coverText,
    publishTips,
  })), null, 2)
}

export function buildDirectorArtifacts(
  roleAgents: VideoRoleAgent[],
  reviews: AgentReviewItem[] = [],
  trace: unknown = undefined,
): DirectorArtifactRecord[] {
  const traceNodes = extractTraceNodes(trace)

  return roleAgents.flatMap((role, roleIndex) => {
    const outputs = role.requiredOutputs?.length ? role.requiredOutputs : [`${role.stage}_OUTPUT`]
    const review = findReviewForRole(role, reviews)
    const node = findTraceNodeForRole(role, traceNodes)
    const artifactOutputs = extractArtifacts(node)

    return outputs.map((output, outputIndex) => {
      const artifact = artifactOutputs.find((item) => item.kind === output) || artifactOutputs[outputIndex]
      const manifestStatus = normalizeArtifactStatus(stringValue(artifact?.status))
      const requiresManifest = requiresMaterializedArtifact(output)
      const fallbackStatus = artifactStatusFor(review, node)
      const status = artifact
        ? manifestStatus || fallbackStatus
        : requiresManifest ? 'pending' : fallbackStatus
      const manifestHumanApproved = booleanValue(artifact?.humanApproved)
      const index = roleIndex + 1

      return {
        id: String(artifact?.id || artifact?.artifactId || `A${String(index).padStart(2, '0')}${outputIndex ? `-${outputIndex + 1}` : ''}`),
        name: String(artifact?.name || displayNameForArtifact(output)),
        kind: output,
        version: artifact ? '第1版' : '-',
        status,
        owner: role.displayName || role.name,
        updatedAt: formatTime(node?.createdAt),
        humanApproved: artifact ? manifestHumanApproved ?? status === 'valid' : false,
        storageRef: displayStorageRef(artifact?.storageRef || artifact?.url || (requiresManifest ? '' : storageHintForKind(output))),
        dependsOn: stringArrayValue(artifact?.dependsOn) || role.requiredInputs,
        metadata: objectValue(artifact?.metadata),
      }
    })
  })
}

export function buildDirectorTraceNodes(trace: unknown): DirectorTraceNode[] {
  return extractTraceNodes(trace).map((node, index) => {
    const input = node.input || {}
    const output = node.output || {}

    // Real tool name: the orchestrator stores "external" in node.name;
    // the actual tool is in input.capabilityTool or node.tool.
    const realTool = stringValue(input.capabilityTool) || node.tool || ''
    const nodeName = node.name || ''

    // Distinguish exec vs review nodes from the node name or id suffix.
    const rawName = realTool || nodeName || node.id || ''
    const rawType = node.type || ''
    const tool = readableToolName(rawName)
    const roleAgent = objectValue(input.roleAgent) || objectValue(output.roleAgent)
    const roleName = stringValue(roleAgent?.displayName) || stringValue(roleAgent?.name) || toolRoleFromName(rawName) || readableNodeType(rawType) || tool || '步骤'
    const stage = stringValue(input.stage) || stringValue(output.stage) || ''
    const outDuration = typeof output.durationMs === 'number' ? output.durationMs : undefined
    const nodeDuration = typeof node.durationMs === 'number' ? node.durationMs : undefined
    const duration = formatDurationMs(outDuration || nodeDuration, node.startedAt || node.createdAt, node.updatedAt || node.completedAt)
    const intent = node.intent || stringValue(input.intent) || ''

    // Extract error from stdout/result if present
    const errStr = stringValue(output.error) || node.error || node.errorMessage || stringValue(output.stderr) || ''
    // Parse stdout JSON for error field
    let stdoutErr = ''
    if (output.stdout) {
      const stdoutStr = stringValue(output.stdout)
      if (stdoutStr) {
        try {
          const parsed = JSON.parse(stdoutStr)
          if (parsed.error) stdoutErr = stringValue(parsed.error as unknown) || ''
        } catch { /* not JSON */ }
      }
    }
    const combinedError = errStr || stdoutErr || ''

    return {
      id: node.id || String(index + 1),
      role: roleName,
      stage,
      status: isReviewTraceNode(node) && normalizeDirectorStatus(node.status) === 'running' ? 'review' : normalizeDirectorStatus(node.status),
      tool,
      rawName: rawName || node.id || '',
      rawType,
      plane: executionPlaneForTool(rawName),
      input: summarizeValue(input.requiredInputs || input.input || intent || input),
      output: summarizeValue(output.artifactKind || output.artifacts || (stringValue(output.stdout) || '').slice(0, 100) || output),
      error: combinedError,
      duration,
      createdAt: node.createdAt || '',
      review: rawType === 'REVIEW_GATE' || Boolean(input.humanReview || output.humanReview),
    }
  })
}

export function nextStageIdAfterReview(stages: DirectorStage[], review: AgentReviewItem | undefined): string | undefined {
  if (!review) return undefined
  const index = stages.findIndex((stage) =>
    stage.reviewId === review.id ||
    stage.id === review.roleAgentId ||
    stage.stage === review.stage ||
    Boolean(review.tool && stage.allowedTools.includes(review.tool)),
  )
  if (index < 0) return undefined
  return stages[index + 1]?.id
}

export function applyOptimisticRunningStage(stages: DirectorStage[], stageId: string | undefined): DirectorStage[] {
  if (!stageId) return stages
  return stages.map((stage) => {
    if (stage.id !== stageId || stage.status !== 'pending') return stage
    return {
      ...stage,
      status: 'running',
      progress: Math.max(stage.progress, 18),
    }
  })
}

export function traceNodeHasError(node: DirectorTraceNode | undefined): boolean {
  return Boolean(node && (node.status === 'failed' || node.status === 'blocked' || node.error))
}

export function deriveNextAction(stages: DirectorStage[]): DirectorNextAction | undefined {
  const reviewStage = stages.find((stage) => stage.status === 'review')
  if (reviewStage) {
    return {
      stageId: reviewStage.id,
      kind: 'review',
      label: `审核${reviewStage.displayName}`,
      description: `${reviewStage.displayName}已产出内容，等待通过、驳回、编辑或重新生成。`,
    }
  }

  const runningStage = stages.find((stage) => stage.status === 'running' || stage.status === 'active')
  if (runningStage) {
    return {
      stageId: runningStage.id,
      kind: 'running',
      label: `${runningStage.displayName}执行中`,
      description: '当前阶段正在运行，完成后会生成新的审核或产物记录。',
    }
  }

  const blockedStage = stages.find((stage) => stage.status === 'blocked' || stage.status === 'failed')
  if (blockedStage) {
    return {
      stageId: blockedStage.id,
      kind: 'blocked',
      label: `处理${blockedStage.displayName}阻断`,
      description: '该阶段被驳回或执行失败，需要根据反馈重新生成或修正输入。',
    }
  }

  return {
    stageId: stages[0]?.id || 'start',
    kind: 'start',
    label: '启动新项目',
    description: '输入一句话需求后，系统会生成多角色计划并开始执行。',
  }
}

export interface DirectorErrorDetail {
  code: string
  nodeId?: string
  artifactKind?: string
  unitId?: string
  rawMessage?: string
}

export function normalizeDirectorErrorMessage(error: unknown): string {
  const raw = rawDirectorErrorMessage(error)

  if (raw.includes('CRITICAL_ARTIFACT_SYNC_FAILED')) {
    return '关键产物写入失败，最终视频无法进入项目产物库。请重新执行当前步骤。'
  }

  if (raw.includes('ARTIFACT_MANIFEST_INVALID')) {
    return '本地任务返回的产物信息不完整，无法写入项目产物库。请重新执行当前步骤。'
  }

  if (raw.includes('RENDER_DEPENDENCY_MISSING')) {
    return '当前项目尚不满足最终渲染条件，请确认预览、本地执行器和前置产物状态。'
  }

  if (raw.includes('PACKAGE_DEPENDENCY_MISSING')) {
    return '最终视频尚未通过质量检查，暂时不能打包。'
  }

  return raw || '操作失败，请稍后重试。'
}

export function formatDirectorErrorMessage(err: unknown, fallback: string) {
  if (err && typeof err === 'object' && 'message' in err && typeof err.message === 'string') {
    if (err.message.includes('CRITICAL_ARTIFACT_SYNC_FAILED')) {
      return '关键产物写入失败，最终视频无法进入项目产物库。\n请重新执行当前步骤。'
    }
    if (err.message.includes('ARTIFACT_MANIFEST_INVALID')) {
      return '本地任务返回的产物信息不完整，无法写入项目产物库。\n请重新执行当前步骤。'
    }
    return err.message
  }
  return fallback
}

export function extractDirectorErrorDetail(err: unknown): DirectorErrorDetail | undefined {
  const msg = rawDirectorErrorMessage(err)
  if (msg) {
    const codeMatch = msg.match(/^(CRITICAL_ARTIFACT_SYNC_FAILED|ARTIFACT_MANIFEST_INVALID|RENDER_DEPENDENCY_MISSING|PACKAGE_DEPENDENCY_MISSING)/)
    if (!codeMatch) return undefined
    const nodeIdMatch = msg.match(/nodeID=(\S+)/)
    const kindMatch = msg.match(/kind=(\S+)/)
    const unitIdMatch = msg.match(/unitID=(\S+)/)
    return {
      code: codeMatch[1],
      nodeId: nodeIdMatch?.[1],
      artifactKind: kindMatch?.[1],
      unitId: unitIdMatch?.[1],
      rawMessage: msg,
    }
  }
  return undefined
}

function rawDirectorErrorMessage(error: unknown): string {
  if (typeof error === 'string') return error
  if (error && typeof error === 'object') {
    const record = error as Record<string, unknown>
    const response = objectValue(record.response)
    const data = objectValue(response?.data)
    const detail = objectValue(data?.detail)
    const backendMessage = stringValue(detail?.rawMessage) || stringValue(data?.message)
    if (backendMessage) return backendMessage
    if (error instanceof Error) return error.message
    const directMessage = stringValue(record.message)
    if (directMessage) return directMessage
    return JSON.stringify(error)
  }
  return JSON.stringify(error ?? '')
}

function findReviewForRole(role: VideoRoleAgent, reviews: AgentReviewItem[]) {
  return reviews.find((review) => {
    if (review.roleAgentId === role.id) return true
    if (review.stage === role.stage) return true
    if (review.stepId?.includes(role.id) || review.stepId?.includes(role.stage)) return true
    if (review.tool && role.allowedTools?.includes(review.tool)) return true
    return false
  })
}

function findTraceNodeForRole(role: VideoRoleAgent, nodes: TraceNodeLike[]) {
  return [...nodes].reverse().find((node) => {
    const input = node.input || {}
    const output = node.output || {}
    const tool = stringValue(input.tool) || stringValue(input.capabilityTool) || stringValue(input.reviewTool) || node.name || node.type || ''
    if (stringValue(input.roleAgentId) === role.id || stringValue(output.roleAgentId) === role.id) return true
    if (stringValue(input.stage) === role.stage || stringValue(output.stage) === role.stage) return true
    if (tool && role.allowedTools?.includes(tool)) return true
    return false
  })
}

function stageStatusFor(
  role: VideoRoleAgent,
  review: AgentReviewItem | undefined,
  node: TraceNodeLike | undefined,
): DirectorStageStatus {
  // When a PENDING review exists, check whether content is still generating.
  // If the trace node is a tool execution (not a review gate) and is still
  // running, show "生成中" so the user knows the system is still working.
  // Once the exec completes and the review gate appears, show "待审核".
  if (review?.status === 'PENDING') {
    if (node?.status) {
      const normalizedStatus = normalizeDirectorStatus(node.status)
      if (!isReviewTraceNode(node) && (normalizedStatus === 'running' || normalizedStatus === 'active')) {
        return 'running'
      }
    }
    return 'review'
  }
  if (review?.status === 'REJECTED') return 'blocked'
  if (review?.status === 'APPROVED') return 'done'
  if (node?.status) {
    const status = normalizeDirectorStatus(node.status)
    if (isReviewTraceNode(node) && (status === 'running' || status === 'pending')) return 'review'
    return status
  }
  if (role.stage === 'proposal') return 'active'
  return 'pending'
}

function isReviewTraceNode(node: TraceNodeLike | undefined): boolean {
  if (!node) return false
  const input = node.input || {}
  const rawType = String(node.type || '').toUpperCase()
  if (rawType === 'REVIEW_GATE') return true
  if (stringValue(input.reviewPhase) || stringValue(input.reviewReason)) return true
  if (node.name?.startsWith('审核-')) return true
  return false
}

function artifactStatusFor(
  review: AgentReviewItem | undefined,
  node: TraceNodeLike | undefined,
): DirectorArtifactStatus {
  if (review?.status === 'PENDING') {
    if (node?.status) {
      const normalizedStatus = normalizeDirectorStatus(node.status)
      if (!isReviewTraceNode(node) && (normalizedStatus === 'running' || normalizedStatus === 'active')) {
        return 'running'
      }
    }
    return 'review'
  }
  if (review?.status === 'REJECTED') return 'blocked'
  if (review?.status === 'APPROVED') return 'valid'
  const status = normalizeDirectorStatus(node?.status)
  if (status === 'done') return 'valid'
  if (status === 'running' || status === 'active') return 'running'
  if (status === 'failed') return 'failed'
  return 'pending'
}

function normalizeDirectorStatus(status?: string): DirectorStageStatus {
  const normalized = (status || '').toUpperCase()
  if (['SUCCESS', 'SUCCEEDED', 'COMPLETED', 'APPROVED'].includes(normalized)) return 'done'
  if (['RUNNING', 'READY'].includes(normalized)) return 'running'
  if (['PENDING', 'CREATED'].includes(normalized)) return 'pending'
  if (['FAILED', 'ERROR', 'CANCELLED'].includes(normalized)) return 'failed'
  if (['REJECTED', 'BLOCKED'].includes(normalized)) return 'blocked'
  return 'pending'
}

function normalizeArtifactStatus(status?: string): DirectorArtifactStatus | undefined {
  const normalized = (status || '').toLowerCase()
  if (['valid', 'review', 'stale', 'pending', 'running', 'failed', 'blocked'].includes(normalized)) return normalized as DirectorArtifactStatus
  if (normalized === 'rejected') return 'blocked'
  return undefined
}

function progressForStatus(status: DirectorStageStatus) {
  const progress: Record<DirectorStageStatus, number> = {
    done: 100,
    active: 20,
    review: 100,
    blocked: 0,
    pending: 0,
    running: 68,
    failed: 0,
  }
  return progress[status]
}

function extractTraceNodes(trace: unknown): TraceNodeLike[] {
  if (!trace || typeof trace !== 'object') return []
  const root = trace as Record<string, unknown>
  const candidates = [
    root.nodes,
    objectValue(root.task)?.nodes,
    objectValue(root.data)?.nodes,
    objectValue(objectValue(root.data)?.task)?.nodes,
  ]
  const nodes = candidates.find(Array.isArray)
  return Array.isArray(nodes) ? nodes.filter(isTraceNodeLike) : []
}

function isTraceNodeLike(value: unknown): value is TraceNodeLike {
  return Boolean(value && typeof value === 'object')
}

function extractArtifacts(node: TraceNodeLike | undefined): Array<Record<string, unknown>> {
  const output = node?.output || {}
  const artifacts = output.artifacts
  if (Array.isArray(artifacts)) return artifacts.filter((item): item is Record<string, unknown> => Boolean(item && typeof item === 'object'))
  if (output.artifactKind) return [{ kind: output.artifactKind, name: output.artifactName, storageRef: output.storageRef }]
  return []
}

// readableToolName maps internal tool names / node types to human-readable Chinese labels.
function readableToolName(raw: string): string {
  const map: Record<string, string> = {
    proposal_generator: '生成创意方案',
    capability_preflight: '环境预检',
    pipeline_selector: '流水线选择',
    knowledge_researcher: '知识调研',
    fact_checker: '事实核查',
    video_script_generator: '生成口播脚本',
    shot_splitter: '拆分分镜',
    card_plan_generator: '生成卡片计划',
    caption_splitter: '拆分子幕',
    video_composition_builder: '构建视频结构',
    composition_quality_checker: '结构质检',
    reference_asset_planner: '素材策略规划',
    asset_policy_generator: '生成素材策略',
    continuity_checker: '一致性检查',
    style_profile_builder: '风格配置',
    stale_tracker: '过期追踪',
    hyperframes_project_generator: '生成 HyperFrames 项目',
    hyperframes_snapshot: '预览快照',
    preview_quality_checker: '预览质检',
    render_strategy_planner: '渲染策略规划',
    render_dependency_guard: '渲染依赖检查',
    hyperframes_renderer: '渲染视频',
    local_job_status_tracker: '本地任务追踪',
    video_prompt_generator: '生成视频提示词',
    keyframe_prompt_generator: '关键帧提示词',
    image_asset_generator: '生成图片素材',
    script_quality_checker: '脚本质检',
    shot_quality_checker: '分镜质检',
    video_prompt_quality_checker: '提示词质检',
    package_quality_checker: '打包质检',
    final_review_generator: '生成终审报告',
    ffmpeg_probe: '视频文件检测',
    artifact_packager: '打包产物',
    publish_copy_generator: '生成发布文案',
    video_package_exporter: '导出视频包',
    material_library_importer: '导入素材库',
    voice_post_process: '语音后处理',
    audio_artifact_packager: '音频打包',
    visual_feasibility_analyzer: '视觉可行性分析',
    REVIEW_GATE: '人工审核',
    CONTROL: '流程控制',
    TOOL: '工具执行',
  }
  if (map[raw]) return map[raw]
  // Handle review gate node names like "proposal_generator_review"
  if (raw.endsWith('_review')) {
    const base = raw.replace(/_review$/, '')
    if (map[base]) return `审核：${map[base]}`
  }
  // Handle exec node names like "proposal_generator_exec"
  if (raw.endsWith('_exec')) {
    const base = raw.replace(/_exec$/, '')
    if (map[base]) return map[base]
  }
  return raw
}

// toolRoleFromName derives a role label from the tool/node name.
function toolRoleFromName(raw: string): string {
  const map: Record<string, string> = {
    proposal_generator: '创意总监',
    capability_preflight: '能力检测',
    pipeline_selector: '流程调度',
    knowledge_researcher: '知识调研员',
    fact_checker: '事实核查员',
    video_script_generator: '脚本编剧',
    shot_splitter: '分镜导演',
    card_plan_generator: '卡片设计师',
    caption_splitter: '字幕编辑',
    video_composition_builder: '结构导演',
    composition_quality_checker: '结构质检员',
    reference_asset_planner: '参考选择',
    asset_policy_generator: '素材策略师',
    continuity_checker: '连续性检查',
    style_profile_builder: '风格配置师',
    stale_tracker: '过期追踪器',
    hyperframes_project_generator: '预览导演',
    hyperframes_snapshot: '预览快照',
    preview_quality_checker: '预览质检员',
    render_strategy_planner: '渲染策略师',
    render_dependency_guard: '渲染制片',
    hyperframes_renderer: '渲染制片',
    local_job_status_tracker: '任务追踪',
    video_prompt_generator: '提示词工程师',
    keyframe_prompt_generator: '关键帧设计师',
    image_asset_generator: '图片生成',
    script_quality_checker: '脚本质检员',
    shot_quality_checker: '分镜质检员',
    video_prompt_quality_checker: '提示词质检员',
    package_quality_checker: '打包质检员',
    final_review_generator: '质量审核',
    ffmpeg_probe: '视频检测',
    artifact_packager: '交付制片',
    publish_copy_generator: '文案生成',
    video_package_exporter: '交付制片',
    REVIEW_GATE: '人工审核',
    CONTROL: '流程控制',
  }
  // Handle _review and _exec suffixes
  const base = raw.replace(/_review$/, '').replace(/_exec$/, '')
  if (map[base]) return map[base]
  if (map[raw]) return map[raw]
  return ''
}

export function reviewDisplayTitle(review: AgentReviewItem | undefined): string {
  if (!review) return '暂无待审核'
  const title = stringValue(review.humanReview?.title)
  if (title) return title
  const labels: Record<string, string> = {
    proposal_generator: '审核创作方案',
    video_script_generator: '审核口播脚本',
    shot_splitter: '审核分镜计划',
    card_plan_generator: '审核卡片计划',
    render_strategy_planner: '审核渲染策略',
  }
  if (review.tool && labels[review.tool]) return labels[review.tool]
  if (review.reviewPhase === 'quality_gate') return '审核创作产物'
  return '审核阶段产物'
}

export function visibleReviewHistory(reviews: AgentReviewItem[] = []): AgentReviewItem[] {
  return reviews.filter((review) => ['PENDING', 'APPROVED', 'REJECTED'].includes(String(review.status)))
}

export function reviewStatusLabel(review: AgentReviewItem | undefined): string {
  switch (String(review?.status || '')) {
    case 'PENDING':
      return '待审核'
    case 'APPROVED':
      return '已通过'
    case 'REJECTED':
      return '已驳回'
    default:
      return '未触达'
  }
}

export function reviewQualityReportLines(review: AgentReviewItem | undefined): string[] {
  if (!review || review.reviewPhase !== 'quality_gate') return []
  const output = objectValue(review.reviewOutput)
  const report = output ? objectValue(output.qualityReport) : undefined
  if (!report) return []

  const lines: string[] = []
  const score = typeof report.score === 'number' ? report.score : Number(report.score)
  if (Number.isFinite(score)) {
    const threshold = review.reviewReason?.match(/(\d+)/)?.[1]
    lines.push(threshold ? `质量评分 ${score}/100，门禁阈值 ${threshold}` : `质量评分 ${score}/100`)
  }
  if (typeof report.passed === 'boolean') {
    lines.push(`门禁结果：${report.passed ? '已通过' : '未通过'}`)
  }

  const issues = Array.isArray(report.issues) ? report.issues : []
  for (const issue of issues) {
    if (typeof issue === 'string') {
      lines.push(issue)
      continue
    }
    const item = objectValue(issue)
    if (!item) continue
    const level = stringValue(item.level)
    const field = stringValue(item.field)
    const message = stringValue(item.message) || JSON.stringify(item)
    lines.push([level && `[${level}]`, field, message].filter(Boolean).join(' '))
  }

  const suggestions = Array.isArray(report.repairSuggestions) ? report.repairSuggestions : []
  for (const suggestion of suggestions) {
    if (typeof suggestion === 'string') {
      lines.push(`建议：${suggestion}`)
    }
  }

  return lines
}

export function reviewOutputText(review: AgentReviewItem | undefined): string {
  if (!review) return ''
  const content = stringValue(review.reviewContent)
  if (content) return normalizeReviewContentText(content)
  const output = objectValue(review.reviewOutput)
  if (!output) return ''
  const summary = stringValue(output.summary)
  if (summary) return summary
  const packageValue = objectValue(output.package)
  if (packageValue) return JSON.stringify(packageValue, null, 2)
  return JSON.stringify(output, null, 2)
}

function normalizeReviewContentText(content: string): string {
  const trimmed = content.trim()
  if (!trimmed) return ''
  const parsed = parseEmbeddedJSON(trimmed)
  if (parsed !== undefined) {
    return JSON.stringify(parsed, null, 2)
  }
  return content
}

function parseEmbeddedJSON(text: string): unknown | undefined {
  const direct = tryParseJSON(text)
  if (direct !== undefined) return direct

  for (let i = 0; i < text.length; i += 1) {
    const open = text[i]
    if (open !== '{' && open !== '[') continue
    const close = open === '{' ? '}' : ']'
    const candidate = extractBalancedJSON(text.slice(i), open, close)
    if (!candidate) continue
    const parsed = tryParseJSON(candidate)
    if (parsed !== undefined) return parsed
  }
  return undefined
}

function tryParseJSON(text: string): unknown | undefined {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

function extractBalancedJSON(text: string, open: string, close: string): string {
  let depth = 0
  let inString = false
  let escaped = false
  for (let i = 0; i < text.length; i += 1) {
    const char = text[i]
    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\' && inString) {
      escaped = true
      continue
    }
    if (char === '"') {
      inString = !inString
      continue
    }
    if (inString) continue
    if (char === open) depth += 1
    if (char === close) {
      depth -= 1
      if (depth === 0) return text.slice(0, i + 1)
    }
  }
  return ''
}

export function displayNameForArtifact(kind: string) {
  const labels: Record<string, string> = {
    VIDEO_PROPOSAL: '创意方案',
    PROJECT_BRIEF: '项目简报',
    STYLE_PROFILE: '风格配置',
    VIDEO_SCRIPT: '视频脚本',
    SCRIPT_SECTIONS: '脚本分段',
    CARD_PLAN: '卡片分镜',
    CAPTION_SEGMENTS: '字幕分段',
    VIDEO_COMPOSITION_SPEC: '视频结构',
    REFERENCE_ASSET_PLAN: '素材策略',
    CONTINUITY_REPORT: '一致性报告',
    HYPERFRAMES_PROJECT: '视频结构',
    PREVIEW_SNAPSHOTS: '预览快照',
    PREVIEW_REPORT: '预览报告',
    VIDEO: '最终视频',
    RENDER_REPORT: '渲染报告',
    FFMPEG_PROBE_REPORT: '视频检测报告',
    FINAL_REVIEW: '最终审核报告',
    PROJECT_PACKAGE: '交付包',
  }
  return labels[kind] || kind
}

function downstreamKindsFor(changedKind: string): string[] {
  const order = [
    'VIDEO_PROPOSAL',
    'VIDEO_SCRIPT',
    'CARD_PLAN',
    'VIDEO_COMPOSITION_SPEC',
    'REFERENCE_ASSET_PLAN',
    'CONTINUITY_REPORT',
    'HYPERFRAMES_PROJECT',
    'PREVIEW_SNAPSHOTS',
    'VIDEO',
    'FINAL_REVIEW',
    'PROJECT_PACKAGE',
  ]
  const explicit: Record<string, string[]> = {
    VIDEO_PROPOSAL: order.slice(1),
    VIDEO_SCRIPT: order.slice(2),
    CARD_PLAN: order.slice(3),
    VIDEO_COMPOSITION_SPEC: ['HYPERFRAMES_PROJECT', 'PREVIEW_SNAPSHOTS', 'VIDEO', 'FINAL_REVIEW', 'PROJECT_PACKAGE'],
    PREVIEW_SNAPSHOTS: ['VIDEO', 'FINAL_REVIEW', 'PROJECT_PACKAGE'],
  }
  if (explicit[changedKind]) return explicit[changedKind]
  const index = order.indexOf(changedKind)
  return index >= 0 ? order.slice(index + 1) : []
}

function storageHintForKind(kind: string) {
  if (kind === 'VIDEO' || kind === 'PROJECT_PACKAGE' || kind.includes('PREVIEW')) return '本地项目目录'
  return '本地项目目录（仅同步索引）'
}

function displayStorageRef(value: unknown): string {
  const ref = stringValue(value)
  if (!ref) return ''
  if (ref.startsWith('local://')) return ref
  if (ref.startsWith('cloud://')) return '本地项目目录（仅同步索引）'
  return ref
}

function requiresMaterializedArtifact(kind: string) {
  return kind === 'VIDEO' || kind === 'PROJECT_PACKAGE'
}

function normalizePublishTopic(topic: string) {
  const cleaned = topic
    .replace(/^请帮我(做|创作|生成)?一个?\d*秒?(图文|动画|口播)?视频[，,:：]*/u, '')
    .replace(/^讲/u, '')
    .trim()
  return cleaned || '视频主题'
}

function publishTagsFor(topic: string, hasPrompt: boolean) {
  const tags = ['视频创作', 'AI', '工作流']
  if (topic.includes('智能体') || topic.toLowerCase().includes('agent')) tags.push('智能体')
  if (topic.includes('故事') || topic.includes('动画')) tags.push('动画短片')
  if (hasPrompt) tags.push('分镜脚本')
  return uniqueStrings(tags)
}

function uniqueStrings(values: string[]) {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))]
}

function titleWithin(value: string, maxLength: number) {
  const cleaned = value.trim()
  if (cleaned.length <= maxLength) return cleaned
  return cleaned.slice(0, Math.max(1, maxLength - 1)) + '…'
}

// readableNodeType maps backend node types to Chinese labels.
function readableNodeType(nodeType: string): string {
  const map: Record<string, string> = {
    TOOL: '工具执行',
    REVIEW_GATE: '人工审核',
    CONTROL: '流程控制',
    GATE: '门禁',
    SYSTEM: '系统',
  }
  return map[nodeType] || nodeType
}

// formatDurationMs formats a duration in ms or computes it from timestamps.
function formatDurationMs(durationMs: number | undefined, createdAt: string | undefined, updatedAt: string | undefined): string {
  if (durationMs && durationMs > 0) {
    if (durationMs < 1000) return `${durationMs}ms`
    return `${(durationMs / 1000).toFixed(1)}s`
  }
  if (createdAt && updatedAt) {
    const start = new Date(createdAt).getTime()
    const end = new Date(updatedAt).getTime()
    const ms = end - start
    if (ms > 0) {
      if (ms < 1000) return `${ms}ms`
      return `${(ms / 1000).toFixed(1)}s`
    }
  }
  return '-'
}

function executionPlaneForTool(tool: string): 'cloud' | 'local' {
  if (tool.includes('hyperframes') || tool.includes('renderer') || tool.includes('local') || tool.includes('ffmpeg')) return 'local'
  return 'cloud'
}

function summarizeValue(value: unknown): string {
  if (value === undefined || value === null) return '-'
  if (typeof value === 'string') return value
  if (Array.isArray(value)) return value.map(summarizeValue).join(' / ') || '-'
  if (typeof value === 'object') {
    const record = value as Record<string, unknown>
    const preferred = record.name || record.kind || record.artifactKind || record.stage || record.roleAgentId
    if (preferred) return summarizeValue(preferred)
    return Object.keys(record).slice(0, 3).join(' / ') || '-'
  }
  return String(value)
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

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}

function booleanValue(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
}

function stringArrayValue(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  const values = value.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
  return values.length ? values : undefined
}

export interface StageStateDisplay {
  icon: 'check' | 'shield' | 'refresh' | 'x' | 'cpu'
  label: string
  colorClass: string
  animate: boolean
  active: boolean
}

export function getStageStateDisplay(status: DirectorStageStatus): StageStateDisplay {
  switch (status) {
    case 'done':
      return { icon: 'check', label: '已通过', colorClass: 'border-green-300 bg-green-50 text-green-700', animate: false, active: false }
    case 'review':
      return { icon: 'shield', label: '待审核', colorClass: 'border-amber-300 bg-amber-50 text-amber-700', animate: false, active: true }
    case 'running':
    case 'active':
      return { icon: 'refresh', label: '生成中', colorClass: 'border-blue-300 bg-blue-50 text-blue-700', animate: true, active: true }
    case 'blocked':
    case 'failed':
      return { icon: 'x', label: '已阻断', colorClass: 'border-red-300 bg-red-50 text-red-700', animate: false, active: false }
    case 'pending':
    default:
      return { icon: 'cpu', label: '等待中', colorClass: 'border-stone-200 bg-stone-50/60 text-stone-400', animate: false, active: false }
  }
}
