import type { AgentReviewItem, AgentStartRunRequest, ArtifactContentResponse, CreateVideoProjectPayload } from '../../utils/types'
import type { ModelCapability, ModelProviderConfig } from '../../services/localAgent'
import type {
  ArtifactSelection,
  CreationView,
  CreatorAction,
  CreatorArtifactDescriptor,
  CreatorProject,
  CreatorVoiceSelection,
  CreatorStep,
  CreatorStepId,
  StepImpact,
  ShotImpact,
  ShotQueueStatus,
  ShotListFilters,
  ShotListItem,
} from './types'

export const CREATOR_CONFLICT_COPY = '内容已更新，请刷新后重试'
export const SHOT_QUEUE_CONFLICT_COPY = '这个 Shot 已有更新，请基于最新版本重试'
export const SHOT_QUEUE_ROW_HEIGHT = 64
export const DEFAULT_SHOT_QUEUE_FILTER: Readonly<{ status: ShotQueueStatus }> = { status: 'needs_attention' }

export function initialShotFilters(projectStatus: string): Readonly<{ status: ShotQueueStatus }> {
  const normalizedStatus = projectStatus.trim().toLocaleUpperCase()
  return { status: normalizedStatus === 'COMPLETED' || normalizedStatus === 'ARCHIVED' ? 'all' : 'needs_attention' }
}

export function isUnfilteredShotPageReset(filters: ShotListFilters, reset: boolean): boolean {
  return reset && filters.status === 'all' && !filters.query?.trim() && !filters.chapter?.trim()
}

export function canApplyShotListRequestState(requestToken: number, latestRequestToken: number, aborted: boolean): boolean {
  return !aborted && isLatestWorkspaceRequest(requestToken, latestRequestToken)
}

export const CREATOR_WORKSPACE_STEP_IDS: readonly CreatorStepId[] = [
  'requirements', 'direction', 'script', 'shots', 'preview', 'delivery',
]

export function creatorProjectProgress(
  projectStatus: string,
  view?: Pick<CreationView, 'steps'>,
): { label: string; percent: number } {
  const total = view?.steps.length || CREATOR_WORKSPACE_STEP_IDS.length
  const terminal = projectStatus === 'COMPLETED' || projectStatus === 'ARCHIVED'
  const complete = terminal ? total : (view?.steps.filter(step => step.state === 'confirmed').length || 0)
  if (!view && !terminal) return { label: '正在准备创作', percent: 0 }
  return {
    label: `已完成 ${complete}/${total} 个步骤`,
    percent: Math.round((complete / total) * 100),
  }
}

export function creatorProjectEntryStep(projectStatus: string, activeStep?: CreatorStepId): CreatorStepId {
  if (projectStatus === 'COMPLETED' || projectStatus === 'ARCHIVED') return 'preview'
  return activeStep ?? 'requirements'
}

export interface CompletedTaskView {
  project: Pick<CreatorProject, 'status'>
  activeStep?: CreatorStepId
}

export interface CompletedTaskMediaAvailability {
  preview?: CreatorMediaState
  delivery?: CreatorMediaState
}

export interface CompletedTaskRepairScope {
  stepId: 'delivery'
  preserveUpstream: true
}

export function completedTaskLandingStep(
  view: CompletedTaskView,
  availability: CompletedTaskMediaAvailability,
): CreatorStepId {
  // A completed task always opens its delivery workspace, including when the
  // media probe reports a recoverable delivery failure.
  void availability
  if (view.project.status === 'COMPLETED' || view.project.status === 'ARCHIVED') return 'delivery'
  return creatorProjectEntryStep(view.project.status, view.activeStep)
}

export function completedRepairScope(
  view: CompletedTaskView,
  availability: CompletedTaskMediaAvailability,
): CompletedTaskRepairScope | undefined {
  const completed = view.project.status === 'COMPLETED' || view.project.status === 'ARCHIVED'
  if (!completed || (availability.delivery !== 'missing' && availability.delivery !== 'unsupported')) return undefined
  return { stepId: 'delivery', preserveUpstream: true }
}

