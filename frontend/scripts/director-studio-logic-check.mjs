import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const tempDir = await mkdtemp(join(tmpdir(), 'director-studio-logic-'))
const outfile = join(tempDir, 'directorStudioLogic.mjs')

try {
  await build({
    entryPoints: [new URL('../src/pages/directorStudioLogic.ts', import.meta.url).pathname],
    outfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })

  const {
    applyOptimisticRunningStage,
    buildDirectorArtifacts,
    buildPublishCopies,
    buildDirectorStages,
    formatDirectorErrorMessage,
    normalizeDirectorErrorMessage,
    nextStageIdAfterReview,
    publishCopiesToJSON,
    publishCopiesToMarkdown,
    reviewDisplayTitle,
    reviewQualityReportLines,
    reviewStatusLabel,
    reviewOutputText,
    buildDirectorTraceNodes,
    traceNodeHasError,
    visibleReviewHistory,
  } = await import(pathToFileURL(outfile))
  const roleAgents = [
    {
      id: 'render_producer',
      name: 'Render Producer',
      displayName: '渲染制片',
      stage: 'render',
      goal: '',
      allowedTools: ['hyperframes_renderer'],
      forbiddenTools: [],
      requiredInputs: [],
      requiredOutputs: ['VIDEO'],
    },
  ]
  const reviews = [
    {
      id: 'review-render',
      nodeId: 'render_review',
      status: 'APPROVED',
      roleAgentId: 'render_producer',
      stage: 'render',
    },
  ]
  const trace = {
    nodes: [
      {
        id: 'render_exec',
        name: 'hyperframes_renderer',
        status: 'SUCCESS',
        input: { stage: 'render', roleAgentId: 'render_producer' },
        output: { success: true, summary: 'render complete but no artifact manifest' },
      },
    ],
  }

  const artifacts = buildDirectorArtifacts(roleAgents, reviews, trace)
  assert.equal(artifacts.length, 1)
  assert.equal(artifacts[0].kind, 'VIDEO')
  assert.equal(artifacts[0].storageRef, '')
  assert.notEqual(artifacts[0].status, 'valid')

  const stagedRoles = [
    { ...roleAgents[0], id: 'script_writer', stage: 'script', displayName: '脚本编剧', allowedTools: ['video_script_generator'], requiredOutputs: ['VIDEO_SCRIPT'] },
    { ...roleAgents[0], id: 'storyboard_artist', stage: 'storyboard', displayName: '卡片设计师', allowedTools: ['card_plan_generator'], requiredOutputs: ['CARD_PLAN'] },
  ]
  const stagedReviews = [{ id: 'review-script', nodeId: 'review-script-node', status: 'PENDING', roleAgentId: 'script_writer', stage: 'script', tool: 'video_script_generator' }]
  const stagedFlow = buildDirectorStages(stagedRoles, stagedReviews, { nodes: [] })
  assert.equal(nextStageIdAfterReview(stagedFlow, stagedReviews[0]), 'storyboard_artist')
  const optimisticFlow = applyOptimisticRunningStage(stagedFlow, 'storyboard_artist')
  assert.equal(optimisticFlow[1].status, 'running')
  assert.equal(stagedFlow[1].status, 'pending')
  const reviewGateFlow = buildDirectorStages(stagedRoles, [], {
    nodes: [
      {
        id: 'script-review',
        name: '审核-video_script_generator',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { tool: 'video_script_generator', stage: 'script', reviewPhase: 'after_artifact' },
        output: { content: '# 口播脚本' },
      },
    ],
  })
  assert.equal(reviewGateFlow[0].status, 'review')

  const proposalArtifacts = buildDirectorArtifacts(
    [{
      id: 'creative_director',
      name: 'Creative Director',
      displayName: '创意总监',
      stage: 'proposal',
      goal: '',
      allowedTools: ['proposal_generator'],
      forbiddenTools: [],
      requiredInputs: [],
      requiredOutputs: ['VIDEO_PROPOSAL'],
    }],
    [],
    { nodes: [{ id: 'proposal', name: 'proposal_generator', status: 'SUCCESS', input: { stage: 'proposal' }, output: { artifacts: [{ kind: 'VIDEO_PROPOSAL', name: '创意方案', storageRef: 'cloud://proposal' }] } }] },
  )
  assert.equal(proposalArtifacts[0].storageRef, '本地项目目录（仅同步索引）')

  const proposalReview = {
    id: 'td39d3460d8-proposal_generator_review',
    nodeId: 'td39d3460d8-proposal_generator_review',
    status: 'PENDING',
    tool: 'proposal_generator',
    reviewPhase: 'after_artifact',
    humanReview: { title: '审核创作方案' },
    reviewContent: '# Proposal Packet\n\n推荐方案：option_a',
  }
  assert.equal(reviewDisplayTitle(proposalReview), '审核创作方案')
  assert.equal(reviewOutputText(proposalReview), '# Proposal Packet\n\n推荐方案：option_a')

  const qualityGateReview = {
    id: 'video_script_generator_quality_gate',
    nodeId: 'video_script_generator_quality_gate',
    status: 'PENDING',
    tool: 'video_script_generator',
    reviewPhase: 'quality_gate',
    reviewReason: '质量门禁：script_quality_checker 评分需 >=85',
    reviewContent: '佛得角第一次站上世界杯舞台，这不是冷门，是一代人的坚持。',
    reviewOutput: { qualityReport: { score: 82, issues: ['事实来源需要更明确'] } },
  }
  assert.equal(reviewDisplayTitle(qualityGateReview), '审核口播脚本')
  assert.equal(reviewOutputText(qualityGateReview), '佛得角第一次站上世界杯舞台，这不是冷门，是一代人的坚持。')
  assert.deepEqual(reviewQualityReportLines(qualityGateReview), [
    '质量评分 82/100，门禁阈值 85',
    '事实来源需要更明确',
  ])

  const noisyJsonReview = {
    id: 'knowledge-review',
    nodeId: 'knowledge-review',
    status: 'PENDING',
    tool: 'knowledge_researcher',
    reviewContent: '我们被要求输出一个JSON，先分析。\n{"facts":["佛得角是西非岛国"],"summary":"佛得角首次晋级世界杯。"}\n以上是最终结果。',
  }
  assert.equal(
    reviewOutputText(noisyJsonReview),
    '{\n  "facts": [\n    "佛得角是西非岛国"\n  ],\n  "summary": "佛得角首次晋级世界杯。"\n}',
  )

  const legacyQualityGateReview = {
    id: 'legacy_quality_gate',
    nodeId: 'legacy_quality_gate',
    status: 'PENDING',
    tool: '__quality_gate__',
    reviewPhase: 'quality_gate',
  }
  assert.equal(reviewDisplayTitle(legacyQualityGateReview), '审核创作产物')

  const visibleReviews = visibleReviewHistory([
    { id: 'future-storyboard', nodeId: 'future-storyboard', status: 'CREATED', tool: 'card_plan_generator' },
    { id: 'proposal-review', nodeId: 'proposal-review', status: 'APPROVED', tool: 'proposal_generator' },
    { id: 'script-review', nodeId: 'script-review', status: 'PENDING', tool: 'video_script_generator' },
    { id: 'rejected-review', nodeId: 'rejected-review', status: 'REJECTED', tool: 'card_plan_generator' },
  ])
  assert.deepEqual(visibleReviews.map((review) => review.id), ['proposal-review', 'script-review', 'rejected-review'])
  assert.equal(reviewStatusLabel(visibleReviews[0]), '已通过')
  assert.equal(reviewStatusLabel(visibleReviews[1]), '待审核')
  assert.equal(reviewStatusLabel(visibleReviews[2]), '已驳回')

  const nestedTraceNodes = buildDirectorTraceNodes({
    data: {
      task: {
        nodes: [
          {
            id: 'gate-1',
            name: '质量门禁-video_script_generator_quality_gate',
            type: 'REVIEW_GATE',
            status: 'FAILED',
            errorMessage: '评分未达标',
            input: { stage: 'script' },
            output: {},
          },
        ],
      },
    },
  })
  assert.equal(nestedTraceNodes.length, 1)
  assert.equal(nestedTraceNodes[0].error, '评分未达标')
  assert.equal(traceNodeHasError(undefined), false)
  assert.equal(traceNodeHasError(nestedTraceNodes[0]), true)

  assert.equal(
    formatDirectorErrorMessage(
      new Error('CRITICAL_ARTIFACT_SYNC_FAILED: critical artifact sync failed: kind=VIDEO unitID=final-video'),
      'fallback',
    ),
    '关键产物写入失败，最终视频无法进入项目产物库。\n请重新执行当前步骤。',
  )
  assert.equal(
    normalizeDirectorErrorMessage({
      message: 'Request failed with status code 400',
      response: { data: { message: 'guard agent plan: agent plan has no steps' } },
    }),
    'guard agent plan: agent plan has no steps',
  )

  const copies = buildPublishCopies('智能体改变的是工作流', 45, artifacts)
  assert.equal(copies.length, 2)
  assert.equal(copies[0].platform, 'xiaohongshu')
  assert.equal(copies[1].platform, 'bilibili')
  assert.ok(copies.every((copy) => copy.title && copy.description && copy.coverText))
  assert.ok(publishCopiesToMarkdown(copies).includes('## 小红书'))
  assert.ok(publishCopiesToMarkdown(copies).includes('## B站'))
  assert.equal(JSON.parse(publishCopiesToJSON(copies))[0].platform, 'xiaohongshu')
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
