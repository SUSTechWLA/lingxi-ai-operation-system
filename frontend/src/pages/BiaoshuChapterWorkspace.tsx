import { useMemo } from 'react'
import { FiAlertTriangle, FiCheckCircle, FiEye, FiFileText, FiLayers } from 'react-icons/fi'

import type { BiaoshuArtifactRecord } from './biaoshuArtifactLogic'
import { buildChapterWorkspace, type BiaoshuChapterWorkspaceItem } from './biaoshuChapterWorkspaceLogic'

export function BiaoshuChapterWorkspace({
  artifacts,
  onView,
}: {
  artifacts: BiaoshuArtifactRecord[]
  onView: (artifact: BiaoshuArtifactRecord) => void
}) {
  const workspace = useMemo(() => buildChapterWorkspace(artifacts), [artifacts])

  return (
    <section className="space-y-4">
      <div className="rounded-xl border border-violet-200 bg-violet-50 p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex items-start gap-3">
            <div className="rounded-lg bg-violet-100 p-2 text-violet-700">
              <FiLayers className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-lg font-black text-violet-950">章节初稿工作区</h3>
              <p className="mt-1 text-sm text-violet-800">按章节管理初稿、扩写稿和字数达标状态，不再与项目产物混排。</p>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-2 text-center text-xs">
            <Metric label="章节" value={workspace.summary.total} tone="text-violet-800" />
            <Metric label="已达标" value={workspace.summary.ready} tone="text-emerald-700" />
            <Metric label="待扩写" value={workspace.summary.short} tone="text-amber-700" />
          </div>
        </div>
      </div>

      {workspace.chapters.length === 0 ? (
        <div className="rounded-xl border border-dashed border-line bg-white px-6 py-12 text-center">
          <FiFileText className="mx-auto h-7 w-7 text-ink-soft" />
          <p className="mt-3 font-bold text-ink">尚未生成章节初稿</p>
          <p className="mt-1 text-sm text-ink-muted">请先在项目产物中生成章节写作任务书，再执行分章撰写。</p>
        </div>
      ) : (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {workspace.chapters.map((chapter) => (
            <ChapterCard key={chapter.chapterNumber} chapter={chapter} onView={onView} />
          ))}
        </div>
      )}

      {workspace.unclassified.length > 0 && (
        <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
          <div className="flex items-center gap-2 font-bold"><FiAlertTriangle /> 待归档章节</div>
          <p className="mt-1 text-xs text-amber-800">有 {workspace.unclassified.length} 个旧章节产物缺少章节号，暂未混入章节列表。</p>
        </div>
      )}
    </section>
  )
}

function Metric({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div className="min-w-16 rounded-lg bg-white/80 px-3 py-2 ring-1 ring-violet-100">
      <div className="text-ink-muted">{label}</div>
      <div className={`mt-0.5 text-lg font-black ${tone}`}>{value}</div>
    </div>
  )
}

function ChapterCard({
  chapter,
  onView,
}: {
  chapter: BiaoshuChapterWorkspaceItem
  onView: (artifact: BiaoshuArtifactRecord) => void
}) {
  const health = chapter.health === 'short'
    ? { label: '字数不足', tone: 'bg-amber-50 text-amber-800 ring-amber-200', icon: <FiAlertTriangle /> }
    : chapter.health === 'ready'
      ? { label: '已达标', tone: 'bg-emerald-50 text-emerald-700 ring-emerald-200', icon: <FiCheckCircle /> }
      : chapter.health === 'failed'
        ? { label: '生成失败', tone: 'bg-red-50 text-red-700 ring-red-200', icon: <FiAlertTriangle /> }
        : { label: '待处理', tone: 'bg-stone-50 text-stone-700 ring-stone-200', icon: <FiFileText /> }

  return (
    <article className="rounded-xl border border-line bg-white p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-xs font-bold text-violet-700">第 {chapter.chapterNumber} 章</p>
          <h4 className="mt-1 line-clamp-2 font-black text-ink">{chapter.title}</h4>
        </div>
        <span className={`inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-1 text-xs font-bold ring-1 ${health.tone}`}>
          {health.icon}{health.label}
        </span>
      </div>
      <div className="mt-4 flex items-center justify-between text-xs text-ink-muted">
        <span>{chapter.wordCount ?? '-'} / {chapter.targetWords ?? '-'} 字</span>
        <span>{chapter.versions.length > 1 ? `${chapter.versions.length} 个版本` : '初稿'}</span>
      </div>
      <div className="mt-3 flex items-center justify-between gap-3 border-t border-line pt-3">
        <span className="truncate text-xs text-ink-soft" title={chapter.current.updatedAt}>{chapter.current.updatedAt}</span>
        <button
          onClick={() => onView(chapter.current)}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary-soft px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-primary-200 hover:bg-primary-100"
        >
          <FiEye /> 查看/修改
        </button>
      </div>
    </article>
  )
}
