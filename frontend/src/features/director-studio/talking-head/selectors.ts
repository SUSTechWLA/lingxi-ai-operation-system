import type {
  TalkingHeadArtifactLike,
  TalkingHeadLayerDisplay,
  TalkingHeadLayerKey,
  TalkingHeadLayerStatus,
} from './types'

const layerLabels: Record<TalkingHeadLayerKey, string> = {
  audio: 'Audio Master',
  ip: 'IP',
  text: 'Text / Graphics',
  broll: 'B-roll',
  composition: 'Composition',
}

const kindLayerAliases: Record<TalkingHeadLayerKey, string[]> = {
  audio: ['AUDIO_MASTER_TIMELINE', 'AUDIO', 'VOICEOVER'],
  ip: ['IP_LAYER', 'IP_VIDEO', 'TALKING_HEAD_VIDEO'],
  text: ['TEXT_LAYER', 'HYPERFRAMES_PROJECT', 'HTML_OVERLAY', 'SUBTITLE'],
  broll: ['BROLL_MANIFEST', 'BROLL_LAYER', 'AIGC_VIDEO', 'AIGC_IMAGE'],
  composition: ['COMPOSITED_SHOT_VIDEO', 'VIDEO_COMPOSITION_SPEC', 'SHOT_VIDEO'],
}

export function buildTalkingHeadLayerDisplays(artifacts: TalkingHeadArtifactLike[]): TalkingHeadLayerDisplay[] {
  return (Object.keys(layerLabels) as TalkingHeadLayerKey[]).map((key) => {
    const artifact = latestLayerArtifact(artifacts, key)
    const metadata = artifact?.metadata || {}
    return {
      key,
      label: layerLabels[key],
      status: layerStatus(artifact?.status, metadata),
      revision: stringValue(metadata.revision) || stringValue(metadata.timelineRevision),
      staleReason: stringValue(metadata.staleReason),
      provider: stringValue(metadata.provider) || stringValue(metadata.providerName),
      model: stringValue(metadata.model),
      executionMode: stringValue(metadata.executionMode),
      productionEligible: booleanValue(metadata.productionEligible),
      artifactId: artifact?.id,
    }
  })
}

function latestLayerArtifact(artifacts: TalkingHeadArtifactLike[], layer: TalkingHeadLayerKey): TalkingHeadArtifactLike | undefined {
  return [...artifacts].reverse().find((artifact) => {
    const metadataLayer = stringValue(artifact.metadata?.layer).toLowerCase()
    if (metadataLayer === layer) return true
    return kindLayerAliases[layer].includes(String(artifact.kind || '').toUpperCase())
  })
}

function layerStatus(rawStatus: string | undefined, metadata: Record<string, unknown>): TalkingHeadLayerStatus {
  const status = (stringValue(metadata.status) || String(rawStatus || '')).toLowerCase()
  if (status === 'stale') return 'stale'
  if (status === 'failed' || status === 'blocked') return 'failed'
  if (status === 'valid' || status === 'approved' || status === 'current') return 'current'
  if (status === 'planned') return 'planned'
  return 'pending'
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function booleanValue(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
}
