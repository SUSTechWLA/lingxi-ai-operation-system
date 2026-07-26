import type { AgentReviewItem, AgentRun, Artifact } from '../../utils/types'

export interface DiagnosticsNode {
  id: string
  name: string
  type: string
  status: string
  startedAt?: string
  completedAt?: string
  durationMs?: number
  retryCount: number
  maxRetry: number
  toolName?: string
  transport?: 'tool' | 'mcp' | 'control' | 'unknown'
  request?: unknown
  response?: unknown
  error?: string
}

export interface DeveloperDiagnosticsSnapshot {
  run: AgentRun
  task?: unknown
  nodes: DiagnosticsNode[]
  reviews: AgentReviewItem[]
  artifacts: Artifact[]
}

const REDACTED = '[REDACTED]'
const CIRCULAR = '[Circular]'
const UNAVAILABLE = '[Unavailable]'

const secretKeys = new Set([
  'authorization',
  'cookie',
  'set-cookie',
  'apikey',
  'api_key',
  'accesstoken',
  'refreshtoken',
  'secret',
  'password',
  'credential',
  'signedurl',
  'signature',
])

function redactLocalPath(value: string): string {
  if (value.startsWith('local://')) return value

  const windowsPath = value.match(/^[A-Za-z]:\\(?:[^\\]+\\)*([^\\]+)\\?$/)
  if (windowsPath) return `<local-path>\\${windowsPath[1]}`

  const unixPath = value.match(/^\/(?:[^/]+\/)*([^/]+)\/?$/)
  if (unixPath) return `<local-path>/${unixPath[1]}`

  return value
}

function redactValue(value: unknown, ancestors: WeakSet<object>): unknown {
  if (typeof value === 'string') return redactLocalPath(value)
  if (value === null || typeof value !== 'object') return value
  if (ancestors.has(value)) return CIRCULAR

  ancestors.add(value)
  try {
    if (Array.isArray(value)) {
      let length = 0
      try {
        length = value.length
      } catch {
        return UNAVAILABLE
      }

      const clone: unknown[] = []
      for (let index = 0; index < length; index += 1) {
        try {
          clone.push(redactValue(value[index], ancestors))
        } catch {
          clone.push(UNAVAILABLE)
        }
      }
      return clone
    }

    let keys: string[]
    try {
      keys = Object.keys(value)
    } catch {
      return UNAVAILABLE
    }

    const clone: Record<string, unknown> = {}
    for (const key of keys) {
      if (secretKeys.has(key.toLowerCase())) {
        clone[key] = REDACTED
        continue
      }

      try {
        clone[key] = redactValue((value as Record<string, unknown>)[key], ancestors)
      } catch {
        clone[key] = UNAVAILABLE
      }
    }
    return clone
  } catch {
    return UNAVAILABLE
  } finally {
    ancestors.delete(value)
  }
}

export function redactDiagnosticValue(value: unknown): unknown {
  try {
    return redactValue(value, new WeakSet())
  } catch {
    return UNAVAILABLE
  }
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  try {
    return value !== null && typeof value === 'object' && !Array.isArray(value)
      ? value as Record<string, unknown>
      : undefined
  } catch {
    return undefined
  }
}

function readField(record: Record<string, unknown>, key: string): unknown {
  try {
    return record[key]
  } catch {
    return undefined
  }
}

function firstField(record: Record<string, unknown>, keys: readonly string[]): unknown {
  for (const key of keys) {
    const value = readField(record, key)
    if (value !== undefined && value !== null) return value
  }
  return undefined
}

function stringField(record: Record<string, unknown>, keys: readonly string[]): string | undefined {
  const value = firstField(record, keys)
  return typeof value === 'string' && value.length > 0 ? value : undefined
}

function nonNegativeInteger(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Math.floor(value)
    : 0
}

export function diagnosticDurationMs(startedAt?: string, completedAt?: string): number | undefined {
  if (typeof startedAt !== 'string' || typeof completedAt !== 'string' || !startedAt || !completedAt) {
    return undefined
  }

  const started = Date.parse(startedAt)
  const completed = Date.parse(completedAt)
  if (!Number.isFinite(started) || !Number.isFinite(completed) || completed < started) return undefined

  return completed - started
}

