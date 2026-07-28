import type {
  AgentReviewItem,
  AgentReviewListResponse,
  AgentRun,
  Artifact,
  ArtifactListResponse,
  VideoProject,
} from '../../utils/types'

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
  serverName?: string
  transport?: 'tool' | 'mcp' | 'control' | 'unknown'
  request?: unknown
  response?: unknown
  error?: string
  errorClass?: string
  relatedArtifactIds?: string[]
}

export interface DeveloperDiagnosticsSnapshot {
  run: AgentRun
  task?: unknown
  nodes: DiagnosticsNode[]
  reviews: AgentReviewItem[]
  artifacts: Artifact[]
}

export type DiagnosticsDataSection = 'run' | 'trace' | 'reviews' | 'artifacts' | 'task' | 'context'

export interface ProjectDiagnosticsLoadResult {
  projectId: string
  runId: string
  run?: AgentRun
  task?: unknown
  context?: unknown[]
  trace?: unknown
  nodes?: DiagnosticsNode[]
  reviews?: AgentReviewItem[]
  artifacts?: Artifact[]
  errors: DiagnosticsDataSection[]
}

export function diagnosticsSnapshotForScope(
  snapshot: ProjectDiagnosticsLoadResult | null,
  projectId?: string,
  runId?: string,
): ProjectDiagnosticsLoadResult | null {
  if (!snapshot || !projectId || !runId) return null
  return snapshot.projectId === projectId && snapshot.runId === runId ? snapshot : null
}

export interface ProjectDiagnosticsApi {
  getRun: (runId: string, signal?: AbortSignal) => Promise<AgentRun>
  getTrace: (runId: string, signal?: AbortSignal) => Promise<unknown>
  getReviews: (runId: string, signal?: AbortSignal) => Promise<AgentReviewListResponse>
  getArtifacts: (projectId: string, signal?: AbortSignal) => Promise<ArtifactListResponse>
  getTask: (taskId: string, signal?: AbortSignal) => Promise<unknown>
  getContext: (taskId: string, signal?: AbortSignal) => Promise<unknown[]>
}

function updatedAtTime(project: Pick<VideoProject, 'updatedAt'>): number {
  const timestamp = Date.parse(project.updatedAt)
  return Number.isFinite(timestamp) ? timestamp : 0
}

export function sortDiagnosticsProjects<T extends Pick<VideoProject, 'id' | 'updatedAt'>>(
  projects: readonly T[],
): T[] {
  return projects
    .map((project, index) => ({ project, index }))
    .sort((left, right) => (
      updatedAtTime(right.project) - updatedAtTime(left.project) ||
      left.index - right.index
    ))
    .map(({ project }) => project)
}

export function selectDiagnosticsProject<
  T extends Pick<VideoProject, 'id' | 'updatedAt' | 'currentRunId'>,
>(projects: readonly T[]): T | undefined {
  const sorted = sortDiagnosticsProjects(projects)
  return sorted.find((project) => Boolean(project.currentRunId)) ?? sorted[0]
}

export function isTerminalAgentRunStatus(status: AgentRun['status'] | string): boolean {
  return status === 'SUCCESS' || status === 'FAILED' || status === 'CANCELLED'
}

function throwIfDiagnosticsAborted(signal?: AbortSignal): void {
  if (!signal?.aborted) return
  const error = new Error('Diagnostics request aborted')
  error.name = 'AbortError'
  throw error
}

