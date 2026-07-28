import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { getCreatorArtifactContent } from '../../../services/creatorApi'
import { getLocalAgentBaseUrl } from '../../../services/localAgent'
import type { ArtifactContentResponse } from '../../../utils/types'
import {
  nextCreatorArtifactTabIndex,
  shouldShowCreatorArtifactSwitcher,
  type CreatorArtifactTabNavigationKey,
  type CreatorReviewArtifact,
} from '../creatorReviewArtifacts'
import { resolveCreatorArtifactMediaUrl } from '../logic'

interface CreatorCurrentArtifactSwitcherProps {
  projectId: string
  artifacts: readonly CreatorReviewArtifact[]
  selectedArtifactId?: string
  onSelect: (artifact: CreatorReviewArtifact) => void
}

interface CompactPreview {
  mediaUrl?: string
  duration?: string
}

const navigationKeys = new Set<CreatorArtifactTabNavigationKey>([
  'ArrowLeft',
  'ArrowRight',
  'Home',
  'End',
])

export default function CreatorCurrentArtifactSwitcher({
  projectId,
  artifacts,
  selectedArtifactId,
  onSelect,
}: CreatorCurrentArtifactSwitcherProps) {
  const [previews, setPreviews] = useState<Record<string, CompactPreview>>({})
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([])

  useEffect(() => {
    const mediaArtifacts = artifacts.filter(artifact => artifact.reviewCategory !== 'text')
    if (mediaArtifacts.length === 0) {
      setPreviews({})
      return () => undefined
    }
    const controller = new AbortController()
    let cursor = 0
    const worker = async () => {
      while (!controller.signal.aborted && cursor < mediaArtifacts.length) {
        const artifact = mediaArtifacts[cursor]
        cursor += 1
        try {
          const content = await getCreatorArtifactContent(artifact.artifactId, controller.signal)
          if (controller.signal.aborted) return
          const mediaUrl = artifact.reviewCategory === 'image'
            ? resolveCreatorArtifactMediaUrl(projectId, content, getLocalAgentBaseUrl())
            : undefined
          setPreviews(current => ({
            ...current,
            [artifact.artifactId]: {
              ...(mediaUrl ? { mediaUrl } : {}),
              ...(previewDuration(content) ? { duration: previewDuration(content) } : {}),
            },
          }))
        } catch {
          if (!controller.signal.aborted) {
            setPreviews(current => ({ ...current, [artifact.artifactId]: {} }))
          }
        }
      }
    }
    void Promise.all(Array.from(
      { length: Math.min(3, mediaArtifacts.length) },
      () => worker(),
    ))
    return () => controller.abort()
  }, [artifacts, projectId])

  if (!shouldShowCreatorArtifactSwitcher(artifacts)) return null

  const selectedIndex = Math.max(0, artifacts.findIndex(artifact => artifact.artifactId === selectedArtifactId))
  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (!navigationKeys.has(event.key as CreatorArtifactTabNavigationKey)) return
    event.preventDefault()
    const nextIndex = nextCreatorArtifactTabIndex(
      index,
      event.key as CreatorArtifactTabNavigationKey,
      artifacts.length,
    )
    const nextArtifact = artifacts[nextIndex]
    if (!nextArtifact) return
    onSelect(nextArtifact)
    tabRefs.current[nextIndex]?.focus()
  }

  return (
    <nav className="creator-current-artifact-switcher" aria-label="当前可审阅内容">
      <div role="tablist" aria-label="切换当前内容">
        {artifacts.map((artifact, index) => {
          const selected = index === selectedIndex
          const preview = previews[artifact.artifactId]
          return (
            <button
              key={artifact.artifactId}
              ref={element => { tabRefs.current[index] = element }}
              type="button"
              role="tab"
              aria-selected={selected}
              aria-controls="creator-proofing-canvas"
              tabIndex={selected ? 0 : -1}
              className={selected ? 'is-selected' : ''}
              onClick={() => onSelect(artifact)}
              onKeyDown={event => handleKeyDown(event, index)}
            >
              {artifact.reviewCategory === 'image' && preview?.mediaUrl ? (
                <img src={preview.mediaUrl} alt="" loading="lazy" />
              ) : (
                <span className={`creator-current-artifact-icon is-${artifact.reviewCategory}`} aria-hidden="true">
                  {categoryIcon(artifact.reviewCategory)}
                </span>
              )}
              <span className="creator-current-artifact-copy">
                <strong>{artifact.reviewLabel}</strong>
                {(artifact.shotLabel || preview?.duration) && (
                  <small>{[artifact.shotLabel, preview?.duration].filter(Boolean).join(' · ')}</small>
                )}
              </span>
            </button>
          )
        })}
      </div>
    </nav>
  )
}

function categoryIcon(category: CreatorReviewArtifact['reviewCategory']): string {
  if (category === 'image') return '图'
  if (category === 'video') return '片'
  if (category === 'audio') return '声'
  return '文'
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
