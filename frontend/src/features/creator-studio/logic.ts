import type {
  CreationView,
  CreatorAction,
  ShotImpact,
  ShotListFilters,
  ShotListItem,
} from './types'

export const CREATOR_CONFLICT_COPY = '内容已更新，请刷新后重试'

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
