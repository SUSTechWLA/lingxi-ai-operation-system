export type HistoricalShotMediaKind = 'image' | 'video' | 'audio'
export type HistoricalShotLayerKey = 'ip' | 'text' | 'enrichment'

export interface HistoricalShotArtifact {
  artifactId: string
  relatedShotId?: string
  artifactType?: string
  kind?: string
  mimeType?: string
  name?: string
  generationKind?: string
  isCurrent?: boolean
  isStale?: boolean
}

export interface HistoricalShot {
  id: string
  sequenceIndex: number
  artifactIds: string[]
  artifacts: HistoricalShotArtifact[]
}

export interface HistoricalShotLoadedArtifact {
  artifactId: string
  content: unknown
  reviewText?: string
  mediaUrl?: string
  mediaUrls?: string[]
}

export interface HistoricalShotReview {
  title: string
  durationSec?: number
  narration?: string
  screenText: string[]
  details: Array<{ label: string; value: string }>
  layers: Array<{ key: HistoricalShotLayerKey; label: string; summary?: string }>
  media: Array<{ artifactId: string; kind: HistoricalShotMediaKind; label: string; url: string }>
  hasReadableContent: boolean
}

const sharedShotContextKinds = new Set([
  'SHOT_LIST', 'VIDEO_PROMPTS', 'KEYFRAME_PROMPTS', 'IP_AROLL_PLAN', 'REFERENCE_ASSET_PLAN',
])
const maxProjectionDepth = 8
const maxArrayEntries = 96

function historicalShotNumber(relatedShotId: string | undefined): number | undefined {
  const match = relatedShotId?.trim().match(/^shot[-_ ]?(\d{1,4})$/iu)
  if (!match) return undefined
  return Number(match[1])
}

function currentArtifact(artifact: HistoricalShotArtifact): boolean {
  return artifact.isCurrent !== false && artifact.isStale !== true
}

/**
 * Read-only creator-facing recovery for completed projects that predate durable
 * Shot state. It retains current per-Shot artifacts plus the shared planning
 * documents needed to reconstruct a readable review dossier.
 */
export function projectHistoricalShots(artifacts: readonly HistoricalShotArtifact[]): HistoricalShot[] {
  const current = artifacts.filter(currentArtifact)
  const shared = current.filter(artifact => sharedShotContextKinds.has((artifact.kind ?? '').trim().toLocaleUpperCase()))
  const groups = new Map<number, HistoricalShotArtifact[]>()
  for (const artifact of current) {
    const sequenceIndex = historicalShotNumber(artifact.relatedShotId)
    if (sequenceIndex === undefined) continue
    const group = groups.get(sequenceIndex) ?? []
    group.push(artifact)
    groups.set(sequenceIndex, group)
  }
  return [...groups.entries()]
    .sort(([left], [right]) => left - right)
    .map(([sequenceIndex, direct]) => {
      const grouped = uniqueArtifacts([...direct, ...shared])
      return {
        id: `historical-shot-${sequenceIndex}`,
        sequenceIndex,
        artifactIds: grouped.map(artifact => artifact.artifactId),
        artifacts: grouped,
      }
    })
}

