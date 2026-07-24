import { useEffect, useState, type PointerEvent, type RefObject } from 'react'
import type { ArtifactContentResponse } from '../../../utils/types'
import type { ArtifactSelection } from '../types'
import type { TextSelectionDraft } from '../textSelection'
import { artifactContentNeedsLocalHydration, classifyArtifactPresentation, safeCreatorReviewText } from '../artifactPresentation'
import { resolveCreatorArtifactMediaUrl } from '../logic'
import { getLocalAgentBaseUrl } from '../../../services/localAgent'
import JsonArtifactViewer from './JsonArtifactViewer'
import MarkdownArtifactViewer from './MarkdownArtifactViewer'
import SimpleVideoPlayer from './SimpleVideoPlayer'
import { CanonicalTextSelectionSurface } from './TextSelectionAssistant'

interface ArtifactProofingCanvasProps {
  content: ArtifactContentResponse | null
  selection?: ArtifactSelection | null
  imageRef?: RefObject<HTMLImageElement>
  onOpenImage?: (src: string) => void
  onImagePointerDown?: (event: PointerEvent<HTMLImageElement>) => void
  onImagePointerUp?: (event: PointerEvent<HTMLImageElement>) => void
  onImagePointerCancel?: () => void
  textSurfaceRef?: RefObject<HTMLPreElement>
  onTextSelectionChange?: (draft: TextSelectionDraft | null) => void
}

