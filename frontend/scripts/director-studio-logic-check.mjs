import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
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
    isActionablePendingReview,
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
  const startupRoles = [
    { ...roleAgents[0], id: 'creative_director', stage: 'proposal', displayName: '创意总监', allowedTools: ['proposal_generator'], requiredOutputs: ['VIDEO_PROPOSAL'] },
    stagedRoles[0],
  ]
  const notStartedFlow = buildDirectorStages(startupRoles, [], { nodes: [] })
  assert.deepEqual(notStartedFlow.map((stage) => stage.status), ['pending', 'pending'])
  const justStartedFlow = buildDirectorStages(startupRoles, [], { nodes: [] }, true)
  assert.deepEqual(justStartedFlow.map((stage) => stage.status), ['active', 'pending'])

  const unactionableStartupReviews = startupRoles.map((role) => ({
    id: `${role.stage}_review`,
    nodeId: `${role.stage}_review`,
    status: 'PENDING',
    stage: role.stage,
    tool: role.allowedTools[0],
    reviewPhase: 'after_artifact',
  }))
  assert.equal(isActionablePendingReview(unactionableStartupReviews[0]), false)
  assert.deepEqual(visibleReviewHistory(unactionableStartupReviews), [])
  const generatingFlow = buildDirectorStages(startupRoles, unactionableStartupReviews, { nodes: [] }, true)
  assert.deepEqual(generatingFlow.map((stage) => stage.status), ['pending', 'pending'])

  const technicalOutputReviews = startupRoles.map((role) => ({
    id: `${role.stage}_technical_review`,
    nodeId: `${role.stage}_technical_review`,
    status: 'PENDING',
    stage: role.stage,
    tool: role.allowedTools[0],
    reviewPhase: 'after_artifact',
    reviewOutput: {
      stage: role.stage,
      status: 'READY',
      stdout: `${role.stage} gate created`,
    },
  }))
  assert.equal(isActionablePendingReview(technicalOutputReviews[0]), false)
  assert.deepEqual(visibleReviewHistory(technicalOutputReviews), [])
  const technicalOutputFlow = buildDirectorStages(startupRoles, technicalOutputReviews, { nodes: [] }, true)
  assert.deepEqual(technicalOutputFlow.map((stage) => stage.status), ['pending', 'pending'])

  const technicalGateTraceFlow = buildDirectorStages(startupRoles, [], {
    nodes: startupRoles.map((role) => ({
      id: `${role.stage}_review`,
      name: `审核-${role.allowedTools[0]}`,
      type: 'REVIEW_GATE',
      status: 'READY',
      input: {
        stage: role.stage,
        tool: role.allowedTools[0],
        reviewPhase: 'after_artifact',
      },
      output: {
        stage: role.stage,
        status: 'READY',
        stdout: `${role.stage} gate created`,
      },
    })),
  }, true)
  assert.deepEqual(technicalGateTraceFlow.map((stage) => stage.status), ['pending', 'pending'])

  const actionableGateTraceFlow = buildDirectorStages(startupRoles, [], {
    nodes: [
      {
        id: 'proposal_review',
        name: '审核-proposal_generator',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: {
          stage: 'proposal',
          tool: 'proposal_generator',
          reviewPhase: 'after_artifact',
        },
        output: { content: '# 创作方向\n\n可以审核的方案正文' },
      },
    ],
  }, true)
  assert.deepEqual(actionableGateTraceFlow.map((stage) => stage.status), ['review', 'pending'])

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

  const nestedParameterFlow = buildDirectorStages(
    [{
      id: 'storyboard_artist',
      name: 'Storyboard Artist',
      displayName: '卡片设计师',
      stage: 'storyboard',
      goal: '拆画面',
      allowedTools: ['card_plan_generator'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_SCRIPT'],
      requiredOutputs: ['CARD_PLAN'],
    }],
    [{
      id: 'shot_splitter_review',
      nodeId: 'shot_splitter_review',
      status: 'PENDING',
      stage: 'storyboard',
      tool: 'shot_splitter',
      reviewPhase: 'after_artifact',
    }],
    {
      nodes: [
        {
          id: 'shot_splitter_exec',
          name: 'external',
          type: 'TOOL',
          status: 'RUNNING',
          input: {
            tool: 'external',
            capabilityTool: 'shot_splitter',
            parameters: {
              stage: 'storyboard',
              roleAgentId: 'storyboard_artist',
            },
          },
          output: {},
        },
        {
          id: 'shot_splitter_review',
          name: '审核-shot_splitter',
          type: 'REVIEW_GATE',
          status: 'READY',
          input: {
            stage: 'storyboard',
            tool: 'shot_splitter',
            sourceNode: 'shot_splitter_exec',
            reviewPhase: 'after_artifact',
          },
          output: {},
        },
      ],
    },
  )
  assert.equal(nestedParameterFlow[0].status, 'running')

  const nestedParameterTrace = buildDirectorTraceNodes({
    nodes: [
      {
        id: 'shot_splitter_exec',
        name: 'external',
        type: 'TOOL',
        status: 'RUNNING',
        input: {
          tool: 'external',
          capabilityTool: 'shot_splitter',
          parameters: { stage: 'storyboard' },
        },
        output: {},
      },
    ],
  })
  assert.equal(nestedParameterTrace[0].stage, 'storyboard')

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

  const compositionRole = {
    id: 'composition_director',
    name: 'Composition Director',
    displayName: '结构导演',
    stage: 'composition',
    goal: '生成时间轴',
    allowedTools: ['video_composition_builder'],
    forbiddenTools: [],
    requiredInputs: ['VIDEO_SCRIPT'],
    requiredOutputs: ['VIDEO_COMPOSITION_SPEC'],
  }
  const missingCompositionTrace = {
    nodes: [
      {
        id: 'composition_exec',
        name: 'external',
        type: 'TOOL',
        status: 'SUCCESS',
        input: { tool: 'video_composition_builder', stage: 'composition', roleAgentId: 'composition_director' },
        output: { content: 'composition finished without artifact manifest' },
      },
      {
        id: 'composition_review_gate',
        name: '审核-视频结构',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { stage: 'composition', requiredOutputs: ['VIDEO_COMPOSITION_SPEC'], reviewPhase: 'after_artifact' },
        output: {},
      },
    ],
  }
  const missingCompositionArtifacts = buildDirectorArtifacts([compositionRole], [], missingCompositionTrace)
  assert.equal(missingCompositionArtifacts[0].status, 'missing')
  const missingCompositionStages = buildDirectorStages([compositionRole], [], missingCompositionTrace, true)
  assert.equal(missingCompositionStages[0].status, 'failed')

  const readyCompositionTrace = {
    nodes: [
      {
        id: 'composition_exec',
        name: 'external',
        type: 'TOOL',
        status: 'SUCCESS',
        input: { tool: 'video_composition_builder', stage: 'composition', roleAgentId: 'composition_director' },
        output: { artifacts: [{ kind: 'VIDEO_COMPOSITION_SPEC', name: '视频结构', storageRef: 'local://projects/vp-1/artifacts/composition/composition/hash/composition.json' }] },
      },
      {
        id: 'composition_review_gate',
        name: '审核-视频结构',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { stage: 'composition', requiredOutputs: ['VIDEO_COMPOSITION_SPEC'], reviewPhase: 'after_artifact' },
        output: {},
      },
    ],
  }
  const readyCompositionStages = buildDirectorStages([compositionRole], [], readyCompositionTrace, true)
  assert.equal(readyCompositionStages[0].status, 'review')

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
    { id: 'ready-script-review', nodeId: 'ready-script-review', status: 'PENDING', tool: 'video_script_generator', reviewContent: '可审核脚本正文' },
    { id: 'rejected-review', nodeId: 'rejected-review', status: 'REJECTED', tool: 'card_plan_generator' },
  ])
  assert.deepEqual(visibleReviews.map((review) => review.id), ['proposal-review', 'ready-script-review', 'rejected-review'])
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

  const pageSource = await readFile(new URL('../src/pages/DirectorStudioPage.tsx', import.meta.url), 'utf8')
  assert.ok(
    pageSource.includes('card col-span-12 overflow-visible p-0'),
    'review page shell must not clip inner review panel borders',
  )
  assert.ok(
    pageSource.includes('mt-4 flex gap-2 overflow-x-auto px-1 py-1'),
    'review history scroller needs padding so item borders are not clipped',
  )
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