export function withCreatorProjectRecord(
  steps: readonly CreatorStep[],
  project: Pick<CreatorProject, 'name' | 'description'>,
): CreatorStep[] {
  const hasProjectRecord = Boolean(project.name.trim() || project.description?.trim())
  if (!hasProjectRecord) return [...steps]
  return steps.map(step => step.id === 'requirements'
    ? {
        ...step,
        hasHistory: true,
        attemptCount: Math.max(1, step.attemptCount),
        artifactCount: Math.max(1, step.artifactCount),
      }
    : step)
}

const STEP_LABELS: Record<CreatorStepId, string> = {
  requirements: '需求',
  direction: '创意方案',
  script: '脚本',
  shots: '分镜与素材',
  preview: '成片预览',
  delivery: '交付',
}

export interface CreationRequestInput {
  prompt: string
  durationSec?: number
  aspectRatio: string
  platform?: string
  materialCount: number
  productionRoute?: CreatorProductionRoute
  aigcPolicy?: CreatorAIGCPolicy
  modelProviders?: Partial<Record<ModelCapability, ModelProviderConfig>>
  voiceSelection?: CreatorVoiceSelection
}

export type CreatorProductionRoute = 'talking_head' | 'cinematic_story'
export type CreatorAIGCPolicy = 'auto' | 'disabled'

export function validateCreatorVoiceSelection(selection: CreatorVoiceSelection): string | undefined {
  if (selection.mode === 'default_ip') return undefined
  if (selection.mode === 'reference_clone') {
    if (!selection.referenceText?.trim()) return '请填写录音中实际说出的完整录音原文。'
    if (!selection.referenceTextVerified) return '请确认录音原文与音频完全一致。'
    if (!selection.usageRightsConfirmed) return '请确认你拥有该录音及声音的使用授权。'
    return undefined
  }
  if (!selection.usageRightsConfirmed) return '请确认你拥有该口播录音的使用授权。'
  return undefined
}

export interface CreationRequest {
  project: CreateVideoProjectPayload
  agentRun: AgentStartRunRequest
}

export async function mapWithConcurrency<T, Result>(
  values: readonly T[],
  concurrency: number,
  map: (value: T, index: number) => Promise<Result>,
  onResult?: (result: Result, index: number) => void,
): Promise<Result[]> {
  const results = new Array<Result>(values.length)
  let nextIndex = 0
  const workerCount = Math.min(values.length, Math.max(1, Math.floor(concurrency)))
  await Promise.all(Array.from({ length: workerCount }, async () => {
    while (nextIndex < values.length) {
      const index = nextIndex
      nextIndex += 1
      const result = await map(values[index], index)
      results[index] = result
      onResult?.(result, index)
    }
  }))
  return results
}

export function buildProjectMaterialStorageRef(projectId: string, materialId: string): string {
  if (!isSafeStorageSegment(projectId) || !isSafeStorageSegment(materialId)) {
    throw new TypeError('project and material ids must be safe path segments')
  }
  return `local://projects/${projectId}/materials/${materialId}`
}

export function resolveCreatorArtifactMediaUrl(
	projectId: string,
	content: ArtifactContentResponse | null | undefined,
	localAgentBaseUrl: string,
): string | undefined {
	const direct = content?.mediaUrl || content?.mediaUrls?.[0]
	const baseUrl = localAgentBaseUrl.replace(/\/+$/, '')
	if (direct) return normalizeCreatorDirectMediaUrl(direct, baseUrl)
	if (!content || content.artifact.projectId !== projectId || !isSafeStorageSegment(projectId)) return undefined
	const metadata = content.artifact.metadata
	const localPath = typeof metadata?.localPath === 'string' ? metadata.localPath.trim() : ''
	if (!baseUrl || metadata?.localOnly !== true) return undefined
	if (localPath) {
		return `${baseUrl}/api/local/media?projectId=${encodeURIComponent(projectId)}&path=${encodeURIComponent(localPath)}`
	}
	const storageRef = typeof content.artifact.storageRef === 'string' ? content.artifact.storageRef.trim() : ''
	const projectStoragePrefix = `local://projects/${projectId}/`
	if (!storageRef.startsWith(projectStoragePrefix)) return undefined
	return `${baseUrl}/api/local/media?projectId=${encodeURIComponent(projectId)}&storageRef=${encodeURIComponent(storageRef)}`
}

