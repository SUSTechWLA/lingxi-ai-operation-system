import type { RefObject } from 'react'
import { safeCreatorReviewText } from '../artifactPresentation'
import type { TextSelectionDraft } from '../textSelection'
import JsonArtifactViewer from './JsonArtifactViewer'
import ReviewableTextSurface from './ReviewableTextSurface'

interface MarkdownArtifactViewerProps {
  content: unknown
  selectionSource?: string
  selectionEnabled?: boolean
  textSurfaceRef?: RefObject<HTMLElement>
  onTextSelectionChange?: (draft: TextSelectionDraft | null) => void
}

export default function MarkdownArtifactViewer({
  content,
  selectionSource,
  selectionEnabled = false,
  textSurfaceRef,
  onTextSelectionChange,
}: MarkdownArtifactViewerProps) {
  const markdown = safeCreatorReviewText(content)
  if (markdown === undefined) {
    return <JsonArtifactViewer content={content}
      selectionSource={selectionSource}
      selectionEnabled={selectionEnabled}
      textSurfaceRef={textSurfaceRef}
      onTextSelectionChange={onTextSelectionChange}
    />
  }
  const reviewSource = selectionSource ?? markdown
  const canSelect = selectionEnabled && reviewSource.length > 0 && textSurfaceRef && onTextSelectionChange

  return (
    <section className="artifact-markdown-viewer" aria-label="文稿审阅">
      <div className="artifact-markdown-layout">
        <ReviewableTextSurface
          source={reviewSource}
          surfaceRef={canSelect ? textSurfaceRef : undefined}
          onSelectionChange={canSelect ? onTextSelectionChange : undefined}
        />
      </div>
    </section>
  )
}
