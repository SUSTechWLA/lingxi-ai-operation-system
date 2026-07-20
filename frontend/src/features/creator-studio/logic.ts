import type { AgentStartRunRequest, CreateVideoProjectPayload } from '../../utils/types'
import type {
  CreationView,
  CreatorAction,
  ShotImpact,
  ShotListFilters,
  ShotListItem,
} from './types'

export const CREATOR_CONFLICT_COPY = '内容已更新，请刷新后重试'

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

export function isTargetOnlyShotImpact(
  impact: Pick<ShotImpact, 'shotId' | 'affectedShotIds' | 'regeneratesOtherShots'>,
  targetShotId: string,
): boolean {
  return impact.shotId === targetShotId &&
    !impact.regeneratesOtherShots &&
    impact.affectedShotIds.length === 1 &&
    impact.affectedShotIds[0] === targetShotId
}
