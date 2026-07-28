/* eslint-disable react-refresh/only-export-components -- source-level diagnostics tests exercise the pure safety helpers. */
import { useMemo, useState } from 'react'
import {
  redactDiagnosticValue,
  serializeRedactedDiagnosticValue,
  type DiagnosticsNode,
} from '../developerDiagnostics'

export type ToolCallFilter = 'all' | 'tool' | 'mcp' | 'failed' | 'retried'

export interface ToolCallGroup {
  key: string
  label: string
  calls: DiagnosticsNode[]
}

interface ToolCallInspectorProps {
  nodes: DiagnosticsNode[]
}

const filters: ReadonlyArray<{ value: ToolCallFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'tool', label: '工具' },
  { value: 'mcp', label: 'MCP' },
  { value: 'failed', label: '失败' },
  { value: 'retried', label: '已重试' },
]

function normalizedStatus(status: string): string {
  return status.trim().toUpperCase() || 'UNKNOWN'
}

function errorClassForCall(call: DiagnosticsNode): string | undefined {
  if (call.errorClass) return call.errorClass
  if (!call.error) return undefined
  const firstLine = call.error.split('\n', 1)[0].trim()
  return firstLine.match(/^([A-Za-z_$][\w.$]*(?:Error|Exception|Failure|Timeout))(?=\s*:|\s|$)/)?.[1] ?? 'Error'
}

function formattedRedactedJson(value: unknown): string {
  const serialized = serializeRedactedDiagnosticValue(value)
  try {
    return JSON.stringify(JSON.parse(serialized), null, 2)
  } catch {
    return serialized
  }
}

function isInspectableCall(call: DiagnosticsNode): boolean {
  return call.transport === 'tool' || call.transport === 'mcp'
}

function safeSearchField(value: string | undefined): string {
  if (!value) return ''
  const redacted = redactDiagnosticValue(value)
  return typeof redacted === 'string' ? redacted : ''
}

function searchableCallMetadata(call: DiagnosticsNode): string {
  return [
    call.toolName,
    call.serverName,
    call.id,
    call.name,
    errorClassForCall(call),
  ].map(safeSearchField).join('\n').toLocaleLowerCase()
}

export function filterToolCalls(
  nodes: readonly DiagnosticsNode[],
  filter: ToolCallFilter,
  query: string,
): DiagnosticsNode[] {
  const search = query.trim().toLocaleLowerCase()
  return nodes.filter((call) => {
    if (!isInspectableCall(call)) return false
    if (filter === 'tool' && call.transport !== 'tool') return false
    if (filter === 'mcp' && call.transport !== 'mcp') return false
    if (filter === 'failed' && normalizedStatus(call.status) !== 'FAILED') return false
    if (filter === 'retried' && call.retryCount < 1) return false
    return !search || searchableCallMetadata(call).includes(search)
  })
}

export function buildToolCallGroups(nodes: readonly DiagnosticsNode[]): ToolCallGroup[] {
  const groups = new Map<string, ToolCallGroup>()
  for (const call of nodes) {
    if (!isInspectableCall(call)) continue
    const serverName = call.serverName || '未知服务'
    const key = call.transport === 'mcp' ? `mcp:${serverName}` : 'tool'
    const existing = groups.get(key)
    if (existing) {
      existing.calls.push(call)
      continue
    }
    groups.set(key, {
      key,
      label: call.transport === 'mcp' ? `MCP · ${serverName}` : '工具调用',
      calls: [call],
    })
  }
  return [...groups.values()]
}

function copyPayloadForCall(call: DiagnosticsNode): Record<string, unknown> {
  return {
    name: call.toolName || call.name,
    serverName: call.serverName,
    node: { id: call.id, name: call.name },
    transport: call.transport,
    status: call.status,
    durationMs: call.durationMs,
    retryCount: call.retryCount,
    errorClass: errorClassForCall(call),
    request: call.request,
    response: call.response,
    error: call.error,
    timestamps: { startedAt: call.startedAt, completedAt: call.completedAt },
    relatedArtifactIds: call.relatedArtifactIds ?? [],
  }
}

export function toolCallCopyText(call: DiagnosticsNode): string {
  return formattedRedactedJson(copyPayloadForCall(call))
}

function formatDuration(durationMs?: number): string {
  if (durationMs === undefined) return '—'
  if (durationMs < 1000) return `${durationMs} ms`
  return `${(durationMs / 1000).toFixed(durationMs % 1000 === 0 ? 0 : 1)} s`
}

function callDisplayName(call: DiagnosticsNode): string {
  const toolName = call.toolName || call.name
  return call.transport === 'mcp' && call.serverName
    ? `${call.serverName} / ${toolName}`
    : toolName
}

