import { useEffect, useMemo, useRef, useState } from 'react'
import { acceptShotCandidate, getCreatorArtifactContent } from '../../../services/creatorApi'
import type { ShotListItem, ShotRegenerationResult, ShotUnit, ShotWorkspace } from '../types'
import {
  projectHistoricalShotReview,
  type HistoricalShot,
  type HistoricalShotLoadedArtifact,
  type HistoricalShotReview,
} from '../completedShotProjection'
import { canSubmitShotDuration, isCreatorConflict, mapWithConcurrency, SHOT_QUEUE_CONFLICT_COPY } from '../logic'
import ShotImprovePanel from './ShotImprovePanel'
import SimpleAudioPlayer from './SimpleAudioPlayer'
import SimpleVideoPlayer from './SimpleVideoPlayer'

interface ShotInspectorProps {
  projectId: string
  workspace: ShotWorkspace | null
  historicalShot?: HistoricalShot
  item?: ShotListItem
  previous?: ShotListItem
  next?: ShotListItem
  totalShots: number
  onShotChanged: (shot: ShotUnit) => Promise<void> | void
  onReload: () => Promise<void> | void
  onRegenerationStarted: (result: ShotRegenerationResult) => Promise<void> | void
}

export default function ShotInspector({ projectId, workspace, historicalShot, item, previous, next, totalShots, onShotChanged, onReload, onRegenerationStarted }: ShotInspectorProps) {
  const [candidateId, setCandidateId] = useState('')
  const [mediaUrl, setMediaUrl] = useState<string | null>(null)
  const [mediaState, setMediaState] = useState<'idle' | 'loading' | 'unavailable'>('idle')
  const [working, setWorking] = useState(false)
  const [error, setError] = useState('')
  const controllerRef = useRef<AbortController | null>(null)
  const acceptControllerRef = useRef<AbortController | null>(null)
  const acceptKeyRef = useRef(new Map<string, string>())
  const currentShotKeyRef = useRef('')
  const mediaRequestRef = useRef(0)
  const shot = workspace?.shot
  const candidates = useMemo(() => shot?.candidates ?? [], [shot?.candidates])
  const selectedCandidate = candidates.find(candidate => candidate.candidateId === candidateId) ?? candidates[0]
  const durationValid = Boolean(shot && canSubmitShotDuration(shot.durationSec) && (!selectedCandidate || canSubmitShotDuration(selectedCandidate.durationSec)))
  const candidateRefs = useMemo(() => selectedCandidate ? videoArtifactIds(selectedCandidate.artifactRefs) : shot ? videoArtifactIds(shot.artifactRefs) : [], [selectedCandidate, shot])

  useEffect(() => {
    setCandidateId(current => candidates.some(candidate => candidate.candidateId === current) ? current : candidates[0]?.candidateId ?? '')
    setError('')
  }, [shot?.id, shot?.version, candidates])

  useEffect(() => {
    currentShotKeyRef.current = `${shot?.id ?? ''}:${shot?.version ?? 0}`
    acceptControllerRef.current?.abort()
    setWorking(false)
  }, [shot?.id, shot?.version])

  useEffect(() => {
    controllerRef.current?.abort()
    if (!candidateRefs.length) {
      setMediaUrl(null)
      setMediaState('unavailable')
      return
    }
    const controller = new AbortController()
    controllerRef.current = controller
    const requestToken = ++mediaRequestRef.current
    setMediaUrl(null)
    setMediaState('loading')
    void (async () => {
      for (const artifactId of candidateRefs) {
        try {
          const content = await getCreatorArtifactContent(artifactId, controller.signal)
          const resolved = content.mediaUrl || content.mediaUrls?.[0]
          if (resolved) return resolved
        } catch (caught) {
          if (controller.signal.aborted) throw caught
        }
      }
      return null
    })().then(resolved => {
      if (controller.signal.aborted || requestToken !== mediaRequestRef.current) return
      setMediaUrl(resolved)
      setMediaState(resolved ? 'idle' : 'unavailable')
    }).catch(() => {
      if (!controller.signal.aborted) setMediaState('unavailable')
    })
    return () => controller.abort()
  }, [candidateRefs])

  useEffect(() => () => { controllerRef.current?.abort(); acceptControllerRef.current?.abort() }, [])

  if (historicalShot) return <HistoricalShotInspector shot={historicalShot} />

  if (!workspace || !shot) return <section className="shot-inspector artifact-review-panel"><p className="artifact-empty">从左侧队列选择一个 Shot 开始审核。</p></section>

  const accept = () => {
    if (!selectedCandidate || !durationValid) {
      setError('这个 Shot 或候选时长无效（必须小于 15 秒），不能接受。')
      return
    }
    const operationKey = `${shot.id}:${selectedCandidate.candidateId}:${shot.version}`
    const idempotencyKey = acceptKeyRef.current.get(operationKey) ?? crypto.randomUUID()
    acceptKeyRef.current.set(operationKey, idempotencyKey)
    const currentShotKey = `${shot.id}:${shot.version}`
    const controller = new AbortController()
    acceptControllerRef.current?.abort()
    acceptControllerRef.current = controller
    setWorking(true)
    setError('')
    void acceptShotCandidate(projectId, shot.id, selectedCandidate.candidateId, {
      baseVersion: shot.version, scope: 'candidate_accept', locks: [],
    }, idempotencyKey, controller.signal).then(async accepted => {
      if (controller.signal.aborted || currentShotKeyRef.current !== currentShotKey) return
      await onShotChanged(accepted)
    }).catch(async caught => {
      if (controller.signal.aborted || currentShotKeyRef.current !== currentShotKey) return
      if (isCreatorConflict(caught)) {
        setError(SHOT_QUEUE_CONFLICT_COPY)
        await onReload()
      } else {
        setError('暂时无法接受这个候选，请稍后重试。')
      }
    }).finally(() => {
      if (!controller.signal.aborted && currentShotKeyRef.current === currentShotKey) setWorking(false)
    })
  }

  return (
    <section className="shot-inspector artifact-review-panel" aria-labelledby="shot-inspector-title">
      <div className="artifact-review-heading"><div><p className="creator-eyebrow">当前镜头</p><h2 id="shot-inspector-title">Shot {item?.sequenceIndex || shot.sequenceIndex} · {shot.title || '未命名镜头'}</h2></div><span className="artifact-state">v{shot.version}</span></div>
      {!durationValid && <p className="creator-form-error" role="alert">这个 Shot 时长无效（必须小于 15 秒），不能接受或重新生成。</p>}
      <div className="shot-candidate-tabs" role="tablist" aria-label="候选版本">{candidates.map((candidate, index) => <button key={candidate.candidateId} type="button" role="tab" aria-selected={candidate.candidateId === selectedCandidate?.candidateId} onClick={() => setCandidateId(candidate.candidateId)}>候选 {index + 1}</button>)}</div>
      <div className="shot-video-stage">
        {mediaState === 'loading' && <p className="artifact-empty">正在加载候选预览…</p>}
        {mediaUrl && <video className="artifact-video-preview" controls src={mediaUrl}>你的浏览器不支持视频预览。</video>}
        {mediaState === 'unavailable' && <p className="artifact-empty">预览不可用：当前候选没有可解析的视频产物。你仍可审核下方元数据。</p>}
      </div>
      <div className="shot-inspector-meta">
        <p><strong>质量检查</strong>{qualityCopy(selectedCandidate?.qaReport?.status || shot.qaStatus)}</p>
        <p><strong>旁白</strong>{shot.narration || '未填写旁白'}</p>
        <p><strong>时间范围</strong>{formatRange(shot)}</p>
        <p><strong>参考</strong>{candidateRefs.length ? `已关联 ${candidateRefs.length} 项视频产物` : '没有可用参考产物'}</p>
      </div>
      <div className="shot-neighbors"><p><strong>前后镜头</strong></p><span>上一镜：{previous ? `Shot ${previous.sequenceIndex} · ${previous.title}` : '无'}</span><span>下一镜：{next ? `Shot ${next.sequenceIndex} · ${next.title}` : '无'}</span></div>
      <div className="artifact-actions"><button type="button" className="creator-primary-button" disabled={!selectedCandidate || working || !durationValid} onClick={accept}>接受这个候选</button></div>
      {error && <p className="creator-form-error" role="alert">{error}</p>}
      <ShotImprovePanel projectId={projectId} workspace={workspace} item={item} totalShots={totalShots} onRegenerationStarted={onRegenerationStarted} onConflict={onReload} />
    </section>
  )
}