export function classifyDiagnosticTransport(node: unknown): DiagnosticsNode['transport'] {
  const record = recordValue(node)
  if (!record) return 'unknown'

  const transport = stringField(record, ['transport'])?.toLowerCase()
  if (transport === 'mcp' || transport?.includes('mcp')) return 'mcp'
  if (transport === 'control' || transport?.includes('control')) return 'control'
  if (transport === 'tool' || transport?.includes('tool')) return 'tool'

  const server = stringField(record, ['server', 'serverName', 'mcpServer'])
  const provider = stringField(record, ['provider'])?.toLowerCase()
  if (server || provider?.includes('mcp')) return 'mcp'

  const type = stringField(record, ['type', 'kind'])?.toLowerCase()
  if (type?.includes('mcp')) return 'mcp'
  if (type?.includes('control')) return 'control'

  const toolName = stringField(record, ['toolName', 'tool', 'name'])
  if (toolName && /(^|[_:.-])mcp([_:.-]|$)/i.test(toolName)) return 'mcp'
  if (toolName || type?.includes('tool')) return 'tool'

  return 'unknown'
}

function traceNodes(trace: unknown): unknown[] {
  const record = recordValue(trace)
  if (!record) return []

  const nodes = readField(record, 'nodes')
  return Array.isArray(nodes) ? nodes : []
}

export function buildDiagnosticsNodes(trace: unknown): DiagnosticsNode[] {
  try {
    return traceNodes(trace)
      .map((value, index) => {
        const record = recordValue(value)
        if (!record) return undefined

        const id = stringField(record, ['id', 'nodeId']) ?? `node-${index + 1}`
        const toolName = stringField(record, ['toolName', 'tool'])
        const startedAt = stringField(record, ['startedAt', 'startTime', 'createdAt'])
        const completedAt = stringField(record, ['completedAt', 'endTime', 'finishedAt'])
        const durationMs = diagnosticDurationMs(startedAt, completedAt)
        const request = firstField(record, ['request', 'input', 'arguments'])
        const response = firstField(record, ['response', 'output', 'result'])
        const errorValue = firstField(record, ['error', 'errorMessage'])
        const redactedError = redactDiagnosticValue(errorValue)

        const node: DiagnosticsNode & { sortIndex: number } = {
          id,
          name: stringField(record, ['name']) ?? toolName ?? id,
          type: stringField(record, ['type', 'kind']) ?? 'unknown',
          status: stringField(record, ['status']) ?? 'unknown',
          retryCount: nonNegativeInteger(firstField(record, ['retryCount', 'retries'])),
          maxRetry: nonNegativeInteger(firstField(record, ['maxRetry', 'maxRetries'])),
          transport: classifyDiagnosticTransport(record),
          sortIndex: index,
        }

        if (startedAt) node.startedAt = startedAt
        if (completedAt) node.completedAt = completedAt
        if (durationMs !== undefined) node.durationMs = durationMs
        if (toolName) node.toolName = toolName
        if (request !== undefined) node.request = redactDiagnosticValue(request)
        if (response !== undefined) node.response = redactDiagnosticValue(response)
        if (typeof redactedError === 'string' && redactedError.length > 0) node.error = redactedError

        return node
      })
      .filter((node): node is DiagnosticsNode & { sortIndex: number } => node !== undefined)
      .sort((left, right) => {
        const leftTime = left.startedAt ? Date.parse(left.startedAt) : Number.POSITIVE_INFINITY
        const rightTime = right.startedAt ? Date.parse(right.startedAt) : Number.POSITIVE_INFINITY
        const normalizedLeftTime = Number.isFinite(leftTime) ? leftTime : Number.POSITIVE_INFINITY
        const normalizedRightTime = Number.isFinite(rightTime) ? rightTime : Number.POSITIVE_INFINITY
        return normalizedLeftTime - normalizedRightTime || left.id.localeCompare(right.id) || left.sortIndex - right.sortIndex
      })
      .map(({ sortIndex, ...node }) => {
        void sortIndex
        return node
      })
  } catch {
    return []
  }
}
