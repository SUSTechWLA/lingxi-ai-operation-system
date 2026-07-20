import type {
  Artifact as GeneratedArtifact,
  ArtifactSelection as GeneratedArtifactSelection,
  CandidateAcceptRequest as GeneratedCandidateAcceptRequest,
  CandidateRestoreRequest as GeneratedCandidateRestoreRequest,
  CreationView as GeneratedCreationView,
  CreatorArtifactVersion as GeneratedCreatorArtifactVersion,
  CreatorStep as GeneratedCreatorStep,
  CreatorTask as GeneratedCreatorTask,
  ProjectMaterial as GeneratedProjectMaterial,
  ProjectMaterialManifest as GeneratedProjectMaterialManifest,
  ProjectMaterialResponse,
  ShotHistoryResponse,
  ShotImpact as GeneratedShotImpact,
  ShotListItem as GeneratedShotListItem,
  ShotPageResponse,
  ShotRegenerationRequest as GeneratedShotRegenerationRequest,
  ShotRegenerationResponse,
  ShotSummary as GeneratedShotSummary,
  ShotWorkspace as GeneratedShotWorkspace,
  StepConfirmRequest as GeneratedStepConfirmRequest,
  StepImpact as GeneratedStepImpact,
  StepMutationResponse,
  StepRestoreRequest as GeneratedStepRestoreRequest,
  StepRevisionMutationRequest as GeneratedStepRevisionMutationRequest,
  StepRevisionPreviewRequest as GeneratedStepRevisionPreviewRequest,
} from '../../utils/api-types.generated'

export type CreatorStepId = GeneratedCreatorStep['id']
export type CreatorStepState = GeneratedCreatorStep['state']
export type CreatorStep = GeneratedCreatorStep
export type CreatorTask = GeneratedCreatorTask
export type CreatorArtifactVersion = GeneratedCreatorArtifactVersion
export type ShotSummary = GeneratedShotSummary
export type CreatorProject = GeneratedCreationView['project']
export type CreationView = GeneratedCreationView
export type Artifact = GeneratedArtifact

export type CreatorAction =
  | { kind: 'start'; stepId: 'requirements'; label: string }
  | { kind: 'review'; stepId: CreatorStepId; label: string }
  | { kind: 'wait'; stepId: CreatorStepId; label: string }
  | { kind: 'fix'; stepId: CreatorStepId; label: string }
  | { kind: 'continue'; stepId: CreatorStepId; label: string }

export type ShotReviewStatus = GeneratedShotListItem['reviewStatus']
export type ShotGenerationStatus = GeneratedShotListItem['generationStatus']
export type ShotListItem = GeneratedShotListItem
export type ShotQueueStatus = 'all' | 'needs_attention' | 'confirmed' | 'generating' | 'failed' | ShotReviewStatus

export interface ShotListQuery {
  cursor?: string
  limit?: number
  status?: ShotQueueStatus
  chapter?: string
  query?: string
}

export interface ShotListFilters {
  status?: ShotQueueStatus
  chapter?: string
  query?: string
}

export type ShotPage = ShotPageResponse['data']
export type ShotUnit = ShotRegenerationResponse['data']['shot']
export type ShotRevision = ShotHistoryResponse['data']['history'][number]
export type ShotWorkspace = GeneratedShotWorkspace
export type ShotImpact = GeneratedShotImpact
export type ShotRegenerationResult = ShotRegenerationResponse['data']

export type ShotRegenerationRequest = GeneratedShotRegenerationRequest
export type ShotRegenerationScope = GeneratedShotRegenerationRequest['scope']
export type ShotLock = GeneratedShotRegenerationRequest['locks'][number]
export type CandidateAcceptRequest = GeneratedCandidateAcceptRequest
export type CandidateRestoreRequest = GeneratedCandidateRestoreRequest

export type ArtifactSelection = GeneratedArtifactSelection
export type StepRevisionPreviewRequest = GeneratedStepRevisionPreviewRequest
export type StepRevisionMutationRequest = GeneratedStepRevisionMutationRequest
export type StepRestoreRequest = GeneratedStepRestoreRequest
export type StepConfirmRequest = GeneratedStepConfirmRequest
export type StepImpact = GeneratedStepImpact
export type StepMutationResult = StepMutationResponse['data']

export type ProjectMaterialKind = GeneratedProjectMaterial['kind']
export type ProjectMaterial = GeneratedProjectMaterial
export type ProjectMaterialManifest = GeneratedProjectMaterialManifest

export interface ProjectMaterialManifestMetadata extends ProjectMaterialManifest {
  artifactType: 'project_source_material_manifest'
  manifestHash: string
  status: string
  humanApproved: boolean
  localOnly: true
}

export type ProjectMaterialManifestArtifact = Artifact
export type RegisterProjectMaterialResult = ProjectMaterialResponse['data']
