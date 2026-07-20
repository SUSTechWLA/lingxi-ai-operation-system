import type { ApiResponse, ArtifactContentResponse } from '../utils/types'
import type {
  CandidateAcceptRequest,
  CandidateRestoreRequest,
  CreationView,
  CreatorArtifactVersion,
  CreatorStepId,
  ProjectMaterial,
  RegisterProjectMaterialResult,
  ShotImpact,
  ShotListQuery,
  ShotPage,
  ShotRegenerationRequest,
  ShotRegenerationResult,
  ShotRevision,
  ShotSummary,
  ShotUnit,
  ShotWorkspace,
  StepConfirmRequest,
  StepImpact,
  StepMutationResult,
  StepRestoreRequest,
  StepRevisionPreviewRequest,
  StepRevisionMutationRequest,
} from '../features/creator-studio/types'
import { api } from './api'

function creatorPath(projectId: string, suffix = ''): string {
  return `/video-projects/${encodeURIComponent(projectId)}${suffix}`
}

function assertPositiveVersion(version: number): void {
  if (!Number.isInteger(version) || version <= 0) {
    throw new TypeError('baseVersion must be a positive integer')
  }
}

function assertIdempotencyKey(idempotencyKey: string): void {
  if (!idempotencyKey.trim()) throw new TypeError('idempotencyKey is required')
}

export async function getCreationView(projectId: string, signal?: AbortSignal): Promise<CreationView> {
  const response = await api.get<ApiResponse<CreationView>>(creatorPath(projectId, '/creation-view'), { signal })
  return response.data.data
}

export async function getCreatorArtifactContent(artifactId: string, signal?: AbortSignal): Promise<ArtifactContentResponse> {
  const response = await api.get<ApiResponse<ArtifactContentResponse>>(
    `/artifacts/${encodeURIComponent(artifactId)}/content`, { signal },
  )
  return response.data.data
}

export async function getStepVersions(
  projectId: string,
  stepId: CreatorStepId,
  signal?: AbortSignal,
): Promise<CreatorArtifactVersion[]> {
  const response = await api.get<ApiResponse<{ versions: CreatorArtifactVersion[] }>>(
    creatorPath(projectId, `/steps/${encodeURIComponent(stepId)}/versions`), { signal },
  )
  return response.data.data.versions
}

export async function previewStepRevision(
  projectId: string,
  stepId: CreatorStepId,
  request: StepRevisionPreviewRequest,
  signal?: AbortSignal,
): Promise<StepImpact> {
  assertPositiveVersion(request.baseVersion)
  const response = await api.post<ApiResponse<StepImpact>>(
    creatorPath(projectId, `/steps/${encodeURIComponent(stepId)}/revision-impact`), request, { signal },
  )
  return response.data.data
}