export async function loadProjectDiagnostics({
  projectId,
  runId,
  signal,
  api,
}: {
  projectId: string
  runId: string
  signal?: AbortSignal
  api: ProjectDiagnosticsApi
}): Promise<ProjectDiagnosticsLoadResult> {
  throwIfDiagnosticsAborted(signal)

  const [runResult, traceResult, reviewsResult, artifactsResult] = await Promise.allSettled([
    api.getRun(runId, signal),
    api.getTrace(runId, signal),
    api.getReviews(runId, signal),
    api.getArtifacts(projectId, signal),
  ])
  throwIfDiagnosticsAborted(signal)

  const result: ProjectDiagnosticsLoadResult = { projectId, runId, errors: [] }
  if (runResult.status === 'fulfilled') result.run = runResult.value
  else result.errors.push('run')

  if (traceResult.status === 'fulfilled') {
    result.trace = traceResult.value
    result.nodes = buildDiagnosticsNodes(traceResult.value)
  } else {
    result.errors.push('trace')
  }
  if (reviewsResult.status === 'fulfilled') result.reviews = reviewsResult.value.reviews ?? []
  else result.errors.push('reviews')
  if (artifactsResult.status === 'fulfilled') result.artifacts = artifactsResult.value.artifacts ?? []
  else result.errors.push('artifacts')

  const taskId = result.run?.taskId
  if (!taskId) return result

  const [taskResult, contextResult] = await Promise.allSettled([
    api.getTask(taskId, signal),
    api.getContext(taskId, signal),
  ])
  throwIfDiagnosticsAborted(signal)

  if (taskResult.status === 'fulfilled') result.task = taskResult.value
  else result.errors.push('task')
  if (contextResult.status === 'fulfilled') result.context = contextResult.value
  else result.errors.push('context')

  return result
}

export function mergeProjectDiagnostics(
  previous: ProjectDiagnosticsLoadResult | null,
  next: ProjectDiagnosticsLoadResult,
): ProjectDiagnosticsLoadResult {
  if (!previous || previous.projectId !== next.projectId || previous.runId !== next.runId) return next

  const failed = new Set(next.errors)
  return {
    ...previous,
    ...next,
    run: failed.has('run') ? previous.run : next.run,
    trace: failed.has('trace') ? previous.trace : next.trace,
    nodes: failed.has('trace') ? previous.nodes : next.nodes,
    reviews: failed.has('reviews') ? previous.reviews : next.reviews,
    artifacts: failed.has('artifacts') ? previous.artifacts : next.artifacts,
    task: failed.has('task') ? previous.task : next.task,
    context: failed.has('context') ? previous.context : next.context,
  }
}

const REDACTED = '[REDACTED]'
const CIRCULAR = '[Circular]'
const UNAVAILABLE = '[Unavailable]'
const ACCESSOR = '[Accessor]'
const FUNCTION = '[Function]'
const SYMBOL = '[Symbol]'
const UNDEFINED = '[Undefined]'

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

function redactWholeLocalPath(value: string): string {
  if (value.startsWith('local://')) return value

  const windowsPath = value.match(/^[A-Za-z]:\\(?:[^\\]+\\)*([^\\]+)\\?$/)
  if (windowsPath) return `<local-path>\\${windowsPath[1]}`

  const unixPath = value.match(/^\/(?:[^/]+\/)*([^/]+)\/?$/)
  if (unixPath) return `<local-path>/${unixPath[1]}`

  return value
}

function localPathPlaceholder(value: string, separator: '/' | '\\'): string {
  const trimmed = value.endsWith(separator) ? value.slice(0, -1) : value
  const fileName = trimmed.slice(trimmed.lastIndexOf(separator) + 1)
  return `<local-path>${separator}${fileName}`
}