export function projectHistoricalShotReview(
  shot: HistoricalShot,
  loadedArtifacts: readonly HistoricalShotLoadedArtifact[],
): HistoricalShotReview {
  const sourceRecords: Record<string, unknown>[] = []
  const referenceURLs: string[] = []
  const descriptorByID = new Map(shot.artifacts.map(artifact => [artifact.artifactId, artifact]))

  for (const loaded of loadedArtifacts) {
    const descriptor = descriptorByID.get(loaded.artifactId)
    if (!descriptor) continue
    const values = decodedArtifactValues(loaded)
    const direct = historicalShotNumber(descriptor.relatedShotId) === shot.sequenceIndex
    for (const value of values) {
      if (direct && isRecord(value)) sourceRecords.push(value)
      collectMatchingShotRecords(value, shot.sequenceIndex, sourceRecords, 0, new Set<object>())
    }
  }

  for (const record of sourceRecords) {
    referenceURLs.push(...stringsForNamedValues(record, ['referenceImages', 'referenceImageUrls', 'referenceFrames']))
  }

  const title = compactTitle(firstReadableString(sourceRecords, ['title', 'shotTitle', 'name', 'dramaticPurpose', 'whyThisShot', 'mainAction', 'visual'])) || `Shot ${shot.sequenceIndex}`
  const narration = firstReadableString(sourceRecords, ['narrationText', 'narration', 'voiceoverText', 'sourceScriptSegment', 'spokenText'])
  const durationSec = firstFiniteNumber(sourceRecords, ['durationSec', 'durationSeconds', 'duration'])
  const screenText = uniqueStrings(sourceRecords.flatMap(record => stringsForNamedValues(record, ['screenText', 'onScreenText', 'textOverlays', 'captions'])))
  const details = compactDetails([
    ['画面与动作', firstReadableString(sourceRecords, ['mainAction', 'visual', 'visualDescription', 'sceneSummary', 'action'])],
    ['镜头', firstReadableString(sourceRecords, ['camera', 'cameraDirection', 'cameraMovement', 'framing'])],
    ['构图', firstReadableString(sourceRecords, ['composition', 'layout'])],
    ['灯光', firstReadableString(sourceRecords, ['lighting', 'lightDesign'])],
    ['场景', firstReadableString(sourceRecords, ['scene', 'location', 'environment'])],
    ['衔接', firstReadableString(sourceRecords, ['transition', 'finalCompositing', 'fusionPlan'])],
  ])
  const layers: HistoricalShotReview['layers'] = [
    { key: 'ip', label: 'IP A-roll', summary: namedSectionSummary(sourceRecords, ['ipArollPlan', 'ipAroll', 'ipArollLayer', 'characterLayer']) },
    { key: 'text', label: '文字层', summary: namedSectionSummary(sourceRecords, ['hyperframesPlan', 'hyperFrames', 'hyperframes', 'textLayer']) },
    { key: 'enrichment', label: '补充画面', summary: namedSectionSummary(sourceRecords, ['aigcPlan', 'aigcLayer', 'enrichmentLayer', 'brollPlan']) },
  ]
  const media = projectHistoricalMedia(shot, loadedArtifacts, referenceURLs)
  return {
    title,
    ...(durationSec !== undefined ? { durationSec } : {}),
    ...(narration ? { narration } : {}),
    screenText,
    details,
    layers,
    media,
    hasReadableContent: Boolean(narration || details.length || layers.some(layer => layer.summary) || screenText.length || media.length),
  }
}

function decodedArtifactValues(artifact: HistoricalShotLoadedArtifact): unknown[] {
  const values = [decodedValue(artifact.content)]
  if (artifact.reviewText) {
    const reviewValue = decodedValue(artifact.reviewText)
    if (reviewValue !== artifact.content) values.push(reviewValue)
  }
  return values
}

