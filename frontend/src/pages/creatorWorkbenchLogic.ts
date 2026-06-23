export interface StageDefinitionLike {
  name: string
  approvalRequired?: boolean
  optional?: boolean
}

export interface RunStageDisplay {
  key: string
  label: string
  status?: string
  approvalStage?: string
}

export interface TraceNodeLike {
  id?: string
  name?: string
  status?: string
}

export const normalizeStageStatus = (status?: string) => {
  if (!status) return undefined
  const normalized = status.toUpperCase()
  const map: Record<string, string> = {
    SUCCESS: 'SUCCEEDED',
    SKIPPED: 'SUCCEEDED',
    READY: 'WAITING_APPROVAL',
    CREATED: 'PENDING',
  }
  return map[normalized] || normalized
}

const statusRank = (status?: string) => {
  const normalized = normalizeStageStatus(status)
  const ranks: Record<string, number> = {
    PENDING: 0,
    RUNNING: 1,
    WAITING_APPROVAL: 2,
    SUCCEEDED: 3,
    FAILED: 3,
    CANCELLED: 3,
    INVALIDATED: 3,
  }
  return normalized ? ranks[normalized] ?? 0 : -1
}

const mergeStageStatus = (previous?: string, incoming?: string) => {
  const previousStatus = normalizeStageStatus(previous)
  const incomingStatus = normalizeStageStatus(incoming)
  if (!previousStatus) return incomingStatus
  if (!incomingStatus) return previousStatus
  if (previousStatus === 'SUCCEEDED' && incomingStatus !== 'FAILED' && incomingStatus !== 'INVALIDATED') {
    return previousStatus
  }
  return statusRank(incomingStatus) >= statusRank(previousStatus) ? incomingStatus : previousStatus
}

export const mergeRunStageStatuses = (
  previous: Record<string, string> | undefined,
  incoming: Record<string, string> | undefined,
) => {
  const next: Record<string, string> = {}
  const keys = new Set([...Object.keys(previous || {}), ...Object.keys(incoming || {})])
  keys.forEach((key) => {
    const status = mergeStageStatus(previous?.[key], incoming?.[key])
    if (status) next[key] = status
  })
  return next
}

export const isStageApproved = (
  stageStatuses: Record<string, string> | undefined,
  stageName: string | undefined,
  requiresApproval: boolean,
) => Boolean(requiresApproval && stageName && normalizeStageStatus(stageStatuses?.[stageName]) === 'SUCCEEDED')

export const isStageConfirmed = (
  stageStatuses: Record<string, string> | undefined,
  stageName: string | undefined,
  requiresApproval: boolean,
  locallyConfirmed: boolean,
) => {
  if (!stageName) return false
  if (locallyConfirmed) return true
  return isStageApproved(stageStatuses, stageName, requiresApproval)
}

export type ReviewStatus = 'CONFIRMED' | 'PENDING_CONFIRMATION' | 'RUNNING' | undefined

export const reviewStatusForStage = (
  stageStatuses: Record<string, string> | undefined,
  stageName: string | undefined,
  requiresApproval: boolean,
  locallyConfirmed: boolean,
): ReviewStatus => {
  if (!stageName) return undefined
  if (isStageConfirmed(stageStatuses, stageName, requiresApproval, locallyConfirmed)) return 'CONFIRMED'
  if (!requiresApproval) return undefined

  const stageStatus = normalizeStageStatus(stageStatuses?.[stageName])
  const execStatus = normalizeStageStatus(stageStatuses?.[`${stageName}_exec`])
  if (stageStatus === 'WAITING_APPROVAL' || (execStatus === 'SUCCEEDED' && stageStatus !== 'SUCCEEDED')) {
    return 'PENDING_CONFIRMATION'
  }
  if (stageStatus === 'RUNNING' || execStatus === 'RUNNING') return 'RUNNING'
  return undefined
}

export const aggregateStageStatus = (statuses: Array<string | undefined>) => {
  const normalized = statuses.map(normalizeStageStatus)
  if (normalized.includes('WAITING_APPROVAL')) return 'WAITING_APPROVAL'
  if (normalized.includes('RUNNING')) return 'RUNNING'
  if (normalized.includes('FAILED')) return 'FAILED'
  if (normalized.includes('SUCCEEDED')) return 'SUCCEEDED'
  if (normalized.includes('CANCELLED')) return 'CANCELLED'
  if (normalized.includes('INVALIDATED')) return 'INVALIDATED'
  if (normalized.includes('PENDING')) return 'PENDING'
  return undefined
}

export const stageNodeIds = (stage: StageDefinitionLike) => {
  const ids: string[] = []
  if (stage.optional) ids.push(`${stage.name}_skip`)
  if (stage.approvalRequired || stage.optional) ids.push(`${stage.name}_exec`)
  ids.push(stage.name)
  return ids
}

export const buildRunStageDisplays = (
  stageDefinitions: StageDefinitionLike[],
  stageStatuses: Record<string, string>,
  stageNameMap: Record<string, string>,
): RunStageDisplay[] => {
  if (stageDefinitions.length === 0) {
    return Object.keys(stageStatuses).map((key) => ({
      key,
      label: stageNameMap[key] || key,
      status: normalizeStageStatus(stageStatuses[key]),
      approvalStage: key,
    }))
  }

  return stageDefinitions.map((stage) => {
    const nodeIds = stageNodeIds(stage)
    let status = aggregateStageStatus(nodeIds.map((id) => stageStatuses[id]))
    const executionStatus = normalizeStageStatus(stageStatuses[`${stage.name}_exec`])
    const approvalStatus = normalizeStageStatus(stageStatuses[stage.name])

    if (stage.approvalRequired && executionStatus === 'SUCCEEDED' && approvalStatus !== 'SUCCEEDED') {
      status = 'WAITING_APPROVAL'
    }

    return {
      key: stage.name,
      label: stageNameMap[stage.name] || stage.name,
      status,
      approvalStage: stage.approvalRequired ? stage.name : undefined,
    }
  })
}

export const mergeApprovedStageStatus = (
  stageStatuses: Record<string, string> | undefined,
  stageName: string,
) => ({
  ...(stageStatuses || {}),
  [`${stageName}_exec`]: 'SUCCEEDED',
  [stageName]: 'SUCCEEDED',
})

export const mergeTraceNodeStatuses = (
  stageStatuses: Record<string, string> | undefined,
  nodes: TraceNodeLike[],
) => {
  const next = { ...(stageStatuses || {}) }
  nodes.forEach((node) => {
    const key = node.id || node.name
    const status = normalizeStageStatus(node.status)
    if (!key || !status) return
    next[key] = mergeStageStatus(next[key], status) || status
  })
  return next
}