export default function ArtifactProofingCanvas({
  content,
  selection,
  imageRef,
  onOpenImage,
  onImagePointerDown,
  onImagePointerUp,
  onImagePointerCancel,
  textSurfaceRef,
  onTextSelectionChange,
}: ArtifactProofingCanvasProps) {
  const [mediaFailed, setMediaFailed] = useState(false)
  const [localText, setLocalText] = useState<{ key: string; text?: string; error?: string } | null>(null)
  const artifact = content?.artifact
  const presentation = classifyArtifactPresentation({ kind: artifact?.kind, mimeType: artifact?.mimeType, name: artifact?.name })
  const resolvedMediaUrl = content && artifact
    ? resolveCreatorArtifactMediaUrl(artifact.projectId, content, getLocalAgentBaseUrl())
    : undefined
  const hydrationKey = `${artifact?.id || 'none'}:${resolvedMediaUrl || 'inline'}`
  const shouldHydrateLocalText = Boolean(
    resolvedMediaUrl &&
    artifact?.storageType === 'local' &&
    artifactContentNeedsLocalHydration(content) &&
    ['json', 'markdown', 'text'].includes(presentation),
  )

  useEffect(() => setMediaFailed(false), [artifact?.id, resolvedMediaUrl])

  useEffect(() => {
    if (!shouldHydrateLocalText || !resolvedMediaUrl) {
      setLocalText(null)
      return () => undefined
    }
    const controller = new AbortController()
    void fetch(resolvedMediaUrl, { signal: controller.signal }).then(async response => {
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const announcedSize = Number(response.headers.get('content-length') || 0)
      if (announcedSize > 8 * 1024 * 1024) throw new Error('内容过大，暂时无法在这里审阅')
      const text = await response.text()
      if (text.length > 8 * 1024 * 1024) throw new Error('内容过大，暂时无法在这里审阅')
      if (!controller.signal.aborted) setLocalText({ key: hydrationKey, text })
    }).catch(error => {
      if (!controller.signal.aborted) setLocalText({ key: hydrationKey, error: error instanceof Error ? error.message : '读取失败' })
    })
    return () => controller.abort()
  }, [hydrationKey, resolvedMediaUrl, shouldHydrateLocalText])

  if (!content || !artifact) return <p className="artifact-empty">正在读取内容…</p>
  if (shouldHydrateLocalText && localText?.key !== hydrationKey) return <p className="artifact-empty" aria-live="polite">正在读取本地产物…</p>
  if (shouldHydrateLocalText && localText?.key === hydrationKey && localText.error) {
    return <div className="artifact-file-fallback" role="alert"><div><strong>关键内容仍在准备中</strong><p>暂时无法读取这份内容，请重新读取当前内容后再试。</p></div></div>
  }
  const displayedContent = localText?.key === hydrationKey && localText.text !== undefined ? localText.text : content.content
  const readableText = safeCreatorReviewText(displayedContent)
  const selectionSource = safeCreatorReviewText(content.reviewText)
  const selectionEnabled = Boolean(textSurfaceRef && onTextSelectionChange)
  if (presentation === 'json') {
    return <JsonArtifactViewer
      content={displayedContent}
      selectionSource={selectionSource}
      selectionEnabled={selectionEnabled}
      textSurfaceRef={textSurfaceRef}
      onTextSelectionChange={onTextSelectionChange}
    />
  }
  if (presentation === 'markdown') {
    return readableText === undefined
      ? <JsonArtifactViewer
          content={displayedContent}
          selectionSource={selectionSource}
          selectionEnabled={selectionEnabled}
          textSurfaceRef={textSurfaceRef}
          onTextSelectionChange={onTextSelectionChange}
        />
      : <MarkdownArtifactViewer
          content={readableText}
          selectionSource={selectionSource}
          selectionEnabled={selectionEnabled}
          textSurfaceRef={textSurfaceRef}
          onTextSelectionChange={onTextSelectionChange}
        />
  }
  if (presentation === 'image' && resolvedMediaUrl && !mediaFailed) {
    const rect = selection?.kind === 'rect' ? selection : null
    return (
      <div className="artifact-image-stage">
        <img
          ref={imageRef}
          className="artifact-image-preview"
          src={resolvedMediaUrl}
          alt={artifact.name || '当前图片内容'}
          onError={() => setMediaFailed(true)}
          onPointerDown={onImagePointerDown}
          onPointerUp={onImagePointerUp}
          onPointerCancel={onImagePointerCancel}
        />
        {rect && <span className="artifact-selection-overlay" aria-label="已选中的图片区域" style={{ left: `${rect.x * 100}%`, top: `${rect.y * 100}%`, width: `${rect.width * 100}%`, height: `${rect.height * 100}%` }} />}
        {onOpenImage && <button type="button" className="artifact-image-open-button" onClick={() => onOpenImage(resolvedMediaUrl)}>全屏审阅</button>}
      </div>
    )
  }
  if (presentation === 'video' && resolvedMediaUrl && !mediaFailed) {
    return <SimpleVideoPlayer src={resolvedMediaUrl} title={artifact.name || '视频产物'} downloadName={artifact.name} onError={() => setMediaFailed(true)} />
  }
  if (presentation === 'audio' && resolvedMediaUrl && !mediaFailed) {
    return <audio className="artifact-audio-preview" controls preload="metadata" src={resolvedMediaUrl} onError={() => setMediaFailed(true)}>当前客户端无法播放音频。</audio>
  }
  if (presentation === 'text') {
    return selectionEnabled && selectionSource !== undefined && textSurfaceRef && onTextSelectionChange
      ? <CanonicalTextSelectionSurface source={selectionSource} surfaceRef={textSurfaceRef} onSelectionChange={onTextSelectionChange} />
      : readableText === undefined
      ? <JsonArtifactViewer content={displayedContent} />
      : <pre className="artifact-raw-preview" tabIndex={0}>{readableText}</pre>
  }
  return <ArtifactFileFallback mediaFailed={mediaFailed} />
}

function ArtifactFileFallback({ mediaFailed }: { mediaFailed: boolean }) {
  return (
    <div className="artifact-file-fallback">
      <span className="artifact-file-icon" aria-hidden="true">▤</span>
      <div>
        <strong>关键内容仍在准备中</strong>
        <p role={mediaFailed ? 'alert' : undefined}>{mediaFailed ? '暂时无法显示预览，请重新读取当前内容后再试。' : '生成完成后会显示在这里。'}</p>
      </div>
    </div>
  )
}