function decodedValue(value: unknown): unknown {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!/^[{[]/.test(trimmed)) return value
  try {
    return JSON.parse(trimmed)
  } catch {
    return value
  }
}

function collectMatchingShotRecords(
  value: unknown,
  sequenceIndex: number,
  output: Record<string, unknown>[],
  depth: number,
  seen: Set<object>,
): void {
  if (depth > maxProjectionDepth || !value || typeof value !== 'object' || seen.has(value)) return
  seen.add(value)
  if (Array.isArray(value)) {
    for (const item of value.slice(0, maxArrayEntries)) collectMatchingShotRecords(item, sequenceIndex, output, depth + 1, seen)
    return
  }
  const record = value as Record<string, unknown>
  const identity = ['shotId', 'relatedShotId', 'id', 'shotID']
    .map(key => typeof record[key] === 'string' ? historicalShotNumber(record[key] as string) : undefined)
    .find(candidate => candidate !== undefined)
  if (identity === sequenceIndex) output.push(record)
  for (const child of Object.values(record)) collectMatchingShotRecords(decodedValue(child), sequenceIndex, output, depth + 1, seen)
}

function firstReadableString(records: readonly Record<string, unknown>[], keys: readonly string[]): string | undefined {
  for (const record of records) {
    for (const key of keys) {
      const values = stringsForNamedValues(record, [key])
      if (values.length) return values[0]
    }
  }
  return undefined
}

function firstFiniteNumber(records: readonly Record<string, unknown>[], keys: readonly string[]): number | undefined {
  for (const record of records) {
    const value = findNamedValue(record, new Set(keys), 0, new Set<object>())
    const numeric = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : Number.NaN
    if (Number.isFinite(numeric) && numeric >= 0) return numeric
  }
  return undefined
}

function stringsForNamedValues(record: Record<string, unknown>, keys: readonly string[]): string[] {
  const keySet = new Set(keys.map(key => key.toLocaleLowerCase()))
  const values: unknown[] = []
  collectNamedValues(record, keySet, values, 0, new Set<object>())
  return uniqueStrings(values.flatMap(readableStrings))
}

function collectNamedValues(value: unknown, keys: ReadonlySet<string>, output: unknown[], depth: number, seen: Set<object>): void {
  if (depth > maxProjectionDepth || !value || typeof value !== 'object' || seen.has(value)) return
  seen.add(value)
  if (Array.isArray(value)) {
    for (const item of value.slice(0, maxArrayEntries)) collectNamedValues(item, keys, output, depth + 1, seen)
    return
  }
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
    if (keys.has(key.toLocaleLowerCase())) output.push(child)
    collectNamedValues(decodedValue(child), keys, output, depth + 1, seen)
  }
}

function findNamedValue(value: unknown, keys: ReadonlySet<string>, depth: number, seen: Set<object>): unknown {
  if (depth > maxProjectionDepth || !value || typeof value !== 'object' || seen.has(value)) return undefined
  seen.add(value)
  if (Array.isArray(value)) {
    for (const item of value.slice(0, maxArrayEntries)) {
      const found = findNamedValue(item, keys, depth + 1, seen)
      if (found !== undefined) return found
    }
    return undefined
  }
  const record = value as Record<string, unknown>
  for (const [key, child] of Object.entries(record)) if (keys.has(key)) return child
  for (const child of Object.values(record)) {
    const found = findNamedValue(decodedValue(child), keys, depth + 1, seen)
    if (found !== undefined) return found
  }
  return undefined
}

function namedSectionSummary(records: readonly Record<string, unknown>[], keys: readonly string[]): string | undefined {
  const keySet = new Set(keys.map(key => key.toLocaleLowerCase()))
  const values: unknown[] = []
  for (const record of records) collectNamedValues(record, keySet, values, 0, new Set<object>())
  const summaries = uniqueStrings(values.flatMap(value => {
    if (typeof value === 'string') return readableStrings(value)
    if (!isRecord(value)) return []
    return ['designSummary', 'description', 'summary', 'prompt', 'role', 'plan']
      .flatMap(key => readableStrings(value[key]))
  }))
  return summaries.slice(0, 2).join(' ')
}

