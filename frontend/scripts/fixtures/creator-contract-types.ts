import type {
  ArtifactSelection,
  ShotCandidate,
  ShotHistoryResponse,
  ShotListItem,
  ShotQAReport,
  ShotWorkspace,
  StepRevisionMutationRequest,
} from '../../src/utils/api-types.generated'

export const staleQAStatus: ShotQAReport['status'] = 'stale'
export const staleCandidateStatus: ShotCandidate['status'] = 'stale'
export const staleListGenerationStatus: ShotListItem['generationStatus'] = 'stale'
export const staleListQAStatus: ShotListItem['qaStatus'] = 'stale'
export const staleListReviewStatus: ShotListItem['reviewStatus'] = 'stale'
export const staleWorkspaceQAStatus: ShotWorkspace['shot']['qaStatus'] = 'stale'
export const staleWorkspaceCandidateStatus: NonNullable<ShotWorkspace['shot']['candidates']>[number]['status'] = 'stale'
export const staleHistoryQAStatus: ShotHistoryResponse['data']['history'][number]['snapshot']['qaStatus'] = 'stale'
export const staleHistoryCandidateStatus: NonNullable<
  ShotHistoryResponse['data']['history'][number]['snapshot']['candidates']
>[number]['status'] = 'stale'

const bothContents = {
  artifactId: 'artifact-1',
  baseVersion: 1,
  confirmedAffectedShotIds: [],
  directContent: 'direct',
  instruction: 'instruction',
  mode: 'direct' as const,
}

// @ts-expect-error a pre-bound value carrying both sibling contents is invalid
export const invalidMutation: StepRevisionMutationRequest = bothContents

const mixedRectAndTime = {
  kind: 'rect' as const,
  x: 0,
  y: 0,
  width: 100,
  height: 100,
  startMs: 0,
  endMs: 1000,
}

// @ts-expect-error closed rect selection cannot carry time-range fields
export const invalidMixedSelection: ArtifactSelection = mixedRectAndTime

export const nullableSelectionMutation: StepRevisionMutationRequest = {
  artifactId: 'artifact-1',
  baseVersion: 1,
  confirmedAffectedShotIds: [],
  directContent: 'direct',
  mode: 'direct',
  selection: null,
}
