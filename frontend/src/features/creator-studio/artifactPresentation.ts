export type ArtifactPresentationKind = 'json' | 'markdown' | 'image' | 'video' | 'audio' | 'text' | 'file'

export interface ArtifactPresentationMetadata {
  kind?: string
  mimeType?: string
  name?: string
}

export interface JsonSummary {
  rootType: 'object' | 'array' | 'string' | 'number' | 'boolean' | 'null'
  itemCount: number
  depth: number
}

export interface ArtifactReviewMetric {
  label: string
  value: string
}

export interface ArtifactReviewCharacter {
  name: string
  description?: string
}

export interface ArtifactReviewSection {
  name: string
  duration?: string
  text?: string
}

export interface ArtifactReviewModel {
  kind: 'script'
  title: string
  summary?: string
  script: string
  metrics: ArtifactReviewMetric[]
  characters: ArtifactReviewCharacter[]
  sections: ArtifactReviewSection[]
  warnings: string[]
}

export type ParsedArtifactJson =
  | { ok: true; value: unknown; raw: string }
  | { ok: false; error: string; raw: string }

export interface MarkdownHeading {
  depth: number
  text: string
  id: string
}

export function classifyArtifactPresentation(metadata: ArtifactPresentationMetadata): ArtifactPresentationKind {
  const mimeType = (metadata.mimeType || '').trim().toLowerCase().split(';', 1)[0]
  const kind = (metadata.kind || '').trim().toUpperCase()
  const name = (metadata.name || '').trim().toLowerCase()
  if (mimeType === 'application/json' || mimeType.endsWith('+json') || kind === 'JSON' || name.endsWith('.json')) return 'json'
  if (mimeType === 'text/markdown' || kind === 'MARKDOWN' || name.endsWith('.md') || name.endsWith('.markdown')) return 'markdown'
  if (mimeType.startsWith('image/') || kind === 'IMAGE') return 'image'
  if (mimeType.startsWith('video/') || kind === 'VIDEO') return 'video'
  if (mimeType.startsWith('audio/') || kind === 'AUDIO') return 'audio'
  if (mimeType.startsWith('text/') || kind === 'LOG') return 'text'
  return 'file'
}

export function parseArtifactJson(content: unknown): ParsedArtifactJson {
  if (typeof content !== 'string') {
    return { ok: true, value: content, raw: JSON.stringify(content, null, 2) }
  }
  try {
    return { ok: true, value: JSON.parse(content), raw: content }
  } catch (error) {
    return {
      ok: false,
      error: error instanceof Error ? error.message : 'JSON 解析失败',
      raw: content,
    }
  }
}

export function buildJsonSummary(value: unknown): JsonSummary {
  return {
    rootType: jsonValueType(value),
    itemCount: Array.isArray(value) ? value.length : isPlainObject(value) ? Object.keys(value).length : value === null ? 0 : 1,
    depth: jsonDepth(value),
  }
}

export function buildArtifactReviewModel(value: unknown): ArtifactReviewModel | null {
  const root = decodedRecord(value)
  if (!root) return null
  const embedded = decodedRecord(root.content)
  const records = [
    decodedRecord(embedded?.package),
    decodedRecord(root.package),
    embedded,
    root,
  ].filter((record): record is Record<string, unknown> => Boolean(record))

  const script = longestReadableString(records, ['detailedScript', 'script', 'transcript', 'narration'])
  const characters = reviewCharacters(firstArray(records, ['characters', 'cast']))
  const sections = reviewSections(firstArray(records, ['sections', 'scriptSpans', 'segments']))
  if (!script && characters.length === 0 && sections.length === 0) return null

  const summary = firstReadableString(records, ['summary', 'logline', 'description'])
  const durationSec = firstFiniteNumber(records, ['estimatedDurationSec', 'targetDurationSec', 'durationSec'])
  const normalizedScript = normalizedReviewText(script)
  const normalizedSummary = normalizedReviewText(summary)
  const deduplicatedSections = sections.map(section => {
    const normalized = normalizedReviewText(section.text)
    return normalized && (normalized === normalizedScript || normalized === normalizedSummary)
      ? { ...section, text: undefined }
      : section
  })
  const title = firstReadableString(records, ['title', 'documentTitle', 'projectName']) || '口播脚本'
  const metrics: ArtifactReviewMetric[] = []
  if (durationSec !== undefined) metrics.push({ label: '时长', value: `${formatReviewNumber(durationSec)} 秒` })
  if (characters.length > 0) metrics.push({ label: '角色', value: String(characters.length) })
  if (deduplicatedSections.length > 0) metrics.push({ label: '段落', value: String(deduplicatedSections.length) })

  return {
    kind: 'script',
    title,
    summary: normalizedSummary && normalizedSummary !== normalizedScript ? summary : undefined,
    script,
    metrics,
    characters,
    sections: deduplicatedSections,
    warnings: reviewWarnings(records),
  }
}

