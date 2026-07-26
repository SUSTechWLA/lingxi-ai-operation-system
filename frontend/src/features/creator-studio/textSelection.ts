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
  if (buildTextSelection(backendSource, selection.start, selection.end)?.text === selection.text) {
    return selection
  }
  if (buildTextSelection(renderedSource, selection.start, selection.end)?.text !== selection.text) return null

  const sourceStart = backendSource.indexOf(renderedSource)
  if (sourceStart < 0 || backendSource.indexOf(renderedSource, sourceStart + 1) >= 0) return null
  return buildTextSelection(
    backendSource,
    sourceStart + selection.start,
    sourceStart + selection.end,
  )
}

function isUtf16Boundary(source: string, index: number): boolean {
  if (index <= 0 || index >= source.length) return true
  const preceding = source.charCodeAt(index - 1)
  const following = source.charCodeAt(index)
  return !(preceding >= 0xD800 && preceding <= 0xDBFF && following >= 0xDC00 && following <= 0xDFFF)
}
