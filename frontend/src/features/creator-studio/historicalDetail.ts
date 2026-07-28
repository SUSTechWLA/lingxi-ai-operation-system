export interface HistoricalDetail {
  id: string
  title: string
  text: string
  titleId: string
  bodyId: string
}

export function buildHistoricalDetail(id: string, title: string, text: string): HistoricalDetail | null {
  const normalizedText = text.trim()
  if (!normalizedText) return null
  const normalizedID = id.trim().toLocaleLowerCase().replace(/[^a-z0-9_-]+/gu, '-').replace(/^-+|-+$/gu, '') || 'content'
  return {
    id: normalizedID,
    title: title.trim() || '完整内容',
    text: normalizedText,
    titleId: `historical-detail-${normalizedID}-title`,
    bodyId: `historical-detail-${normalizedID}-body`,
  }
}
