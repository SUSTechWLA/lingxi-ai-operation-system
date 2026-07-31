/**
 * Error Log — lightweight telemetry for the Desktop/Web frontend.
 *
 * Posts structured error records to the local agent's /api/local/logs endpoint
 * so they appear in local-agent.jsonl alongside server-side events.
 *
 * This is fire-and-forget: failures in the logger itself are silent.
 * In Electron, errors are also forwarded to the main process console.
 */
const LOCAL_LOGS_URL = '/api/local/logs'

interface ErrorLogEntry {
  source: string
  level: 'warn' | 'error' | 'fatal'
  message: string
  fields?: Record<string, unknown>
}

let logErrorEnabled = true

/** Disable error logging (useful in tests). */
export function disableErrorLog(): void {
  logErrorEnabled = false
}

/** Enable error logging (default). */
export function enableErrorLog(): void {
  logErrorEnabled = true
}

async function postLog(entry: ErrorLogEntry): Promise<void> {
  if (!logErrorEnabled) return
  try {
    await fetch(LOCAL_LOGS_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(entry),
    })
  } catch {
    // fire-and-forget — don't cascade
  }
}

/**
 * Log an error that occurred in the frontend (API call, component render, etc.).
 * These records appear in the local agent's local-agent.jsonl file.
 */
export function logFrontendError(
  source: string,
  error: unknown,
  fields?: Record<string, unknown>,
): void {
  const message = error instanceof Error ? error.message : String(error)
  const safeFields: Record<string, unknown> = { ...fields }
  if (error instanceof Error && error.stack) {
    safeFields.stack = error.stack.split('\n').slice(0, 5).join('\n')
  }
  void postLog({ source, level: 'error', message, fields: safeFields })
  // Also emit to Electron console so it's visible during dev
  console.error(`[${source}]`, error, safeFields)
}

/**
 * Log a warning (non-fatal but noteworthy).
 */
export function logFrontendWarn(
  source: string,
  message: string,
  fields?: Record<string, unknown>,
): void {
  void postLog({ source, level: 'warn', message, fields })
  console.warn(`[${source}]`, message, fields)
}