export async function reviseStep(
  projectId: string,
  stepId: CreatorStepId,
  request: StepRevisionMutationRequest,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<StepMutationResult> {
  assertPositiveVersion(request.baseVersion)
  assertIdempotencyKey(idempotencyKey)
  const response = await api.post<ApiResponse<StepMutationResult>>(
    creatorPath(projectId, `/steps/${encodeURIComponent(stepId)}/revisions`), request,
    { headers: { 'Idempotency-Key': idempotencyKey }, signal },
  )
  return response.data.data
}

export async function confirmStep(
  projectId: string,
  stepId: CreatorStepId,
  request: StepConfirmRequest,
  signal?: AbortSignal,
): Promise<CreationView> {
  const response = await api.post<ApiResponse<CreationView>>(
    creatorPath(projectId, `/steps/${encodeURIComponent(stepId)}/confirm`), request, { signal },
  )
  return response.data.data
}

export async function restoreStepVersion(
  projectId: string,
  stepId: CreatorStepId,
  version: number,
  request: StepRestoreRequest,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<StepMutationResult> {
  assertPositiveVersion(version)
  assertPositiveVersion(request.baseVersion)
  assertIdempotencyKey(idempotencyKey)
  const response = await api.post<ApiResponse<StepMutationResult>>(
    creatorPath(projectId, `/steps/${encodeURIComponent(stepId)}/versions/${version}/restore`), request,
    { headers: { 'Idempotency-Key': idempotencyKey }, signal },
  )
  return response.data.data
}

export async function registerProjectMaterial(
  projectId: string,
  material: ProjectMaterial,
  signal?: AbortSignal,
): Promise<RegisterProjectMaterialResult> {
  const response = await api.post<ApiResponse<RegisterProjectMaterialResult>>(
    creatorPath(projectId, '/materials'), material, { signal },
  )
  return response.data.data
}

export async function listShots(
  projectId: string,
  query: ShotListQuery = {},
  signal?: AbortSignal,
): Promise<ShotPage> {
  const response = await api.get<ApiResponse<ShotPage>>(creatorPath(projectId, '/shots'), { params: query, signal })
  return response.data.data
}

export async function getShotSummary(projectId: string, signal?: AbortSignal): Promise<ShotSummary> {
  const response = await api.get<ApiResponse<{ summary: ShotSummary }>>(creatorPath(projectId, '/shots/summary'), { signal })
  return response.data.data.summary
}

export async function getShotWorkspace(
  projectId: string,
  shotId: string,
  signal?: AbortSignal,
): Promise<ShotWorkspace> {
  const response = await api.get<ApiResponse<{ workspace: ShotWorkspace }>>(
    creatorPath(projectId, `/shots/${encodeURIComponent(shotId)}/workspace`), { signal },
  )
  return response.data.data.workspace
}

export async function getShotHistory(
  projectId: string,
  shotId: string,
  signal?: AbortSignal,
): Promise<ShotRevision[]> {
  const response = await api.get<ApiResponse<{ history: ShotRevision[] }>>(
    creatorPath(projectId, `/shots/${encodeURIComponent(shotId)}/history`), { signal },
  )
  return response.data.data.history
}

export async function previewShotRegeneration(
  projectId: string,
  shotId: string,
  signal?: AbortSignal,
): Promise<ShotImpact> {
  const response = await api.post<ApiResponse<{ impact: ShotImpact }>>(
    creatorPath(projectId, `/shots/${encodeURIComponent(shotId)}/regeneration-impact`), undefined, { signal },
  )
  return response.data.data.impact
}

export async function regenerateShot(
  projectId: string,
  shotId: string,
  request: ShotRegenerationRequest,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<ShotRegenerationResult> {
  assertPositiveVersion(request.baseVersion)
  assertIdempotencyKey(idempotencyKey)
  const response = await api.post<ApiResponse<ShotRegenerationResult>>(
    creatorPath(projectId, `/shots/${encodeURIComponent(shotId)}/regenerations`), request,
    { headers: { 'Idempotency-Key': idempotencyKey }, signal },
  )
  return response.data.data
}

export async function acceptShotCandidate(
  projectId: string,
  shotId: string,
  candidateId: string,
  request: CandidateAcceptRequest,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<ShotUnit> {
  assertPositiveVersion(request.baseVersion)
  assertIdempotencyKey(idempotencyKey)
  const response = await api.post<ApiResponse<{ shot: ShotUnit }>>(
    creatorPath(projectId, `/shots/${encodeURIComponent(shotId)}/candidates/${encodeURIComponent(candidateId)}/accept`),
    request, { headers: { 'Idempotency-Key': idempotencyKey }, signal },
  )
  return response.data.data.shot
}

export async function restoreShotCandidate(
  projectId: string,
  shotId: string,
  candidateId: string,
  request: CandidateRestoreRequest,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<ShotUnit> {
  assertPositiveVersion(request.baseVersion)
  assertIdempotencyKey(idempotencyKey)
  const response = await api.post<ApiResponse<{ shot: ShotUnit }>>(
    creatorPath(projectId, `/shots/${encodeURIComponent(shotId)}/candidates/${encodeURIComponent(candidateId)}/restore`),
    request, { headers: { 'Idempotency-Key': idempotencyKey }, signal },
  )
  return response.data.data.shot
}
