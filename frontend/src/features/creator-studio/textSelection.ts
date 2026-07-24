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

function isUtf16Boundary(source: string, index: number): boolean {
  if (index <= 0 || index >= source.length) return true
  const preceding = source.charCodeAt(index - 1)
  const following = source.charCodeAt(index)
  return !(preceding >= 0xD800 && preceding <= 0xDBFF && following >= 0xDC00 && following <= 0xDFFF)
}