function normalizeCreatorDirectMediaUrl(direct: string, localAgentBaseUrl: string): string {
	if (!direct.startsWith('/') || !localAgentBaseUrl) return direct
	try {
		const localAgent = new URL(localAgentBaseUrl)
		if (localAgent.protocol !== 'http:' && localAgent.protocol !== 'https:') return direct
		const normalized = new URL(direct, localAgent)
		if (normalized.origin !== localAgent.origin || normalized.pathname !== '/api/local/media') return direct
		return normalized.toString()
	} catch {
		return direct
	}
}

export type CreatorMediaState = 'loading' | 'playable' | 'missing' | 'unsupported' | 'service_unavailable' | 'unavailable'

export function creatorMediaStateAfterHttpProbe(status: number | undefined): CreatorMediaState {
	if (status === 404) return 'missing'
	if (status !== undefined && status >= 200 && status < 300) return 'loading'
	return 'service_unavailable'
}

export function creatorMediaStateAfterLoadedMetadata(): CreatorMediaState {
	return 'playable'
}

export function creatorMediaStateAfterMediaError(
	mediaErrorCode: number | undefined,
	deliveryStatus: number | undefined,
	localSource: boolean,
): CreatorMediaState {
	if (!localSource) return mediaErrorCode === 3 || mediaErrorCode === 4 ? 'unsupported' : 'unavailable'
	if (deliveryStatus === 404) return 'missing'
	if (deliveryStatus === undefined || deliveryStatus < 200 || deliveryStatus >= 300) return 'service_unavailable'
	return mediaErrorCode === 3 || mediaErrorCode === 4 ? 'unsupported' : 'service_unavailable'
}

export function isCreatorLocalMediaUrl(src: string, localAgentBaseUrl: string): boolean {
	try {
		const localAgent = new URL(localAgentBaseUrl)
		const source = new URL(src)
		return (source.protocol === 'http:' || source.protocol === 'https:') &&
			source.origin === localAgent.origin &&
			source.username === '' &&
			source.password === '' &&
			source.pathname === '/api/local/media'
	} catch {
		return false
	}
}

export function canApplyCreatorMediaProbe(
	probeToken: number,
	latestProbeToken: number,
	probeSrc: string,
	currentSrc: string,
	aborted: boolean,
): boolean {
	return !aborted && probeToken === latestProbeToken && probeSrc === currentSrc
}

export function canonicalCreatorMediaElementSrc(src: string, documentBase: string): string {
	try {
		return new URL(src, documentBase).toString()
	} catch {
		return src
	}
}

export function canApplyCreatorMediaElementEvent(
	eventCurrentSrc: string,
	expectedSrc: string,
	eventToken: number,
	latestToken: number,
): boolean {
	return eventToken === latestToken && (eventCurrentSrc === '' || eventCurrentSrc === expectedSrc)
}

export function creatorStartIdempotencyKey(projectId: string): string {
  if (!isSafeStorageSegment(projectId)) throw new TypeError('project id must be a safe path segment')
  return `creator-start:${projectId}`
}

export function prioritizeCreationViewProjects<T extends { status: string; updatedAt: string }>(projects: readonly T[]): T[] {
  return [...projects].sort((left, right) => {
    const leftHistory = left.status === 'COMPLETED' || left.status === 'ARCHIVED'
    const rightHistory = right.status === 'COMPLETED' || right.status === 'ARCHIVED'
    return Number(leftHistory) - Number(rightHistory) || Date.parse(right.updatedAt) - Date.parse(left.updatedAt)
  })
}

