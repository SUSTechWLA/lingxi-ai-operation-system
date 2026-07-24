import { useEffect, useMemo, useState } from 'react'
import { getCreatorArtifactContent } from '../../../services/creatorApi'
import { getLocalAgentBaseUrl } from '../../../services/localAgent'
import type { ArtifactContentResponse } from '../../../utils/types'
import { resolveCreatorArtifactMediaUrl } from '../logic'
import type { CreatorReviewArtifact, CreatorReviewCategory } from '../creatorReviewArtifacts'

interface CreatorContentLibraryProps {
  projectId: string
  artifacts: readonly CreatorReviewArtifact[]
  selectedArtifactId?: string
  onSelect: (artifact: CreatorReviewArtifact) => void
}

const tabs: ReadonlyArray<{ category: CreatorReviewCategory; label: string }> = [
  { category: 'text', label: '文字与提示词' },
  { category: 'image', label: '参考图' },
  { category: 'video', label: '视频片段' },
  { category: 'audio', label: '语音' },
]

function tabId(category: CreatorReviewCategory): string {
  return `creator-content-tab-${category}`
}

function tabPanelId(category: CreatorReviewCategory): string {
  return `creator-content-panel-${category}`
}

export default function CreatorContentLibrary({
  projectId,
  artifacts,
  selectedArtifactId,
  onSelect,
}: CreatorContentLibraryProps) {
  const [activeCategory, setActiveCategory] = useState<CreatorReviewCategory>('text')
  const counts = useMemo(() => Object.fromEntries(tabs.map(({ category }) => [
    category,
    artifacts.filter((artifact) => artifact.reviewCategory === category).length,
  ])) as Record<CreatorReviewCategory, number>, [artifacts])

  useEffect(() => {
    if (counts[activeCategory] > 0 || artifacts.length === 0) return
    const firstAvailable = tabs.find(({ category }) => counts[category] > 0)
    if (firstAvailable) setActiveCategory(firstAvailable.category)
  }, [activeCategory, artifacts.length, counts])

  const activeArtifacts = artifacts.filter((artifact) => artifact.reviewCategory === activeCategory)
  return (
    <aside className="creator-content-library" aria-label="关键内容库">
      <div className="creator-content-tabs" role="tablist" aria-label="内容类别">
        {tabs.map(tab => (
          <button
            type="button"
            role="tab"
            key={tab.category}
            id={tabId(tab.category)}
            aria-controls={tabPanelId(tab.category)}
            aria-selected={activeCategory === tab.category}
            className={activeCategory === tab.category ? 'is-active' : ''}
            onClick={() => setActiveCategory(tab.category)}
          >
            <span>{tab.label}</span>
            <strong>{counts[tab.category]}</strong>
          </button>
        ))}
      </div>
      {tabs.map(tab => (
        <div
          key={tab.category}
          id={tabPanelId(tab.category)}
          role="tabpanel"
          aria-labelledby={tabId(tab.category)}
          hidden={activeCategory !== tab.category}
        >
          {activeCategory === tab.category && (activeArtifacts.length === 0 ? (
            <p className="creator-content-empty">关键内容仍在准备中，生成完成后会显示在这里。</p>
          ) : (
            <div className="creator-content-cards">
              {activeArtifacts.map(artifact => (
                <CreatorContentCard
                  key={artifact.artifactId}
                  projectId={projectId}
                  artifact={artifact}
                  active
                  selected={selectedArtifactId === artifact.artifactId}
                  showVersion={artifacts.some(candidate =>
                    candidate.reviewCategory === artifact.reviewCategory &&
                    candidate.artifactId !== artifact.artifactId &&
                    !candidate.isCurrent,
                  )}
                  onSelect={() => onSelect(artifact)}
                />
              ))}
            </div>
          ))}
        </div>
      ))}
    </aside>
  )
}

