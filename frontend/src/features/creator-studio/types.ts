import type {
  Artifact as GeneratedArtifact,
  CreationView as GeneratedCreationView,
  CreatorArtifactVersion as GeneratedCreatorArtifactVersion,
  CreatorStep as GeneratedCreatorStep,
  CreatorTask as GeneratedCreatorTask,
  ProjectMaterialManifest as GeneratedProjectMaterialManifest,
  ShotHistoryResponse,
  ShotImpact as GeneratedShotImpact,
  ShotListItem as GeneratedShotListItem,
  ShotRegenerationResponse,
  ShotSummary as GeneratedShotSummary,
  ShotWorkspace as GeneratedShotWorkspace,
} from '../../utils/api-types.generated'

export type CreatorStepId =
  | 'requirements'
  | 'direction'
  | 'script'
  | 'shots'
  | 'preview'
  | 'delivery'

export type CreatorStepState =
  | 'not_started'
  | 'generating'
  | 'needs_review'
  | 'confirmed'
  | 'needs_attention'
  | 'failed'

export type CreatorStep = GeneratedCreatorStep
export type CreatorTask = GeneratedCreatorTask
export type CreatorArtifactVersion = GeneratedCreatorArtifactVersion
export type ShotSummary = GeneratedShotSummary
export type CreatorProject = Omit<GeneratedCreationView['project'], 'config'> & {
  config?: Record<string, unknown>
}
export type CreationView = Omit<GeneratedCreationView, 'project'> & { project: CreatorProject }
export type Artifact = Omit<GeneratedArtifact, 'metadata'> & { metadata?: Record<string, unknown> }

export type CreatorAction =
  | { kind: 'start'; stepId: 'requirements'; label: string }
  | { kind: 'review'; stepId: CreatorStepId; label: string }
  | { kind: 'wait'; stepId: CreatorStepId; label: string }
  | { kind: 'fix'; stepId: CreatorStepId; label: string }
  | { kind: 'continue'; stepId: CreatorStepId; label: string }

export type ShotReviewStatus = 'pending' | 'approved' | 'rejected' | 'stale'
export type ShotGenerationStatus =
  | 'PLANNED'
  | 'GENERATING'
  | 'CANDIDATE_RENDERED'
  | 'SHOT_QA_RUNNING'
  | 'SHOT_QA_PASSED'
  | 'SHOT_QA_FAILED'
  | 'HUMAN_REVIEW_REQUIRED'
  | 'ACCEPTED_FOR_ASSEMBLY'
  | 'queued'
  | 'dispatching'
  | 'running'
  | 'failed'
  | 'cancelled'

export type ShotListItem = Omit<GeneratedShotListItem, 'reviewStatus' | 'generationStatus'> & {
  reviewStatus: ShotReviewStatus
  generationStatus: ShotGenerationStatus
}

export interface ShotListQuery {
  cursor?: string
  limit?: number
  status?: ShotReviewStatus
  chapter?: string
  query?: string
}

export interface ShotListFilters {
  status?: ShotReviewStatus
  chapter?: string
  query?: string
}

export interface ShotPage {
  shots: ShotListItem[]
  nextCursor: string
  total: number
}

export type ShotUnit = ShotRegenerationResponse['data']['shot']
export type ShotRevision = ShotHistoryResponse['data']['history'][number]
export type ShotWorkspace = Omit<GeneratedShotWorkspace, 'impact'> & { impact: ShotImpact }
export type ShotImpact = GeneratedShotImpact
export type ShotRegenerationResult = ShotRegenerationResponse['data']

export type ShotRegenerationScope =
  | 'prompt'
  | 'reference'
  | 'base_media'
  | 'overlay'
  | 'audio_alignment'
  | 'full_shot'

export type ShotLock =
  | 'duration'
  | 'narration'
  | 'character'
  | 'wardrobe'
  | 'scene'
  | 'camera'
  | 'first_frame'
  | 'last_frame'
  | 'reference_set'
  | 'accepted_overlay'

export interface ShotRegenerationRequest {
  baseVersion: number
  scope: ShotRegenerationScope
  locks: ShotLock[]
  instruction?: string
}

export interface CandidateAcceptRequest {
  baseVersion: number
  scope: 'candidate_accept'
  locks: ShotLock[]
}

export interface CandidateRestoreRequest {
  baseVersion: number
  scope: 'candidate_restore'
  locks: ShotLock[]
}

export type ArtifactSelection =
  | { kind: 'rect'; x: number; y: number; width: number; height: number }
  | { kind: 'time'; startMs: number; endMs: number }

interface StepRevisionBase {
  artifactId: string
  baseVersion: number
  runId?: string
  reviewId?: string
  selection?: ArtifactSelection
}

export type StepRevisionPreviewRequest = StepRevisionBase & (
  | { mode: 'direct'; directContent: string; instruction?: never }
  | { mode: 'instruction'; instruction: string; directContent?: never }
)

export type StepRevisionRequest = StepRevisionPreviewRequest & {
  confirmedAffectedShotIds: string[]
}

export interface StepRestoreRequest {
  baseVersion: number
  runId?: string
  reviewId?: string
  reason?: string
  confirmedAffectedShotIds: string[]
}

export interface StepConfirmRequest {
  artifactId: string
  runId?: string
  reviewId?: string
  comment?: string
}

export interface StepImpact {
  affectedStepIds: CreatorStepId[]
  affectedShotIds?: string[]
  requiresConfirmation: boolean
}

export interface StepMutationResult {
  artifact: Artifact
  impact: StepImpact
  view: CreationView
}

export type ProjectMaterialKind = 'image' | 'audio' | 'video' | 'document'

export interface ProjectMaterial {
  name: string
  kind: ProjectMaterialKind
  storageRef: string
  mimeType: string
  sizeBytes: number
  contentHash: string
}

export type ProjectMaterialManifest = Omit<GeneratedProjectMaterialManifest, 'materials'> & {
  materials: ProjectMaterial[]
}

export interface ProjectMaterialManifestMetadata extends ProjectMaterialManifest {
  artifactType: 'project_source_material_manifest'
  manifestHash: string
  status: string
  humanApproved: boolean
  localOnly: true
}

export type ProjectMaterialManifestArtifact = Omit<Artifact, 'metadata'> & {
  metadata?: ProjectMaterialManifestMetadata
}

export interface RegisterProjectMaterialResult {
  material: ProjectMaterial
  artifact: ProjectMaterialManifestArtifact
}
