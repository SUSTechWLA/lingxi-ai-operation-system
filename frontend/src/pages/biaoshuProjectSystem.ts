import type { BiaoshuProjectManifest, BiaoshuProjectStatus, BiaoshuProjectListResponse } from '../services/localAgent'
import type { BiaoshuArtifactRecord, BiaoshuArtifactStatus } from './biaoshuArtifactLogic'

export interface BiaoshuProjectHistoryItem {
  runId: string
  projectId: string
  projectName: string
  bidFilePath: string
  status: BiaoshuProjectStatus
  currentStage: string
  createdAt: string
  updatedAt: string
  artifactCount: number
  validArtifactCount: number
  managedProject: boolean
}

function normalizeProjectStatus(status: string): BiaoshuProjectStatus {
  if (
    status === 'CREATED' ||
    status === 'RUNNING' ||
    status === 'SUCCESS' ||
    status === 'FAILED' ||
    status === 'UNKNOWN'
  ) {
    return status
  }
  return 'UNKNOWN'
}

export function biaoshuManagedProjectToHistoryItem(
  project: BiaoshuProjectManifest,
): BiaoshuProjectHistoryItem {
  const summary = biaoshuProjectSummary(project)
  return {
    runId: project.runs[0]?.runId || project.projectId,
    projectId: project.projectId,
    projectName: project.projectName,
    bidFilePath: project.sourceFiles[0]?.path || '',
    status: normalizeProjectStatus(project.status),
    currentStage: project.currentStage,
    createdAt: project.createdAt,
    updatedAt: project.updatedAt,
    artifactCount: summary.artifactCount,
    validArtifactCount: summary.validArtifactCount,
    managedProject: true,
  }
}

export function biaoshuManagedProjectsToHistory(
  projects: BiaoshuProjectManifest[],
): BiaoshuProjectHistoryItem[] {
  return projects.map((project) => biaoshuManagedProjectToHistoryItem(project))
}

export function selectBestBiaoshuManagedProject(
  projects: BiaoshuProjectManifest[],
): BiaoshuProjectManifest | null {
  if (projects.length === 0) return null
  if (projects.length === 1) return projects[0]

  const scored = projects.map((project, index) => ({
    project,
    index,
    score: projectScore(project),
  }))
  scored.sort((a, b) => {
    if (b.score !== a.score) return b.score - a.score
    const bTime = new Date(b.project.updatedAt).getTime()
    const aTime = new Date(a.project.updatedAt).getTime()
    if (bTime !== aTime) return bTime - aTime
    return a.index - b.index
  })
  return scored[0].project
}

function projectScore(project: BiaoshuProjectManifest): number {
  const validArtifacts = project.artifacts.filter(
    (artifact) => artifact.status === 'valid' && artifact.storageRef,
  )
  const stageWeight: Record<string, number> = {
    word_exported: 90,
    draft_merged: 80,
    wordcheck_ready: 70,
    chapters_ready: 60,
    outline_ready: 50,
    scoring_ready: 45,
    context_ready: 40,
    analysis_ready: 30,
    raw_parsed: 20,
    created: 10,
    failed: 0,
  }
  return (stageWeight[project.currentStage] || 0) + validArtifacts.length
}

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
    summary:
      typeof artifact.metadata?.summary === 'string'
        ? artifact.metadata.summary
        : '-',
    sourceTool: sourceToolForArtifactKind(artifact.kind),
    metadata: artifact.metadata,
  }))
}

export function biaoshuProjectSummary(
  project: Pick<BiaoshuProjectManifest, 'artifacts' | 'status' | 'currentStage'>,
) {
  const artifactCount = project.artifacts.length
  const validArtifactCount = project.artifacts.filter(
    (artifact) => artifact.status === 'valid',
  ).length
  return {
    status: project.status,
    currentStage: project.currentStage,
    artifactCount,
    validArtifactCount,
  }
}

function normalizeArtifactStatus(status: string): BiaoshuArtifactStatus {
  if (
    status === 'valid' ||
    status === 'review' ||
    status === 'pending' ||
    status === 'running' ||
    status === 'failed' ||
    status === 'missing'
  ) {
    return status
  }
  if (status === 'stale') return 'pending'
  return 'pending'
}

function ownerForArtifactKind(kind: string): string {
  if (kind === 'BID_RAW_TEXT') return '文件解析'
  if (kind === 'BID_ANALYSIS') return 'AI分析'
  if (kind === 'BID_PROJECT_CONTEXT') return '信息确认'
  if (kind === 'BID_SCORING_BREAKDOWN') return '评分拆解'
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
  if (kind === 'BID_SCORING_BREAKDOWN') return 'scoring_breakdown_generator'
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

export function legacyProjectsToHistory(response: BiaoshuProjectListResponse): BiaoshuProjectHistoryItem[] {
  return response.projects.map((p) => ({
    runId: p.runId,
    projectId: '',
    projectName: p.projectName,
    bidFilePath: p.bidFilePath,
    status: normalizeProjectStatus(p.status),
    currentStage: '',
    createdAt: p.createdAt,
    updatedAt: p.updatedAt,
    artifactCount: p.artifactCount ?? 0,
    validArtifactCount: p.validArtifactCount ?? 0,
    managedProject: false,
  }))
}
