export function unwrapApiData<T>(payload: T | { data?: T } | null | undefined): T | undefined {
  if (payload == null) return undefined
  if (typeof payload !== 'object') return payload as T
  const record = payload as Record<string, unknown>
  if ('data' in record) return record.data as T | undefined
  return payload as T
}
