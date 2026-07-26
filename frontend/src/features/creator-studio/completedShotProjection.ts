import type { CreatorReviewCategory } from './creatorReviewArtifacts'

export type HistoricalShotLayer = 'IP A-roll' | '文字层' | '补充 / AIGC 层' | '旁白' | '输出成片'

export interface HistoricalShotArtifact {
  artifactId: string
  relatedShotId?: string
  reviewCategory: CreatorReviewCategory
  reviewLabel: string
  artifactType?: string
  kind?: string
}

export interface HistoricalShot {
  id: string
  sequenceIndex: number
  artifactIds: string[]
  layers: HistoricalShotLayer[]
}

const layerOrder: readonly HistoricalShotLayer[] = ['IP A-roll', '文字层', '补充 / AIGC 层', '旁白', '输出成片']

function historicalShotNumber(relatedShotId: string | undefined): number | undefined {
  const match = relatedShotId?.trim().match(/^shot[-_ ]?(\d{1,4})$/iu)
  if (!match) return undefined
  return Number(match[1])
}

function historicalLayer(artifact: HistoricalShotArtifact): HistoricalShotLayer {
  const semantics = `${artifact.artifactType ?? ''} ${artifact.kind ?? ''} ${artifact.reviewLabel}`.toLocaleLowerCase()
  if (/\bip[-_ ]?a?roll\b|talking[_ -]?head/.test(semantics)) return 'IP A-roll'
  if (/composited|composition|final[_ -]?output|\boutput\b|合成视频|成片/.test(semantics)) return '输出成片'
  if (artifact.reviewCategory === 'audio') return '旁白'
  if (/hyperframes|text|字幕|文字|prompt|提示词/.test(semantics) || artifact.reviewCategory === 'text') return '文字层'
  return '补充 / AIGC 层'
}

/**
 * Read-only creator-facing recovery for legacy completed projects that predate
 * durable Shot state. The raw IDs remain only for stable internal grouping and
 * are never intended for display.
 */
export function projectHistoricalShots(artifacts: readonly HistoricalShotArtifact[]): HistoricalShot[] {
  const groups = new Map<number, { artifactIds: string[]; layers: Set<HistoricalShotLayer> }>()
  for (const artifact of artifacts) {
    const sequenceIndex = historicalShotNumber(artifact.relatedShotId)
    if (sequenceIndex === undefined) continue
    const group = groups.get(sequenceIndex) ?? { artifactIds: [], layers: new Set<HistoricalShotLayer>() }
    group.artifactIds.push(artifact.artifactId)
    group.layers.add(historicalLayer(artifact))
    groups.set(sequenceIndex, group)
  }
  return [...groups.entries()]
    .sort(([left], [right]) => left - right)
    .map(([sequenceIndex, group]) => ({
      id: `historical-shot-${sequenceIndex}`,
      sequenceIndex,
      artifactIds: group.artifactIds,
      layers: layerOrder.filter(layer => group.layers.has(layer)),
    }))
}
