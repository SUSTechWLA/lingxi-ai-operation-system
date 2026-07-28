/* eslint-disable react-refresh/only-export-components -- director contract tests exercise the pure package helpers. */
import { useMemo, useState } from 'react'
import {
  retryLatestFailedAgentNode,
  selectLatestFailedAgentNode,
} from '../../../services/api'
import type { VideoProject } from '../../../utils/types'
import {
  redactDiagnosticValue,
  type ProjectDiagnosticsLoadResult,
} from '../developerDiagnostics'

type RecoveryAction = 'refresh' | 'retry' | 'copy' | 'download'

interface DiagnosticPackage {
  generatedAt: string
  project: { id: string; name: string }
  run: unknown
  task: unknown
  nodes: unknown[]
  reviews: unknown[]
  artifacts: unknown[]
  contexts: unknown[]
}

interface RecoveryPanelProps {
  project: Pick<VideoProject, 'id' | 'name'>
  diagnostics: ProjectDiagnosticsLoadResult
  onRefresh: () => Promise<void>
}

const MEDIA_BODY_REMOVED = '[MEDIA BODY REMOVED]'

const mediaBodyKeys = new Set([
  'base64',
  'blob',
  'body',
  'bytes',
  'content',
  'data',
  'filebody',
  'inlinejson',
  'mediabody',
  'raw',
  'rawcontent',
])

const diagnosticHashKeys = new Set(['checksum', 'contenthash', 'hash', 'prompthash'])

function isMediaDataUrl(value: string): boolean {
  const normalized = value.trimStart()
  return /^data:(?:image|audio|video)\//i.test(normalized) ||
    /^data:application\/(?:octet-stream|pdf|zip|x-zip-compressed)(?:[;,])/i.test(normalized)
}

function isLongEncodedBinary(value: string, key?: string): boolean {
  if (key && diagnosticHashKeys.has(key.toLowerCase())) return false
  const compact = value.replace(/\s/g, '')
  return compact.length >= 256 && compact.length % 4 === 0 && /^[A-Za-z0-9+/]+={0,2}$/.test(compact)
}

function isBinaryDiagnosticValue(value: object): boolean {
  try {
    if (value instanceof ArrayBuffer || ArrayBuffer.isView(value)) return true
    if (typeof Blob !== 'undefined' && value instanceof Blob) return true
  } catch {
    return true
  }
  return false
}

export function stripDiagnosticMediaBodies(
  value: unknown,
  key?: string,
  seen: WeakMap<object, unknown> = new WeakMap(),
): unknown {
  if (typeof value === 'string') {
    return isMediaDataUrl(value) || isLongEncodedBinary(value, key) ? MEDIA_BODY_REMOVED : value
  }
  if (!value || typeof value !== 'object') return value
  if (isBinaryDiagnosticValue(value)) return MEDIA_BODY_REMOVED
  if (value instanceof Date) return value.toISOString()

  const existing = seen.get(value)
  if (existing !== undefined) return existing

  let descriptors: PropertyDescriptorMap
  try {
    descriptors = Object.getOwnPropertyDescriptors(value)
  } catch {
    return MEDIA_BODY_REMOVED
  }

  const sanitized: unknown[] | Record<string, unknown> = Array.isArray(value) ? [] : {}
  seen.set(value, sanitized)
  for (const [propertyKey, descriptor] of Object.entries(descriptors)) {
    if (!descriptor.enumerable || mediaBodyKeys.has(propertyKey.toLowerCase())) continue
    if (!('value' in descriptor)) {
      if (!Array.isArray(sanitized)) sanitized[propertyKey] = '[Accessor]'
      continue
    }
    const nestedValue = stripDiagnosticMediaBodies(descriptor.value, propertyKey, seen)
    if (Array.isArray(sanitized)) {
      if (/^\d+$/.test(propertyKey)) sanitized[Number(propertyKey)] = nestedValue
      continue
    }
    sanitized[propertyKey] = nestedValue
  }
  return sanitized
}

export const selectLatestFailedNode = selectLatestFailedAgentNode

export function buildDiagnosticPackage({
  generatedAt = new Date().toISOString(),
  project,
  diagnostics,
}: {
  generatedAt?: string
  project: Pick<VideoProject, 'id' | 'name'>
  diagnostics: ProjectDiagnosticsLoadResult
}): DiagnosticPackage {
  const diagnosticPackage = {
    generatedAt,
    project: { id: project.id, name: project.name },
    run: diagnostics.run,
    task: diagnostics.task,
    nodes: diagnostics.nodes ?? [],
    reviews: diagnostics.reviews ?? [],
    artifacts: diagnostics.artifacts ?? [],
    contexts: diagnostics.context ?? [],
  }
  const packageWithoutMediaBodies = stripDiagnosticMediaBodies(diagnosticPackage)
  return redactDiagnosticValue(packageWithoutMediaBodies) as DiagnosticPackage
}

export function serializeDiagnosticPackage(diagnosticPackage: DiagnosticPackage): string {
  return JSON.stringify(redactDiagnosticValue(diagnosticPackage), null, 2) ?? 'null'
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : '未知错误'
}

