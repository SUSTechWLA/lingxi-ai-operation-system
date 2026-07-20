import { useEffect, useMemo, useRef, useState } from 'react'
import { acceptShotCandidate, getCreatorArtifactContent } from '../../../services/creatorApi'
import type { ShotListItem, ShotUnit, ShotWorkspace } from '../types'
import { canSubmitShotDuration, isCreatorConflict, SHOT_QUEUE_CONFLICT_COPY } from '../logic'
import ShotImprovePanel from './ShotImprovePanel'

interface ShotInspectorProps {
  projectId: string
  workspace: ShotWorkspace | null
  item?: ShotListItem
  previous?: ShotListItem
  next?: ShotListItem
  totalShots: number
  onShotChanged: (shot: ShotUnit) => Promise<void> | void
  onReload: () => Promise<void> | void
}

export default function ShotInspector({ projectId, workspace, item, previous, next, totalShots, onShotChanged, onReload }: ShotInspectorProps) {
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
      <div className="shot-candidate-tabs" role="tablist" aria-label="候选版本">{candidates.map(candidate => <button key={candidate.candidateId} type="button" role="tab" aria-selected={candidate.candidateId === selectedCandidate?.candidateId} onClick={() => setCandidateId(candidate.candidateId)}>候选 {candidate.attemptIndex || 1}</button>)}</div>
      <div className="shot-video-stage">
        {mediaState === 'loading' && <p className="artifact-empty">正在加载候选预览…</p>}
        {mediaUrl && <video className="artifact-video-preview" controls src={mediaUrl}>你的浏览器不支持视频预览。</video>}
        {mediaState === 'unavailable' && <p className="artifact-empty">预览不可用：当前候选没有可解析的视频产物。你仍可审核下方元数据。</p>}
      </div>
      <div className="shot-inspector-meta">
        <p><strong>质量检查</strong>{selectedCandidate?.qaReport?.summary || qualityCopy(selectedCandidate?.qaReport?.status || shot.qaStatus)}</p>
        <p><strong>旁白</strong>{shot.narration || '未填写旁白'}</p>
        <p><strong>时间范围</strong>{formatRange(shot)}</p>
        <p><strong>参考</strong>{candidateRefs.length ? `已关联 ${candidateRefs.length} 项视频产物` : '没有可用参考产物'}</p>
      </div>
      <div className="shot-neighbors"><p><strong>前后镜头</strong></p><span>上一镜：{previous ? `Shot ${previous.sequenceIndex} · ${previous.title}` : '无'}</span><span>下一镜：{next ? `Shot ${next.sequenceIndex} · ${next.title}` : '无'}</span></div>
      <div className="artifact-actions"><button type="button" className="creator-primary-button" disabled={!selectedCandidate || working || !durationValid} onClick={accept}>接受这个候选</button></div>
      {error && <p className="creator-form-error" role="alert">{error}</p>}
      <ShotImprovePanel projectId={projectId} workspace={workspace} item={item} totalShots={totalShots} onRegenerated={async () => onReload()} onConflict={onReload} />
    </section>
  )
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
