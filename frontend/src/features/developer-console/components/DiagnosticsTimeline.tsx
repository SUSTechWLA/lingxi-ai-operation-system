import { useMemo, useState, type ReactNode } from 'react'
import {
  serializeRedactedDiagnosticValue,
  type DiagnosticsNode,
} from '../developerDiagnostics'

interface DiagnosticsTimelineProps {
  nodes: DiagnosticsNode[]
}

const timestampFormatter = new Intl.DateTimeFormat('zh-CN', {
  dateStyle: 'short',
  timeStyle: 'medium',
})

function normalizedStatus(status: string): string {
  return status.trim().toUpperCase() || 'UNKNOWN'
}

function formatTimestamp(value?: string): string {
  if (!value) return '—'
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestampFormatter.format(timestamp) : value
}

function formatDuration(durationMs?: number): string {
  if (durationMs === undefined) return '—'
  if (durationMs < 1000) return `${durationMs} ms`
  if (durationMs < 60_000) return `${(durationMs / 1000).toFixed(durationMs % 1000 === 0 ? 0 : 1)} s`
  const minutes = Math.floor(durationMs / 60_000)
  const seconds = Math.floor((durationMs % 60_000) / 1000)
  return `${minutes} min ${seconds} s`
}

function conciseErrorClass(error?: string): string | undefined {
  if (!error) return undefined
  const firstLine = error.split('\n', 1)[0].trim()
  const namedClass = firstLine.match(/^([A-Za-z_$][\w.$]*(?:Error|Exception|Failure|Timeout))(?=\s*:|\s|$)/)?.[1]
  return namedClass ?? 'Error'
}

function formattedDiagnosticJson(value: unknown): string {
  const serialized = serializeRedactedDiagnosticValue(value)
  try {
    return JSON.stringify(JSON.parse(serialized), null, 2)
  } catch {
    return serialized
  }
}

function highlightedJson(json: string, query: string): ReactNode {
  const search = query.trim()
  if (!search) return json

  const lowerJson = json.toLocaleLowerCase()
  const lowerSearch = search.toLocaleLowerCase()
  const parts: ReactNode[] = []
  let cursor = 0
  let match = lowerJson.indexOf(lowerSearch)
  let matchIndex = 0
  while (match !== -1) {
    parts.push(json.slice(cursor, match))
    parts.push(<mark key={`${match}-${matchIndex}`}>{json.slice(match, match + search.length)}</mark>)
    cursor = match + search.length
    matchIndex += 1
    match = lowerJson.indexOf(lowerSearch, cursor)
  }
  parts.push(json.slice(cursor))
  return parts
}

export default function DiagnosticsTimeline({ nodes }: DiagnosticsTimelineProps) {
  if (nodes.length === 0) {
    return (
      <div className="diagnostics-timeline-empty" role="status">
        <p>暂无轨迹节点</p>
        <span>此运行尚未返回可展示的节点数据。</span>
      </div>
    )
  }

  return (
    <ol className="diagnostics-timeline" aria-label="节点执行时间线">
      {nodes.map((node, index) => {
        const status = normalizedStatus(node.status)
        const errorClass = conciseErrorClass(node.error)
        return (
          <li key={node.id} data-status={status}>
            <span className="diagnostics-timeline-marker" aria-hidden="true" />
            <article>
              <header>
                <div className="min-w-0">
                  <span>节点 {index + 1} · {node.type}</span>
                  <h3>{node.name}</h3>
                  <code title={node.id}>{node.id}</code>
                </div>
                <strong className="diagnostics-status-pill" data-status={status}>{node.status}</strong>
              </header>

              <dl className="diagnostics-node-metadata">
                <div><dt>开始</dt><dd>{formatTimestamp(node.startedAt)}</dd></div>
                <div><dt>结束</dt><dd>{formatTimestamp(node.completedAt)}</dd></div>
                <div><dt>耗时</dt><dd>{formatDuration(node.durationMs)}</dd></div>
                <div><dt>重试</dt><dd>{node.retryCount}{node.maxRetry > 0 ? ` / ${node.maxRetry}` : ''}</dd></div>
              </dl>

              {errorClass ? (
                <div className="diagnostics-node-error" role="alert">
                  <strong>{errorClass}</strong>
                  <span>{node.error}</span>
                </div>
              ) : null}

              {node.request !== undefined || node.response !== undefined ? (
                <div className="diagnostics-json-blocks">
                  {node.request !== undefined ? <RawDiagnosticJson label="输入" value={node.request} /> : null}
                  {node.response !== undefined ? <RawDiagnosticJson label="输出" value={node.response} /> : null}
                </div>
              ) : null}
            </article>
          </li>
        )
      })}
    </ol>
  )
}

function RawDiagnosticJson({ label, value }: { label: string; value: unknown }) {
  const [query, setQuery] = useState('')
  const json = useMemo(() => formattedDiagnosticJson(value), [value])

  return (
    <details className="diagnostics-json-panel">
      <summary>{label} · 已脱敏 JSON</summary>
      <label>
        <span className="sr-only">搜索{label} JSON</span>
        <input
          type="search"
          value={query}
          placeholder="搜索 JSON"
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>
      <pre tabIndex={0}><code>{highlightedJson(json, query)}</code></pre>
    </details>
  )
}
