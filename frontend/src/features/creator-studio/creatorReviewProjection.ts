import type { ArtifactReviewModel } from './artifactPresentation'

export interface CreatorReviewProjection {
  canonicalText?: string
  excerpt: string
  document: ArtifactReviewModel | null
}

const BODY_FIELDS = ['detailedScript', 'script', 'transcript', 'narration', 'prompt'] as const
const FALLBACK_FIELDS = ['description', 'summary', 'title', 'text', 'value'] as const
const WRAPPER_FIELDS = ['artifacts', 'content', 'package', 'payload', 'data'] as const
const NEUTRAL_CREATOR_CONTENT_MESSAGE = '内容已生成，选择后可查看。'
const MAX_CREATOR_PAYLOAD_DEPTH = 6
const MAX_CREATOR_ARRAY_ENTRIES = 64
const CREATOR_EXCERPT_LENGTH = 128

interface ProjectionCandidate {
  text: string
  document: ArtifactReviewModel | null
}

export function projectCreatorReviewContent(content: unknown): CreatorReviewProjection {
  const bodyCandidates: ProjectionCandidate[] = []
  const fallbackCandidates: ProjectionCandidate[] = []

  for (const candidate of creatorPayloadCandidates(content, 0)) {
    const body = longestCreatorField(candidate, BODY_FIELDS)
    if (body) bodyCandidates.push({ text: body, document: creatorReviewDocument(candidate, body) })

    const fallback = readableCreatorScalar(candidate) || longestCreatorField(candidate, FALLBACK_FIELDS)
    if (fallback) fallbackCandidates.push({ text: fallback, document: null })
  }

  const selected = longestCandidate(bodyCandidates) || longestCandidate(fallbackCandidates)
  if (!selected) return { excerpt: NEUTRAL_CREATOR_CONTENT_MESSAGE, document: null }
  return {
    canonicalText: selected.text,
    excerpt: compactCreatorExcerpt(selected.text),
    document: selected.document,
  }
}

export function creatorReviewExcerpt(content: unknown): string {
  return projectCreatorReviewContent(content).excerpt
}

function* creatorPayloadCandidates(value: unknown, depth: number, seen = new Set<object>()): Generator<unknown> {
  if (depth > MAX_CREATOR_PAYLOAD_DEPTH) return
  const decoded = decodedCreatorPayload(value)
  if (decoded !== value) {
    yield* creatorPayloadCandidates(decoded, depth + 1, seen)
    return
  }

  yield value
  if (depth === MAX_CREATOR_PAYLOAD_DEPTH || !value || typeof value !== 'object') return
  if (seen.has(value)) return
  seen.add(value)

  if (Array.isArray(value)) {
    for (const item of value.slice(0, MAX_CREATOR_ARRAY_ENTRIES)) {
      yield* creatorPayloadCandidates(item, depth + 1, seen)
    }
    return
  }

  const record = value as Record<string, unknown>
  for (const key of WRAPPER_FIELDS) {
    if (key in record) yield* creatorPayloadCandidates(record[key], depth + 1, seen)
  }
}

function decodedCreatorPayload(value: unknown): unknown {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!trimmed || !/^[{[]/.test(trimmed)) return value
  try {
    return JSON.parse(trimmed)
  } catch {
    return value
  }
}

function longestCreatorField(value: unknown, fields: readonly string[]): string | undefined {
  if (!isCreatorRecord(value)) return undefined
  const values = fields
    .map(field => readableCreatorString(value[field]))
    .filter((candidate): candidate is string => Boolean(candidate))
  return values.sort((left, right) => right.length - left.length)[0]
}

function readableCreatorScalar(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  if (decodedCreatorPayload(value) !== value) return undefined
  return readableCreatorString(value)
}

function readableCreatorString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const text = value.trim()
  if (
    !text ||
    text.startsWith('{') ||
    (text.startsWith('[') && /^(?:\[|\{|"|\]|-?\d|true\b|false\b|null\b)/.test(text.slice(1).trimStart())) ||
    decodedCreatorPayload(text) !== text
  ) return undefined
  return text
}

function longestCandidate(candidates: readonly ProjectionCandidate[]): ProjectionCandidate | undefined {
  return candidates.reduce<ProjectionCandidate | undefined>(
    (longest, candidate) => !longest || candidate.text.length > longest.text.length ? candidate : longest,
    undefined,
  )
}

function compactCreatorExcerpt(value: string): string {
  const compact = value.replace(/\s+/g, ' ').trim()
  return compact.length > CREATOR_EXCERPT_LENGTH ? `${compact.slice(0, CREATOR_EXCERPT_LENGTH)}…` : compact
}

function isCreatorRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function creatorReviewDocument(value: unknown, script: string): ArtifactReviewModel | null {
  if (!isCreatorRecord(value)) return null
  const title = readableCreatorString(value.title) || '口播脚本'
  const summary = longestCreatorField(value, ['summary', 'description'])
  return {
    kind: 'script',
    title,
    summary: summary && summary !== script ? summary : undefined,
    script,
    metrics: [],
    characters: [],
    sections: [],
    warnings: [],
  }
}
