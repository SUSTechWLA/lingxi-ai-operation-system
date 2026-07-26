import type { RefObject } from 'react'
import { buildTextSelectionFromLengths, type TextSelectionDraft } from '../textSelection'

interface ReviewableTextSurfaceProps {
  source: string
  surfaceRef?: RefObject<HTMLElement>
  onSelectionChange?: (draft: TextSelectionDraft | null) => void
}

export function selectionFromDomRange(
  root: HTMLElement | null,
  source: string,
  range: Range,
): TextSelectionDraft['selection'] | null {
  if (!root || !root.contains(range.startContainer) || !root.contains(range.endContainer)) return null

  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const textNodes: Text[] = []
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    textNodes.push(node as Text)
  }
  const startNodeIndex = textNodes.indexOf(range.startContainer as Text)
  const endNodeIndex = textNodes.indexOf(range.endContainer as Text)
  if (startNodeIndex < 0 || endNodeIndex < 0) return null

  const selection = buildTextSelectionFromLengths(
    source,
    textNodes.map(node => node.data.length),
    startNodeIndex,
    range.startOffset,
    endNodeIndex,
    range.endOffset,
  )
  if (!selection) return null

  const firstNodeIndex = Math.min(startNodeIndex, endNodeIndex)
  const lastNodeIndex = Math.max(startNodeIndex, endNodeIndex)
  const renderedSlice = textNodes.slice(firstNodeIndex, lastNodeIndex + 1).map((node, index, nodes) => {
    const from = index === 0
      ? (firstNodeIndex === startNodeIndex ? range.startOffset : range.endOffset)
      : 0
    const to = index === nodes.length - 1
      ? (lastNodeIndex === endNodeIndex ? range.endOffset : range.startOffset)
      : node.data.length
    return node.data.slice(Math.min(from, to), Math.max(from, to))
  }).join('')
  return renderedSlice === selection.text ? selection : null
}

export default function ReviewableTextSurface({
  source,
  surfaceRef,
  onSelectionChange,
}: ReviewableTextSurfaceProps) {
  const capture = () => {
    if (!onSelectionChange) return
    const browserSelection = window.getSelection()
    const range = browserSelection?.rangeCount === 1 ? browserSelection.getRangeAt(0) : null
    const selection = range ? selectionFromDomRange(surfaceRef?.current ?? null, source, range) : null
    if (!range || !selection) {
      onSelectionChange(null)
      return
    }
    const rangeBounds = range.getBoundingClientRect()
    const surfaceBounds = surfaceRef?.current?.getBoundingClientRect()
    onSelectionChange({
      selection,
      anchor: {
        left: rangeBounds.width > 0 ? rangeBounds.left + rangeBounds.width / 2 : (surfaceBounds?.left ?? 0) + 16,
        top: rangeBounds.height > 0 ? rangeBounds.bottom + 8 : (surfaceBounds?.top ?? 0) + 16,
      },
    })
  }

  return (
    <div
      ref={node => {
        if (surfaceRef) (surfaceRef as { current: HTMLElement | null }).current = node
      }}
      className="creator-reviewable-text artifact-canonical-manuscript"
      tabIndex={onSelectionChange ? 0 : undefined}
      role="document"
      aria-label="可划选的原稿"
      onMouseUp={capture}
      onKeyUp={capture}
    >{source.split('\n').map((line, index) => (
      <span key={index}>{index === 0 ? line : `\n${line}`}</span>
    ))}</div>
  )
}
