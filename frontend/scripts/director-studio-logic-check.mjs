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
    buildShotReviewGroups,
    buildDirectorStages,
    formatDirectorErrorMessage,
    findPublishCopyArtifact,
    getArtifactViewerSelection,
    normalizeDirectorErrorMessage,
    nextStageIdAfterReview,
    publishCopiesToJSON,
    publishCopiesToMarkdown,
    reviewDisplayTitle,
    reviewQualityReportLines,
    reviewStatusLabel,
    reviewOutputText,
    buildDirectorTraceNodes,
    externalGenerationGuideSteps,
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
  const createdDagFlow = buildDirectorStages(startupRoles, [], {
    nodes: [
      {
        id: 'proposal_generator_exec',
        name: 'external',
        type: 'TOOL',
        status: 'CREATED',
        input: { tool: 'external', capabilityTool: 'proposal_generator' },
        output: {},
      },
      {
        id: 'video_script_generator_exec',
        name: 'external',
        type: 'TOOL',
        status: 'CREATED',
        input: { tool: 'external', capabilityTool: 'video_script_generator' },
        output: {},
      },
    ],
  }, true)
  assert.deepEqual(createdDagFlow.map((stage) => stage.status), ['active', 'pending'])
  const bridgedToolFlow = buildDirectorStages(startupRoles, [], {
    nodes: [
      {
        id: 'video_script_generator_exec',
        name: 'external',
        type: 'TOOL',
        status: 'RUNNING',
        input: { tool: 'external', capabilityTool: 'video_script_generator' },
        output: {},
      },
    ],
  }, true)
  assert.deepEqual(bridgedToolFlow.map((stage) => stage.status), ['pending', 'running'])

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
  assert.deepEqual(generatingFlow.map((stage) => stage.status), ['active', 'pending'])

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
  assert.deepEqual(technicalOutputFlow.map((stage) => stage.status), ['active', 'pending'])

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
  assert.deepEqual(technicalGateTraceFlow.map((stage) => stage.status), ['active', 'pending'])

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
  const materializedCompositionArtifacts = buildDirectorArtifacts([compositionRole], [], missingCompositionTrace, [
    {
      id: 'art-composition-1',
      projectId: 'vp-1',
      stageName: 'composition',
      unitId: 'composition',
      kind: 'VIDEO_COMPOSITION_SPEC',
      name: '视频结构',
      version: 2,
      status: 'valid',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/composition/composition/hash/composition.json',
      dependsOn: ['VIDEO_SCRIPT'],
      metadata: { producedByRole: '结构导演' },
      updatedAt: '2026-06-29T08:00:00Z',
    },
  ])
  assert.equal(materializedCompositionArtifacts[0].id, 'art-composition-1')
  assert.equal(materializedCompositionArtifacts[0].version, '第2版')
  assert.equal(materializedCompositionArtifacts[0].status, 'valid')
  assert.equal(materializedCompositionArtifacts[0].humanApproved, true)
  assert.equal(materializedCompositionArtifacts[0].storageRef, 'local://projects/vp-1/artifacts/composition/composition/hash/composition.json')

  const artifactsWithPublishCopy = buildDirectorArtifacts([compositionRole], [], missingCompositionTrace, [
    {
      id: 'art-publish-copy-1',
      projectId: 'vp-1',
      stageName: 'publish',
      unitId: 'publish-copy',
      kind: 'PUBLISH_COPY',
      name: '发布文案.json',
      version: 1,
      status: 'valid',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/publish/publish-copy/hash/publish_copy.json',
      metadata: { artifactType: 'publish_copy' },
      updatedAt: '2026-06-29T08:00:00Z',
    },
  ])
  assert.ok(artifactsWithPublishCopy.some((artifact) => artifact.kind === 'PUBLISH_COPY'))
  assert.equal(findPublishCopyArtifact(artifactsWithPublishCopy)?.id, 'art-publish-copy-1')

  const shotReviewGroups = buildShotReviewGroups([
    {
      id: 'shot-packet-1',
      kind: 'SHOT_REVIEW_PACKET',
      name: 'SHOT_01 审核包',
      status: 'review',
      owner: '分镜导演',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://shot-1/review.json',
      metadata: { relatedShotId: 'SHOT_01', narrationText: '佛得角是西非岛国。', artifactType: 'shot_review_packet' },
    },
    {
      id: 'shot-ref-1',
      kind: 'REFERENCE_ASSET_PLAN',
      name: '人物三视角参考图',
      status: 'valid',
      owner: '参考选择',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/character-views.png',
      metadata: { relatedShotId: 'SHOT_01', referenceRole: 'character', viewSet: ['front', 'side', 'back'] },
    },
    {
      id: 'shot-audio-1',
      kind: 'SHOT_AUDIO',
      name: 'SHOT_01 口播音频',
      status: 'valid',
      owner: '音频',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/audio.wav',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'shot_audio' },
    },
    {
      id: 'shot-video-1',
      kind: 'SHOT_VIDEO_CLIP',
      name: 'SHOT_01 视频片段',
      status: 'pending',
      owner: '视频',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://shot-1/clip.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'shot_video_clip' },
    },
    {
      id: 'shot-packet-2',
      kind: 'SHOT_REVIEW_PACKET',
      name: 'SHOT_02 审核包',
      status: 'valid',
      owner: '分镜导演',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-2/review.json',
      metadata: { shotId: 'SHOT_02', scriptText: '世界杯出线是小国奇迹。', artifactType: 'shot_review_packet' },
    },
  ])
  assert.equal(shotReviewGroups.length, 2)
  assert.equal(shotReviewGroups[0].shotId, 'SHOT_01')
  assert.equal(shotReviewGroups[0].status, 'review')
  assert.equal(shotReviewGroups[0].narrationText, '佛得角是西非岛国。')
  assert.deepEqual(shotReviewGroups[0].referenceRoles, ['character'])
  assert.deepEqual(shotReviewGroups[0].artifactCounts, { total: 4, references: 1, media: 2, reviewPackets: 1 })
  assert.equal(shotReviewGroups[1].shotId, 'SHOT_02')
  assert.equal(shotReviewGroups[1].status, 'valid')

  assert.deepEqual(
    getArtifactViewerSelection('art-publish-copy-1', artifactsWithPublishCopy.find((artifact) => artifact.id === 'art-publish-copy-1')),
    { selectedId: undefined, shouldLoad: false, placeholder: undefined },
  )

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
  const explainableQualityGateReview = {
    ...qualityGateReview,
    reviewOutput: {
      qualityReport: {
        score: 95,
        passed: true,
        analysisSummary: '脚本结构完整，开头钩子明确，时长与事实引用都满足本轮要求。',
        rubricBreakdown: [
          { criterion: '结构完整性', score: 20, maxScore: 20, reason: '包含钩子、主体和总结。' },
          { criterion: '口播自然度', score: 18, maxScore: 20, reason: '表达清楚，个别句子可更短。' },
        ],
        keepDoing: ['保留开头的反差钩子', '继续引用本次知识材料中的事实'],
      },
    },
  }
  assert.deepEqual(reviewQualityReportLines(explainableQualityGateReview), [
    '质量评分 95/100，门禁阈值 85',
    '门禁结果：已通过',
    '分析：脚本结构完整，开头钩子明确，时长与事实引用都满足本轮要求。',
    '结构完整性：20/20，包含钩子、主体和总结。',
    '口播自然度：18/20，表达清楚，个别句子可更短。',
    '保持：保留开头的反差钩子',
    '保持：继续引用本次知识材料中的事实',
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

  const rawTopic = '帮我介绍一下佛得角国家以及说明佛得角世界杯从小组赛出线是一个奇迹'
  assert.deepEqual(buildPublishCopies(rawTopic), [])

  const copies = buildPublishCopies({
    publishCopies: [
      {
        platform: 'xiaohongshu',
        title: '佛得角出线为什么是奇迹',
        description: '这条口播先介绍佛得角，再解释小国足球如何突破人口、资源与历史成绩限制。',
        keywords: ['佛得角', '世界杯', '小国奇迹'],
        coverText: '小国出线奇迹',
        publishTips: ['前两行直接写结论'],
      },
      {
        platform: 'bilibili',
        title: '佛得角：一个小国的世界杯奇迹',
        description: '从国家背景、足球基础和小组赛突围难度三个层次说明佛得角出线的罕见性。',
        tags: ['佛得角', '世界杯', '足球史'],
        coverText: '佛得角奇迹',
        publishTips: ['分区选择足球或知识'],
      },
    ],
  })
  assert.equal(copies.length, 2)
  assert.equal(copies[0].platform, 'xiaohongshu')
  assert.equal(copies[1].platform, 'bilibili')
  assert.equal(copies[0].tags[0], '佛得角')
  assert.ok(!copies.some((copy) => copy.title.includes('帮我介绍一下')))
  assert.ok(copies.every((copy) => copy.title && copy.description && copy.coverText))
  assert.ok(publishCopiesToMarkdown(copies).includes('## 小红书'))
  assert.ok(publishCopiesToMarkdown(copies).includes('## B站'))
  assert.equal(JSON.parse(publishCopiesToJSON(copies))[0].platform, 'xiaohongshu')

  const videoGuideSteps = externalGenerationGuideSteps({
    kind: 'video',
    references: [{ id: 'keyframe-1' }, { id: 'scene-1' }],
    target: { durationSec: 8, aspectRatio: '16:9', resolution: '1920x1080' },
    promptCharLimit: 2000,
    referenceImageLimit: 6,
  })
  assert.ok(videoGuideSteps[0].includes('没有可用的图片或视频 API 配置'))
  assert.ok(videoGuideSteps.some((step) => step.includes('复制 Prompt') && step.includes('浏览器')))
  assert.ok(videoGuideSteps.some((step) => step.includes('每个 shot 单独生成') && step.includes('转场放在本 shot 结尾')))
  assert.ok(videoGuideSteps.some((step) => step.includes('上传结果') && step.includes('素材库')))

  const pageSource = await readFile(new URL('../src/pages/DirectorStudioPage.tsx', import.meta.url), 'utf8')
  assert.ok(
    pageSource.includes('card col-span-12 overflow-visible p-0'),
    'review page shell must not clip inner review panel borders',
  )
  assert.ok(
    pageSource.includes('mt-4 flex gap-2 overflow-x-auto px-1 py-1'),
    'review history scroller needs padding so item borders are not clipped',
  )
  assert.ok(
    pageSource.includes('createVideoProject'),
    'director studio should create a video project before starting a dynamic agent run',
  )
  assert.ok(
    pageSource.includes('fetchProjectArtifacts'),
    'director studio should refresh project artifacts as part of the shared state source',
  )
  assert.ok(
    pageSource.includes('projectId: nextProject.id'),
    'dynamic agent run context must include the bound project id',
  )
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
