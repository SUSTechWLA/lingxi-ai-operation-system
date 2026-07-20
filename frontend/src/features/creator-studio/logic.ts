import type { AgentStartRunRequest, CreateVideoProjectPayload } from '../../utils/types'
import type {
  ArtifactSelection,
  CreationView,
  CreatorAction,
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
export const CREATOR_WORKSPACE_STEP_IDS: readonly CreatorStepId[] = [
  'requirements', 'direction', 'script', 'shots', 'preview', 'delivery',
]

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
  const context = {
    topic: prompt,
    durationSec,
    targetDurationSec: durationSec,
    aspectRatio: input.aspectRatio,
    platform,
    materialCount,
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
      mode: 'aigc_shot',
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
        ...context,
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

export function isCreatorStepReadable(step: Pick<CreatorStep, 'state'>): boolean {
  return step.state !== 'not_started'
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