function CreatorContentCard({
  projectId,
  artifact,
  active,
  selected,
  showVersion,
  onSelect,
}: {
  projectId: string
  artifact: CreatorReviewArtifact
  active: boolean
  selected: boolean
  showVersion: boolean
  onSelect: () => void
}) {
  const [preview, setPreview] = useState<ArtifactContentResponse | null>(null)
  const [failed, setFailed] = useState(false)
  const [retries, setRetries] = useState(0)

  useEffect(() => {
    setPreview(null)
    setFailed(false)
    if (!active) return () => undefined
    const controller = new AbortController()
    void getCreatorArtifactContent(artifact.artifactId, controller.signal)
      .then(response => {
        if (!controller.signal.aborted && response.artifact.id === artifact.artifactId) setPreview(response)
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true)
      })
    return () => controller.abort()
  }, [active, artifact.artifactId, retries])

  const mediaUrl = preview
    ? resolveCreatorArtifactMediaUrl(projectId, preview, getLocalAgentBaseUrl())
    : undefined
  const duration = preview ? previewDuration(preview) : undefined
  const mediaPreviewMissing = Boolean(
    preview &&
    artifact.reviewCategory !== 'text' &&
    !mediaUrl,
  )
  const mediaLoadKey = `${artifact.artifactId}:${retries}`
  const handleMediaError = () => setFailed(true)

  return (
    <article className={`creator-content-card is-${artifact.reviewCategory}${selected ? ' is-selected' : ''}`}>
      <button type="button" className="creator-content-card-select" aria-pressed={selected} onClick={onSelect}>
        <span>
          <strong>{artifact.reviewLabel}</strong>
          {artifact.shotLabel && <small>{artifact.shotLabel}</small>}
        </span>
        {showVersion && <em>版本 {artifact.version}</em>}
      </button>
      {failed || mediaPreviewMissing ? (
        <div className="creator-content-preview-error" role="status">
          <span>预览暂时无法读取</span>
          {retries < 2 && (
            <button type="button" onClick={() => setRetries(value => value + 1)}>重新加载预览</button>
          )}
        </div>
      ) : !preview ? (
        <div className="creator-content-preview-loading" aria-live="polite">正在准备预览…</div>
      ) : artifact.reviewCategory === 'text' ? (
        <p className="creator-content-excerpt">{readableExcerpt(preview.content)}</p>
      ) : artifact.reviewCategory === 'image' && mediaUrl ? (
        <img key={mediaLoadKey} src={mediaUrl} alt={`${artifact.reviewLabel}预览`} loading="lazy" onError={handleMediaError} />
      ) : artifact.reviewCategory === 'video' && mediaUrl ? (
        <video
          key={mediaLoadKey}
          src={videoPreviewUrl(mediaUrl)}
          aria-label={`${artifact.reviewLabel}预览`}
          controls
          muted
          playsInline
          preload="auto"
          onError={handleMediaError}
        />
      ) : artifact.reviewCategory === 'audio' && mediaUrl ? (
        <div className="creator-content-audio">
          <audio key={mediaLoadKey} src={mediaUrl} aria-label={`${artifact.reviewLabel}播放控件`} controls preload="metadata" onError={handleMediaError} />
          {duration && <span>{duration}</span>}
        </div>
      ) : null}
    </article>
  )
}

function videoPreviewUrl(mediaUrl: string): string {
  return `${mediaUrl.split('#', 1)[0]}#t=0.001`
}

function readableExcerpt(value: unknown): string {
  if (typeof value === 'string') return compactExcerpt(value)
  if (!value || typeof value !== 'object') return '内容已生成，选择后可阅读全文。'
  const record = value as Record<string, unknown>
  for (const key of ['text', 'script', 'prompt', 'description', 'summary', 'title', 'narration', 'content', 'value']) {
    const candidate = record[key]
    if (typeof candidate === 'string' && candidate.trim()) return compactExcerpt(candidate)
  }
  for (const candidate of Object.values(record)) {
    if (Array.isArray(candidate)) {
      const readable = candidate.find(item => typeof item === 'string' && item.trim())
      if (typeof readable === 'string') return compactExcerpt(readable)
    }
  }
  return '内容已生成，选择后可阅读全文。'
}

function compactExcerpt(value: string): string {
  const compact = value.replace(/\s+/g, ' ').trim()
  return compact.length > 128 ? `${compact.slice(0, 128)}…` : compact
}

function previewDuration(content: ArtifactContentResponse): string | undefined {
  const metadata = content.artifact.metadata
  const seconds = typeof metadata?.durationSec === 'number'
    ? metadata.durationSec
    : typeof metadata?.durationMs === 'number'
      ? metadata.durationMs / 1000
      : undefined
  if (seconds === undefined || !Number.isFinite(seconds) || seconds < 0) return undefined
  const minutes = Math.floor(seconds / 60)
  const remainder = Math.round(seconds % 60)
  return minutes > 0 ? `${minutes}:${String(remainder).padStart(2, '0')}` : `${remainder} 秒`
}
