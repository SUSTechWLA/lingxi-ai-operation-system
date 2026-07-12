import type { BiaoshuArtifactRecord } from './biaoshuArtifactLogic'

export type BiaoshuChapterHealth = 'ready' | 'short' | 'pending' | 'failed'

export interface BiaoshuChapterWorkspaceItem {
  chapterNumber: number
  title: string
  current: BiaoshuArtifactRecord
  versions: BiaoshuArtifactRecord[]
  wordCount?: number
  targetWords?: number
  health: BiaoshuChapterHealth
}

export interface BiaoshuChapterWorkspace {
  chapters: BiaoshuChapterWorkspaceItem[]
  unclassified: BiaoshuArtifactRecord[]
  summary: {
    total: number
    ready: number
    short: number
    unclassified: number
  }
}

export function buildChapterWorkspace(artifacts: BiaoshuArtifactRecord[]): BiaoshuChapterWorkspace {
  const grouped = new Map<number, BiaoshuArtifactRecord[]>()
  const unclassified: BiaoshuArtifactRecord[] = []

  for (const artifact of artifacts) {
    if (artifact.kind !== 'BID_CHAPTERS') continue
    const chapterNumber = chapterNumberForArtifact(artifact)
    if (!chapterNumber) {
      unclassified.push(artifact)
      continue
    }
    const current = grouped.get(chapterNumber) || []
    current.push(artifact)
    grouped.set(chapterNumber, current)
  }

  const chapters = Array.from(grouped.entries())
    .map(([chapterNumber, versions]) => buildChapterItem(chapterNumber, versions))
    .sort((left, right) => left.chapterNumber - right.chapterNumber)

  return {
    chapters,
    unclassified,
    summary: {
      total: chapters.length,
      ready: chapters.filter((chapter) => chapter.health === 'ready').length,
      short: chapters.filter((chapter) => chapter.health === 'short').length,
      unclassified: unclassified.length,
    },
  }
}

function buildChapterItem(
  chapterNumber: number,
  versions: BiaoshuArtifactRecord[],
): BiaoshuChapterWorkspaceItem {
  const orderedVersions = [...versions].sort(compareChapterVersions)
  const current = orderedVersions[0]
  const metadata = current.metadata || {}
  const wordCount = numberMetadata(metadata.wordCount)
  const targetWords = numberMetadata(metadata.targetWords)
  const title = stringMetadata(metadata.chapterTitle) || current.name

  return {
    chapterNumber,
    title,
    current,
    versions: orderedVersions,
    wordCount,
    targetWords,
    health: chapterHealth(current, wordCount, targetWords),
  }
}

function chapterNumberForArtifact(artifact: BiaoshuArtifactRecord): number | undefined {
  const metadataNumber = numberMetadata(artifact.metadata?.chapterNumber)
  if (metadataNumber) return metadataNumber

  const idMatch = artifact.id.match(/^chapter-(\d+)(?:-|$)/i)
  if (idMatch) return Number(idMatch[1])

  const nameMatch = artifact.name.match(/第\s*(\d+)\s*章/)
  if (nameMatch) return Number(nameMatch[1])
  return undefined
}

function compareChapterVersions(left: BiaoshuArtifactRecord, right: BiaoshuArtifactRecord): number {
  const sourcePriority = chapterSourcePriority(right) - chapterSourcePriority(left)
  if (sourcePriority !== 0) return sourcePriority
  return parseUpdatedAt(right.updatedAt) - parseUpdatedAt(left.updatedAt)
}

function chapterSourcePriority(artifact: BiaoshuArtifactRecord): number {
  if (artifact.sourceTool === 'chapter_expansion') return 2
  if (artifact.sourceTool === 'chapter_generation') return 1
  return 0
}

function chapterHealth(
  artifact: BiaoshuArtifactRecord,
  wordCount?: number,
  targetWords?: number,
): BiaoshuChapterHealth {
  if (artifact.status === 'failed' || artifact.status === 'missing') return 'failed'
  if (artifact.status !== 'valid') return 'pending'
  if (wordCount !== undefined && targetWords !== undefined && wordCount < targetWords) return 'short'
  return 'ready'
}

function numberMetadata(value: unknown): number | undefined {
  const number = Number(value)
  return Number.isInteger(number) && number > 0 ? number : undefined
}

function stringMetadata(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function parseUpdatedAt(value: string): number {
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? 0 : parsed
}
