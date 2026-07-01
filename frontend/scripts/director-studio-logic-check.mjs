import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const tempDir = await mkdtemp(join(tmpdir(), 'director-studio-logic-'))
const outfile = join(tempDir, 'directorStudioLogic.mjs')
const apiResponseOutfile = join(tempDir, 'apiResponse.mjs')

try {
  await build({
    entryPoints: [new URL('../src/pages/directorStudioLogic.ts', import.meta.url).pathname],
    outfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })
  await build({
    entryPoints: [new URL('../src/utils/apiResponse.ts', import.meta.url).pathname],
    outfile: apiResponseOutfile,
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
    canStartProject,
    formatDirectorErrorMessage,
    findFinalVideoArtifact,
    findPublishCopyArtifact,
    getArtifactViewerSelection,
    isProjectSessionStarted,
    localArtifactIdFromStorageRef,
    localServiceStatusDisplay,
    normalizeDirectorErrorMessage,
    nextStageIdAfterReview,
    nextSelectedReviewId,
    overviewProjectStatus,
    projectPrimaryAction,
    publishCopiesToJSON,
    publishCopiesToMarkdown,
    reviewDisplayTitle,
    reviewQualityReportLines,
    reviewStatusLabel,
    reviewOutputText,
    buildDirectorTraceNodes,
    creationProfileSummary,
    externalGenerationGuideSteps,
    timeWindowPlanSummary,
    traceNodeHasError,
    unresolvedMaterialDependencyCount,
    visibleReviewHistory,
    isActionablePendingReview,
  } = await import(pathToFileURL(outfile))
  const { unwrapApiData } = await import(pathToFileURL(apiResponseOutfile))
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
  assert.equal(canStartProject(true, false, false), true)
  assert.equal(canStartProject(true, false, true), false)
  assert.equal(isProjectSessionStarted(false, 'RUNNING', 'FAILED'), false)
  assert.equal(isProjectSessionStarted(false, 'RUNNING', 'SUCCESS'), false)
  assert.equal(isProjectSessionStarted(false, 'RUNNING', 'RUNNING'), true)
  assert.equal(overviewProjectStatus(justStartedFlow, 'RUNNING'), 'active')
  assert.equal(overviewProjectStatus(justStartedFlow, 'RUNNING', 'SUCCESS'), 'done')
  assert.equal(overviewProjectStatus(notStartedFlow), 'pending')
  const rawPreflight = {
    pipeline: 'wf-guided-image-text-video',
    status: 'passed',
    canStart: true,
    capabilityMenu: { localRunner: { available: true } },
  }
  assert.equal(unwrapApiData({ data: rawPreflight }), rawPreflight)
  assert.equal(unwrapApiData(rawPreflight), rawPreflight)
  assert.deepEqual(localServiceStatusDisplay('unknown', true), { label: '本地在线', tone: 'ok' })
  assert.deepEqual(localServiceStatusDisplay('unhealthy', false), { label: '本地离线', tone: 'error' })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'RUNNING',
    runStatus: 'RUNNING',
    stages: justStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'stop', label: '停止项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'RUNNING',
    runStatus: 'FAILED',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'RUNNING',
    runStatus: 'SUCCESS',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'DRAFT',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'DRAFT',
    runStatus: 'CANCELLED',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'PAUSED',
    runStatus: 'CANCELLED',
    stages: justStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })

  const preRenderReview = {
    id: 'render-review-before',
    nodeId: 'render_review_before',
    status: 'PENDING',
    stage: 'render',
    tool: 'hyperframes_renderer',
    reviewReason: '渲染视频耗时较长且会产生大文件，必须用户确认后执行。',
  }
  const preRenderFlow = buildDirectorStages(roleAgents, [preRenderReview], {
    nodes: [
      {
        id: 'render_review_before',
        name: '审核-render',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { stage: 'render', tool: 'hyperframes_renderer', reviewPhase: 'before_execute' },
        output: {},
      },
      {
        id: 'render_exec',
        name: 'external',
        type: 'TOOL',
        status: 'CREATED',
        input: { tool: 'external', capabilityTool: 'hyperframes_renderer', stage: 'render' },
        output: {},
      },
    ],
  }, true)
  assert.equal(isActionablePendingReview(preRenderReview), true)
  assert.equal(visibleReviewHistory([preRenderReview]).length, 1)
  assert.equal(nextSelectedReviewId('review-proposal', 'review-script', [
    { id: 'review-proposal', status: 'APPROVED' },
    { id: 'review-script', status: 'PENDING', reviewContent: 'script ready' },
  ], 'review-proposal'), 'review-script')
  assert.equal(nextSelectedReviewId('review-proposal', 'review-script', [
    { id: 'review-proposal', status: 'APPROVED' },
    { id: 'review-script', status: 'PENDING', reviewContent: 'script ready' },
  ], 'review-script'), 'review-proposal')
  assert.equal(nextSelectedReviewId('missing-review', undefined, [
    { id: 'review-proposal', status: 'APPROVED' },
  ]), 'review-proposal')
  assert.equal(preRenderFlow[0].status, 'review')
  assert.equal(preRenderFlow[0].reviewId, 'render-review-before')
  assert.equal(reviewDisplayTitle(preRenderReview), '审核最终渲染')

  const failedAfterPreRenderApprovalFlow = buildDirectorStages(roleAgents, [{
    ...preRenderReview,
    status: 'APPROVED',
  }], {
    nodes: [
      {
        id: 'render_review_before',
        name: '审核-render',
        type: 'REVIEW_GATE',
        status: 'SUCCESS',
        input: { stage: 'render', tool: 'hyperframes_renderer', reviewPhase: 'before_execute' },
        output: { approved: true },
      },
      {
        id: 'render_exec',
        name: 'external',
        type: 'TOOL',
        status: 'FAILED',
        input: { tool: 'external', capabilityTool: 'hyperframes_renderer', stage: 'render' },
        output: {},
        error: 'HyperFrames 渲染已禁用',
      },
    ],
  }, true)
  assert.equal(failedAfterPreRenderApprovalFlow[0].status, 'failed')

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

  const duplicateScriptReviewsFlow = buildDirectorStages(
    [stagedRoles[0]],
    [
      {
        id: 'knowledge-approved',
        nodeId: 'knowledge-approved',
        status: 'APPROVED',
        stage: 'script',
        tool: 'knowledge_researcher',
        reviewContent: '已通过的知识调研',
      },
      {
        id: 'script-pending',
        nodeId: 'script-pending',
        status: 'PENDING',
        stage: 'script',
        tool: 'video_script_generator',
        reviewContent: '当前待审核口播脚本',
      },
    ],
    { nodes: [] },
  )
  assert.equal(duplicateScriptReviewsFlow[0].status, 'review')
  assert.equal(duplicateScriptReviewsFlow[0].reviewId, 'script-pending')

  const knowledgeReviewFlow = buildDirectorStages(
    [
      { ...startupRoles[0], allowedTools: ['proposal_generator'] },
      { ...stagedRoles[0], allowedTools: ['video_script_generator', 'script_quality_checker', 'knowledge_researcher'] },
    ],
    [
      {
        id: 'knowledge-pending',
        nodeId: 'knowledge_researcher_review',
        status: 'PENDING',
        tool: 'knowledge_researcher',
        reviewContent: '佛得角是西非岛国。',
      },
    ],
    { nodes: [] },
  )
  assert.equal(knowledgeReviewFlow[0].status, 'review')
  assert.equal(knowledgeReviewFlow[0].reviewId, 'knowledge-pending')
  assert.equal(knowledgeReviewFlow[1].status, 'pending')

  const scriptQualityGateFlow = buildDirectorStages(
    [{
      ...stagedRoles[0],
      allowedTools: ['video_script_generator', 'script_quality_checker'],
    }],
    [
      {
        id: 'script-approved-before-quality',
        nodeId: 'script-approved-before-quality',
        status: 'APPROVED',
        stage: 'script',
        tool: 'video_script_generator',
        reviewContent: '已通过的口播脚本',
      },
      {
        id: 'script-quality-gate-pending',
        nodeId: 'video_script_generator_quality_gate',
        stepId: 'video_script_generator_quality_gate',
        status: 'PENDING',
        reviewPhase: 'quality_gate',
        reviewReason: '质量门禁：script_quality_checker 评分需 >=85',
        reviewOutput: { qualityReport: { score: 82, issues: ['事实年份错误'] } },
      },
    ],
    {
      nodes: [
        {
          id: 'script-exec',
          name: 'external',
          type: 'TOOL',
          status: 'SUCCESS',
          input: { tool: 'external', capabilityTool: 'video_script_generator', stage: 'script', roleAgentId: 'script_writer' },
          output: { artifacts: [{ kind: 'VIDEO_SCRIPT', name: '视频脚本', storageRef: 'local://script.json' }] },
        },
        {
          id: 'script-quality',
          name: 'external',
          type: 'TOOL',
          status: 'SUCCESS',
          input: { tool: 'external', capabilityTool: 'script_quality_checker' },
          output: { artifacts: [{ kind: 'JSON', name: '脚本质检报告' }] },
        },
      ],
    },
  )
  assert.equal(scriptQualityGateFlow[0].status, 'review')
  assert.equal(scriptQualityGateFlow[0].reviewId, 'script-quality-gate-pending')

  const qualityGateScopedFlow = buildDirectorStages(
    [
      {
        ...stagedRoles[0],
        allowedTools: ['video_script_generator', 'script_quality_checker'],
      },
      {
        id: 'quality_reviewer',
        name: 'Quality Reviewer',
        displayName: '质量审核',
        stage: 'quality',
        goal: '',
        allowedTools: ['ffmpeg_probe', 'final_review_generator'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'],
      },
    ],
    [
      {
        id: 'script-quality-gate-pending',
        nodeId: 'video_script_generator_quality_gate',
        stepId: 'video_script_generator_quality_gate',
        status: 'PENDING',
        reviewPhase: 'quality_gate',
        reviewReason: '质量门禁：script_quality_checker 评分需 >=85',
        reviewOutput: { qualityReport: { score: 82, issues: ['事实年份错误'] } },
      },
    ],
    { nodes: [] },
  )
  assert.equal(qualityGateScopedFlow[0].status, 'review')
  assert.equal(qualityGateScopedFlow[1].status, 'pending')

  const storyboardGateAliasFlow = buildDirectorStages(
    [
      {
        ...stagedRoles[1],
        allowedTools: ['card_plan_generator'],
      },
      {
        id: 'quality_reviewer',
        name: 'Quality Reviewer',
        displayName: '质量审核',
        stage: 'quality',
        goal: '',
        allowedTools: ['ffmpeg_probe', 'final_review_generator'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'],
      },
    ],
    [
      {
        id: 'beat-plan-quality-gate-pending',
        nodeId: 'beat_plan_quality_gate',
        stepId: 'beat_plan_quality_gate',
        status: 'PENDING',
        tool: 'shot_splitter',
        reviewPhase: 'quality_gate',
        reviewReason: '质量门禁：shot_quality_checker 评分需 >=85',
        reviewOutput: { qualityReport: { score: 30, issues: ['总时长超出目标'] } },
      },
    ],
    { nodes: [] },
  )
  assert.equal(storyboardGateAliasFlow[0].status, 'review')
  assert.equal(storyboardGateAliasFlow[1].status, 'pending')

  const referenceToolAliasFlow = buildDirectorStages(
    [
      {
        id: 'reference_selector',
        name: 'Reference Selector',
        displayName: '参考资产选择器',
        stage: 'reference',
        goal: '',
        allowedTools: ['style_reference_selector'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['REFERENCE_PACKAGE'],
      },
      {
        id: 'quality_reviewer',
        name: 'Quality Reviewer',
        displayName: '质量审核',
        stage: 'quality',
        goal: '',
        allowedTools: ['ffmpeg_probe', 'final_review_generator'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'],
      },
    ],
    [
      {
        id: 'video-prompt-review',
        nodeId: 'video_prompt_generator_review',
        stepId: 'video_prompt_generator_review',
        status: 'PENDING',
        tool: 'video_prompt_generator',
        reviewContent: 'SHOT_01 keyframe prompt',
      },
    ],
    { nodes: [] },
  )
  assert.equal(referenceToolAliasFlow[0].status, 'review')
  assert.equal(referenceToolAliasFlow[1].status, 'pending')

  const previewPendingWithUnmaterializedOutputsFlow = buildDirectorStages(
    [{
      id: 'preview_director',
      name: 'Preview Director',
      displayName: '预览导演',
      stage: 'preview',
      goal: '',
      allowedTools: ['hyperframes_project_generator', 'hyperframes_snapshot'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_COMPOSITION_SPEC'],
      requiredOutputs: ['HYPERFRAMES_PROJECT', 'PREVIEW_SNAPSHOTS'],
    }],
    [
      {
        id: 'preview-review',
        nodeId: 'preview_review',
        status: 'PENDING',
        stage: 'preview',
        tool: 'hyperframes_project_generator',
        reviewContent: 'HyperFrames 项目已生成，等待审核。',
      },
    ],
    {
      nodes: [
        {
          id: 'preview_exec',
          name: 'external',
          type: 'TOOL',
          status: 'SUCCESS',
          input: { tool: 'external', capabilityTool: 'hyperframes_project_generator', stage: 'preview' },
          output: { artifacts: [{ kind: 'HTML', name: 'index.html', storageRef: '' }] },
        },
        {
          id: 'preview_review',
          name: '审核-preview',
          type: 'REVIEW_GATE',
          status: 'READY',
          input: { stage: 'preview', tool: 'hyperframes_project_generator', reviewPhase: 'after_artifact' },
          output: {},
        },
      ],
    },
  )
  assert.equal(previewPendingWithUnmaterializedOutputsFlow[0].status, 'review')

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

  const approvedReviewFallbackRoles = [
    {
      id: 'creative_director',
      name: 'Creative Director',
      displayName: '创意总监',
      stage: 'proposal',
      goal: '定方向',
      allowedTools: ['proposal_generator'],
      forbiddenTools: [],
      requiredInputs: [],
      requiredOutputs: ['VIDEO_PROPOSAL'],
    },
    {
      id: 'script_writer',
      name: 'Script Writer',
      displayName: '脚本编剧',
      stage: 'script',
      goal: '写脚本',
      allowedTools: ['video_script_generator'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_PROPOSAL'],
      requiredOutputs: ['VIDEO_SCRIPT'],
    },
    {
      id: 'storyboard_artist',
      name: 'Storyboard Artist',
      displayName: '分镜导演',
      stage: 'storyboard',
      goal: '拆画面',
      allowedTools: ['shot_splitter'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_SCRIPT'],
      requiredOutputs: ['CARD_PLAN'],
    },
  ]
  const approvedReviewFallbackReviews = [
    {
      id: 'proposal-approved',
      nodeId: 'proposal-review',
      status: 'APPROVED',
      roleAgentId: 'creative_director',
      stage: 'proposal',
      tool: 'proposal_generator',
      reviewContent: '# 创意方案\n\n佛得角国家介绍与世界杯奇迹。',
    },
    {
      id: 'script-approved',
      nodeId: 'script-review',
      status: 'APPROVED',
      roleAgentId: 'script_writer',
      stage: 'script',
      tool: 'video_script_generator',
      reviewOutput: { script: '佛得角是西非岛国，人口不多，却踢出了世界杯奇迹。' },
    },
    {
      id: 'storyboard-approved',
      nodeId: 'storyboard-review',
      status: 'APPROVED',
      roleAgentId: 'storyboard_artist',
      stage: 'storyboard',
      tool: 'shot_splitter',
      reviewOutput: {
        shotList: [
          {
            shotId: 'SHOT_01',
            durationSec: 7,
            narrationText: '佛得角是西非岛国。',
            visual: '地图上突出佛得角群岛。',
          },
        ],
        totalDurationSec: 7,
      },
    },
  ]
  const approvedReviewFallbackTrace = {
    nodes: approvedReviewFallbackRoles.map((role) => ({
      id: `${role.stage}-exec`,
      name: 'external',
      type: 'TOOL',
      status: 'SUCCESS',
      input: { tool: 'external', capabilityTool: role.allowedTools[0], stage: role.stage, roleAgentId: role.id },
      output: { content: `${role.stage} finished without artifact manifest` },
    })),
  }
  const approvedReviewFallbackStages = buildDirectorStages(
    approvedReviewFallbackRoles,
    approvedReviewFallbackReviews,
    approvedReviewFallbackTrace,
    true,
  )
  assert.deepEqual(approvedReviewFallbackStages.map((stage) => stage.status), ['done', 'done', 'done'])
  const approvedReviewFallbackArtifacts = buildDirectorArtifacts(
    approvedReviewFallbackRoles,
    approvedReviewFallbackReviews,
    approvedReviewFallbackTrace,
  )
  assert.deepEqual(approvedReviewFallbackArtifacts.map((artifact) => artifact.status), ['valid', 'valid', 'valid'])
  assert.deepEqual(approvedReviewFallbackArtifacts.map((artifact) => artifact.humanApproved), [true, true, true])
  const proposalSelection = getArtifactViewerSelection(undefined, approvedReviewFallbackArtifacts[0])
  assert.equal(proposalSelection.shouldLoad, false)
  assert.match(proposalSelection.placeholder || '', /佛得角国家介绍/)
  const storyboardSelection = getArtifactViewerSelection(undefined, approvedReviewFallbackArtifacts[2])
  assert.match(storyboardSelection.placeholder || '', /分镜队列/)
  assert.match(storyboardSelection.placeholder || '', /SHOT_01/)

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
  assert.equal(
    localArtifactIdFromStorageRef('local://projects/vp-1/artifacts/final-video/final-video/hash/final.mp4'),
    'final-video',
  )
  assert.equal(
    localArtifactIdFromStorageRef('local://projects/20260701090839-40404040/manual-final.mp4'),
    undefined,
  )
  const finalVideoCandidates = [
    {
      id: 'shot-video-result-1',
      kind: 'VIDEO',
      name: 'SHOT_01 视频回填结果',
      stageName: 'external_generation_result',
      unitId: 'extgen_video_SHOT_01',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/shot-video-result-1/shot-video-result-1/hash/result.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_result', generationKind: 'video', externalGenerationRequestId: 'extgen_video_SHOT_01' },
    },
    {
      id: 'art-render-placeholder',
      kind: 'VIDEO',
      name: 'final.mp4',
      stageName: 'render',
      unitId: 'final-video',
      status: 'valid',
      owner: '渲染制片',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/20260701090839-40404040/manual-final.mp4',
      metadata: { manualUpload: true, status: 'manual_upload_required' },
    },
    {
      id: 'art-final-upload',
      kind: 'VIDEO',
      name: 'final_video.mp4',
      stageName: 'external_generation_result',
      unitId: 'final-video',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/codex-smoke-final-video/codex-smoke-final-video/hash/final.mp4',
      metadata: { artifactType: 'external_generation_result', generationKind: 'video', generationRequestId: 'final-video', tags: ['final_video'] },
    },
  ]
  assert.equal(findFinalVideoArtifact(finalVideoCandidates)?.id, 'art-final-upload')
  assert.equal(findFinalVideoArtifact(finalVideoCandidates.slice(0, 2))?.id, 'art-render-placeholder')

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
      id: 'shot-video-request-1',
      kind: 'EXTERNAL_GENERATION_REQUEST',
      name: 'SHOT_01 视频生成请求',
      status: 'review',
      owner: '素材依赖点',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'inline://extgen-video',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_request', generationKind: 'video', externalGenerationRequestId: 'extgen_video_SHOT_01' },
    },
    {
      id: 'shot-video-result-1',
      kind: 'VIDEO',
      name: 'SHOT_01 视频回填结果',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/result.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_result', generationKind: 'video', externalGenerationRequestId: 'extgen_video_SHOT_01' },
    },
    {
      id: 'shot-storyboard-result-1',
      kind: 'IMAGE',
      name: 'SHOT_01 故事板上传结果',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/storyboard.png',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_result', assetType: 'image', tags: ['manual_shot_upload', 'shot_storyboard'] },
    },
    {
      id: 'shot-generation-plan',
      kind: 'SHOT_GENERATION_PLAN',
      name: 'Shot generation plan',
      status: 'valid',
      owner: '策略规划',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'inline://shot-generation-plan',
      metadata: {
        artifactType: 'shot_generation_plan',
        inlineContent: {
          shotGenerationPlans: [
            { shotId: 'SHOT_01', mode: 'hybrid_aigc_bg_html_overlay', reason: '需要 AIGC 背景视频配合 HyperFrames 精确文字层。', riskLevel: 'medium' },
            { shotId: 'SHOT_02', mode: 'html_only', reason: '纯文字和图表动画可由 HyperFrames 完成。', riskLevel: 'low' },
          ],
        },
      },
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
  assert.deepEqual(shotReviewGroups[0].artifactCounts, { total: 7, references: 1, media: 2, reviewPackets: 1 })
  assert.deepEqual(shotReviewGroups[0].slots.map((slot) => slot.kind), ['prompt', 'reference', 'storyboard', 'base-media', 'overlay', 'video'])
  assert.deepEqual(shotReviewGroups[0].generationStrategy, {
    mode: 'hybrid_aigc_bg_html_overlay',
    label: 'Hybrid',
    reason: '需要 AIGC 背景视频配合 HyperFrames 精确文字层。',
    riskLevel: 'medium',
  })
  assert.equal(shotReviewGroups[0].slots.find((slot) => slot.kind === 'prompt')?.dependencyRequests.some((artifact) => artifact.id === 'shot-video-request-1'), false)
  assert.equal(shotReviewGroups[0].slots.find((slot) => slot.kind === 'base-media')?.dependencyRequests.some((artifact) => artifact.id === 'shot-video-request-1'), false)
  assert.equal(unresolvedMaterialDependencyCount(shotReviewGroups), 0)
  assert.ok(shotReviewGroups[0].slots.find((slot) => slot.kind === 'storyboard')?.artifacts.some((artifact) => artifact.id === 'shot-storyboard-result-1'))
  assert.equal(shotReviewGroups[1].shotId, 'SHOT_02')
  assert.equal(shotReviewGroups[1].status, 'valid')
  assert.equal(shotReviewGroups[1].generationStrategy?.label, 'HyperFrames')

  const profileArtifacts = [
    {
      id: 'video-creation-profile-1',
      kind: 'VIDEO_CREATION_PROFILE',
      name: '创作主线',
      status: 'valid',
      owner: '创意总监',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'inline://video-creation-profile',
      metadata: {
        profileId: 'cinematic_story',
        primaryArtifact: 'CONTINUITY_BIBLE',
        qualityContract: ['aigc_time_windows_3_15s'],
      },
    },
    {
      id: 'time-window-plan-1',
      kind: 'TIME_WINDOW_PLAN',
      name: '时间窗计划',
      status: 'valid',
      owner: '结构导演',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'inline://time-window-plan',
      metadata: {
        windows: [
          {
            id: 'SHOT_01_TW_01',
            shotId: 'SHOT_01_TW_01',
            parentShotId: 'SHOT_01',
            durationSec: 8,
            aigcEligible: true,
          },
        ],
      },
    },
  ]
  const profileSummary = creationProfileSummary(profileArtifacts)
  assert.equal(profileSummary.profileId, 'cinematic_story')
  assert.equal(profileSummary.label, '影视剧情')
  assert.equal(profileSummary.primaryArtifact, 'CONTINUITY_BIBLE')
  assert.ok(profileSummary.qualityContract.includes('aigc_time_windows_3_15s'))
  assert.equal(creationProfileSummary([{
    ...profileArtifacts[0],
    metadata: { profileId: 'talking_head' },
  }]).label, '口播解说')
  assert.equal(creationProfileSummary([]).label, '未选择')
  assert.equal(creationProfileSummary([{
    ...profileArtifacts[0],
    metadata: { cloudPayloadStored: true },
    inlineJson: JSON.stringify({
      profileId: 'cinematic_story',
      primaryArtifact: 'CONTINUITY_BIBLE',
      qualityContract: ['aigc_time_windows_3_15s'],
    }),
  }]).profileId, 'cinematic_story')
  assert.equal(creationProfileSummary([{
    ...profileArtifacts[0],
    metadata: {
      cloudPayloadStored: true,
      inlineContent: {
        creationProfile: {
          profileId: 'talking_head',
          primaryArtifact: 'VIDEO_SCRIPT',
          qualityContract: ['script_timeline_first'],
        },
      },
    },
  }]).primaryArtifact, 'VIDEO_SCRIPT')
  assert.deepEqual(timeWindowPlanSummary(profileArtifacts), {
    totalWindows: 1,
    aigcWindowCount: 1,
    invalidDurationCount: 0,
  })
  assert.deepEqual(timeWindowPlanSummary([{
    ...profileArtifacts[1],
    metadata: {
      timeWindowPlan: {
        windows: [
          { durationSec: '16', aigcEligible: 'true' },
          { durationSec: 2, aigcEligible: false },
          { aigcEligible: true },
        ],
      },
    },
  }]), {
    totalWindows: 3,
    aigcWindowCount: 2,
    invalidDurationCount: 1,
  })
  assert.deepEqual(timeWindowPlanSummary([{
    ...profileArtifacts[1],
    metadata: { cloudPayloadStored: true },
    inlineJson: JSON.stringify({
      timeWindowPlan: {
        windows: [
          { durationSec: 8, aigcEligible: 'yes' },
          { durationSec: 2, aigcEligible: 'off' },
        ],
      },
    }),
  }]), {
    totalWindows: 2,
    aigcWindowCount: 1,
    invalidDurationCount: 0,
  })
  assert.deepEqual(timeWindowPlanSummary([{
    ...profileArtifacts[1],
    metadata: {
      cloudPayloadStored: true,
      inlineContent: {
        timeWindows: [
          { durationSec: 16, aigcEligible: true },
        ],
      },
    },
  }]), {
    totalWindows: 1,
    aigcWindowCount: 1,
    invalidDurationCount: 1,
  })

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

  const shotJsonReview = {
    id: 'shot-review',
    nodeId: 'shot-review',
    status: 'PENDING',
    tool: 'shot_splitter',
    reviewContent: JSON.stringify({
      shotList: [
        {
          shotId: 'SHOT_01',
          durationSec: 7,
          narrationText: '佛得角是西非岛国。',
          visual: '地图上突出佛得角群岛。',
          materialLibraryHints: ['佛得角群岛地图', '足球场'],
        },
        {
          shotId: 'SHOT_02',
          durationSec: 8,
          narrationText: '世界杯出线是小国奇迹。',
          visual: '球迷庆祝与比分数据可视化。',
        },
      ],
      shotAssetPackages: [{ shotId: 'SHOT_01' }, { shotId: 'SHOT_02' }],
      totalDurationSec: 15,
    }),
  }
  const shotReviewText = reviewOutputText(shotJsonReview)
  assert.match(shotReviewText, /分镜队列/)
  assert.match(shotReviewText, /当前先审核：SHOT_01/)
  assert.match(shotReviewText, /佛得角是西非岛国/)
  assert.ok(!shotReviewText.includes('shotAssetPackages'), shotReviewText)

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

  const apiSource = await readFile(new URL('../src/services/api.ts', import.meta.url), 'utf8')
  const agentRunTimeout = Number(apiSource.match(/const AGENT_RUN_REQUEST_TIMEOUT_MS = (\d+)/)?.[1] || 0)
  assert.ok(
    agentRunTimeout >= 300000,
    'agent run start timeout must allow slow provider-backed planning and fallback',
  )
  assert.ok(
    apiSource.includes('AGENT_RUN_REQUEST_TIMEOUT_MS') &&
      /api\.post[\s\S]*?\('\/agent\/runs', payload,\s*\{[\s\S]*timeout:\s*AGENT_RUN_REQUEST_TIMEOUT_MS/.test(apiSource),
    'startAgentRun must override the default 30000ms axios timeout for slower provider-backed planning',
  )
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
