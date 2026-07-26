import { useEffect, useState, type PointerEvent, type RefObject } from 'react'
import type { ArtifactContentResponse } from '../../../utils/types'
import type { ArtifactSelection } from '../types'
import type { TextSelectionDraft } from '../textSelection'
import { artifactContentNeedsLocalHydration, classifyArtifactPresentation, safeCreatorReviewText } from '../artifactPresentation'
import { projectCreatorReviewContent } from '../creatorReviewProjection'
import { resolveCreatorArtifactMediaUrl } from '../logic'
import { mediaReviewAfterPlaybackFailure } from '../mediaRange'
import { getLocalAgentBaseUrl } from '../../../services/localAgent'
import JsonArtifactViewer from './JsonArtifactViewer'
import MarkdownArtifactViewer from './MarkdownArtifactViewer'
import MediaRangeControls from './MediaRangeControls'
import SimpleAudioPlayer from './SimpleAudioPlayer'
import SimpleVideoPlayer, { type MediaPlaybackState } from './SimpleVideoPlayer'
import { CanonicalTextSelectionSurface } from './TextSelectionAssistant'

type TimeSelection = Extract<ArtifactSelection, { kind: 'time' }>

interface ArtifactProofingCanvasProps {
  content: ArtifactContentResponse | null
  reviewLabel?: string
  selection?: ArtifactSelection | null
  imageRef?: RefObject<HTMLImageElement>
  onOpenImage?: (src: string) => void
  onImagePointerDown?: (event: PointerEvent<HTMLImageElement>) => void
  onImagePointerUp?: (event: PointerEvent<HTMLImageElement>) => void
  onImagePointerCancel?: () => void
  textSurfaceRef?: RefObject<HTMLPreElement>
  onTextSelectionChange?: (draft: TextSelectionDraft | null) => void
  mediaRangeResetKey?: string | number
  mediaRangeDisabled?: boolean
  mediaRangePending?: boolean
  onMediaSelectionChange?: (selection: TimeSelection | null) => void
  onMediaRangePendingChange?: (pending: boolean) => void
}

const EMPTY_PLAYBACK_STATE: MediaPlaybackState = { currentTimeMs: 0, durationMs: 0, isPlaying: false }

export default function ArtifactProofingCanvas({
  content,
  reviewLabel,
  selection,
  imageRef,
  onOpenImage,
  onImagePointerDown,
  onImagePointerUp,
  onImagePointerCancel,
  textSurfaceRef,
  onTextSelectionChange,
  mediaRangeResetKey,
  mediaRangeDisabled,
  mediaRangePending,
  onMediaSelectionChange,
  onMediaRangePendingChange,
}: ArtifactProofingCanvasProps) {
  const [mediaFailed, setMediaFailed] = useState(false)
  const [playbackAvailable, setPlaybackAvailable] = useState(false)
  const [playbackState, setPlaybackState] = useState<MediaPlaybackState>(EMPTY_PLAYBACK_STATE)
  const [localText, setLocalText] = useState<{ key: string; text?: string; error?: string } | null>(null)
  const artifact = content?.artifact
  const presentation = classifyArtifactPresentation({ kind: artifact?.kind, mimeType: artifact?.mimeType, name: artifact?.name })
  const semanticMediaLabel = reviewLabel || (presentation === 'image'
    ? '当前图片'
    : presentation === 'audio' ? '当前语音' : '当前视频')

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
    setPlaybackState(EMPTY_PLAYBACK_STATE)
    setPlaybackAvailable(false)
  }, [hydrationKey])

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
  const displayedProjection = projectCreatorReviewContent(displayedContent)
  const readableText = safeCreatorReviewText(displayedProjection.canonicalText)
  const selectionSource = safeCreatorReviewText(content.reviewText)
  const selectionEnabled = Boolean(textSurfaceRef && onTextSelectionChange)
  const invalidateMediaReview = () => {
    const next = mediaReviewAfterPlaybackFailure({
      playbackAvailable,
      selection: selection?.kind === 'time' ? selection : null,
      pending: mediaRangePending ?? false,
    })
    setPlaybackAvailable(next.playbackAvailable)
    onMediaSelectionChange?.(next.selection)
    onMediaRangePendingChange?.(next.pending)
  }
  const markPlaybackReady = () => setPlaybackAvailable(true)
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
          alt={`${semanticMediaLabel}预览`}
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
    return (
      <div className="artifact-media-proofing">
        <SimpleVideoPlayer
          src={resolvedMediaUrl}
          title={semanticMediaLabel}
          downloadName={semanticMediaLabel}
          onError={() => {
            invalidateMediaReview()
            setMediaFailed(true)
          }}
          onPlaybackStateChange={setPlaybackState}
        />
        {onMediaSelectionChange && <MediaRangeControls
          currentTimeMs={playbackState.currentTimeMs}
          selection={selection?.kind === 'time' ? selection : null}
          resetKey={`${hydrationKey}:${mediaRangeResetKey ?? 0}`}
          disabled={mediaRangeDisabled}
          onSelectionChange={onMediaSelectionChange}
          onPendingChange={onMediaRangePendingChange}
        />}
      </div>
    )
  }
  if (presentation === 'audio' && resolvedMediaUrl && !mediaFailed) {
    return (
      <div className="artifact-media-proofing">
        <SimpleAudioPlayer
          src={resolvedMediaUrl}
          title={semanticMediaLabel}
          downloadName={semanticMediaLabel}
          onError={invalidateMediaReview}
          onReady={markPlaybackReady}
          onPlaybackStateChange={setPlaybackState}
        />
        {playbackAvailable && onMediaSelectionChange && <MediaRangeControls
          currentTimeMs={playbackState.currentTimeMs}
          selection={selection?.kind === 'time' ? selection : null}
          resetKey={`${hydrationKey}:${mediaRangeResetKey ?? 0}`}
          disabled={mediaRangeDisabled}
          onSelectionChange={onMediaSelectionChange}
          onPendingChange={onMediaRangePendingChange}
        />}
      </div>
    )
  }
  if (presentation === 'text') {
    return selectionEnabled && selectionSource !== undefined && textSurfaceRef && onTextSelectionChange
      ? <CanonicalTextSelectionSurface source={selectionSource} surfaceRef={textSurfaceRef} onSelectionChange={onTextSelectionChange} />
      : <JsonArtifactViewer content={displayedContent} />
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
