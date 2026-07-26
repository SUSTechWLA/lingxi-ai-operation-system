/* eslint-disable react-refresh/only-export-components -- diagnostics tests exercise the pure registry helpers. */
import { useEffect, useMemo, useRef, useState, type SyntheticEvent, type ReactNode } from 'react'
import { fetchArtifactContent, fetchArtifactHistory } from '../../../services/api'
import type { Artifact, ArtifactContentResponse } from '../../../utils/types'
import {
  redactDiagnosticValue,
  serializeRedactedDiagnosticValue,
} from '../developerDiagnostics'

export type ArtifactCurrencyFilter = 'all' | 'current' | 'history'
export type ArtifactFacingFilter = 'all' | 'creator' | 'technical'

export interface ArtifactRegistryFilters {
  currency: ArtifactCurrencyFilter
  kind: string
  stage: string
  status: string
  facing: ArtifactFacingFilter
  query: string
}

export interface ArtifactLineageEntry {
  id: string
  parentId: string | undefined
  version: number
  isCurrent: boolean
  relationship: 'current' | 'selected' | 'parent' | 'history'
}

interface ArtifactRegistryProps {
  artifacts: Artifact[]
  scopeKey: string
}

const defaultFilters: ArtifactRegistryFilters = {
  currency: 'all',
  kind: 'all',
  stage: 'all',
  status: 'all',
  facing: 'all',
  query: '',
}

function metadataRecord(artifact: Artifact): Record<string, unknown> {
  return artifact.metadata && typeof artifact.metadata === 'object' ? artifact.metadata : {}
}

