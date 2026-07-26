/* eslint-disable react-refresh/only-export-components -- diagnostics tests exercise the pure gate mapping helper. */
import type { AgentReviewItem } from '../../../utils/types'
import { redactDiagnosticValue, type DiagnosticsNode } from '../developerDiagnostics'

export interface GateInspectionItem {
  id: string
  kind: 'human' | 'quality' | 'control'
  title: string
  status: 'PENDING' | 'APPROVED' | 'REJECTED'
  stage?: string
  blockingDownstream: boolean
  sourceArtifactId?: string
  sourceNodeId?: string
  reviewer?: string
  comment?: string
  time?: string
}

interface GateInspectorProps {
  reviews: AgentReviewItem[]
  nodes: DiagnosticsNode[]
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function safeString(value: unknown): string | undefined {
  if (typeof value !== 'string' || !value.trim()) return undefined
  const redacted = redactDiagnosticValue(value.trim())
  return typeof redacted === 'string' && redacted ? redacted : undefined
}

function firstString(records: readonly Record<string, unknown>[], keys: readonly string[]): string | undefined {
  for (const record of records) {
    for (const key of keys) {
      const value = safeString(record[key])
      if (value) return value
    }
  }
  return undefined
}

function gateStatus(status: string): GateInspectionItem['status'] {
  const normalized = status.trim().toUpperCase()
  if (['APPROVED', 'SUCCESS', 'SUCCEEDED', 'COMPLETED'].includes(normalized)) return 'APPROVED'
  if (['REJECTED', 'FAILED', 'FAILURE', 'CANCELLED'].includes(normalized)) return 'REJECTED'
  return 'PENDING'
}

function gateNodeKind(node: DiagnosticsNode): GateInspectionItem['kind'] | undefined {
  const signals = `${node.type} ${node.name}`.toLocaleLowerCase()
  if (signals.includes('quality')) return 'quality'
  if (node.transport === 'control' || /(?:^|[\s_.:/-])(control|gate|review)(?:$|[\s_.:/-])/.test(signals)) return 'control'
  return undefined
}

function nodeSourceArtifact(node: DiagnosticsNode): string | undefined {
  if (node.relatedArtifactIds?.[0]) return safeString(node.relatedArtifactIds[0])
  return firstString([recordValue(node.response), recordValue(node.request)], ['sourceArtifactId', 'artifactId'])
}

function nodeBlocksDownstream(node: DiagnosticsNode): boolean {
  for (const record of [recordValue(node.response), recordValue(node.request)]) {
    if (typeof record.blocksDownstream === 'boolean') return record.blocksDownstream
    if (typeof record.blockingDownstream === 'boolean') return record.blockingDownstream
  }
  return false
}

export function buildGateInspectionItems(
  reviews: readonly AgentReviewItem[],
  nodes: readonly DiagnosticsNode[],
): GateInspectionItem[] {
  const reviewItems = reviews.map((review): GateInspectionItem => {
    const raw = recordValue(review)
    const output = recordValue(review.reviewOutput)
    const humanReview = recordValue(review.humanReview)
    const kind = review.reviewPhase === 'quality_gate' ? 'quality' : 'human'
    const sourceArtifactId = firstString([raw, output], ['sourceArtifactId', 'artifactId']) ?? safeString(review.artifactId)
    const sourceNodeId = safeString(review.sourceNodeId) ?? safeString(review.nodeId)
    const reviewer = firstString([raw, output, humanReview], ['reviewerId', 'reviewer', 'reviewedBy', 'decisionBy'])
    const comment = firstString([raw, output, humanReview], ['reviewComment', 'comment', 'decisionComment'])
    const time = firstString([raw, output, humanReview], ['reviewedAt', 'createdAt', 'decidedAt', 'updatedAt'])
    const stage = safeString(review.stage)
    return {
      id: safeString(review.id) ?? 'unknown-review',
      kind,
      title: kind === 'human'
        ? safeString(review.humanReview?.title) ?? safeString(review.reviewReason) ?? safeString(review.id) ?? '人工审核'
        : safeString(review.reviewReason) ?? safeString(review.id) ?? '质量门禁',
      status: gateStatus(review.status),
      ...(stage ? { stage } : {}),
      blockingDownstream: review.blocksDownstream === true,
      ...(sourceArtifactId ? { sourceArtifactId } : {}),
      ...(sourceNodeId ? { sourceNodeId } : {}),
      ...(reviewer ? { reviewer } : {}),
      ...(comment ? { comment } : {}),
      ...(time ? { time } : {}),
    }
  })

  const reviewedNodeIds = new Set(
    reviews.flatMap((review) => [safeString(review.id), safeString(review.nodeId)])
      .filter((id): id is string => Boolean(id)),
  )

  const nodeItems = nodes.flatMap((node): GateInspectionItem[] => {
    const kind = gateNodeKind(node)
    if (!kind || reviewedNodeIds.has(node.id)) return []
    const sourceArtifactId = nodeSourceArtifact(node)
    const sourceNodeId = safeString(node.id)
    return [{
      id: safeString(node.id) ?? 'unknown-node',
      kind,
      title: safeString(node.name) ?? safeString(node.id) ?? '控制节点',
      status: gateStatus(node.status),
      blockingDownstream: nodeBlocksDownstream(node),
      ...(sourceArtifactId ? { sourceArtifactId } : {}),
      ...(sourceNodeId ? { sourceNodeId } : {}),
    }]
  })

  return [...reviewItems, ...nodeItems]
}

const gateKindLabel: Record<GateInspectionItem['kind'], string> = {
  human: '人工审核',
  quality: '质量门禁',
  control: '控制节点',
}

export default function GateInspector({ reviews, nodes }: GateInspectorProps) {
  const items = buildGateInspectionItems(reviews, nodes)
  return (
    <section className="diagnostics-gate-inspector" aria-labelledby="gate-inspector-title">
      <div className="diagnostics-gate-heading">
        <div><p>只读检查</p><h3 id="gate-inspector-title">人工审核与质量控制门禁</h3></div>
        <span role="status">{items.length} 项</span>
      </div>
      {items.length === 0 ? <div className="diagnostics-gate-empty" role="status">当前运行没有门禁或质量控制节点。</div> : (
        <ol className="diagnostics-gate-list">
          {items.map((item, index) => (
            <li key={`${item.kind}:${item.id}:${index}`} data-status={item.status}>
              <header><span>{gateKindLabel[item.kind]}</span><span className="diagnostics-status-pill" data-status={item.status}>{item.status}</span></header>
              <h4>{item.title}</h4>
              <dl>
                <GateField label="门禁 ID" value={item.id} code />
                <GateField label="阶段" value={item.stage} />
                <GateField label="阻塞下游" value={item.blockingDownstream ? '是' : '否'} />
                <GateField label="来源产物" value={item.sourceArtifactId} code />
                <GateField label="来源节点" value={item.sourceNodeId} code />
                <GateField label="审核人" value={item.reviewer} />
                <GateField label="审核意见" value={item.comment} />
                <GateField label="审核时间" value={item.time} />
              </dl>
            </li>
          ))}
        </ol>
      )}
      <p className="diagnostics-gate-readonly">此诊断视图仅展示证据，不提供审核决策操作。</p>
    </section>
  )
}

function GateField({ label, value, code = false }: { label: string; value?: string; code?: boolean }) {
  return <div><dt>{label}</dt><dd>{code ? <code>{value ?? '—'}</code> : value ?? '—'}</dd></div>
}