function HistoricalShotInspector({ shot }: { shot: HistoricalShot }) {
  const [loadedArtifacts, setLoadedArtifacts] = useState<HistoricalShotLoadedArtifact[]>([])
  const [loadState, setLoadState] = useState<'loading' | 'ready' | 'partial'>('loading')
  const [expandedImage, setExpandedImage] = useState<{ src: string; label: string } | null>(null)
  const artifactKey = shot.artifactIds.join('|')

  useEffect(() => {
    const controller = new AbortController()
    setLoadedArtifacts([])
    setLoadState('loading')
    setExpandedImage(null)
    void mapWithConcurrency(shot.artifactIds, 3, async artifactId => {
      try {
        const response = await getCreatorArtifactContent(artifactId, controller.signal)
        return {
          artifactId,
          content: response.content,
          reviewText: response.reviewText,
          mediaUrl: response.mediaUrl,
          mediaUrls: response.mediaUrls,
        } satisfies HistoricalShotLoadedArtifact
      } catch (caught) {
        if (controller.signal.aborted) throw caught
        return null
      }
    }).then(results => {
      if (controller.signal.aborted) return
      const available = results.filter((item): item is NonNullable<typeof item> => item !== null)
      setLoadedArtifacts(available)
      setLoadState(available.length === shot.artifactIds.length ? 'ready' : 'partial')
    }).catch(() => {
      if (!controller.signal.aborted) setLoadState('partial')
    })
    return () => controller.abort()
  }, [artifactKey, shot.artifactIds])

  useEffect(() => {
    if (!expandedImage) return
    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') setExpandedImage(null)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [expandedImage])

  const review = useMemo(() => projectHistoricalShotReview(shot, loadedArtifacts), [loadedArtifacts, shot])
  const videos = review.media.filter(item => item.kind === 'video')
  const images = review.media.filter(item => item.kind === 'image')
  const audio = review.media.filter(item => item.kind === 'audio')

  return (
    <section className="shot-inspector historical-shot-dossier artifact-review-panel" aria-labelledby="shot-inspector-title">
      <div className="artifact-review-heading historical-shot-heading">
        <div>
          <p className="creator-eyebrow">Shot 审核档案</p>
          <h2 id="shot-inspector-title">Shot {shot.sequenceIndex} · {review.title}</h2>
          <p>集中审阅这一镜的旁白、画面、三层设计和实际媒体。</p>
        </div>
        <span className="artifact-state">{review.durationSec !== undefined ? `${review.durationSec} 秒` : '已完成'}</span>
      </div>

      {loadState === 'loading' && <p className="artifact-empty" role="status">正在整理这一镜的创作内容…</p>}
      {loadState === 'partial' && <p className="historical-shot-read-notice" role="status">部分旧媒体无法读取，下面仍展示已经找回的脚本与画面设计。</p>}
      {loadState !== 'loading' && !review.hasReadableContent && <p className="artifact-empty">这个旧项目没有留下可读的 Shot 内容，可从“分镜与素材”步骤重新生成。</p>}

      {review.narration && <section className="historical-shot-narration" aria-labelledby="historical-shot-narration-title">
        <p className="creator-eyebrow">旁白</p>
        <h3 id="historical-shot-narration-title">这一镜说什么</h3>
        <blockquote>{review.narration}</blockquote>
      </section>}

      {(review.details.length > 0 || review.screenText.length > 0) && <section className="historical-shot-section" aria-labelledby="historical-shot-visual-title">
        <div className="historical-shot-section-heading">
          <div><p className="creator-eyebrow">镜头设计</p><h3 id="historical-shot-visual-title">画面与动作</h3></div>
          {review.screenText.length > 0 && <div className="historical-shot-screen-text" aria-label="画面文字">{review.screenText.map(text => <span key={text}>{text}</span>)}</div>}
        </div>
        <dl className="historical-shot-details">{review.details.map(item => <div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}</dl>
      </section>}

      <section className="historical-shot-section" aria-labelledby="historical-shot-layers-title">
        <p className="creator-eyebrow">画面分层</p>
        <h3 id="historical-shot-layers-title">三层如何配合</h3>
        <div className="historical-shot-layers">
          <HistoricalLayerCard index="01" title="IP A-roll" summary={layerSummary(review, 'ip')} empty="这个旧 Shot 没有留下角色口播设计说明。" />
          <HistoricalLayerCard index="02" title="文字层" summary={layerSummary(review, 'text')} empty="这个旧 Shot 没有留下文字动效设计说明。" />
          <HistoricalLayerCard index="03" title="补充画面" summary={layerSummary(review, 'enrichment')} empty="这个旧 Shot 没有留下补充素材设计说明。" />
        </div>
      </section>

      <section className="historical-shot-section historical-shot-media" aria-labelledby="historical-shot-media-title">
        <p className="creator-eyebrow">实际产物</p>
        <h3 id="historical-shot-media-title">视频、参考图与语音</h3>
        {videos.length > 0 ? <div className="historical-shot-videos">{videos.map(item => <article key={`${item.artifactId}:${item.url}`}><h4>{item.label}</h4><SimpleVideoPlayer src={item.url} title={`Shot ${shot.sequenceIndex} ${item.label}`} downloadName={`shot-${shot.sequenceIndex}.mp4`} /></article>)}</div> : <p className="artifact-empty">这一镜没有可单独播放的视频片段，可在“成片预览”查看完整视频。</p>}
        {images.length > 0 ? <div className="historical-shot-images">{images.map(item => <button key={`${item.artifactId}:${item.url}`} type="button" onClick={() => setExpandedImage({ src: item.url, label: item.label })} aria-label={`放大查看 ${item.label}`}><img src={item.url} alt={`Shot ${shot.sequenceIndex} ${item.label}`} /><span>{item.label} · 点击放大</span></button>)}</div> : <p className="artifact-empty">这一镜没有可预览的独立参考图。</p>}
        {audio.length > 0 ? <div className="historical-shot-audio">{audio.map(item => <SimpleAudioPlayer key={`${item.artifactId}:${item.url}`} src={item.url} title={`Shot ${shot.sequenceIndex} ${item.label}`} downloadName={`shot-${shot.sequenceIndex}-audio`} />)}</div> : <p className="artifact-empty">这一镜没有可单独播放的语音文件，旁白文字仍可在上方审核。</p>}
      </section>

      {expandedImage && <div className="historical-shot-image-backdrop" role="presentation" onPointerDown={event => { if (event.currentTarget === event.target) setExpandedImage(null) }}>
        <aside className="historical-shot-image-dialog" role="dialog" aria-modal="true" aria-label={`查看 ${expandedImage.label}`}>
          <button type="button" className="creator-secondary-button" onClick={() => setExpandedImage(null)}>关闭</button>
          <img src={expandedImage.src} alt={`放大的 ${expandedImage.label}`} />
        </aside>
      </div>}
    </section>
  )
}

function HistoricalLayerCard({ index, title, summary, empty }: { index: string; title: string; summary?: string; empty: string }) {
  return <article><span>{index}</span><h4>{title}</h4><p>{summary || empty}</p></article>
}

function layerSummary(review: HistoricalShotReview, key: HistoricalShotReview['layers'][number]['key']): string | undefined {
  return review.layers.find(layer => layer.key === key)?.summary
}

function qualityCopy(status: string | undefined): string {
  if (status === 'SHOT_QA_FAILED') return '质量检查未通过，需要进一步调整。'
  if (status === 'HUMAN_REVIEW_REQUIRED') return '需要人工确认。'
  if (status === 'SHOT_QA_PASSED') return '质量检查已通过。'
  return '尚未生成质量检查说明。'
}

function videoArtifactIds(refs: Record<string, string | undefined> | undefined): string[] {
  if (!refs) return []
  return [refs.compositedShotVideoArtifactId, refs.videoClipArtifactId, refs.htmlPreviewVideoArtifactId, refs.htmlOverlayVideoArtifactId, refs.aigcBackgroundVideoArtifactId].filter((value): value is string => Boolean(value))
}

function formatRange(shot: ShotWorkspace['shot']): string {
  if (shot.startMs !== undefined || shot.endMs !== undefined) return `${Math.round((shot.startMs ?? 0) / 1000)}s – ${Math.round((shot.endMs ?? shot.durationSec * 1000) / 1000)}s`
  return `${shot.startSec ?? 0}s – ${shot.endSec ?? shot.durationSec}s（时长 ${shot.durationSec}s）`
}
