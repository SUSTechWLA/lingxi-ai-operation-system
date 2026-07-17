import { buildTalkingHeadLayerDisplays } from '../selectors'
import type { TalkingHeadArtifactLike, TalkingHeadLayerStatus } from '../types'

const statusLabels: Record<TalkingHeadLayerStatus, string> = {
  current: 'current',
  planned: 'planned',
  stale: 'stale',
  pending: 'pending',
  failed: 'failed',
}

export function TalkingHeadLayerInspector({ artifacts }: { artifacts: TalkingHeadArtifactLike[] }) {
  const layers = buildTalkingHeadLayerDisplays(artifacts)
  return (
    <section className="rounded-lg border border-line bg-background-card p-4" aria-label="Talking head layer inspector">
      <div className="text-xs font-black text-primary-dark">分层状态 · 后端 authoritative state</div>
      <div className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
        {layers.map((layer) => (
          <div key={layer.key} className="rounded-lg bg-white p-3 ring-1 ring-line">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-black text-ink">{layer.label}</span>
              <span className="rounded-full bg-background-card px-2 py-0.5 text-[10px] font-black text-ink-muted">{statusLabels[layer.status]}</span>
            </div>
            <div className="mt-2 text-[11px] leading-5 text-ink-muted">
              {layer.staleReason || layer.revision || '等待结构化 layer artifact'}
            </div>
            {layer.executionMode && layer.executionMode !== 'real' ? (
              <div className="mt-2 text-[10px] font-black text-warning">{layer.executionMode} · 不可直接进入 strict production</div>
            ) : null}
          </div>
        ))}
      </div>
    </section>
  )
}
