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
  tool: string
  plane: 'cloud' | 'local'
  input: string
  output: string
  duration: string
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
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  createdAt?: string
  updatedAt?: string
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
        storageRef: String(artifact?.storageRef || artifact?.url || (requiresManifest ? '' : storageHintForKind(output))),
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
    const roleAgent = objectValue(input.roleAgent) || objectValue(output.roleAgent)
    const roleName = stringValue(roleAgent?.displayName) || stringValue(roleAgent?.name) || stringValue(input.roleAgentId) || '系统'
    const tool = node.name || node.type || 'unknown_tool'
    const stage = stringValue(input.stage) || stringValue(output.stage) || ''

    return {
      id: node.id || String(index + 1),
      role: roleName,
      stage,
      status: normalizeDirectorStatus(node.status),
      tool,
      plane: executionPlaneForTool(tool),
      input: summarizeValue(input.requiredInputs || input.input || input),
      output: summarizeValue(output.artifactKind || output.artifacts || output),
      duration: '-',
      review: Boolean(input.humanReview || output.humanReview),
    }
  })
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
  const raw =
    typeof error === 'string'
      ? error
      : error instanceof Error
        ? error.message
        : JSON.stringify(error ?? '')

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
  if (err && typeof err === 'object' && 'message' in err && typeof err.message === 'string') {
    const msg: string = err.message
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
    const tool = node.name || node.type || ''
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
  if (review?.status === 'PENDING') return 'review'
  if (review?.status === 'REJECTED') return 'blocked'
  if (review?.status === 'APPROVED') return 'done'
  if (node?.status) return normalizeDirectorStatus(node.status)
  if (role.stage === 'proposal') return 'active'
  return 'pending'
}

function artifactStatusFor(
  review: AgentReviewItem | undefined,
  node: TraceNodeLike | undefined,
): DirectorArtifactStatus {
  if (review?.status === 'PENDING') return 'review'
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
  return '云端项目库'
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