function stringMetadata(artifact: Artifact, keys: readonly string[]): string | undefined {
  const metadata = metadataRecord(artifact)
  for (const key of keys) {
    const value = metadata[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return undefined
}

function artifactStatus(artifact: Artifact): string {
  return artifact.status?.trim() || stringMetadata(artifact, ['status', 'artifactStatus']) || (artifact.isCurrent ? 'current' : 'stale')
}

function artifactIsStale(artifact: Artifact): boolean {
  const metadata = metadataRecord(artifact)
  return !artifact.isCurrent || metadata.stale === true || artifactStatus(artifact).toLocaleLowerCase() === 'stale'
}

function isCreatorFacing(artifact: Artifact): boolean {
  const metadata = metadataRecord(artifact)
  if (typeof metadata.creatorFacing === 'boolean') return metadata.creatorFacing
  if (typeof metadata.creator_facing === 'boolean') return metadata.creator_facing
  const audience = stringMetadata(artifact, ['audience', 'visibility', 'surface'])?.toLocaleLowerCase()
  return audience === 'creator' || audience === 'creator-facing' || audience === 'creator_facing'
}

function artifactProducer(artifact: Artifact): string {
  return [artifact.producedByRole, artifact.producedByTool, artifact.producedByNode]
    .filter((value): value is string => Boolean(value?.trim()))
    .join(' / ') || artifact.provider || artifact.model || '—'
}

function safeDisplayText(value: unknown): string {
  if (value === undefined || value === null || value === '') return '—'
  const redacted = redactDiagnosticValue(value)
  return typeof redacted === 'string' ? redacted : serializeRedactedDiagnosticValue(redacted)
}

function formattedRedactedJson(value: unknown): string {
  const serialized = serializeRedactedDiagnosticValue(value)
  try {
    return JSON.stringify(JSON.parse(serialized), null, 2)
  } catch {
    return serialized
  }
}

function artifactSearchText(artifact: Artifact): string {
  return [safeDisplayText(artifact.name), safeDisplayText(artifact.id)].join('\n').toLocaleLowerCase()
}

export function filterArtifactRegistryRows(
  artifacts: readonly Artifact[],
  filters: ArtifactRegistryFilters,
): Artifact[] {
  const query = filters.query.trim().toLocaleLowerCase()
  return artifacts
    .filter((artifact) => {
      if (filters.currency === 'current' && !artifact.isCurrent) return false
      if (filters.currency === 'history' && artifact.isCurrent) return false
      if (filters.kind !== 'all' && artifact.kind !== filters.kind) return false
      if (filters.stage !== 'all' && artifact.stageName !== filters.stage) return false
      if (filters.status !== 'all' && artifactStatus(artifact).toLocaleLowerCase() !== filters.status.toLocaleLowerCase()) return false
      if (filters.facing === 'creator' && !isCreatorFacing(artifact)) return false
      if (filters.facing === 'technical' && isCreatorFacing(artifact)) return false
      return !query || artifactSearchText(artifact).includes(query)
    })
    .map((artifact, index) => ({ artifact, index }))
    .sort((left, right) => {
      const leftTime = Date.parse(left.artifact.createdAt)
      const rightTime = Date.parse(right.artifact.createdAt)
      const timeDifference = (Number.isFinite(rightTime) ? rightTime : 0) - (Number.isFinite(leftTime) ? leftTime : 0)
      return timeDifference || left.index - right.index
    })
    .map(({ artifact }) => artifact)
}

export function buildArtifactLineage(
  artifact: Artifact,
  history: readonly Artifact[],
): ArtifactLineageEntry[] {
  const unique = new Map<string, Artifact>()
  unique.set(artifact.id, artifact)
  for (const version of history) unique.set(version.id, version)

  const parentIds = new Set<string>()
  let parentId = artifact.parentId
  while (parentId && !parentIds.has(parentId)) {
    parentIds.add(parentId)
    parentId = unique.get(parentId)?.parentId
  }

  return [...unique.values()]
    .sort((left, right) => right.version - left.version || left.id.localeCompare(right.id))
    .map((version) => ({
      id: version.id,
      parentId: version.parentId,
      version: version.version,
      isCurrent: version.isCurrent,
      relationship: version.id === artifact.id
        ? artifact.isCurrent ? 'current' : 'selected'
        : parentIds.has(version.id)
          ? 'parent'
          : version.isCurrent
            ? 'current'
            : 'history',
    }))
}

export default function ArtifactRegistry({ artifacts, scopeKey }: ArtifactRegistryProps) {
  const [filters, setFilters] = useState<ArtifactRegistryFilters>(defaultFilters)
  const kinds = useMemo(() => [...new Set(artifacts.map((artifact) => artifact.kind))].sort(), [artifacts])
  const stages = useMemo(() => [...new Set(artifacts.map((artifact) => artifact.stageName))].sort(), [artifacts])
  const statuses = useMemo(() => [...new Set(artifacts.map(artifactStatus))].sort(), [artifacts])
  const filteredArtifacts = useMemo(() => filterArtifactRegistryRows(artifacts, filters), [artifacts, filters])

  return (
    <section className="diagnostics-artifact-registry" aria-labelledby="artifact-registry-title">
      <div className="diagnostics-artifact-heading">
        <div><p>技术产物注册表</p><h3 id="artifact-registry-title">产物、版本与血缘</h3></div>
        <span role="status">{filteredArtifacts.length} / {artifacts.length} 条</span>
      </div>

      <div className="diagnostics-artifact-filters" aria-label="产物筛选">
        <FilterSelect label="版本范围" value={filters.currency} options={[
          ['all', '当前与历史'], ['current', '仅当前'], ['history', '仅历史'],
        ]} onChange={(currency) => setFilters((current) => ({ ...current, currency: currency as ArtifactCurrencyFilter }))} />
        <FilterSelect label="类型" value={filters.kind} options={[['all', '全部类型'], ...kinds.map((kind) => [kind, kind])]} onChange={(kind) => setFilters((current) => ({ ...current, kind }))} />
        <FilterSelect label="阶段" value={filters.stage} options={[['all', '全部阶段'], ...stages.map((stage) => [stage, stage])]} onChange={(stage) => setFilters((current) => ({ ...current, stage }))} />
        <FilterSelect label="状态" value={filters.status} options={[['all', '全部状态'], ...statuses.map((status) => [status, status])]} onChange={(status) => setFilters((current) => ({ ...current, status }))} />
        <FilterSelect label="面向对象" value={filters.facing} options={[
          ['all', '全部产物'], ['creator', '创作者可见'], ['technical', '技术产物'],
        ]} onChange={(facing) => setFilters((current) => ({ ...current, facing: facing as ArtifactFacingFilter }))} />
        <label className="diagnostics-artifact-search"><span>名称或 ID</span><input type="search" value={filters.query} placeholder="搜索名称或 ID" onChange={(event) => setFilters((current) => ({ ...current, query: event.target.value }))} /></label>
      </div>

      {filteredArtifacts.length === 0 ? <div className="diagnostics-artifact-empty" role="status">没有匹配的产物。</div> : (
        <div className="diagnostics-artifact-list">
          <div className="diagnostics-artifact-columns" aria-hidden="true">
            {['类型 / 名称', '阶段 / 单元', '当前 / 陈旧 / 状态', '版本', '生产者', '父级 / 血缘', '内容哈希', '存储类型 / 引用', '创建时间'].map((label) => <span key={label}>{label}</span>)}
          </div>
          {filteredArtifacts.map((artifact) => <ArtifactRegistryRow artifact={artifact} key={`${scopeKey}:${artifact.id}`} scopeKey={scopeKey} />)}
        </div>
      )}
    </section>
  )
}

function FilterSelect({ label, value, options, onChange }: { label: string; value: string; options: string[][]; onChange: (value: string) => void }) {
  return <label><span>{label}</span><select value={value} onChange={(event) => onChange(event.target.value)}>{options.map(([optionValue, optionLabel]) => <option value={optionValue} key={optionValue}>{optionLabel}</option>)}</select></label>
}

function ArtifactRegistryRow({ artifact, scopeKey }: { artifact: Artifact; scopeKey: string }) {
  const controllerRef = useRef<AbortController | null>(null)
  const [content, setContent] = useState<ArtifactContentResponse>()
  const [history, setHistory] = useState<Artifact[]>()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const cancelHydration = () => {
    const controller = controllerRef.current
    if (controller) controller.abort()
    controllerRef.current = null
  }

  useEffect(() => () => cancelHydration(), [artifact.id, scopeKey])

  const toggle = (event: SyntheticEvent<HTMLDetailsElement>) => {
    cancelHydration()
    setContent(undefined)
    setHistory(undefined)
    setError('')
    if (!event.currentTarget.open) { setLoading(false); return }

    const controller = new AbortController()
    controllerRef.current = controller
    setLoading(true)
    void Promise.allSettled([
      fetchArtifactContent(artifact.id, controller.signal),
      fetchArtifactHistory(artifact.id, controller.signal),
    ]).then(([contentResult, historyResult]) => {
      if (controller.signal.aborted || controllerRef.current !== controller) return
      if (contentResult.status === 'fulfilled') setContent(contentResult.value)
      if (historyResult.status === 'fulfilled') setHistory(historyResult.value.history ?? [])
      const failed = [contentResult, historyResult].filter((result) => result.status === 'rejected').length
      if (failed > 0) setError(failed === 2 ? '无法读取产物内容与版本历史。' : '部分展开数据暂时不可用。')
      setLoading(false)
    })
  }

  const status = artifactStatus(artifact)
  const lineage = history ? buildArtifactLineage(artifact, history) : []
  return (
    <details className="diagnostics-artifact-row" data-status={status.toUpperCase()} onToggle={toggle}>
      <summary className="diagnostics-artifact-grid">
        <ArtifactCell label="类型 / 名称"><strong>{safeDisplayText(artifact.kind)}</strong><span>{safeDisplayText(artifact.name)}</span><code>{safeDisplayText(artifact.id)}</code></ArtifactCell>
        <ArtifactCell label="阶段 / 单元"><strong>{safeDisplayText(artifact.stageName)}</strong><span>{safeDisplayText(artifact.unitId)}</span></ArtifactCell>
        <ArtifactCell label="当前 / 陈旧 / 状态"><span>{artifact.isCurrent ? '当前' : '历史'} · {artifactIsStale(artifact) ? '陈旧' : '有效'}</span><span className="diagnostics-status-pill" data-status={status.toUpperCase()}>{safeDisplayText(status)}</span></ArtifactCell>
        <ArtifactCell label="版本"><strong>v{artifact.version}</strong></ArtifactCell>
        <ArtifactCell label="生产者"><span>{safeDisplayText(artifactProducer(artifact))}</span></ArtifactCell>
        <ArtifactCell label="父级 / 血缘"><code>{safeDisplayText(artifact.parentId)}</code><span>{artifact.parentId ? '有父版本' : '根版本'}</span></ArtifactCell>
        <ArtifactCell label="内容哈希"><code>{safeDisplayText(artifact.contentHash)}</code></ArtifactCell>
        <ArtifactCell label="存储类型 / 引用"><strong>{safeDisplayText(artifact.storageType)}</strong><code>{safeDisplayText(artifact.storageRef)}</code></ArtifactCell>
        <ArtifactCell label="创建时间"><time dateTime={artifact.createdAt}>{safeDisplayText(artifact.createdAt)}</time></ArtifactCell>
      </summary>
      <div className="diagnostics-artifact-expanded">
        <p>内容与版本历史仅在展开时请求；以下技术 JSON 已统一脱敏。</p>
        {loading ? <p role="status">正在读取内容与版本历史…</p> : null}
        {error ? <p className="diagnostics-artifact-error" role="alert">{error}</p> : null}
        {content ? <RedactedJsonPanel label="产物内容" value={content} /> : null}
        {history ? <section className="diagnostics-artifact-lineage" aria-label="版本血缘"><h4>版本血缘</h4>{lineage.length === 0 ? <p>没有版本历史。</p> : <ol>{lineage.map((entry) => <li key={entry.id} data-relationship={entry.relationship}><strong>v{entry.version} · {entry.relationship}</strong><code>{safeDisplayText(entry.id)}</code><span>父级 {safeDisplayText(entry.parentId)}</span></li>)}</ol>}</section> : null}
      </div>
    </details>
  )
}

function ArtifactCell({ label, children }: { label: string; children: ReactNode }) {
  return <span className="diagnostics-artifact-cell" data-label={label}>{children}</span>
}

function RedactedJsonPanel({ label, value }: { label: string; value: unknown }) {
  const json = useMemo(() => formattedRedactedJson(value), [value])
  return <section className="diagnostics-artifact-json" aria-label={`${label}，已脱敏 JSON`}><h4>{label}</h4><pre tabIndex={0}><code>{json}</code></pre></section>
}