export function homogeneousJsonColumns(value: unknown): string[] {
  if (!Array.isArray(value) || value.length === 0 || value.some(item => !isPlainObject(item))) return []
  const first = Object.keys(value[0] as Record<string, unknown>)
  if (first.length === 0 || first.length > 12) return []
  const firstKeys = new Set(first)
  for (const row of value as Record<string, unknown>[]) {
    const keys = Object.keys(row)
    if (keys.length !== first.length || keys.some(key => !firstKeys.has(key))) return []
    if (keys.some(key => !isJsonScalar(row[key]))) return []
  }
  return first
}

export function filterJsonTree(value: unknown, query: string): unknown | undefined {
  const normalized = query.trim().toLocaleLowerCase()
  if (!normalized) return value
  return filterJsonValue(value, normalized)
}

export function markdownHeadings(markdown: string): MarkdownHeading[] {
  const headings: MarkdownHeading[] = []
  const used = new Map<string, number>()
  for (const line of markdown.split(/\r?\n/)) {
    const match = /^(#{1,6})\s+(.+?)\s*$/.exec(line)
    if (!match) continue
    const text = match[2].replace(/\s+#+\s*$/, '').trim()
    if (!text) continue
    const base = markdownHeadingId(text)
    const count = used.get(base) || 0
    used.set(base, count + 1)
    headings.push({ depth: match[1].length, text, id: count === 0 ? base : `${base}-${count + 1}` })
  }
  return headings
}

export function jsonScalarText(value: unknown): string {
  if (value === null) return 'null'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

export function artifactContentText(content: unknown): string {
  if (typeof content === 'string') return content
  if (content === undefined || content === null) return ''
  return JSON.stringify(content, null, 2)
}

export function safeCreatorReviewText(content: unknown): string | undefined {
  if (typeof content !== 'string') return undefined
  const trimmed = content.trimStart()
  if (trimmed.startsWith('{')) return undefined
  if (trimmed.startsWith('[')) {
    const firstArrayValue = trimmed.slice(1).trimStart()
    if (!firstArrayValue || /^(?:\[|\{|"|\]|-?\d|true\b|false\b|null\b)/.test(firstArrayValue)) return undefined
  }
  return content
}

export function creatorDirectEditText(
  presentation: ArtifactPresentationKind,
  reviewText: unknown,
): string | undefined {
  if (presentation !== 'markdown' && presentation !== 'text') return undefined
  return safeCreatorReviewText(reviewText)
}

export function reconcileCreatorEditMode(
  mode: 'instruction' | 'direct',
  directEditText: string | undefined,
): 'instruction' | 'direct' {
  return mode === 'direct' && directEditText === undefined ? 'instruction' : mode
}

export function artifactContentNeedsLocalHydration(value: { content?: unknown } | null | undefined): boolean {
  const content = value?.content
  if (typeof content === 'string') {
    return content.startsWith('内容保存在本地系统中。')
  }
  if (!isPlainObject(content)) return content === undefined || content === null
  return content.contentAvailability === 'local-agent' ||
    (content.localOnly === true && content.cloudPayloadStored === false && typeof content.storageRef === 'string')
}

export function downloadText(name: string, content: string, mimeType: string) {
  const blob = new Blob([content], { type: `${mimeType};charset=utf-8` })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = safeArtifactDownloadName(name)
  anchor.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 0)
}

export function safeArtifactDownloadName(name: string): string {
  const sanitized = Array.from(name, character => {
    const code = character.charCodeAt(0)
    return code < 32 || '\\/:*?"<>|'.includes(character) ? '_' : character
  }).join('')
  return sanitized.slice(0, 160) || 'artifact.json'
}

function filterJsonValue(value: unknown, query: string): unknown | undefined {
  if (isJsonScalar(value)) return jsonScalarText(value).toLocaleLowerCase().includes(query) ? value : undefined
  if (Array.isArray(value)) {
    const matches = value.map(item => filterJsonValue(item, query)).filter(item => item !== undefined)
    return matches.length > 0 ? matches : undefined
  }
  if (!isPlainObject(value)) return undefined
  const record = value as Record<string, unknown>
  const directScalarMatch = Object.values(record).some(item => isJsonScalar(item) && jsonScalarText(item).toLocaleLowerCase().includes(query))
  if (directScalarMatch) return value
  const result: Record<string, unknown> = {}
  for (const [key, child] of Object.entries(record)) {
    if (key.toLocaleLowerCase().includes(query)) {
      result[key] = child
      continue
    }
    const filtered = filterJsonValue(child, query)
    if (filtered !== undefined) result[key] = filtered
  }
  return Object.keys(result).length > 0 ? result : undefined
}

function jsonValueType(value: unknown): JsonSummary['rootType'] {
  if (value === null) return 'null'
  if (Array.isArray(value)) return 'array'
  if (isPlainObject(value)) return 'object'
  if (typeof value === 'number') return 'number'
  if (typeof value === 'boolean') return 'boolean'
  return 'string'
}

function jsonDepth(value: unknown): number {
  if (Array.isArray(value)) return 1 + Math.max(0, ...value.map(jsonDepth))
  if (isPlainObject(value)) return 1 + Math.max(0, ...Object.values(value).map(jsonDepth))
  return 1
}

function decodedRecord(value: unknown): Record<string, unknown> | undefined {
  if (isPlainObject(value)) return value
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  if (!trimmed.startsWith('{')) return undefined
  try {
    const parsed = JSON.parse(trimmed)
    return isPlainObject(parsed) ? parsed : undefined
  } catch {
    return undefined
  }
}

function firstArray(records: readonly Record<string, unknown>[], keys: readonly string[]): unknown[] {
  for (const record of records) {
    for (const key of keys) {
      if (Array.isArray(record[key]) && record[key].length > 0) return record[key] as unknown[]
    }
  }
  return []
}

function firstReadableString(records: readonly Record<string, unknown>[], keys: readonly string[]): string {
  for (const record of records) {
    for (const key of keys) {
      const value = readableString(record[key])
      if (value) return value
    }
  }
  return ''
}

function longestReadableString(records: readonly Record<string, unknown>[], keys: readonly string[]): string {
  const values: string[] = []
  for (const record of records) {
    for (const key of keys) {
      const value = readableString(record[key])
      if (value && !values.some(existing => normalizedReviewText(existing) === normalizedReviewText(value))) values.push(value)
    }
  }
  return values.sort((left, right) => right.length - left.length)[0] || ''
}

function readableString(value: unknown): string {
  if (typeof value !== 'string') return ''
  const trimmed = value.trim()
  if (!trimmed || (trimmed.startsWith('{') && decodedRecord(trimmed))) return ''
  return trimmed
}

function firstFiniteNumber(records: readonly Record<string, unknown>[], keys: readonly string[]): number | undefined {
  for (const record of records) {
    for (const key of keys) {
      const value = record[key]
      if (typeof value === 'number' && Number.isFinite(value) && value >= 0) return value
    }
  }
  return undefined
}

function reviewCharacters(values: readonly unknown[]): ArtifactReviewCharacter[] {
  const characters: ArtifactReviewCharacter[] = []
  const names = new Set<string>()
  for (const value of values) {
    const record = isPlainObject(value) ? value : undefined
    const name = typeof value === 'string'
      ? value.trim()
      : readableString(record?.name) || readableString(record?.title) || readableString(record?.id)
    if (!name || names.has(name)) continue
    names.add(name)
    characters.push({
      name,
      description: readableString(record?.description) || readableString(record?.role) || undefined,
    })
  }
  return characters
}

function reviewSections(values: readonly unknown[]): ArtifactReviewSection[] {
  return values.flatMap((value, index) => {
    if (typeof value === 'string') return [{ name: `第 ${index + 1} 段`, text: value.trim() || undefined }]
    if (!isPlainObject(value)) return []
    const name = readableString(value.name) || readableString(value.title) || readableString(value.id) || `第 ${index + 1} 段`
    const duration = typeof value.durationSec === 'number' && Number.isFinite(value.durationSec)
      ? `${formatReviewNumber(value.durationSec)} 秒`
      : undefined
    const text = firstReadableString([value], ['text', 'scriptText', 'content', 'description']) || undefined
    return [{ name, duration, text }]
  })
}

function reviewWarnings(records: readonly Record<string, unknown>[]): string[] {
  const warnings: string[] = []
  for (const record of records) {
    for (const key of ['factCheckWarnings', 'warnings', 'issues']) {
      const values = record[key]
      if (!Array.isArray(values)) continue
      for (const value of values) {
        const text = typeof value === 'string'
          ? value.trim()
          : isPlainObject(value)
            ? readableString(value.message) || readableString(value.description)
            : ''
        if (text && !warnings.includes(text)) warnings.push(text)
      }
    }
  }
  return warnings
}

function normalizedReviewText(value: unknown): string {
  return typeof value === 'string' ? value.normalize('NFKC').replace(/\s+/g, '').trim() : ''
}

function formatReviewNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1).replace(/\.0$/, '')
}

function markdownHeadingId(text: string): string {
  const normalized = text.normalize('NFKC').toLocaleLowerCase()
    .replace(/[^\p{Letter}\p{Number}\s_-]/gu, '')
    .trim()
    .replace(/[\s_]+/g, '-')
  return normalized || 'section'
}

function isJsonScalar(value: unknown): value is string | number | boolean | null {
  return value === null || ['string', 'number', 'boolean'].includes(typeof value)
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