function readableStrings(value: unknown): string[] {
  if (typeof value === 'string') {
    const text = value.trim()
    if (!text || /^[{[]/.test(text)) return []
    return [text]
  }
  if (Array.isArray(value)) return value.slice(0, maxArrayEntries).flatMap(readableStrings)
  if (isRecord(value)) {
    return ['text', 'label', 'title', 'content', 'value', 'url', 'imageUrl', 'mediaUrl', 'src']
      .flatMap(key => readableStrings(value[key]))
  }
  return []
}

function projectHistoricalMedia(
  shot: HistoricalShot,
  loadedArtifacts: readonly HistoricalShotLoadedArtifact[],
  referenceURLs: readonly string[],
): HistoricalShotReview['media'] {
  const descriptorByID = new Map(shot.artifacts.map(artifact => [artifact.artifactId, artifact]))
  const result: HistoricalShotReview['media'] = []
  const seen = new Set<string>()
  const append = (artifactId: string, kind: HistoricalShotMediaKind, label: string, url: string) => {
    const trimmed = url.trim()
    if (!previewableURL(trimmed) || seen.has(trimmed)) return
    seen.add(trimmed)
    result.push({ artifactId, kind, label, url: trimmed })
  }
  for (const loaded of loadedArtifacts) {
    const descriptor = descriptorByID.get(loaded.artifactId)
    if (!descriptor || historicalShotNumber(descriptor.relatedShotId) !== shot.sequenceIndex) continue
    const kind = historicalMediaKind(descriptor)
    if (!kind) continue
    const urls = uniqueStrings([loaded.mediaUrl ?? '', ...(loaded.mediaUrls ?? [])])
    for (const url of urls) append(loaded.artifactId, kind, historicalMediaLabel(descriptor, kind), url)
  }
  for (const url of uniqueStrings(referenceURLs)) append(`historical-reference-${shot.sequenceIndex}`, 'image', '参考图', url)
  const order: Record<HistoricalShotMediaKind, number> = { video: 0, image: 1, audio: 2 }
  return result.sort((left, right) => order[left.kind] - order[right.kind])
}

function historicalMediaKind(artifact: HistoricalShotArtifact): HistoricalShotMediaKind | undefined {
  const semantics = `${artifact.artifactType ?? ''} ${artifact.kind ?? ''} ${artifact.mimeType ?? ''} ${artifact.name ?? ''}`.toLocaleLowerCase()
  if (/video|hyperframes|composited/.test(semantics)) return 'video'
  if (/image|keyframe|reference|\.png|\.jpe?g|\.webp/.test(semantics)) return 'image'
  if (/audio|narration|voice|\.wav|\.mp3|\.m4a/.test(semantics)) return 'audio'
  return undefined
}

function historicalMediaLabel(artifact: HistoricalShotArtifact, kind: HistoricalShotMediaKind): string {
  const semantics = `${artifact.artifactType ?? ''} ${artifact.kind ?? ''}`.toLocaleLowerCase()
  if (/composited/.test(semantics)) return '完整 Shot'
  if (/hyperframes/.test(semantics)) return '文字层预览'
  if (kind === 'image') return '参考图'
  if (kind === 'audio') return '旁白'
  return '补充画面'
}

function previewableURL(value: string): boolean {
  return /^(?:https?:|blob:|data:image\/|\/)/iu.test(value)
}

function compactDetails(items: ReadonlyArray<readonly [string, string | undefined]>): Array<{ label: string; value: string }> {
  const seen = new Set<string>()
  return items.flatMap(([label, value]) => {
    const text = value?.trim()
    if (!text || seen.has(text)) return []
    seen.add(text)
    return [{ label, value: text }]
  })
}

function compactTitle(value: string | undefined): string | undefined {
  const text = value?.replace(/\s+/g, ' ').trim()
  if (!text) return undefined
  return text.length > 42 ? `${text.slice(0, 42)}…` : text
}

function uniqueArtifacts(artifacts: readonly HistoricalShotArtifact[]): HistoricalShotArtifact[] {
  const seen = new Set<string>()
  return artifacts.filter(artifact => {
    if (!artifact.artifactId || seen.has(artifact.artifactId)) return false
    seen.add(artifact.artifactId)
    return true
  })
}

function uniqueStrings(values: readonly string[]): string[] {
  const seen = new Set<string>()
  return values.flatMap(value => {
    const text = value.trim()
    if (!text || seen.has(text)) return []
    seen.add(text)
    return [text]
  })
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
