import { useEffect, useMemo, useRef, useState, type UIEvent } from 'react'
import { getCreatorArtifactContent } from '../../../services/creatorApi'
import type { ShotListFilters, ShotListItem, ShotQueueStatus } from '../types'
import type { HistoricalShot } from '../completedShotProjection'
import { SHOT_QUEUE_ROW_HEIGHT, shotQueueWindow } from '../logic'

interface ShotReviewQueueProps {
  items: readonly ShotListItem[]
  total: number
  selectedShotId?: string
  filters: ShotListFilters
  loading?: boolean
  hasMore: boolean
  historicalShots?: readonly HistoricalShot[]
  selectedHistoricalShotId?: string
  onFiltersChange: (filters: ShotListFilters) => void
  onSelect: (shotId: string) => void
  onSelectHistorical: (shotId: string) => void
  onLoadMore: () => void
}

const FILTERS: readonly { value: ShotQueueStatus; label: string }[] = [
  { value: 'needs_attention', label: '需要处理' },
  { value: 'all', label: '全部' },
  { value: 'confirmed', label: '已确认' },
  { value: 'generating', label: '生成中' },
  { value: 'failed', label: '失败' },
]

export default function ShotReviewQueue({ items, total, selectedShotId, filters, loading = false, hasMore, historicalShots, selectedHistoricalShotId, onFiltersChange, onSelect, onSelectHistorical, onLoadMore }: ShotReviewQueueProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewportHeight, setViewportHeight] = useState(480)
  const [thumbnails, setThumbnails] = useState<Record<string, string | null>>({})
  const thumbnailCacheRef = useRef(new Map<string, string | null>())
  const chapters = useMemo(() => [...new Set(items.map(item => item.chapter).filter(Boolean))].sort(), [items])
  const window = shotQueueWindow({ total: items.length, scrollTop, viewportHeight })
  const visibleItems = useMemo(() => items.slice(window.start, window.end).filter((item): item is ShotListItem => Boolean(item?.thumbnailRef)), [items, window.start, window.end])

  useEffect(() => {
    const controller = new AbortController()
    const unresolved = visibleItems.filter(item => !thumbnailCacheRef.current.has(item.thumbnailRef!))
    setThumbnails(Object.fromEntries(visibleItems.map(item => [item.id, thumbnailCacheRef.current.get(item.thumbnailRef!) ?? null])))
    if (!unresolved.length) return () => controller.abort()
    let cursor = 0
    const resolveNext = async () => {
      while (!controller.signal.aborted && cursor < unresolved.length) {
        const item = unresolved[cursor++]
        try {
          const content = await getCreatorArtifactContent(item.thumbnailRef!, controller.signal)
          thumbnailCacheRef.current.set(item.thumbnailRef!, content.mediaUrl || content.mediaUrls?.[0] || null)
        } catch {
          if (!controller.signal.aborted) thumbnailCacheRef.current.set(item.thumbnailRef!, null)
        }
      }
    }
    void Promise.all(Array.from({ length: Math.min(3, unresolved.length) }, resolveNext)).then(() => {
      if (!controller.signal.aborted) setThumbnails(Object.fromEntries(visibleItems.map(item => [item.id, thumbnailCacheRef.current.get(item.thumbnailRef!) ?? null])))
    })
    return () => controller.abort()
  }, [visibleItems])

  const onScroll = (event: UIEvent<HTMLDivElement>) => {
    const target = event.currentTarget
    setScrollTop(target.scrollTop)
    setViewportHeight(target.clientHeight || 480)
    const lastVisible = Math.min(items.length, Math.ceil((target.scrollTop + target.clientHeight) / SHOT_QUEUE_ROW_HEIGHT))
    if (hasMore && lastVisible >= Math.max(0, items.length - 6)) onLoadMore()
  }
  const updateFilters = (next: ShotListFilters) => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0
    setScrollTop(0)
    onFiltersChange(next)
  }

  if (historicalShots?.length) {
    return (
      <aside className="shot-review-queue" aria-label="历史 Shot 回看">
        <div className="shot-review-queue-heading">
          <div>
            <p className="creator-eyebrow">历史任务回看</p>
            <h2>Shot 回看</h2>
          </div>
          <span>{historicalShots.length} 个</span>
        </div>
        <div className="shot-review-history-list">
          {historicalShots.map(shot => (
            <button
              key={shot.id}
              type="button"
              className={`shot-queue-row${selectedHistoricalShotId === shot.id ? ' is-selected' : ''}`}
              aria-current={selectedHistoricalShotId === shot.id ? 'true' : undefined}
              onClick={() => onSelectHistorical(shot.id)}
            >
              <span className="shot-queue-copy"><strong>Shot {shot.sequenceIndex}</strong><small>{shot.layers.join(' · ')}</small></span>
              <span className="shot-queue-status">历史回看</span>
            </button>
          ))}
        </div>
      </aside>
    )
  }

  return (
    <aside className="shot-review-queue" aria-label="Shot 审核队列" aria-busy={loading}>
      <div className="shot-review-queue-heading">
        <div>
          <p className="creator-eyebrow">逐条审核</p>
          <h2>Shot 队列</h2>
        </div>
        <span>{total} 个</span>
      </div>
      <div className="shot-queue-filters">
        <label>状态
          <select value={filters.status ?? 'needs_attention'} onChange={event => updateFilters({ ...filters, status: event.target.value as ShotQueueStatus })}>
            {FILTERS.map(filter => <option key={filter.value} value={filter.value}>{filter.label}</option>)}
          </select>
        </label>
        <label>章节
          <select value={filters.chapter ?? ''} onChange={event => updateFilters({ ...filters, chapter: event.target.value || undefined })}>
            <option value="">全部章节</option>
            {chapters.map(chapter => <option key={chapter} value={chapter}>{chapter}</option>)}
          </select>
        </label>
        <label className="shot-queue-search">查找
          <input value={filters.query ?? ''} onChange={event => updateFilters({ ...filters, query: event.target.value || undefined })} placeholder="标题或旁白" />
        </label>
      </div>
      <div className="shot-queue-scroll" ref={scrollRef} onScroll={onScroll} style={{ height: 480 }}>
        <div style={{ height: window.totalHeight, position: 'relative' }}>
          <div style={{ transform: `translateY(${window.offsetTop}px)` }}>
            {window.items.map(index => {
              const item = items[index]
              if (!item) return null
              const chapterStart = index === 0 || items[index - 1]?.chapter !== item.chapter
              return (
                <button
                  key={item.id}
                  type="button"
                  className={`shot-queue-row${selectedShotId === item.id ? ' is-selected' : ''}`}
                  aria-current={selectedShotId === item.id ? 'true' : undefined}
                  onClick={() => onSelect(item.id)}
                >
                  <span className="shot-queue-thumb" aria-hidden="true">{thumbnails[item.id] ? <img src={thumbnails[item.id] ?? ''} alt="" /> : <i>{item.thumbnailRef ? '预览' : '无预览'}</i>}</span>
                  <span className="shot-queue-copy">
                    {chapterStart && item.chapter && <small>{item.chapter}</small>}
                    <strong>Shot {item.sequenceIndex || index + 1} · {item.title || '未命名镜头'}</strong>
                  </span>
                  <span className={`shot-queue-status ${queueStatus(item) === '失败' ? 'is-failed' : `is-${item.generationStatus}`}`}>{queueStatus(item)}</span>
                </button>
              )
            })}
          </div>
        </div>
      </div>
      {loading && <p className="shot-queue-loading" aria-live="polite">正在更新队列…</p>}
      {!loading && items.length === 0 && <p className="artifact-empty">没有符合条件的 Shot。</p>}
    </aside>
  )
}

function queueStatus(item: ShotListItem): string {
  if (item.generationStatus === 'failed' || item.generationStatus === 'cancelled' || item.generationStatus === 'SHOT_QA_FAILED' || item.qaStatus === 'SHOT_QA_FAILED') return '失败'
  if (['queued', 'dispatching', 'running', 'GENERATING', 'SHOT_QA_RUNNING'].includes(item.generationStatus)) return '生成中'
  if (item.reviewStatus === 'approved') return '已确认'
  return '待审核'
}