function redactLocalPathsInText(value: string): string {
  return value
    .replace(
      /\\\\[^\\\s"'<>|?*,;:()[\]{}]+\\[^\\\s"'<>|?*,;:()[\]{}]+(?:\\[^\\\s"'<>|?*,;:()[\]{}]+)*/g,
      (path) => localPathPlaceholder(path, '\\'),
    )
    .replace(
      /\b[A-Za-z]:\\(?:[^\\\s"'<>|?*,;:()[\]{}]+\\)+[^\\\s"'<>|?*,;:()[\]{}]+/g,
      (path) => localPathPlaceholder(path, '\\'),
    )
    .replace(
      /(^|[\s("'=])((?:\/[^/\s"'<>?,;:()[\]{}]+)+)/g,
      (_match, prefix: string, path: string) => `${prefix}${localPathPlaceholder(path, '/')}`,
    )
}

function redactLocalPathsOutsideHttpUrls(value: string): string {
  const httpUrl = /\bhttps?:\/\/[^\s<>"']+/gi
  let redacted = ''
  let cursor = 0

  for (const match of value.matchAll(httpUrl)) {
    const index = match.index
    redacted += redactLocalPathsInText(value.slice(cursor, index))
    redacted += match[0]
    cursor = index + match[0].length
  }

  return redacted + redactLocalPathsInText(value.slice(cursor))
}

function redactDiagnosticString(value: string): string {
  const wholePath = redactWholeLocalPath(value)
  if (wholePath !== value) return wholePath

  const secretRedacted = value
    .replace(
      /([?&](?:x-amz-(?:credential|signature|security-token)|x-goog-(?:credential|signature)|googleaccessid|signature|sig|access[_-]?token|token)=)[^&#\s]+/gi,
      (_match, prefix: string) => `${prefix}${REDACTED}`,
    )
    .replace(
      /(\bauthorization\s*[:=]\s*)(?:["']?)(?:[A-Za-z][\w-]*\s+)?[^\s,;"']+(?:["']?)/gi,
      (_match, prefix: string) => `${prefix}${REDACTED}`,
    )
    .replace(/\bBearer\s+[A-Za-z0-9._~+/=-]+/gi, `Bearer ${REDACTED}`)
    .replace(
      /(\b(?:set-cookie|cookie)\s*:\s*)(?:["']?)((?:[^=\s;,]+=[^;\s,"']+)(?:\s*;\s*[^=\s;,]+=[^;\s,"']+)*)(?:["']?)/gi,
      (_match, prefix: string) => `${prefix}${REDACTED}`,
    )
    .replace(
      /(\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|credential|password|secret|token|signature)\b\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;&]+)/gi,
      (_match, prefix: string) => `${prefix}${REDACTED}`,
    )

  return redactLocalPathsOutsideHttpUrls(secretRedacted)
}

function redactValue(value: unknown, ancestors: WeakSet<object>): unknown {
  if (typeof value === 'string') return redactDiagnosticString(value)
  if (typeof value === 'bigint') return String(value)
  if (typeof value === 'function') return FUNCTION
  if (typeof value === 'symbol') return SYMBOL
  if (typeof value === 'undefined') return UNDEFINED
  if (value === null || typeof value !== 'object') return value
  if (ancestors.has(value)) return CIRCULAR

  ancestors.add(value)
  try {
    if (Array.isArray(value)) {
      let lengthDescriptor: PropertyDescriptor | undefined
      let keys: string[]
      try {
        lengthDescriptor = Object.getOwnPropertyDescriptor(value, 'length')
        keys = Object.keys(value)
      } catch {
        return UNAVAILABLE
      }

      const length = typeof lengthDescriptor?.value === 'number' ? lengthDescriptor.value : 0
      const clone: unknown[] = new Array(length)
      for (const key of keys) {
        if (!/^(0|[1-9]\d*)$/.test(key)) continue
        const index = Number(key)
        if (!Number.isSafeInteger(index) || index >= length) continue

        try {
          const descriptor = Object.getOwnPropertyDescriptor(value, key)
          clone[index] = descriptor && 'value' in descriptor
            ? redactValue(descriptor.value, ancestors)
            : ACCESSOR
        } catch {
          clone[index] = UNAVAILABLE
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
        const descriptor = Object.getOwnPropertyDescriptor(value, key)
        const redacted = descriptor && 'value' in descriptor
          ? redactValue(descriptor.value, ancestors)
          : ACCESSOR
        Object.defineProperty(clone, key, {
          configurable: true,
          enumerable: true,
          value: redacted,
          writable: true,
        })
      } catch {
        Object.defineProperty(clone, key, {
          configurable: true,
          enumerable: true,
          value: UNAVAILABLE,
          writable: true,
        })
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

export function serializeRedactedDiagnosticValue(value: unknown): string {
  try {
    return JSON.stringify(redactDiagnosticValue(value)) ?? 'null'
  } catch {
    return JSON.stringify(UNAVAILABLE)
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

function stringListField(record: Record<string, unknown>, keys: readonly string[]): string[] {
  const value = firstField(record, keys)
  if (typeof value === 'string' && value.length > 0) return [value]
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string' && item.length > 0)
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

function hasTransportSignal(value: string, signal: 'mcp' | 'control' | 'tool'): boolean {
  const normalized = value.toLowerCase()
  return normalized === signal || normalized.startsWith(`${signal}__`) ||
    ['_', ':', '.', '/', '-'].some((separator) => normalized.startsWith(`${signal}${separator}`))
}

export function classifyDiagnosticTransport(node: unknown): DiagnosticsNode['transport'] {
  const record = recordValue(node)
  if (!record) return 'unknown'

  const transport = stringField(record, ['transport'])?.toLowerCase()
  if (transport && hasTransportSignal(transport, 'mcp')) return 'mcp'
  if (transport && hasTransportSignal(transport, 'control')) return 'control'
  if (transport && hasTransportSignal(transport, 'tool')) return 'tool'

  const server = stringField(record, ['server', 'serverName', 'mcpServer', 'mcp_server'])
  const provider = stringField(record, ['provider'])?.toLowerCase()
  if (server || (provider && hasTransportSignal(provider, 'mcp'))) return 'mcp'

  const type = stringField(record, ['type', 'kind'])?.toLowerCase()
  if (type && hasTransportSignal(type, 'mcp')) return 'mcp'
  if (type && hasTransportSignal(type, 'control')) return 'control'
  if (type && hasTransportSignal(type, 'tool')) return 'tool'

  const toolName = stringField(record, ['toolName', 'tool'])
  const name = stringField(record, ['name'])
  if ([toolName, name].some((value) => value && hasTransportSignal(value, 'mcp'))) return 'mcp'
  if ([toolName, name].some((value) => value && hasTransportSignal(value, 'control'))) return 'control'
  if (toolName || (name && hasTransportSignal(name, 'tool'))) return 'tool'

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
        const serverName = stringField(record, ['server', 'serverName', 'mcpServer', 'mcp_server'])
        const startedAt = stringField(record, ['startedAt', 'startTime', 'createdAt'])
        const completedAt = stringField(record, ['completedAt', 'endTime', 'finishedAt'])
        const durationMs = diagnosticDurationMs(startedAt, completedAt)
        const request = firstField(record, ['request', 'input', 'arguments'])
        const response = firstField(record, ['response', 'output', 'result'])
        const errorValue = firstField(record, ['error', 'errorMessage'])
        const redactedError = errorValue === undefined ? undefined : redactDiagnosticValue(errorValue)
        const responseRecord = recordValue(response)
        const relatedArtifactIds = [...new Set([
          ...stringListField(record, ['relatedArtifactIds', 'artifactIds', 'outputArtifactIds', 'artifactId']),
          ...(responseRecord ? stringListField(responseRecord, ['relatedArtifactIds', 'artifactIds', 'outputArtifactIds', 'artifactId']) : []),
        ])]

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
        if (serverName) node.serverName = serverName
        if (request !== undefined) node.request = redactDiagnosticValue(request)
        if (response !== undefined) node.response = redactDiagnosticValue(response)
        if (typeof redactedError === 'string' && redactedError.length > 0) node.error = redactedError
        const errorClass = stringField(record, ['errorClass', 'exceptionClass', 'errorType'])
        if (errorClass) node.errorClass = errorClass
        if (relatedArtifactIds.length > 0) node.relatedArtifactIds = relatedArtifactIds

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