export default function ToolCallInspector({ nodes }: ToolCallInspectorProps) {
  const [filter, setFilter] = useState<ToolCallFilter>('all')
  const [query, setQuery] = useState('')
  const filteredCalls = useMemo(() => filterToolCalls(nodes, filter, query), [filter, nodes, query])
  const groups = useMemo(() => buildToolCallGroups(filteredCalls), [filteredCalls])

  return (
    <section className="diagnostics-tool-inspector" aria-labelledby="tool-call-inspector-title">
      <div className="diagnostics-tool-heading">
        <div>
          <p>调用检查器</p>
          <h3 id="tool-call-inspector-title">工具与 MCP 调用</h3>
        </div>
        <span role="status">{filteredCalls.length} 条</span>
      </div>

      <div className="diagnostics-tool-controls">
        <div className="diagnostics-tool-filters" role="group" aria-label="按调用类型筛选">
          {filters.map((item) => (
            <button
              key={item.value}
              type="button"
              aria-pressed={filter === item.value}
              onClick={() => setFilter(item.value)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <label className="diagnostics-tool-search">
          <span className="sr-only">搜索工具、服务、节点或错误名称</span>
          <input
            type="search"
            value={query}
            placeholder="搜索工具、服务、节点或错误"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
      </div>

      {groups.length === 0 ? (
        <div className="diagnostics-tool-empty" role="status">
          <p>没有匹配的工具调用</p>
          <span>尝试调整筛选条件或搜索词。</span>
        </div>
      ) : groups.map((group) => (
        <section className="diagnostics-tool-group" key={group.key} aria-label={group.label}>
          <h4>{group.label} <span>{group.calls.length}</span></h4>
          <div className="diagnostics-tool-list">
            {group.calls.map((call) => <ToolCallRow call={call} key={call.id} />)}
          </div>
        </section>
      ))}
    </section>
  )
}

function ToolCallRow({ call }: { call: DiagnosticsNode }) {
  const status = normalizedStatus(call.status)
  const errorClass = errorClassForCall(call)
  return (
    <details className="diagnostics-tool-call" data-status={status}>
      <summary>
        <span className="diagnostics-tool-name">
          <strong>{callDisplayName(call)}</strong>
          <code title={call.id}>{call.name} · {call.id}</code>
        </span>
        <span className="diagnostics-tool-row-metadata">
          <span aria-label={`传输：${call.transport ?? 'unknown'}`}>传输 {call.transport?.toUpperCase() ?? 'UNKNOWN'}</span>
          <span className="diagnostics-status-pill" data-status={status} aria-label={`状态：${call.status}`}>{call.status}</span>
          <span aria-label={`耗时：${formatDuration(call.durationMs)}`}>耗时 {formatDuration(call.durationMs)}</span>
          <span aria-label={`重试次数：${call.retryCount}`}>重试 {call.retryCount}</span>
          <span aria-label={`错误类：${errorClass ?? '无'}`}>错误 {errorClass ?? '—'}</span>
        </span>
      </summary>
      <div className="diagnostics-tool-expanded">
        <div className="diagnostics-tool-expanded-heading">
          <p>展开内容均经过统一脱敏处理。</p>
          <CopyJsonButton label="复制整条调用" value={copyPayloadForCall(call)} />
        </div>
        <div className="diagnostics-tool-payloads">
          <PayloadPanel label="请求" value={call.request} />
          <PayloadPanel label="响应" value={call.response} />
          <PayloadPanel label="错误" value={call.error ?? null} />
          <PayloadPanel label="时间戳" value={{ startedAt: call.startedAt, completedAt: call.completedAt }} />
          <PayloadPanel label="关联产物 ID" value={call.relatedArtifactIds ?? []} />
        </div>
      </div>
    </details>
  )
}

function PayloadPanel({ label, value }: { label: string; value: unknown }) {
  const json = useMemo(() => formattedRedactedJson(value), [value])
  return (
    <section className="diagnostics-tool-payload" aria-label={`${label}，已脱敏 JSON`}>
      <header>
        <h5>{label}</h5>
        <CopyJsonButton label={`复制${label}`} value={value} />
      </header>
      <pre tabIndex={0}><code>{json}</code></pre>
    </section>
  )
}

function CopyJsonButton({ label, value }: { label: string; value: unknown }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(formattedRedactedJson(value))
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }
  return (
    <button type="button" onClick={() => { void copy() }} aria-label={`${label}（已脱敏 JSON）`}>
      {copied ? '已复制' : label}
    </button>
  )
}
