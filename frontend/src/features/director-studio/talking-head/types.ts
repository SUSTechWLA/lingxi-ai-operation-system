export type TalkingHeadLayerKey = 'audio' | 'ip' | 'text' | 'broll' | 'composition'

export type TalkingHeadLayerStatus = 'current' | 'planned' | 'stale' | 'pending' | 'failed'

export interface TalkingHeadArtifactLike {
  id: string
  kind: string
  status?: string
  stageName?: string
  metadata?: Record<string, unknown>
}

export interface TalkingHeadLayerDisplay {
  key: TalkingHeadLayerKey
  label: string
  status: TalkingHeadLayerStatus
  revision?: string
  staleReason?: string
  provider?: string
  model?: string
  executionMode?: string
  productionEligible?: boolean
  artifactId?: string
}
