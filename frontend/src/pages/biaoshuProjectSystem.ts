import type { BiaoshuProjectManifest } from '../services/localAgent'
import type { BiaoshuArtifactRecord, BiaoshuArtifactStatus } from './biaoshuArtifactLogic'

export function biaoshuProjectToArtifacts(
  project: Pick<BiaoshuProjectManifest, 'artifacts' | 'updatedAt'>,
): BiaoshuArtifactRecord[] {
  return project.artifacts.map((artifact) => ({
    id: artifact.id,
    name: artifact.name || artifact.kind,
    kind: artifact.kind,
    version: '-',
    status: normalizeArtifactStatus(artifact.status),
    owner: ownerForArtifactKind(artifact.kind),
    updatedAt: formatProjectTime(artifact.updatedAt || project.updatedAt),
    storageRef: artifact.storageRef,
    summary: typeof artifact.metadata?.summary === 'string' ? artifact.metadata.summary : '-',
    sourceTool: sourceToolForArtifactKind(artifact.kind),
    metadata: artifact.metadata,
  }))
}

export function biaoshuProjectSummary(project: Pick<BiaoshuProjectManifest, 'artifacts' | 'status' | 'currentStage'>) {
  const artifactCount = project.artifacts.length
  const validArtifactCount = project.artifacts.filter((artifact) => artifact.status === 'valid').length
  return {
    status: project.status,
    currentStage: project.currentStage,
    artifactCount,
    validArtifactCount,
  }
}

function normalizeArtifactStatus(status: string): BiaoshuArtifactStatus {
  if (status === 'valid' || status === 'review' || status === 'pending' || status === 'running' || status === 'failed' || status === 'missing') {
    return status
  }
  if (status === 'stale') return 'pending'
  return 'pending'
}

function ownerForArtifactKind(kind: string): string {
  if (kind === 'BID_RAW_TEXT') return '文件解析'
  if (kind === 'BID_ANALYSIS') return 'AI分析'
  if (kind === 'BID_PROJECT_CONTEXT') return '信息确认'
  if (kind === 'BID_OUTLINE') return '大纲规划'
  if (kind === 'BID_CHAPTERS') return '章节编写'
  if (kind === 'WORD_COUNT_REPORT') return '质量检查'
  if (kind === 'MERGED_DRAFT') return '成稿整合'
  if (kind === 'TECHNICAL_BID_DOCX') return 'Word 导出'
  return '标书项目'
}

function sourceToolForArtifactKind(kind: string): string {
  if (kind === 'BID_RAW_TEXT') return 'parse_bid_files'
  if (kind === 'BID_ANALYSIS') return 'bid_analysis_report'
  if (kind === 'BID_PROJECT_CONTEXT') return 'project_context_report'
  if (kind === 'BID_OUTLINE') return 'outline_generator'
  if (kind === 'BID_CHAPTERS') return 'chapter_writer'
  if (kind === 'WORD_COUNT_REPORT') return 'chapter_word_checker'
  if (kind === 'MERGED_DRAFT') return 'merge_chapters'
  if (kind === 'TECHNICAL_BID_DOCX') return 'convert_to_word'
  return 'biaoshu_project'
}

function formatProjectTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