function isSafeStorageSegment(value: string): boolean {
  return value.length > 0 && value !== '.' && value !== '..' && !/[/%\\?#]/.test(value)
}

export function buildCreationRequest(input: CreationRequestInput): CreationRequest {
  const prompt = input.prompt.trim()
  const durationSec = input.durationSec
  const platform = input.platform?.trim()
  const materialCount = Math.max(0, Math.floor(input.materialCount))
  const productionRoute = input.productionRoute ?? 'talking_head'
  const aigcPolicy = input.aigcPolicy ?? 'auto'
  const projectMode = productionRoute === 'talking_head' ? 'voice_visual' : 'aigc_shot'
  const aigcEnabled = aigcPolicy !== 'disabled'
  const ipRenderMode = productionRoute === 'talking_head' ? 'production' : undefined
  const voiceSelection = productionRoute === 'talking_head'
    ? input.voiceSelection ?? {
        mode: 'default_ip' as const,
        provider: 'gpt_sovits_local' as const,
        voiceId: 'main_ip_warm_knowledge_host_v1',
      }
    : undefined
  const requiredLayers = productionRoute === 'talking_head'
    ? ['ip_aroll', 'hyperframes_text']
    : ['aigc_main', 'hyperframes_text']
  const designedLayers = ['ip_aroll', 'hyperframes_text', 'aigc_enrichment']
  const layerExecutionPolicy = {
    ip_aroll: productionRoute === 'talking_head' ? 'required' : 'optional',
    hyperframes_text: 'required',
    aigc_enrichment: aigcEnabled ? 'auto' : 'disabled',
  }
  const modelProviders = modelProvidersForPolicy(input.modelProviders, aigcEnabled)
  const modelProviderRefs = buildProjectModelProviderRefs(modelProviders)
  const context = {
    topic: prompt,
    durationSec,
    targetDurationSec: durationSec,
    aspectRatio: input.aspectRatio,
    platform,
    materialCount,
    productionRoute,
    canonicalProfileId: productionRoute,
    videoType: projectMode,
    aigcEnabled,
    aigcProvider: aigcEnabled ? 'auto' : 'disabled',
    aigcPolicy,
    visualLayerContract: 'shot_visual_layers_v1',
    designedLayers,
    layerExecutionPolicy,
    requiredLayers,
    ...(ipRenderMode ? { ipRenderMode } : {}),
    ...(voiceSelection ? { voiceSelection } : {}),
    ...(modelProviders && Object.keys(modelProviders).length > 0
      ? { modelProviders }
      : {}),
  }
  const durationCopy = durationSec ? `一支 ${durationSec} 秒` : ''
  const platformCopy = platform ? `适合${platform}发布的` : ''
  const message = durationCopy || platformCopy
    ? `创作${durationCopy}${durationCopy && platformCopy ? '、' : ''}${platformCopy}视频：${prompt}`
    : `创作视频：${prompt}`

  return {
    project: {
      name: prompt.slice(0, 40) || '视频创作项目',
      description: prompt,
      mode: projectMode,
      skillName: 'video-creator',
      skillVersion: 'v4.0',
      workflowName: 'dynamic-agent-video-creation',
      workflowVersion: 'v4.0',
      generationMode: 'provider_api',
      aspectRatio: input.aspectRatio,
      ...(durationSec ? { targetDurationSec: durationSec } : {}),
      language: 'zh-CN',
      config: {
        entry: 'creator_studio',
        topic: prompt,
        durationSec,
        targetDurationSec: durationSec,
        aspectRatio: input.aspectRatio,
        platform,
        materialCount,
        productionRoute,
        canonicalProfileId: productionRoute,
        aigcEnabled,
        aigcProvider: aigcEnabled ? 'auto' : 'disabled',
        aigcPolicy,
        visualLayerContract: 'shot_visual_layers_v1',
        designedLayers,
        layerExecutionPolicy,
        requiredLayers,
        ...(ipRenderMode ? { ipRenderMode } : {}),
        ...(voiceSelection ? { voiceSelection } : {}),
        ...(Object.keys(modelProviderRefs).length > 0 ? { modelProviderRefs } : {}),
      },
    },
    agentRun: {
      message,
      domain: 'video_creation',
      mode: 'dynamic_agent',
      context,
    },
  }
}

function modelProvidersForPolicy(
  providers: Partial<Record<ModelCapability, ModelProviderConfig>> | undefined,
  aigcEnabled: boolean,
): Partial<Record<ModelCapability, ModelProviderConfig>> | undefined {
  if (!providers || aigcEnabled) return providers
  return providers.text_to_text ? { text_to_text: providers.text_to_text } : undefined
}

export function buildProjectModelProviderRefs(
  providers?: Partial<Record<ModelCapability, ModelProviderConfig>>,
): Partial<Record<ModelCapability, { source: 'local_agent'; baseUrl: string; model: string }>> {
  const refs: Partial<Record<ModelCapability, { source: 'local_agent'; baseUrl: string; model: string }>> = {}
  for (const capability of ['text_to_text', 'text_to_image', 'text_to_video'] as const) {
    const provider = providers?.[capability]
    if (!provider?.apiKey || !provider.baseUrl.trim() || !provider.model.trim()) continue
    refs[capability] = {
      source: 'local_agent',
      baseUrl: provider.baseUrl.trim(),
      model: provider.model.trim(),
    }
  }
  return refs
}

export function nextCreatorAction(view: Pick<CreationView, 'steps'>): CreatorAction {
  const step = view.steps.find(item =>
    item.state === 'failed' || item.state === 'needs_attention' ||
    item.state === 'needs_review' || item.state === 'generating',
  ) ?? view.steps.find(item => item.state === 'not_started') ?? view.steps[view.steps.length - 1]

  if (!step) return { kind: 'start', stepId: 'requirements', label: '开始创作' }
  if (step.state === 'needs_review') return { kind: 'review', stepId: step.id, label: `审核${step.label}` }
  if (step.state === 'generating') return { kind: 'wait', stepId: step.id, label: `${step.label}生成中` }
  if (step.state === 'failed' || step.state === 'needs_attention') {
    return { kind: 'fix', stepId: step.id, label: `处理${step.label}` }
  }
  return { kind: 'continue', stepId: step.id, label: `继续${step.label}` }
}

export function nextPendingCreatorReview(reviews: readonly AgentReviewItem[]): AgentReviewItem | undefined {
  return reviews.find(review => review.status === 'PENDING')
}

export function creatorAgentReviewContent(review: AgentReviewItem): string {
  const qualityReport = creatorAgentReviewQualityReport(review)
  if (review.reviewPhase === 'quality_gate' && qualityReport) {
    const score = finiteNumber(qualityReport.score)
    const passed = qualityReport.passed === true
    const lines = [
      score === undefined ? '质量检查结果' : `质量评分：${score}/100`,
      `状态：${passed ? '通过' : '未通过'}`,
    ]
    const summary = stringValue(qualityReport.analysisSummary)
    if (summary) lines.push('', summary)
    const issues = recordList(qualityReport.issues)
    if (issues.length > 0) {
      lines.push('', '发现的问题：')
      for (const issue of issues) {
        const label = stringValue(issue.field)
        const message = stringValue(issue.message)
        if (label || message) lines.push(`- ${label ? `${label}：` : ''}${message}`)
      }
    }
    const suggestions = stringList(qualityReport.repairSuggestions)
    if (suggestions.length > 0) {
      lines.push('', '建议修改：', ...suggestions.map(suggestion => `- ${suggestion}`))
    }
    return lines.join('\n')
  }
  const content = review.reviewContent?.trim()
  if (content) return content
  return reviewOutputText(review.reviewOutput)
}

export function creatorAgentReviewCanRegenerate(review: AgentReviewItem): boolean {
  return review.status === 'PENDING'
}

export function creatorAgentReviewApprovalBlocked(review: AgentReviewItem): boolean {
  if (review.reviewPhase !== 'quality_gate') return false
  const qualityReport = creatorAgentReviewQualityReport(review)
  if (!qualityReport) return false
  const score = finiteNumber(qualityReport.score)
  return qualityReport.passed === false || (score !== undefined && score < 85)
}

export function creatorAgentReviewRegenerationHint(review: AgentReviewItem): string {
  const qualityReport = creatorAgentReviewQualityReport(review)
  const suggestions = stringList(qualityReport?.repairSuggestions)
  if (suggestions.length > 0) return suggestions.join('\n')
  return ''
}

function creatorAgentReviewQualityReport(review: AgentReviewItem): Record<string, unknown> | undefined {
  const report = review.reviewOutput?.qualityReport
  if (!report || typeof report !== 'object' || Array.isArray(report)) return undefined
  const record = report as Record<string, unknown>
  const packaged = record.package
  return packaged && typeof packaged === 'object' && !Array.isArray(packaged)
    ? packaged as Record<string, unknown>
    : record
}

function reviewOutputText(output: Record<string, unknown> | undefined): string {
  if (!output || Object.keys(output).length === 0) return ''
  const preferred = output.script ?? output.summary ?? output.content ?? output.result
  if (typeof preferred === 'string') return preferred
  try {
    return JSON.stringify(output, null, 2)
  } catch {
    return ''
  }
}

function finiteNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function stringList(value: unknown): string[] {
  return Array.isArray(value) ? value.map(stringValue).filter(Boolean) : []
}

function recordList(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter(item => Boolean(item) && typeof item === 'object' && !Array.isArray(item)) as Record<string, unknown>[]
    : []
}

export function creatorStepForAgentReview(
  review: Pick<AgentReviewItem, 'stage' | 'stepId' | 'tool'>,
): CreatorStepId {
  const key = `${review.stage ?? ''} ${review.stepId ?? ''} ${review.tool ?? ''}`.toLowerCase()
  if (/visual_qa|publish_copy|delivery|package/.test(key)) return 'delivery'
  if (/preview|render/.test(key)) return 'preview'
  if (/audio_master|time_window|visual_alignment|shot_generation|video_prompt|ip_aroll|mcp_generation|storyboard/.test(key)) return 'shots'
  if (/script/.test(key)) return 'script'
  if (/proposal|direction|creative/.test(key)) return 'direction'
  return 'requirements'
}

export function canSubmitShotDuration(durationSec: number): boolean {
  return Number.isFinite(durationSec) && durationSec > 0 && durationSec < 15
}

export function selectShotListItems(
  items: readonly ShotListItem[],
  filters: ShotListFilters,
): ShotListItem[] {
  const query = filters.query?.trim().toLocaleLowerCase()
  return items
    .filter(item => !filters.status || item.reviewStatus === filters.status)
    .filter(item => !filters.chapter || item.chapter === filters.chapter)
    .filter(item => !query || `${item.title} ${item.id}`.toLocaleLowerCase().includes(query))
    .map((item, originalIndex) => ({ item, originalIndex }))
    .sort((left, right) =>
      left.item.sequenceIndex - right.item.sequenceIndex ||
      left.item.id.localeCompare(right.item.id) ||
      left.originalIndex - right.originalIndex,
    )
    .map(({ item }) => item)
}

export function shotQueueWindow(input: {
  total: number
  scrollTop: number
  viewportHeight: number
  rowHeight?: number
  overscan?: number
}): { start: number; end: number; items: number[]; offsetTop: number; totalHeight: number } {
  const rowHeight = input.rowHeight ?? SHOT_QUEUE_ROW_HEIGHT
  const overscan = input.overscan ?? 2
  const total = Math.max(0, Math.floor(input.total))
  const visible = Math.max(1, Math.ceil(Math.max(0, input.viewportHeight) / rowHeight))
  const start = Math.max(0, Math.floor(Math.max(0, input.scrollTop) / rowHeight) - overscan)
  const end = Math.min(total, start + visible + overscan * 2)
  return {
    start,
    end,
    items: Array.from({ length: Math.max(0, end - start) }, (_, index) => start + index),
    offsetTop: start * rowHeight,
    totalHeight: total * rowHeight,
  }
}

export function selectedShotAfterAppend<T extends { id: string }>(
  selectedShotId: string | undefined,
  current: readonly T[],
  appended: readonly T[],
): string | undefined {
  return selectedShotId || current[0]?.id || appended[0]?.id
}

export function selectedShotAfterReplacement<T extends { id: string }>(
  selectedShotId: string | undefined,
  replacement: readonly T[],
): string | undefined {
  return replacement.some(item => item.id === selectedShotId) ? selectedShotId : replacement[0]?.id
}

export function isShotRetryEligible(shot: {
  qaStatus?: string
  candidates?: readonly { status?: string; qaReport?: { status?: string } | null }[]
}): boolean {
  if (shot.qaStatus === 'SHOT_QA_FAILED') return true
  return (shot.candidates ?? []).some(candidate =>
    candidate.status === 'SHOT_QA_FAILED' || candidate.qaReport?.status === 'SHOT_QA_FAILED',
  )
}

export function adoptCreatorShotTask<T extends { id: string }>(tasks: readonly T[], task: T): T[] {
  const index = tasks.findIndex(item => item.id === task.id)
  if (index < 0) return [...tasks, task]
  return tasks.map(item => item.id === task.id ? task : item)
}

export function isTargetOnlyShotImpact(
  impact: Pick<ShotImpact, 'shotId' | 'affectedShotIds' | 'regeneratesOtherShots'>,
  targetShotId: string,
): boolean {
  return impact.shotId === targetShotId &&
    !impact.regeneratesOtherShots &&
    impact.affectedShotIds.length === 1 &&
    impact.affectedShotIds[0] === targetShotId
}

export function creatorStepLabel(stepId: CreatorStepId): string {
  return STEP_LABELS[stepId]
}

export function isCreatorStepReadable(step: Pick<CreatorStep, 'state'> & Partial<Pick<CreatorStep, 'hasHistory'>>): boolean {
  return step.state !== 'not_started' || step.hasHistory === true
}

export function creatorStepRegenerationIdempotencyKey(projectId: string, stepId: CreatorStepId, nonce: string): string {
  return `creator-regeneration:${projectId}:${stepId}:${nonce}`
}

export function selectCreatorStepArtifact<T extends Pick<CreatorArtifactDescriptor, 'artifactId' | 'isCurrent'> & Partial<Pick<CreatorArtifactDescriptor, 'kind' | 'isStale'>>>(
  artifacts: readonly T[],
  preferredArtifactId?: string,
  currentArtifactId?: string,
  preferredKind?: string,
): T | undefined {
  const normalizedKind = preferredKind?.trim().toLowerCase()
  const matchesKind = (item: T) => normalizedKind && item.kind?.trim().toLowerCase() === normalizedKind
  return artifacts.find(item => item.artifactId === preferredArtifactId) ??
    artifacts.find(item => matchesKind(item) && item.isCurrent && item.isStale !== true) ??
    artifacts.find(item => matchesKind(item) && item.isStale !== true) ??
    artifacts.find(item => matchesKind(item)) ??
    artifacts.find(item => item.artifactId === currentArtifactId) ??
    artifacts.find(item => item.isCurrent) ??
    artifacts[0]
}

export function canConfirmCreatorStep(step: Pick<CreatorStep, 'state' | 'allowedActions'>): boolean {
  return step.state === 'needs_review' && step.allowedActions.includes('confirm')
}

export function formatStepImpact(impact: Pick<StepImpact, 'affectedStepIds'>): string {
  const labels = impact.affectedStepIds.map(creatorStepLabel)
  return labels.length > 0 ? `${labels.join('、')}需要更新` : '本步骤将更新'
}

export function normalizeRectSelection(
  rect: { x: number; y: number; width: number; height: number },
  bounds: { width: number; height: number },
): Extract<ArtifactSelection, { kind: 'rect' }> {
  const left = Math.min(rect.x, rect.x + rect.width)
  const top = Math.min(rect.y, rect.y + rect.height)
  const right = Math.max(rect.x, rect.x + rect.width)
  const bottom = Math.max(rect.y, rect.y + rect.height)
  const width = Math.max(1, bounds.width)
  const height = Math.max(1, bounds.height)
  const x = rounded(clamp(left / width, 0, 1))
  const y = rounded(clamp(top / height, 0, 1))
  return {
    kind: 'rect',
    x,
    y,
    width: rounded(clamp(right / width, x, 1) - x),
    height: rounded(clamp(bottom / height, y, 1) - y),
  }
}

export function createTimeSelection(firstMs: number, secondMs: number): Extract<ArtifactSelection, { kind: 'time' }> {
  const startMs = Math.max(0, Math.min(firstMs, secondMs))
  return { kind: 'time', startMs, endMs: Math.max(startMs + 1, firstMs, secondMs) }
}

export function creatorMutationIdempotencyKey(
  projectId: string,
  stepId: CreatorStepId,
  mutation: Record<string, unknown>,
): string {
  return `creator-mutation:${projectId}:${stepId}:${stableFingerprint(mutation)}`
}

export function creatorPollDelay(attempt: number): number {
  return Math.min(5000, 500 * (2 ** Math.max(0, Math.floor(attempt))))
}

export function isCreatorConflict(error: unknown): boolean {
  if (!error || typeof error !== 'object' || !('response' in error)) return false
  const response = error.response
  return Boolean(response && typeof response === 'object' && 'status' in response && response.status === 409)
}

export interface WorkspaceArtifactSelection {
  stepId: CreatorStepId
  artifactId: string
  version: number
}

export interface KeyedWorkspaceArtifact<T> {
  key: string
  value: T
}

export function workspaceArtifactKey(selection: WorkspaceArtifactSelection): string {
  return `${selection.stepId}:${selection.artifactId}:${selection.version}`
}

export function isCurrentWorkspaceArtifact<T>(
  result: KeyedWorkspaceArtifact<T> | null | undefined,
  selection: WorkspaceArtifactSelection | null | undefined,
): boolean {
  return Boolean(result && selection && result.key === workspaceArtifactKey(selection))
}

export function creatorArtifactLoadState<T>(
  selection: WorkspaceArtifactSelection | null | undefined,
  result: KeyedWorkspaceArtifact<T> | null | undefined,
  loadError: string,
): 'empty' | 'loading' | 'ready' | 'error' {
  if (!selection) return 'empty'
  if (isCurrentWorkspaceArtifact(result, selection)) return 'ready'
  return loadError ? 'error' : 'loading'
}

export function isLatestWorkspaceRequest(requestToken: number, latestToken: number): boolean {
  return requestToken === latestToken
}

export function didSelectedShotTaskChange(
  previous: readonly { id: string; shotId?: string; status: string }[],
  next: readonly { id: string; shotId?: string; status: string }[],
  selectedShotId?: string,
): boolean {
  if (!selectedShotId) return false
  const signature = (tasks: readonly { id: string; shotId?: string; status: string }[]) => tasks
    .filter(task => task.shotId === selectedShotId)
    .map(task => `${task.id}:${task.status}`)
    .sort()
    .join('|')
  return signature(previous) !== signature(next)
}

// Delivery is intentionally fail-closed: a current media URL is not a final
// deliverable until the server artifact explicitly records a passed review.
export function deliveryArtifactPassesFinalReview(value: unknown): boolean {
	if (!value || typeof value !== 'object') return false
	const record = value as Record<string, unknown>
	if (deliveryArtifactPassesFinalReview(record.content)) return true
	if (record.artifact && typeof record.artifact === 'object') {
		const artifact = record.artifact as Record<string, unknown>
		if (deliveryArtifactPassesFinalReview(artifact.metadata)) return true
	}
	if (typeof record.finalQaStatus === 'string' && record.finalQaStatus.toLowerCase() === 'passed') return true
	for (const key of ['finalQa', 'finalQualityCheck']) {
		const value = record[key]
		if (value && typeof value === 'object') {
			const check = value as Record<string, unknown>
			if (check.passed === true || (typeof check.status === 'string' && check.status.toLowerCase() === 'passed')) return true
		}
	}
	return false
}

function stableFingerprint(value: unknown): string {
  const text = stableStringify(value)
  let hash = 2166136261
  for (let index = 0; index < text.length; index += 1) {
    hash ^= text.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0).toString(36)
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value))
}

function rounded(value: number): number {
  return Math.round(value * 1_000_000) / 1_000_000
}

function stableStringify(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${Object.keys(record).sort().map(key => `${JSON.stringify(key)}:${stableStringify(record[key])}`).join(',')}}`
}
