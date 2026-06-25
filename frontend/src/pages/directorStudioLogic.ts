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
      const status = artifactStatusFor(review, node)
      const index = roleIndex + 1

      return {
        id: `A${String(index).padStart(2, '0')}${outputIndex ? `-${outputIndex + 1}` : ''}`,
        name: String(artifact?.name || displayNameForArtifact(output)),
        kind: output,
        version: artifact ? '第1版' : '-',
        status,
        owner: role.displayName || role.name,
        updatedAt: formatTime(node?.createdAt),
        humanApproved: status === 'valid',
        storageRef: String(artifact?.storageRef || artifact?.url || storageHintForKind(output)),
        dependsOn: role.requiredInputs,
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

function displayNameForArtifact(kind: string) {
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
