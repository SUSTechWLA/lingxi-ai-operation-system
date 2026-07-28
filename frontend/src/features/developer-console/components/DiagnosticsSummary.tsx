import { useMemo, useState } from 'react'
import type { VideoProject } from '../../../utils/types'
import { diagnosticDurationMs, type ProjectDiagnosticsLoadResult } from '../developerDiagnostics'

interface DiagnosticsSummaryProps {
  project: VideoProject
  diagnostics: ProjectDiagnosticsLoadResult
}

interface DiagnosticsProjectSelectorProps {
  projects: VideoProject[]
  selectedProjectId: string
  onSelect: (projectId: string) => void
}

const timestampFormatter = new Intl.DateTimeFormat('zh-CN', {
  dateStyle: 'medium',
  timeStyle: 'medium',
})

function formatTimestamp(value: string): string {
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestampFormatter.format(timestamp) : '未知'
}

function formatDuration(durationMs: number | undefined): string {
  if (durationMs === undefined) return '计算中'
  if (durationMs < 1000) return `${durationMs} ms`

  const totalSeconds = Math.floor(durationMs / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return `${hours} 小时 ${minutes} 分 ${seconds} 秒`
  if (minutes > 0) return `${minutes} 分 ${seconds} 秒`
  return `${seconds} 秒`
}

function normalizedStatus(status: string): string {
  return status.trim().toUpperCase() || 'UNKNOWN'
}

export function DiagnosticsProjectSelector({
  projects,
  selectedProjectId,
  onSelect,
}: DiagnosticsProjectSelectorProps) {
  if (projects.length === 0) return null
  const selectedProject = projects.find((project) => project.id === selectedProjectId)

  return (
    <div className="diagnostics-project-selector">
      <label htmlFor="diagnostics-project">
        诊断项目
        <select
          id="diagnostics-project"
          value={selectedProjectId}
          onChange={(event) => onSelect(event.target.value)}
        >
          {projects.map((project) => (
            <option key={project.id} value={project.id}>
              {project.name}{project.currentRunId ? '' : '（无运行）'}
            </option>
          ))}
        </select>
      </label>
      {selectedProject?.currentRunId ? (
        <p title={selectedProject.currentRunId}>Run: {selectedProject.currentRunId}</p>
      ) : null}
    </div>
  )
}

export default function DiagnosticsSummary({ project, diagnostics }: DiagnosticsSummaryProps) {
  const { run } = diagnostics
  const [copiedId, setCopiedId] = useState('')
  const nodeStatusTotals = useMemo(() => {
    const totals = new Map<string, number>()
    for (const node of diagnostics.nodes ?? []) {
      const status = normalizedStatus(node.status)
      totals.set(status, (totals.get(status) ?? 0) + 1)
    }
    return [...totals].sort(([left], [right]) => left.localeCompare(right))
  }, [diagnostics.nodes])
  if (!run) return null

  const pendingGateCount = diagnostics.reviews?.filter((review) => review.status === 'PENDING').length ?? 0
  const failedNodeCount = diagnostics.nodes?.filter((node) => normalizedStatus(node.status) === 'FAILED').length ?? 0
  const elapsedMs = diagnosticDurationMs(run.createdAt, run.updatedAt)

  const copyId = async (value: string) => {
    if (!navigator.clipboard?.writeText) return
    try {
      await navigator.clipboard.writeText(value)
      setCopiedId(value)
    } catch {
      setCopiedId('')
    }
  }

  return (
    <div className="diagnostics-summary">
      <div className="diagnostics-summary-heading">
        <div className="min-w-0">
          <p>项目</p>
          <h3>{project.name}</h3>
        </div>
        <span className="diagnostics-status-pill" data-status={normalizedStatus(run.status)}>
          运行状态 · {run.status}
        </span>
      </div>

      <div className="diagnostics-id-list" aria-label="运行标识">
        <CopyableIdentifier label="项目 ID" value={project.id} copied={copiedId === project.id} onCopy={copyId} />
        <CopyableIdentifier label="运行 ID" value={run.id} copied={copiedId === run.id} onCopy={copyId} />
        <CopyableIdentifier label="任务 ID" value={run.taskId ?? '未关联'} copied={Boolean(run.taskId) && copiedId === run.taskId} onCopy={run.taskId ? copyId : undefined} />
      </div>

      <dl className="diagnostics-metric-grid">
        <DiagnosticsMetric label="耗时" value={formatDuration(elapsedMs)} />
        <DiagnosticsMetric label="节点总数" value={String(diagnostics.nodes?.length ?? 0)} />
        <DiagnosticsMetric label="待处理门禁" value={String(pendingGateCount)} tone={pendingGateCount > 0 ? 'warning' : 'success'} />
        <DiagnosticsMetric label="失败节点" value={String(failedNodeCount)} tone={failedNodeCount > 0 ? 'danger' : 'success'} />
        <DiagnosticsMetric label="产物" value={String(diagnostics.artifacts?.length ?? 0)} />
        <DiagnosticsMetric label="审核记录" value={String(diagnostics.reviews?.length ?? 0)} />
      </dl>

      <div className="diagnostics-summary-details">
        <dl>
          <div>
            <dt>创建时间</dt>
            <dd>{formatTimestamp(run.createdAt)}</dd>
          </div>
          <div>
            <dt>更新时间</dt>
            <dd>{formatTimestamp(run.updatedAt)}</dd>
          </div>
        </dl>
        <div>
          <p>节点状态</p>
          <div className="diagnostics-status-totals">
            {nodeStatusTotals.length > 0 ? nodeStatusTotals.map(([status, total]) => (
              <span key={status} data-status={status}>{status} <strong>{total}</strong></span>
            )) : <span data-status="UNKNOWN">暂无节点</span>}
          </div>
        </div>
      </div>
    </div>
  )
}

function CopyableIdentifier({
  label,
  value,
  copied,
  onCopy,
}: {
  label: string
  value: string
  copied: boolean
  onCopy?: (value: string) => Promise<void>
}) {
  return (
    <div>
      <span>{label}</span>
      <code title={value}>{value}</code>
      {onCopy ? (
        <button type="button" onClick={() => { void onCopy(value) }} aria-label={`复制${label}`}>
          {copied ? '已复制' : '复制'}
        </button>
      ) : null}
    </div>
  )
}

function DiagnosticsMetric({
  label,
  value,
  tone = 'neutral',
}: {
  label: string
  value: string
  tone?: 'neutral' | 'success' | 'warning' | 'danger'
}) {
  return (
    <div data-tone={tone}>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}
