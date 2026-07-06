import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const tempDir = await mkdtemp(join(tmpdir(), 'biaoshu-artifact-logic-'))
const outfile = join(tempDir, 'biaoshuArtifactLogic.mjs')

try {
  await build({
    entryPoints: [fileURLToPath(new URL('../src/pages/biaoshuArtifactLogic.ts', import.meta.url))],
    outfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })

  const {
    buildBiaoshuArtifacts,
    createBiaoshuFallbackRun,
    createManualReportArtifact,
    createManualProjectContextArtifact,
    createManualOutlineArtifact,
    createManualScoringBreakdownArtifact,
    displayNameForBiaoshuArtifact,
    biaoshuArtifactToCopyText,
    deriveBiaoshuProjectContextPath,
    deriveBiaoshuProjectContextQuestionnairePath,
    deriveBiaoshuOutlinePath,
    deriveBiaoshuScoringBreakdownPath,
    mergeManualBiaoshuArtifacts,
    mergeManualReportArtifact,
  } = await import(pathToFileURL(outfile))

  // Build and import biaoshuProjectSystem.ts separately
  const systemOutfile = join(tempDir, 'biaoshuProjectSystem.mjs')
  await build({
    entryPoints: [fileURLToPath(new URL('../src/pages/biaoshuProjectSystem.ts', import.meta.url))],
    outfile: systemOutfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })
  const {
    biaoshuProjectToArtifacts,
    biaoshuProjectSummary,
    biaoshuManagedProjectToHistoryItem,
    biaoshuManagedProjectsToHistory,
    selectBestBiaoshuManagedProject,
  } = await import(pathToFileURL(systemOutfile))

  const run = {
    id: 'run-bid-1',
    status: 'RUNNING',
    domain: 'bid_writing',
    message: 'generate technical bid',
    createdAt: '2026-06-29T08:00:00Z',
    updatedAt: '2026-06-29T08:01:00Z',
    plan: {
      goal: 'generate technical bid document',
      domain: 'bid_writing',
      mode: 'dynamic_agent',
      steps: [
        { id: 'parse', tool: 'parse_bid_files', arguments: {}, expectedOutput: ['BID_RAW_TEXT', 'BID_ANALYSIS'] },
        { id: 'outline', tool: 'outline_generator', arguments: {}, expectedOutput: ['BID_OUTLINE'] },
        { id: 'word', tool: 'convert_to_word', arguments: {}, expectedOutput: ['TECHNICAL_BID_DOCX'] },
      ],
    },
  }

  const artifacts = buildBiaoshuArtifacts(run, {
    nodes: [
      {
        id: 'parse_exec',
        name: 'parse_bid_files',
        status: 'SUCCESS',
        output: {
          summary: 'parsed tender raw text',
          stdout: JSON.stringify({
            raw_text_path: 'E:/bid/out/00_raw.md',
            report_path: 'E:/bid/out/00_report.md',
          }),
        },
        completedAt: '2026-06-29T08:02:00Z',
      },
      {
        id: 'word_exec',
        name: 'external',
        status: 'SUCCESS',
        input: { capabilityTool: 'convert_to_word' },
        output: {
          output_file: 'E:/bid/out/technical_bid.docx',
          summary: 'Word exported',
        },
        completedAt: '2026-06-29T08:05:00Z',
      },
    ],
  })

  assert.equal(artifacts.length, 4)
  assert.equal(artifacts[0].kind, 'BID_RAW_TEXT')
  assert.equal(artifacts[0].status, 'valid')
  assert.equal(artifacts[0].storageRef, 'E:/bid/out/00_raw.md')
  assert.equal(artifacts[1].kind, 'BID_ANALYSIS')
  assert.equal(artifacts[1].status, 'valid')
  assert.equal(artifacts[1].storageRef, 'E:/bid/out/00_report.md')
  assert.equal(artifacts[2].kind, 'BID_OUTLINE')
  assert.equal(artifacts[2].status, 'pending')
  assert.equal(artifacts[3].kind, 'TECHNICAL_BID_DOCX')
  assert.equal(artifacts[3].storageRef, 'E:/bid/out/technical_bid.docx')
  assert.equal(displayNameForBiaoshuArtifact('MERGED_DRAFT'), '整合成稿')

  const copyText = biaoshuArtifactToCopyText(artifacts[3])
  assert.match(copyText, /TECHNICAL_BID_DOCX/)
  assert.match(copyText, /Word exported/)

  assert.equal(
    deriveBiaoshuProjectContextPath('E:/bid/out/00_analysis_report.md'),
    'E:/bid/out/01_项目背景信息确认表.md',
  )
  assert.equal(
    deriveBiaoshuProjectContextQuestionnairePath('E:\\bid\\out\\00_analysis_report.md'),
    'E:\\bid\\out\\01_项目背景信息问答记录.json',
  )
  assert.equal(
    deriveBiaoshuOutlinePath('E:/bid/out/00_analysis_report.md'),
    'E:/bid/out/03_技术标四级大纲.md',
  )
  assert.equal(
    deriveBiaoshuScoringBreakdownPath('E:/bid/out/00_analysis_report.md'),
    'E:/bid/out/02_评分标准拆解表.md',
  )

  const regeneratedReport = createManualReportArtifact(
    {
      id: 'manual-report-1',
      name: 'Bid analysis report',
      storageRef: 'E:/bid/out/00_regenerated_report.md',
      summary: 'Regenerated report',
      metadata: { previous: 'kept' },
    },
    'E:/bid/out/00_regenerated_report.md',
    'E:/bid/source.pdf',
  )
  const mergedArtifacts = mergeManualReportArtifact(artifacts, regeneratedReport)
  assert.equal(mergedArtifacts.length, artifacts.length)
  assert.equal(mergedArtifacts[1].kind, 'BID_ANALYSIS')
  assert.equal(mergedArtifacts[1].storageRef, 'E:/bid/out/00_regenerated_report.md')
  assert.equal(mergedArtifacts[1].metadata.sourceFile, 'E:/bid/source.pdf')
  assert.equal(mergedArtifacts[1].metadata.manualGenerated, true)

  const regeneratedContext = createManualProjectContextArtifact(
    {
      id: 'manual-context-1',
      name: 'Project context',
      storageRef: 'E:/bid/out/01_项目背景信息确认表.md',
      summary: 'Recovered context report',
      metadata: { recovered: true },
    },
    'E:/bid/out/01_项目背景信息确认表.md',
    'E:/bid/source.pdf',
  )
  const regeneratedScoring = createManualScoringBreakdownArtifact(
    {
      id: 'manual-scoring-1',
      name: 'Scoring breakdown',
      storageRef: 'E:/bid/out/02_评分标准拆解表.md',
      summary: 'Recovered scoring breakdown',
      metadata: { recovered: true },
    },
    'E:/bid/out/02_评分标准拆解表.md',
    'E:/bid/source.pdf',
  )
  const regeneratedOutline = createManualOutlineArtifact(
    {
      id: 'manual-outline-1',
      name: 'Technical outline',
      storageRef: 'E:/bid/out/03_技术标四级大纲.md',
      summary: 'Recovered outline',
      metadata: { recovered: true },
    },
    'E:/bid/out/03_技术标四级大纲.md',
    'E:/bid/source.pdf',
  )
  const mergedManualArtifacts = mergeManualBiaoshuArtifacts(
    artifacts,
    [regeneratedContext, regeneratedOutline],
  )

  assert.equal(mergedManualArtifacts.length, artifacts.length + 1)
  assert.equal(mergedManualArtifacts[2].kind, 'BID_PROJECT_CONTEXT')
  assert.equal(mergedManualArtifacts[2].storageRef, 'E:/bid/out/01_项目背景信息确认表.md')
  assert.equal(mergedManualArtifacts[3].kind, 'BID_OUTLINE')
  assert.equal(mergedManualArtifacts[3].storageRef, 'E:/bid/out/03_技术标四级大纲.md')

  assert.equal(regeneratedScoring.kind, 'BID_SCORING_BREAKDOWN')
  assert.equal(regeneratedScoring.owner, '评分拆解')

  const fallbackRun = createBiaoshuFallbackRun({
    runId: 'agent_run_missing',
    projectName: '养护',
    bidFilePath: 'E:/yhbs/招标文件/招标文件_converted.docx',
    status: 'SUCCESS',
    createdAt: '2026-07-04T07:20:18.000Z',
    updatedAt: '2026-07-04T08:20:18.000Z',
  })

  assert.equal(fallbackRun.id, 'agent_run_missing')
  assert.equal(fallbackRun.domain, 'bid_writing')
  assert.equal(fallbackRun.status, 'SUCCESS')
  assert.equal(fallbackRun.metadata.projectName, '养护')
  assert.equal(fallbackRun.metadata.bidFilePath, 'E:/yhbs/招标文件/招标文件_converted.docx')
  assert.equal(fallbackRun.metadata.localHistoryFallback, true)
  assert.equal(fallbackRun.plan.steps.length >= 4, true)
  assert.equal(fallbackRun.plan.steps[0].tool, 'parse_bid_files')
  assert.deepEqual(fallbackRun.plan.steps[1].expectedOutput, ['BID_ANALYSIS'])

  const projectSummary = biaoshuProjectSummary({
    projectId: 'bp_1',
    projectName: '养护',
    status: 'SUCCESS',
    currentStage: 'context_ready',
    artifacts: [
      { id: 'a1', kind: 'BID_ANALYSIS', name: '解析报告', status: 'valid', storageRef: 'E:/out/a.md', mimeType: 'text/markdown', dependsOn: [], createdAt: '2026-07-05T00:00:00Z', updatedAt: '2026-07-05T00:00:00Z', metadata: {} },
      { id: 'a2', kind: 'BID_PROJECT_CONTEXT', name: '背景确认表', status: 'valid', storageRef: 'E:/out/c.md', mimeType: 'text/markdown', dependsOn: [], createdAt: '2026-07-05T00:00:00Z', updatedAt: '2026-07-05T00:00:00Z', metadata: {} },
    ],
    updatedAt: '2026-07-05T00:00:00Z',
  })
  assert.equal(projectSummary.validArtifactCount, 2)
  assert.equal(projectSummary.artifactCount, 2)

  const projectArtifacts = biaoshuProjectToArtifacts({
    projectId: 'bp_1',
    projectName: '养护',
    status: 'SUCCESS',
    currentStage: 'context_ready',
    artifacts: [
      { id: 'a2', kind: 'BID_PROJECT_CONTEXT', name: '背景确认表', status: 'valid', storageRef: 'E:/out/c.md', mimeType: 'text/markdown', dependsOn: [], createdAt: '2026-07-05T00:00:00Z', updatedAt: '2026-07-05T00:00:00Z', metadata: {} },
    ],
    updatedAt: '2026-07-05T00:00:00Z',
  })
  assert.equal(projectArtifacts[0].kind, 'BID_PROJECT_CONTEXT')
  assert.equal(projectArtifacts[0].storageRef, 'E:/out/c.md')

  // Task 1: managed project → history item
  const managedProject = {
    projectId: 'bp_1',
    projectName: '养护',
    status: 'SUCCESS',
    currentStage: 'context_ready',
    runs: [{ runId: 'run-1' }],
    sourceFiles: [{ path: 'E:/bid/source.pdf' }],
    createdAt: '2026-07-05T00:00:00Z',
    updatedAt: '2026-07-05T00:00:00Z',
    artifacts: [
      { id: 'a2', kind: 'BID_PROJECT_CONTEXT', name: '背景确认表', status: 'valid', storageRef: 'E:/out/c.md', mimeType: 'text/markdown', dependsOn: [], createdAt: '2026-07-05T00:00:00Z', updatedAt: '2026-07-05T00:00:00Z', metadata: {} },
    ],
  }
  const historyFromManaged = biaoshuManagedProjectToHistoryItem(managedProject)
  assert.equal(historyFromManaged.projectId, 'bp_1')
  assert.equal(historyFromManaged.managedProject, true)

  const historyList = biaoshuManagedProjectsToHistory([managedProject])
  assert.equal(historyList.length, 1)
  assert.equal(historyList[0].projectName, '养护')

  const best = selectBestBiaoshuManagedProject([managedProject])
  assert.notEqual(best, null)
  assert.equal(best.projectId, 'bp_1')

  const emptyBest = selectBestBiaoshuManagedProject([])
  assert.equal(emptyBest, null)

  const scoredTestProjects = [
    { ...managedProject, projectId: 'bp_low', currentStage: 'created', artifacts: [], updatedAt: '2026-01-01T00:00:00Z' },
    { ...managedProject, projectId: 'bp_high', currentStage: 'context_ready', updatedAt: '2026-07-05T00:00:00Z' },
  ]
  const bestOfTwo = selectBestBiaoshuManagedProject(scoredTestProjects)
  assert.equal(bestOfTwo.projectId, 'bp_high')

  console.log('biaoshuArtifactLogic tests passed')
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