export default function RecoveryPanel({ project, diagnostics, onRefresh }: RecoveryPanelProps) {
  const [pendingAction, setPendingAction] = useState<RecoveryAction | null>(null)
  const [feedback, setFeedback] = useState<{ tone: 'success' | 'failure'; message: string } | null>(null)
  const taskId = diagnostics.run?.taskId
  const latestFailedNode = useMemo(
    () => selectLatestFailedNode(diagnostics.nodes ?? []),
    [diagnostics.nodes],
  )
  const taskLabel = taskId ? `任务 ${taskId}` : '未关联任务'
  const nodeLabel = latestFailedNode
    ? `节点 ${latestFailedNode.name || latestFailedNode.id}（${latestFailedNode.id}）`
    : '无失败节点'
  const controlsDisabled = pendingAction !== null

  const runAction = async (action: RecoveryAction, operation: () => Promise<string>) => {
    if (pendingAction) return
    setPendingAction(action)
    setFeedback(null)
    try {
      const message = await operation()
      setFeedback({ tone: 'success', message })
    } catch (error) {
      setFeedback({ tone: 'failure', message: `${taskLabel} / ${nodeLabel}：${errorMessage(error)}` })
    } finally {
      setPendingAction(null)
    }
  }

  const refreshSnapshot = () => runAction('refresh', async () => {
    await onRefresh()
    return `${taskLabel}（运行 ${diagnostics.runId}）的快照已刷新；当前视图在请求期间保持可见。`
  })

  const retryLatestFailedNode = () => {
    if (!taskId || !latestFailedNode) return
    const confirmed = window.confirm(
      `确认重试${taskLabel}的${nodeLabel}吗？这只会重新排队该失败节点，不会重试其他节点。`,
    )
    if (!confirmed) return
    return runAction('retry', async () => {
      const result = await retryLatestFailedAgentNode(taskId, latestFailedNode.id)
      return `已为任务 ${result.taskId} 重新排队节点 ${latestFailedNode.name || result.nodeId}（${result.nodeId}）；诊断快照保持不变，请刷新查看进展。`
    })
  }

  const copyDiagnosticPackage = () => runAction('copy', async () => {
    const serialized = serializeDiagnosticPackage(buildDiagnosticPackage({ project, diagnostics }))
    await navigator.clipboard.writeText(serialized)
    return `已复制${taskLabel}（运行 ${diagnostics.runId}）的脱敏诊断包；未包含媒体文件正文或原始内容。`
  })

  const downloadDiagnosticPackage = () => runAction('download', async () => {
    const serialized = serializeDiagnosticPackage(buildDiagnosticPackage({ project, diagnostics }))
    const url = URL.createObjectURL(new Blob([serialized], { type: 'application/json;charset=utf-8' }))
    try {
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `diagnostics-${project.id}-${diagnostics.runId}.json`
      anchor.click()
    } finally {
      URL.revokeObjectURL(url)
    }
    return `已下载${taskLabel}（运行 ${diagnostics.runId}）的脱敏诊断 JSON；未包含媒体文件正文或原始内容。`
  })

  return (
    <section className="diagnostics-recovery-panel" aria-labelledby="diagnostics-recovery-title">
      <header className="diagnostics-recovery-heading">
        <div>
          <p>Scoped recovery</p>
          <h3 id="diagnostics-recovery-title">范围恢复与诊断包</h3>
        </div>
        <span>{project.name}</span>
      </header>

      <dl className="diagnostics-recovery-scope">
        <div><dt>项目</dt><dd>{project.name}（{project.id}）</dd></div>
        <div><dt>运行</dt><dd>{diagnostics.runId}</dd></div>
        <div><dt>任务</dt><dd>{taskId ?? '未关联'}</dd></div>
        <div><dt>最新失败节点</dt><dd>{latestFailedNode ? `${latestFailedNode.name || latestFailedNode.id}（${latestFailedNode.id}）` : '无'}</dd></div>
      </dl>

      <div className="diagnostics-recovery-actions">
        <RecoveryActionButton
          disabled={controlsDisabled}
          pending={pendingAction === 'refresh'}
          label="刷新快照"
          pendingLabel="正在刷新快照…"
          description={`重新读取${taskLabel}（运行 ${diagnostics.runId}），请求期间保留当前快照。`}
          onClick={() => { void refreshSnapshot() }}
        />
        <RecoveryActionButton
          disabled={controlsDisabled || !taskId || !latestFailedNode}
          pending={pendingAction === 'retry'}
          label="重试最新失败节点"
          pendingLabel="正在重试节点…"
          description={latestFailedNode
            ? `确认后只重新排队${taskLabel}的${nodeLabel}。`
            : `${taskLabel}当前没有可重试的失败节点。`}
          onClick={() => { void retryLatestFailedNode() }}
          danger
        />
        <RecoveryActionButton
          disabled={controlsDisabled}
          pending={pendingAction === 'copy'}
          label="复制脱敏诊断包"
          pendingLabel="正在复制诊断包…"
          description={`复制${taskLabel}（运行 ${diagnostics.runId}）的脱敏 JSON，不含媒体正文。`}
          onClick={() => { void copyDiagnosticPackage() }}
        />
        <RecoveryActionButton
          disabled={controlsDisabled}
          pending={pendingAction === 'download'}
          label="下载脱敏诊断 JSON"
          pendingLabel="正在生成诊断 JSON…"
          description={`下载${taskLabel}（运行 ${diagnostics.runId}）的脱敏 JSON，不含媒体正文。`}
          onClick={() => { void downloadDiagnosticPackage() }}
        />
      </div>

      {feedback ? (
        <p className="diagnostics-recovery-feedback" data-tone={feedback.tone} role="status">
          {feedback.message}
        </p>
      ) : null}
    </section>
  )
}

function RecoveryActionButton({
  disabled,
  pending,
  label,
  pendingLabel,
  description,
  onClick,
  danger = false,
}: {
  disabled: boolean
  pending: boolean
  label: string
  pendingLabel: string
  description: string
  onClick: () => void
  danger?: boolean
}) {
  return (
    <div className="diagnostics-recovery-action" data-danger={danger || undefined}>
      <button type="button" disabled={disabled} onClick={onClick}>{pending ? pendingLabel : label}</button>
      <p>{description}</p>
    </div>
  )
}
