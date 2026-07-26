import type { ArtifactSelection } from './types'

export interface TextSelectionDraft {
  selection: Extract<ArtifactSelection, { kind: 'text' }>
  anchor: { left: number; top: number }
}

export function buildTextSelection(
  source: string,
  start: number,
  end: number,
): Extract<ArtifactSelection, { kind: 'text' }> | null {
  if (!Number.isInteger(start) || !Number.isInteger(end)) return null
  const normalizedStart = Math.min(start, end)
  const normalizedEnd = Math.max(start, end)
  if (normalizedStart < 0 || normalizedEnd > source.length || normalizedStart === normalizedEnd) return null
  if (normalizedEnd - normalizedStart > 4_000) return null
  if (!isUtf16Boundary(source, normalizedStart) || !isUtf16Boundary(source, normalizedEnd)) return null
  return {
    kind: 'text',
    start: normalizedStart,
    end: normalizedEnd,
    text: source.slice(normalizedStart, normalizedEnd),
  }
}

export function buildTextSelectionFromLengths(
  source: string,
  textNodeLengths: readonly number[],
  startNodeIndex: number,
  startOffset: number,
  endNodeIndex: number,
  endOffset: number,
): Extract<ArtifactSelection, { kind: 'text' }> | null {
  if (
    ![startNodeIndex, startOffset, endNodeIndex, endOffset].every(Number.isInteger) ||
    startNodeIndex < 0 || endNodeIndex < 0 ||
    startNodeIndex >= textNodeLengths.length || endNodeIndex >= textNodeLengths.length
  ) return null
  if (textNodeLengths.some(length => !Number.isInteger(length) || length < 0)) return null
  if (
    startOffset < 0 || startOffset > textNodeLengths[startNodeIndex] ||
    endOffset < 0 || endOffset > textNodeLengths[endNodeIndex]
  ) return null

  const precedingLength = (nodeIndex: number) => textNodeLengths
    .slice(0, nodeIndex)
    .reduce((total, length) => total + length, 0)
  return buildTextSelection(
    source,
    precedingLength(startNodeIndex) + startOffset,
    precedingLength(endNodeIndex) + endOffset,
  )
}

export function rebaseTextSelection(
  backendSource: string,
  renderedSource: string,
  selection: Extract<ArtifactSelection, { kind: 'text' }>,
): Extract<ArtifactSelection, { kind: 'text' }> | null {
  if (backendSource === renderedSource) {
    return selection
  }
  if (buildTextSelection(renderedSource, selection.start, selection.end)?.text !== selection.text) return null

  const jsonRange = canonicalJSONStringRange(backendSource, renderedSource)
  if (jsonRange) {
    const start = jsonRange.boundaries[selection.start]
    const end = jsonRange.boundaries[selection.end]
    if (start === undefined || end === undefined) return null
    return buildTextSelection(backendSource, start, end)
  }

  const sourceStart = backendSource.indexOf(renderedSource)
  if (sourceStart < 0 || backendSource.indexOf(renderedSource, sourceStart + 1) >= 0) return null
  return buildTextSelection(
    backendSource,
    sourceStart + selection.start,
    sourceStart + selection.end,
  )
}

const CREATOR_TEXT_FIELD_PRIORITY = [
  'detailedScript', 'script', 'transcript', 'narration', 'prompt',
  'description', 'summary', 'title', 'text', 'value',
] as const

interface JSONStringToken {
  decoded: string
  fullStart: number
  fullEnd: number
  boundaries: number[]
}

function canonicalJSONStringRange(source: string, renderedSource: string): JSONStringToken | null {
  const ranked = collectCanonicalJSONStringRanges(source, renderedSource, offset => offset, 0)
  if (ranked.length === 0) return null
  const bestPriority = Math.min(...ranked.map(candidate => candidate.priority))
  const best = ranked.filter(candidate => candidate.priority === bestPriority)
  return best.length === 1 ? best[0].token : null
}

function collectCanonicalJSONStringRanges(
  source: string,
  renderedSource: string,
  toRootOffset: (offset: number) => number,
  depth: number,
): Array<{ token: JSONStringToken; priority: number }> {
  if (depth > 6) return []
  try {
    JSON.parse(source)
  } catch {
    return []
  }
  const tokens = scanJSONStringTokens(source)
  const candidates: Array<{ token: JSONStringToken; priority: number }> = []
  tokens.forEach((token, index) => {
    const rootToken = { ...token, boundaries: token.boundaries.map(toRootOffset) }
    const normalized = token.decoded.trim()
    if (normalized === renderedSource && index > 0) {
      const key = tokens[index - 1]
      if (/^\s*:\s*$/.test(source.slice(key.fullEnd, token.fullStart))) {
        const priority = CREATOR_TEXT_FIELD_PRIORITY.indexOf(key.decoded as typeof CREATOR_TEXT_FIELD_PRIORITY[number])
        const leadingUnits = token.decoded.length - token.decoded.trimStart().length
        const normalizedEnd = leadingUnits + normalized.length
        if (
          priority >= 0 &&
          token.decoded.slice(leadingUnits, normalizedEnd) === renderedSource &&
          rootToken.boundaries.length > normalizedEnd
        ) {
          candidates.push({
            token: { ...rootToken, boundaries: rootToken.boundaries.slice(leadingUnits, normalizedEnd + 1) },
            priority,
          })
        }
      }
    }
    if (depth < 6 && /^[{[]/.test(token.decoded.trimStart())) {
      candidates.push(...collectCanonicalJSONStringRanges(
        token.decoded,
        renderedSource,
        offset => {
          const sourceOffset = token.boundaries[offset]
          return sourceOffset === undefined ? -1 : toRootOffset(sourceOffset)
        },
        depth + 1,
      ))
    }
  })
  return candidates
}

function scanJSONStringTokens(source: string): JSONStringToken[] {
  const tokens: JSONStringToken[] = []
  for (let index = 0; index < source.length; index += 1) {
    if (source[index] !== '"') continue
    const fullStart = index
    let decoded = ''
    const boundaries = [index + 1]
    index += 1
    while (index < source.length && source[index] !== '"') {
      if (source[index] !== '\\') {
        decoded += source[index]
        index += 1
        boundaries.push(index)
        continue
      }
      const escape = source[index + 1]
      if (escape === 'u') {
        const hex = source.slice(index + 2, index + 6)
        if (!/^[0-9a-f]{4}$/i.test(hex)) return []
        decoded += String.fromCharCode(Number.parseInt(hex, 16))
        index += 6
        boundaries.push(index)
        continue
      }
      const escapedCharacters: Record<string, string> = {
        '"': '"', '\\': '\\', '/': '/', b: '\b', f: '\f', n: '\n', r: '\r', t: '\t',
      }
      if (!(escape in escapedCharacters)) return []
      decoded += escapedCharacters[escape]
      index += 2
      boundaries.push(index)
    }
    if (index >= source.length) return []
    tokens.push({ decoded, fullStart, fullEnd: index + 1, boundaries })
  }
  return tokens
}

function isUtf16Boundary(source: string, index: number): boolean {
  if (index <= 0 || index >= source.length) return true
  const preceding = source.charCodeAt(index - 1)
  const following = source.charCodeAt(index)
  return !(preceding >= 0xD800 && preceding <= 0xDBFF && following >= 0xDC00 && following <= 0xDFFF)
}
