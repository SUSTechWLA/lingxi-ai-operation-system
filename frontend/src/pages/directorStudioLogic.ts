import type { AgentReviewItem, VideoRoleAgent } from '../utils/types'

export type DirectorNavKey = 'overview' | 'review' | 'trace' | 'assets' | 'roles' | 'export' | 'system'
export type DirectorStageStatus = 'done' | 'active' | 'review' | 'blocked' | 'pending' | 'running' | 'failed'
export type DirectorArtifactStatus = 'valid' | 'review' | 'stale' | 'pending' | 'running' | 'failed' | 'blocked' | 'missing'

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

export interface DirectorShotReviewGroup {
  shotId: string
  status: DirectorArtifactStatus
  title: string
  narrationText: string
  referenceRoles: string[]
  artifactCounts: {
    total: number
    references: number
    media: number
    reviewPackets: number
  }
  artifacts: DirectorArtifactRecord[]
}

export interface ExternalGenerationGuideRequest {
  kind: 'image' | 'video'
  references?: unknown[]
  target?: {
    aspectRatio?: string
    durationSec?: number
    resolution?: string
  }
  promptCharLimit?: number
  referenceImageLimit?: number
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

export function externalGenerationGuideSteps(request: ExternalGenerationGuideRequest): string[] {
  const kindLabel = request.kind === 'image' ? '参考图或关键帧' : 'AIGC 视频'
  const platformLabel = request.kind === 'image' ? '图片生成平台' : '视频生成平台'
  const referenceCount = Array.isArray(request.references) ? request.references.length : 0
  const referenceLimit = request.referenceImageLimit || 6
  const targetParts = [
    request.target?.aspectRatio ? `画幅 ${request.target.aspectRatio}` : '',
    request.target?.resolution ? `分辨率 ${request.target.resolution}` : '',
    request.target?.durationSec ? `时长 ${request.target.durationSec}s` : '',
  ].filter(Boolean)
  const targetText = targetParts.length ? `，参数按 ${targetParts.join('、')} 设置` : ''

  return [
    '当前没有可用的图片或视频 API 配置，系统不会自动生成素材；请用下面的 Prompt 在浏览器中的外部生成平台完成。',
    `点击“复制 Prompt”，在浏览器打开你常用的${platformLabel}，把 Prompt 粘贴进去${targetText}。`,
    referenceCount > 0
      ? `如平台支持参考图，按顺序添加本卡片列出的参考图，最多使用 ${referenceLimit} 张。`
      : '如果没有参考图，直接使用 Prompt 生成，不需要等待系统补图。',
    `每个 shot 单独生成，尽量不要引用其他 shot 的未确认画面；如果需要转场，把转场放在本 shot 结尾。`,
    `生成完成后导出${kindLabel}文件，回到本页点击“上传结果”，系统会登记到素材库并关联当前 shot。`,
  ]
}

interface RoleTraceMatch {
  execNode?: TraceNodeLike
  reviewNode?: TraceNodeLike
  currentNode?: TraceNodeLike
}

export function buildDirectorStages(
  roleAgents: VideoRoleAgent[],
  reviews: AgentReviewItem[] = [],
  trace: unknown = undefined,
  projectStarted = false,
): DirectorStage[] {
  const traceNodes = extractTraceNodes(trace)

  const stages = roleAgents.map((role) => {
    const review = findReviewForRole(role, reviews)
    const match = findTraceNodesForRole(role, traceNodes)
    const status = stageStatusFor(role, review, match, projectStarted)

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

  if (stages.some((stage) => !['pending', 'active'].includes(stage.status))) {
    return stages.map((stage) => stage.status === 'active' ? { ...stage, status: 'pending', progress: progressForStatus('pending') } : stage)
  }

  if (
    projectStarted &&
    stages.length > 0 &&
    stages.every((stage) => stage.status === 'pending')
  ) {
    return stages.map((stage, index) => index === 0 ? { ...stage, status: 'active', progress: progressForStatus('active') } : stage)
  }

  return stages
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

export function buildPublishCopies(publishCopyContent: unknown): PublishCopy[] {
  const parsed = parsePublishCopyPayload(publishCopyContent)
  if (!parsed) return []

  const records = publishCopyRecords(parsed)
  return records
    .map((record, index) => normalizePublishCopy(record, index))
    .filter((copy): copy is PublishCopy => Boolean(copy))
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

function parsePublishCopyPayload(content: unknown): unknown {
  if (!content) return undefined
  if (typeof content === 'string') {
    const trimmed = content.trim()
    if (!trimmed) return undefined
    const parsed = tryParseJSON(trimmed)
    if (parsed) return parsed
    return parsePublishCopyMarkdown(trimmed)
  }
  return content
}

function publishCopyRecords(payload: unknown): Record<string, unknown>[] {
  if (Array.isArray(payload)) return payload.map(objectValue).filter((item): item is Record<string, unknown> => Boolean(item))
  const root = unwrapPublishCopyPayload(payload)
  if (!root) return []

  for (const key of ['publishCopies', 'platformCopies', 'copies']) {
    const value = root[key]
    if (Array.isArray(value)) return value.map(objectValue).filter((item): item is Record<string, unknown> => Boolean(item))
  }

  const platforms = objectValue(root.platforms)
  if (platforms) {
    return Object.entries(platforms)
      .flatMap(([platform, value]) => {
        const record = objectValue(value)
        return record ? [{ platform, ...record }] : []
      })
  }

  if (hasPublishCopyFields(root)) {
    return [
      { platform: 'xiaohongshu', ...root },
      { platform: 'bilibili', ...root },
    ]
  }

  return []
}

function unwrapPublishCopyPayload(payload: unknown): Record<string, unknown> | undefined {
  const root = objectValue(payload)
  if (!root) return undefined
  for (const key of ['publishCopy', 'publish_copy', 'data', 'package']) {
    const nested = objectValue(root[key])
    if (nested && (hasPublishCopyFields(nested) || nested.publishCopies || nested.platformCopies || nested.copies || nested.platforms)) {
      return nested
    }
  }
  return root
}

function normalizePublishCopy(record: Record<string, unknown>, index: number): PublishCopy | undefined {
  const title = firstString(record, ['title', 'headline', 'videoTitle'])
  const description = firstString(record, ['description', 'intro', 'summary', 'body', 'copy'])
  if (!title || !description) return undefined

  const platform = normalizePublishPlatform(firstString(record, ['platform', 'platformId']) || (index === 1 ? 'bilibili' : 'xiaohongshu'))
  const tags = normalizePublishTags(record.keywords || record.tags || record.hashtags)
  const coverText = firstString(record, ['coverText', 'cover_text', 'cover', 'coverTitle']) || title
  const publishTips = normalizeStringList(record.publishTips || record.tips || record.suggestions)

  return {
    platform,
    platformName: platform === 'bilibili' ? 'B站' : '小红书',
    title,
    description,
    tags,
    coverText,
    publishTips,
  }
}

function normalizePublishPlatform(value: string): PublishPlatform {
  const normalized = value.trim().toLowerCase()
  if (normalized.includes('bilibili') || normalized.includes('b站') || normalized === 'bili') return 'bilibili'
  return 'xiaohongshu'
}

function normalizePublishTags(value: unknown): string[] {
  return uniqueStrings(normalizeStringList(value).map((tag) => tag.replace(/^#+/, ''))).slice(0, 10)
}

function normalizeStringList(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item || '').trim()).filter(Boolean)
  if (typeof value === 'string') {
    return value
      .split(/[,\n，、\s]+/u)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return []
}

function firstString(record: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function hasPublishCopyFields(record: Record<string, unknown>): boolean {
  return Boolean(firstString(record, ['title', 'headline', 'videoTitle']) && firstString(record, ['description', 'intro', 'summary', 'body', 'copy']))
}

function parsePublishCopyMarkdown(markdown: string): Record<string, unknown>[] {
  const sections = markdown.split(/^##\s+/mu).map((section) => section.trim()).filter(Boolean)
  return sections
    .map((section) => {
      const lines = section.split('\n')
      const heading = lines.shift() || ''
      const body = lines.join('\n')
      const record: Record<string, unknown> = {
        platform: heading,
        title: markdownField(body, ['标题']),
        description: markdownField(body, ['正文', '简介', '描述']),
        tags: markdownField(body, ['标签', '关键词']),
        coverText: markdownField(body, ['封面文案', '封面']),
      }
      const tips = markdownListAfterHeading(body, ['发布建议', '平台适配建议'])
      if (tips.length) record.publishTips = tips
      return record
    })
    .filter(hasPublishCopyFields)
}

function markdownField(markdown: string, labels: string[]): string {
  for (const label of labels) {
    const inline = new RegExp(`\\*\\*${label}\\*\\*\\s*[：:]\\s*([^\\n]+)`, 'u').exec(markdown)
    if (inline?.[1]) return inline[1].trim()
    const plain = new RegExp(`^${label}\\s*[：:]\\s*([^\\n]+)`, 'mu').exec(markdown)
    if (plain?.[1]) return plain[1].trim()
  }
  return ''
}

function markdownListAfterHeading(markdown: string, labels: string[]): string[] {
  for (const label of labels) {
    const pattern = new RegExp(`\\*\\*${label}\\*\\*\\s*[：:]?\\s*\\n([\\s\\S]*?)(?:\\n\\*\\*|\\n##|$)`, 'u')
    const match = pattern.exec(markdown)
    if (!match?.[1]) continue
    return match[1]
      .split('\n')
      .map((line) => line.replace(/^\s*[-*]\s*/, '').trim())
      .filter(Boolean)
  }
  return []
}

export function buildDirectorArtifacts(
  roleAgents: VideoRoleAgent[],
  reviews: AgentReviewItem[] = [],
  trace: unknown = undefined,
  projectArtifacts: Array<Record<string, unknown>> = [],
): DirectorArtifactRecord[] {
  const traceNodes = extractTraceNodes(trace)

  const projected = roleAgents.flatMap((role, roleIndex) => {
    const outputs = role.requiredOutputs?.length ? role.requiredOutputs : [`${role.stage}_OUTPUT`]
    const review = findReviewForRole(role, reviews)
    const match = findTraceNodesForRole(role, traceNodes)
    const node = match.execNode || match.currentNode
    const artifactOutputs = extractArtifacts(node)

    return outputs.map((output, outputIndex) => {
      const projectArtifact = findProjectArtifactForOutput(projectArtifacts, output, role)
      const artifact = projectArtifact || artifactOutputs.find((item) => item.kind === output) || artifactOutputs[outputIndex]
      const manifestStatus = normalizeArtifactStatus(stringValue(artifact?.status))
      const requiresManifest = requiresMaterializedArtifact(output)
      const fallbackStatus = artifactStatusFor(review, node)
      const missingManifest = !artifact && requiresManifest && execSucceeded(node)
      const status = artifact
        ? manifestStatus || fallbackStatus
        : missingManifest ? 'missing' : requiresManifest ? 'pending' : fallbackStatus
      const manifestHumanApproved = booleanValue(artifact?.humanApproved)
      const index = roleIndex + 1

      return {
        id: String(artifact?.id || artifact?.artifactId || `A${String(index).padStart(2, '0')}${outputIndex ? `-${outputIndex + 1}` : ''}`),
        name: String(artifact?.name || displayNameForArtifact(output)),
        kind: output,
        version: artifact ? artifactVersionLabel(artifact) : '-',
        status,
        owner: role.displayName || role.name,
        updatedAt: formatTime(stringValue(artifact?.updatedAt) || stringValue(artifact?.createdAt) || node?.createdAt),
        humanApproved: artifact ? manifestHumanApproved ?? status === 'valid' : false,
        storageRef: displayStorageRef(artifact?.storageRef || artifact?.url || (requiresManifest ? '' : storageHintForKind(output))),
        dependsOn: stringArrayValue(artifact?.dependsOn) || stringArrayValue(objectValue(artifact?.metadata)?.dependsOn) || role.requiredInputs,
        metadata: objectValue(artifact?.metadata),
      }
    })
  })

  const projectedIds = new Set(projected.map((artifact) => artifact.id))
  const extraProjectArtifacts = projectArtifacts
    .filter((artifact) => shouldExposeUnprojectedArtifact(artifact, projectedIds))
    .map(projectArtifactRecord)

  return [...projected, ...extraProjectArtifacts]
}

export function findPublishCopyArtifact(artifacts: DirectorArtifactRecord[]): DirectorArtifactRecord | undefined {
  return artifacts.find((artifact) => {
    if (artifact.kind === 'PUBLISH_COPY') return true
    const artifactType = stringValue(artifact.metadata?.artifactType) || stringValue(artifact.metadata?.artifact_kind)
    return artifactType === 'publish_copy'
  })
}

export interface ArtifactViewerSelection {
  selectedId?: string
  shouldLoad: boolean
  placeholder?: string
}

export function getArtifactViewerSelection(
  currentSelectedId: string | undefined,
  artifact: DirectorArtifactRecord | undefined,
): ArtifactViewerSelection {
  if (!artifact) return { selectedId: undefined, shouldLoad: false, placeholder: undefined }
  if (currentSelectedId === artifact.id) return { selectedId: undefined, shouldLoad: false, placeholder: undefined }
  if (!isInspectableDirectorArtifact(artifact)) {
    return {
      selectedId: artifact.id,
      shouldLoad: false,
      placeholder: '该产物还没有 materialized artifact ID，等待对应阶段生成完成后可查看正文。',
    }
  }
  return { selectedId: artifact.id, shouldLoad: true, placeholder: undefined }
}

export function buildShotReviewGroups(artifacts: DirectorArtifactRecord[]): DirectorShotReviewGroup[] {
  const groups = new Map<string, DirectorArtifactRecord[]>()
  for (const artifact of artifacts) {
    const shotId = shotIdForArtifact(artifact)
    if (!shotId) continue
    const list = groups.get(shotId) || []
    list.push(artifact)
    groups.set(shotId, list)
  }

  return Array.from(groups.entries())
    .sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' }))
    .map(([shotId, shotArtifacts]) => {
      const reviewPacket = shotArtifacts.find(isShotReviewPacket)
      const references = shotArtifacts.filter(isShotReferenceArtifact)
      const media = shotArtifacts.filter(isShotMediaArtifact)
      return {
        shotId,
        status: aggregateShotStatus(shotArtifacts),
        title: shotTitle(shotId, reviewPacket || shotArtifacts[0]),
        narrationText: shotNarrationText(reviewPacket || shotArtifacts[0]),
        referenceRoles: uniqueStrings(references.map((artifact) => stringValue(artifact.metadata?.referenceRole) || stringValue(artifact.metadata?.role) || displayNameForArtifact(artifact.kind))),
        artifactCounts: {
          total: shotArtifacts.length,
          references: references.length,
          media: media.length,
          reviewPackets: shotArtifacts.filter(isShotReviewPacket).length,
        },
        artifacts: shotArtifacts,
      }
    })
}

function findProjectArtifactForOutput(
  projectArtifacts: Array<Record<string, unknown>>,
  output: string,
  role: VideoRoleAgent,
): Record<string, unknown> | undefined {
  return projectArtifacts.find((artifact) => {
    const metadata = objectValue(artifact.metadata)
    const kind = stringValue(artifact.kind)
    const metadataKind = stringValue(metadata?.artifactKind) || stringValue(metadata?.kind)
    const stageName = stringValue(artifact.stageName)
    if (kind === output || metadataKind === output) return true
    return stageName === role.stage && semanticKindForStage(stageName) === output
  })
}

function shouldExposeUnprojectedArtifact(artifact: Record<string, unknown>, projectedIds: Set<string>): boolean {
  const id = String(artifact.id || artifact.artifactId || '')
  if (!id || projectedIds.has(id)) return false
  const metadata = objectValue(artifact.metadata)
  const kind = stringValue(artifact.kind) || stringValue(metadata?.artifactKind) || stringValue(metadata?.kind)
  const artifactType = stringValue(metadata?.artifactType) || stringValue(metadata?.artifact_kind)
  const shotId = firstString(metadata || {}, ['relatedShotId', 'shotId', 'shotID', 'related_shot_id'])
  return kind === 'PUBLISH_COPY' ||
    artifactType === 'publish_copy' ||
    artifactType === 'external_generation_request' ||
    artifactType === 'external_generation_result' ||
    Boolean(shotId) ||
    Boolean(kind && (kind.startsWith('SHOT_') || kind === 'HYPERFRAMES_SHOT'))
}

function projectArtifactRecord(artifact: Record<string, unknown>): DirectorArtifactRecord {
  const metadata = objectValue(artifact.metadata)
  const artifactType = stringValue(metadata?.artifactType) || stringValue(metadata?.artifact_kind)
  const rawKind = stringValue(artifact.kind) || stringValue(metadata?.artifactKind) || stringValue(metadata?.kind) || 'JSON'
  const kind = artifactType === 'external_generation_request' ? 'EXTERNAL_GENERATION_REQUEST' : rawKind
  const requestStatus = stringValue(metadata?.status)
  const status = artifactType === 'external_generation_request' && requestStatus === 'pending_upload'
    ? 'review'
    : normalizeArtifactStatus(stringValue(artifact.status)) || 'valid'
  return {
    id: String(artifact.id || artifact.artifactId || ''),
    name: String(artifact.name || displayNameForArtifact(kind)),
    kind,
    version: artifactVersionLabel(artifact),
    status,
    owner: artifactType === 'external_generation_request'
      ? '素材依赖点'
      : stringValue(metadata?.producedByRole) || stringValue(metadata?.owner) || '项目产物',
    updatedAt: formatTime(stringValue(artifact.updatedAt) || stringValue(artifact.createdAt)),
    humanApproved: booleanValue(artifact.humanApproved) ?? status === 'valid',
    storageRef: displayStorageRef(artifact.storageRef || artifact.url || ''),
    dependsOn: stringArrayValue(artifact.dependsOn) || stringArrayValue(metadata?.dependsOn),
    metadata,
  }
}

function shotIdForArtifact(artifact: DirectorArtifactRecord): string {
  const metadata = artifact.metadata || {}
  const direct = firstString(metadata, ['relatedShotId', 'shotId', 'shotID', 'related_shot_id'])
  if (direct) return direct
  const unitMatch = [artifact.id, artifact.name, artifact.storageRef]
    .map((value) => /SHOT[_-]?\d+/i.exec(value || '')?.[0])
    .find(Boolean)
  return unitMatch ? unitMatch.replace(/shot/i, 'SHOT').replace(/SHOT-/, 'SHOT_') : ''
}

function isShotReviewPacket(artifact: DirectorArtifactRecord): boolean {
  const artifactType = stringValue(artifact.metadata?.artifactType) || stringValue(artifact.metadata?.artifact_kind)
  return artifact.kind === 'SHOT_REVIEW_PACKET' || artifactType === 'shot_review_packet'
}

function isShotReferenceArtifact(artifact: DirectorArtifactRecord): boolean {
  const metadata = artifact.metadata || {}
  if (stringValue(metadata.referenceRole) || stringValue(metadata.role)) return true
  const artifactType = stringValue(metadata.artifactType) || stringValue(metadata.artifact_kind)
  return artifactType === 'shot_reference' || artifact.kind === 'REFERENCE_ASSET_PLAN'
}

function isShotMediaArtifact(artifact: DirectorArtifactRecord): boolean {
  const mediaKinds = ['SHOT_ASSET_PACKAGE', 'SHOT_AUDIO', 'SHOT_KEYFRAME', 'SHOT_VIDEO_CLIP', 'SHOT_SUBTITLE', 'HYPERFRAMES_SHOT']
  if (mediaKinds.includes(artifact.kind)) return true
  const artifactType = stringValue(artifact.metadata?.artifactType) || stringValue(artifact.metadata?.artifact_kind)
  return ['shot_asset_package', 'shot_audio', 'shot_keyframe', 'shot_video_clip', 'shot_subtitle', 'hyperframes_shot'].includes(artifactType || '')
}

function aggregateShotStatus(artifacts: DirectorArtifactRecord[]): DirectorArtifactStatus {
  const priority: DirectorArtifactStatus[] = ['failed', 'blocked', 'review', 'stale', 'running', 'pending', 'valid', 'missing']
  for (const status of priority) {
    if (artifacts.some((artifact) => artifact.status === status)) return status
  }
  return 'pending'
}

function shotTitle(shotId: string, artifact: DirectorArtifactRecord | undefined): string {
  if (!artifact) return shotId
  return firstString(artifact.metadata || {}, ['title', 'visual', 'name']) || artifact.name || shotId
}

function shotNarrationText(artifact: DirectorArtifactRecord | undefined): string {
  if (!artifact) return ''
  return firstString(artifact.metadata || {}, ['narrationText', 'scriptText', 'voiceoverText', 'subtitleText'])
}

function semanticKindForStage(stageName: string | undefined): string | undefined {
  const map: Record<string, string> = {
    proposal: 'VIDEO_PROPOSAL',
    script: 'VIDEO_SCRIPT',
    storyboard: 'CARD_PLAN',
    composition: 'VIDEO_COMPOSITION_SPEC',
    reference: 'REFERENCE_ASSET_PLAN',
    continuity: 'CONTINUITY_REPORT',
    preview: 'PREVIEW_SNAPSHOTS',
    render: 'VIDEO',
    quality: 'FINAL_REVIEW',
    package: 'PROJECT_PACKAGE',
    publish: 'PUBLISH_COPY',
    publish_copy: 'PUBLISH_COPY',
    publish_package: 'PUBLISH_COPY',
  }
  return stageName ? map[stageName] : undefined
}

function artifactVersionLabel(artifact: Record<string, unknown>): string {
  const version = artifact.version
  if (typeof version === 'number' && Number.isFinite(version)) return `第${version}版`
  if (typeof version === 'string' && version.trim()) {
    return version.startsWith('第') ? version : `第${version}版`
  }
  return '第1版'
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
    const parameters = objectValue(input.parameters)
    const roleAgent = objectValue(input.roleAgent) || objectValue(parameters?.roleAgent) || objectValue(output.roleAgent)
    const roleName = stringValue(roleAgent?.displayName) || stringValue(roleAgent?.name) || toolRoleFromName(rawName) || readableNodeType(rawType) || tool || '步骤'
    const stage = nodeFieldString(node, 'stage') || stringValue(output.stage) || ''
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

function findTraceNodesForRole(role: VideoRoleAgent, nodes: TraceNodeLike[]): RoleTraceMatch {
  const reversed = [...nodes].reverse()

  const matchNode = (node: TraceNodeLike): boolean => {
    const output = node.output || {}
    const tool = nodeToolName(node)
    if (nodeFieldString(node, 'roleAgentId') === role.id || stringValue(output.roleAgentId) === role.id) return true
    if (nodeFieldString(node, 'stage') === role.stage || stringValue(output.stage) === role.stage) return true
    if (tool && role.allowedTools?.includes(tool)) return true
    return false
  }

  // Prefer the latest exec / tool node over review gate nodes.
  // When both an exec node and its review gate exist in the trace,
  // the exec node's RUNNING/SUCCESS status tells us whether content
  // is still generating — whereas the review gate only tells us a
  // review slot exists. Preferring the exec node lets stageStatusFor
  // correctly show "生成中" while the tool runs and "待审核" once
  // the exec completes and the review gate is ready.
  const execNode = reversed.find((node) => matchNode(node) && !isReviewTraceNode(node))
  const reviewNode = reversed.find((node) => matchNode(node) && isReviewTraceNode(node))
  const execStatus = normalizeDirectorStatus(execNode?.status)
  if (execNode && (execStatus === 'running' || execStatus === 'active')) {
    return { execNode, reviewNode, currentNode: execNode }
  }
  if (reviewNode) {
    return { execNode, reviewNode, currentNode: reviewNode }
  }
  if (execNode) {
    return { execNode, currentNode: execNode }
  }

  const currentNode = reversed.find(matchNode)
  return { currentNode }
}

function stageStatusFor(
  role: VideoRoleAgent,
  review: AgentReviewItem | undefined,
  match: RoleTraceMatch,
  projectStarted: boolean,
): DirectorStageStatus {
  const execNode = match.execNode
  const reviewNode = match.reviewNode
  const node = match.currentNode
  const execStatus = normalizeDirectorStatus(execNode?.status)
  const hasRequiredArtifacts = hasRequiredMaterializedArtifacts(role, execNode) || hasReviewArtifactsForRole(role, review)
  const missingRequiredArtifacts = requiredMaterializedArtifactsMissing(role, execNode)

  if (execNode && (execStatus === 'running' || execStatus === 'active')) {
    return 'running'
  }
  if (execStatus === 'failed' || missingRequiredArtifacts) {
    return 'failed'
  }

  // When a PENDING review exists, check whether content is still generating.
  // If the trace node is a tool execution (not a review gate) and is still
  // running, show "生成中" so the user knows the system is still working.
  // Once the exec completes and the review gate appears, show "待审核".
  if (review?.status === 'PENDING') {
    if (isActionablePendingReview(review) || hasRequiredArtifacts) return 'review'
    return 'pending'
  }
  if (review?.status === 'REJECTED') return 'blocked'
  if (review?.status === 'APPROVED') return 'done'
  if (reviewNode) {
    const reviewStatus = normalizeDirectorStatus(reviewNode.status)
    if (reviewStatus === 'failed') return 'failed'
    if (reviewStatus === 'pending' || reviewStatus === 'running') {
      if (isActionableTraceReviewNode(reviewNode) || hasRequiredArtifacts) return 'review'
      return projectStarted ? 'pending' : 'pending'
    }
  }
  if (node?.status) {
    const status = normalizeDirectorStatus(node.status)
    if (isReviewTraceNode(node) && (status === 'running' || status === 'pending')) {
      return isActionableTraceReviewNode(node) ? 'review' : projectStarted ? 'running' : 'pending'
    }
    return status
  }
  if (projectStarted && role.stage === 'proposal') return 'active'
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

function isActionableTraceReviewNode(node: TraceNodeLike | undefined): boolean {
  if (!isReviewTraceNode(node)) return false
  return hasDecisionReviewOutput(node?.output)
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

function execSucceeded(node: TraceNodeLike | undefined): boolean {
  return Boolean(node && !isReviewTraceNode(node) && normalizeDirectorStatus(node.status) === 'done')
}

function hasRequiredMaterializedArtifacts(role: VideoRoleAgent, node: TraceNodeLike | undefined): boolean {
  const required = (role.requiredOutputs || []).filter(requiresMaterializedArtifact)
  if (!required.length) return false
  const artifacts = extractArtifacts(node)
  if (!artifacts.length) return false
  return required.every((kind) => artifacts.some((artifact) => stringValue(artifact.kind) === kind))
}

function requiredMaterializedArtifactsMissing(role: VideoRoleAgent, node: TraceNodeLike | undefined): boolean {
  const required = (role.requiredOutputs || []).filter(requiresMaterializedArtifact)
  if (!required.length || !execSucceeded(node)) return false
  const artifacts = extractArtifacts(node)
  return required.some((kind) => !artifacts.some((artifact) => stringValue(artifact.kind) === kind))
}

function hasReviewArtifactsForRole(role: VideoRoleAgent, review: AgentReviewItem | undefined): boolean {
  if (!review || !Array.isArray(review.reviewArtifacts) || review.reviewArtifacts.length === 0) return false
  const required = (role.requiredOutputs || []).filter(requiresMaterializedArtifact)
  if (!required.length) return true
  return required.every((kind) => review.reviewArtifacts?.some((artifact) => stringValue(artifact.kind) === kind))
}

function normalizeDirectorStatus(status?: string): DirectorStageStatus {
  const normalized = (status || '').toUpperCase()
  if (['SUCCESS', 'SUCCEEDED', 'COMPLETED', 'APPROVED'].includes(normalized)) return 'done'
  if (['RUNNING'].includes(normalized)) return 'running'
  if (['READY', 'PENDING', 'CREATED'].includes(normalized)) return 'pending'
  if (['FAILED', 'ERROR', 'CANCELLED'].includes(normalized)) return 'failed'
  if (['REJECTED', 'BLOCKED'].includes(normalized)) return 'blocked'
  return 'pending'
}

function normalizeArtifactStatus(status?: string): DirectorArtifactStatus | undefined {
  const normalized = (status || '').toLowerCase()
  if (['valid', 'review', 'stale', 'pending', 'running', 'failed', 'blocked', 'missing'].includes(normalized)) return normalized as DirectorArtifactStatus
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

function nodeToolName(node: TraceNodeLike): string {
  const input = node.input || {}
  const directTool = stringValue(input.tool)
  const capabilityTool = stringValue(input.capabilityTool)
  const parameterTool = stringValue(objectValue(input.parameters)?.tool)
  if (directTool === 'external') {
    return capabilityTool || parameterTool || directTool
  }
  return directTool ||
    capabilityTool ||
    stringValue(input.reviewTool) ||
    parameterTool ||
    node.tool ||
    node.name ||
    node.type ||
    ''
}

function nodeFieldString(node: TraceNodeLike, key: string): string | undefined {
  const input = node.input || {}
  const direct = stringValue(input[key])
  if (direct) return direct
  const parameters = objectValue(input.parameters)
  return stringValue(parameters?.[key])
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
  return reviews.filter((review) => {
    if (review.status === 'PENDING') return isActionablePendingReview(review)
    return ['APPROVED', 'REJECTED'].includes(String(review.status))
  })
}

export function isActionablePendingReview(review: AgentReviewItem | undefined): boolean {
  if (!review || review.status !== 'PENDING') return false
  if (typeof review.reviewContent === 'string' && review.reviewContent.trim()) return true
  if (Array.isArray(review.reviewArtifacts) && review.reviewArtifacts.length > 0) return true
  return hasDecisionReviewOutput(review.reviewOutput)
}

function hasDecisionReviewOutput(output: Record<string, unknown> | undefined): boolean {
  if (!output || Object.keys(output).length === 0) return false

  for (const key of ['content', 'script', 'text', 'markdown', 'summary']) {
    if (typeof output[key] === 'string' && output[key].trim()) return true
  }

  if (Array.isArray(output.artifacts) && output.artifacts.length > 0) return true

  for (const key of ['proposalPacket', 'qualityReport', 'package', 'cardPlan', 'shotList', 'compositionSpec', 'preview', 'renderReport', 'publishCopy', 'finalReview']) {
    const value = output[key]
    if (value && typeof value === 'object') {
      if (Array.isArray(value)) {
        if (value.length > 0) return true
      } else if (Object.keys(value as Record<string, unknown>).length > 0) {
        return true
      }
    }
  }

  if (typeof output.stdout === 'string' && output.stdout.trim()) {
    const parsed = tryParseJSON(output.stdout)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return hasDecisionReviewOutput(parsed as Record<string, unknown>)
    }
  }

  return false
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
  const analysis = stringValue(report.analysisSummary) || stringValue(report.analysis) || stringValue(report.summary)
  if (analysis) {
    lines.push(`分析：${analysis}`)
  }

  const rubricBreakdown = Array.isArray(report.rubricBreakdown) ? report.rubricBreakdown : []
  for (const item of rubricBreakdown) {
    const rubric = objectValue(item)
    if (!rubric) continue
    const criterion = stringValue(rubric.criterion) || stringValue(rubric.dimension) || stringValue(rubric.name)
    const itemScore = numberValue(rubric.score)
    const maxScore = numberValue(rubric.maxScore)
    const reason = stringValue(rubric.reason) || stringValue(rubric.comment)
    const scoreText = itemScore !== undefined && maxScore !== undefined
      ? `${itemScore}/${maxScore}`
      : itemScore !== undefined ? String(itemScore) : ''
    const prefix = [criterion, scoreText].filter(Boolean).join('：')
    if (prefix || reason) {
      lines.push([prefix, reason].filter(Boolean).join('，'))
    }
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
  const keepDoing = Array.isArray(report.keepDoing) ? report.keepDoing : Array.isArray(report.whatWorked) ? report.whatWorked : []
  for (const item of keepDoing) {
    if (typeof item === 'string' && item.trim()) {
      lines.push(`保持：${item}`)
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
    SHOT_LIST: 'Shot清单',
    KEYFRAME_PROMPTS: '关键帧提示词',
    VIDEO_PROMPTS: '视频提示词',
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
    SHOT_REVIEW_PACKET: 'Shot审核包',
    SHOT_ASSET_PACKAGE: 'Shot独立素材包',
    SHOT_AUDIO: 'Shot口播音频',
    SHOT_KEYFRAME: 'Shot关键帧',
    SHOT_VIDEO_CLIP: 'Shot视频片段',
    SHOT_SUBTITLE: 'Shot字幕',
    HYPERFRAMES_SHOT: 'HyperFrames片段',
    EXTERNAL_GENERATION_REQUEST: '素材依赖请求',
    PUBLISH_COPY: '发布文案',
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
    'PUBLISH_COPY',
    'PROJECT_PACKAGE',
  ]
  const explicit: Record<string, string[]> = {
    VIDEO_PROPOSAL: order.slice(1),
    VIDEO_SCRIPT: order.slice(2),
    CARD_PLAN: order.slice(3),
    VIDEO_COMPOSITION_SPEC: ['HYPERFRAMES_PROJECT', 'PREVIEW_SNAPSHOTS', 'VIDEO', 'FINAL_REVIEW', 'PUBLISH_COPY', 'PROJECT_PACKAGE'],
    PREVIEW_SNAPSHOTS: ['VIDEO', 'FINAL_REVIEW', 'PUBLISH_COPY', 'PROJECT_PACKAGE'],
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
  return [
    'VIDEO_PROPOSAL',
    'VIDEO_SCRIPT',
    'CARD_PLAN',
    'CAPTION_PLAN',
    'SHOT_LIST',
    'KEYFRAME_PROMPTS',
    'VIDEO_PROMPTS',
    'VIDEO_COMPOSITION_SPEC',
    'REFERENCE_ASSET_PLAN',
    'STYLE_PROFILE',
    'CONTINUITY_REPORT',
    'HYPERFRAMES_PROJECT',
    'PREVIEW_SNAPSHOTS',
    'VIDEO',
    'FFMPEG_PROBE_REPORT',
    'FINAL_REVIEW',
    'SHOT_REVIEW_PACKET',
    'SHOT_ASSET_PACKAGE',
    'SHOT_AUDIO',
    'SHOT_KEYFRAME',
    'SHOT_VIDEO_CLIP',
    'SHOT_SUBTITLE',
    'HYPERFRAMES_SHOT',
    'PUBLISH_COPY',
    'PROJECT_PACKAGE',
  ].includes(kind)
}

function uniqueStrings(values: string[]) {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))]
}

function isInspectableDirectorArtifact(artifact: DirectorArtifactRecord) {
  return Boolean(artifact.id && !/^A\d{2}/.test(artifact.id))
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

function numberValue(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return undefined
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
  dotColor: string
  animate: boolean
  active: boolean
}

export function getStageStateDisplay(status: DirectorStageStatus): StageStateDisplay {
  switch (status) {
    case 'done':
      return { icon: 'check', label: '已通过', colorClass: 'border-green-300 bg-green-50 text-green-700', dotColor: 'bg-green-500', animate: false, active: false }
    case 'review':
      return { icon: 'shield', label: '待审核', colorClass: 'border-amber-400 bg-amber-50 text-amber-700', dotColor: 'bg-amber-500', animate: true, active: true }
    case 'running':
    case 'active':
      return { icon: 'refresh', label: '生成中', colorClass: 'border-line bg-primary-soft text-primary-dark', dotColor: 'bg-primary', animate: true, active: true }
    case 'blocked':
    case 'failed':
      return { icon: 'x', label: '已阻断', colorClass: 'border-red-300 bg-red-50 text-red-700', dotColor: 'bg-red-500', animate: false, active: false }
    case 'pending':
    default:
      return { icon: 'cpu', label: '等待中', colorClass: 'border-stone-200 bg-stone-50/60 text-stone-400', dotColor: 'bg-stone-300', animate: false, active: false }
  }
}
