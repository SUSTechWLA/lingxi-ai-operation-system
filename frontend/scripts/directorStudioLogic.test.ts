import assert from 'node:assert/strict'

import {
  applyOptimisticRunningStage,
  buildDirectorArtifacts,
  buildDirectorStages,
  buildDirectorTraceNodes,
  canStartFinalRender,
  deriveNextAction,
  downstreamStaleArtifacts,
  nextStageIdAfterReview,
  stageActionLabel,
  traceNodeHasError,
} from '../src/pages/directorStudioLogic.ts'
import type { AgentReviewItem, VideoRoleAgent } from '../src/utils/types.ts'

const roleAgents: VideoRoleAgent[] = [
  {
    id: 'creative_director',
    name: 'Creative Director',
    displayName: '创意总监',
    stage: 'proposal',
    goal: '确定创作方向',
    allowedTools: ['proposal_generator'],
    requiredOutputs: ['VIDEO_PROPOSAL'],
    humanReview: { required: true, reviewFocus: ['主题是否准确'] },
  },
  {
    id: 'script_writer',
    name: 'Script Writer',
    displayName: '脚本编剧',
    stage: 'script',
    goal: '生成口播脚本',
    allowedTools: ['video_script_generator'],
    requiredOutputs: ['VIDEO_SCRIPT'],
    humanReview: { required: true, reviewFocus: ['口播是否自然'] },
  },
]

const reviews: AgentReviewItem[] = [
  {
    id: 'review-script',
    nodeId: 'review-script-node',
    status: 'PENDING',
    stepId: 'script_writer_1',
    tool: 'video_script_generator',
    stage: 'script',
    roleAgentId: 'script_writer',
    requiredOutputs: ['VIDEO_SCRIPT'],
    humanReview: { required: true, reviewFocus: ['口播是否自然'] },
  },
]

const stages = buildDirectorStages(roleAgents, reviews, {
  nodes: [
    {
      id: 'proposal-1',
      name: 'proposal_generator',
      status: 'SUCCESS',
      input: { roleAgentId: 'creative_director', stage: 'proposal' },
      output: { artifacts: [{ name: '创意方案', kind: 'VIDEO_PROPOSAL' }] },
      createdAt: '2026-06-25T10:00:00Z',
    },
  ],
})

assert.equal(stages.length, 2)
assert.deepEqual(stages.map((stage) => stage.id), ['creative_director', 'script_writer'])
assert.equal(stages[0].status, 'done')
assert.equal(stages[1].status, 'review')
assert.equal(stages[1].reviewId, 'review-script')
assert.deepEqual(stages[1].reviewFocus, ['口播是否自然'])

const artifacts = buildDirectorArtifacts(roleAgents, reviews, {
  nodes: [
    {
      id: 'proposal-1',
      name: 'proposal_generator',
      status: 'SUCCESS',
      input: { roleAgentId: 'creative_director', stage: 'proposal' },
      output: { artifacts: [{ name: '创意方案', kind: 'VIDEO_PROPOSAL', storageRef: 'cloud://proposal' }] },
      createdAt: '2026-06-25T10:00:00Z',
    },
  ],
})

assert.equal(artifacts.length, 2)
assert.equal(artifacts[0].status, 'valid')
assert.equal(artifacts[0].storageRef, '本地项目目录（仅同步索引）')
assert.equal(artifacts[1].status, 'review')
assert.equal(artifacts[1].humanApproved, false)

const traceNodes = buildDirectorTraceNodes({
  nodes: [
    {
      id: 'script-1',
      name: 'video_script_generator',
      status: 'RUNNING',
      input: { roleAgentId: 'script_writer', roleAgent: { displayName: '脚本编剧' }, stage: 'script' },
      output: { artifactKind: 'VIDEO_SCRIPT' },
      createdAt: '2026-06-25T10:01:00Z',
    },
  ],
})

assert.equal(traceNodes[0].role, '脚本编剧')
assert.equal(traceNodes[0].status, 'running')
assert.equal(traceNodes[0].tool, 'video_script_generator')
assert.equal(traceNodeHasError(undefined), false)

const stagedFlow = buildDirectorStages(
  [
    { ...roleAgents[1] },
    {
      id: 'storyboard_artist',
      name: 'Storyboard Artist',
      displayName: '卡片设计师',
      stage: 'storyboard',
      goal: '生成卡片计划',
      allowedTools: ['card_plan_generator'],
      requiredOutputs: ['CARD_PLAN'],
    },
  ],
  reviews,
  { nodes: [] },
)
assert.equal(nextStageIdAfterReview(stagedFlow, reviews[0]), 'storyboard_artist')

const optimisticStages = applyOptimisticRunningStage(stagedFlow, 'storyboard_artist')
assert.equal(optimisticStages[1].status, 'running')
assert.equal(optimisticStages[1].progress, 18)
assert.equal(stagedFlow[1].status, 'pending')

const actualRunningStages = applyOptimisticRunningStage(
  stagedFlow.map((stage) => stage.id === 'storyboard_artist' ? { ...stage, status: 'review' } : stage),
  'storyboard_artist',
)
assert.equal(actualRunningStages[1].status, 'review')

const reviewGateStages = buildDirectorStages(roleAgents, [], {
  nodes: [
    {
      id: 'proposal-review',
      name: '审核-proposal_generator',
      type: 'REVIEW_GATE',
      status: 'READY',
      input: { tool: 'proposal_generator', stage: 'proposal', reviewPhase: 'after_artifact' },
      output: { content: '# 创作方案' },
    },
  ],
})
assert.equal(reviewGateStages[0].status, 'review')

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
assert.equal(nestedTraceNodes[0].tool, '质量门禁-video_script_generator_quality_gate')
assert.equal(nestedTraceNodes[0].error, '评分未达标')
assert.equal(traceNodeHasError(nestedTraceNodes[0]), true)

const nextAction = deriveNextAction(stages)
assert.equal(nextAction?.stageId, 'script_writer')
assert.equal(nextAction?.kind, 'review')

assert.deepEqual(
  roleAgents.map((role) => stageActionLabel(role.stage)),
  ['定方向', '写脚本'],
)

assert.deepEqual(
  downstreamStaleArtifacts('VIDEO_SCRIPT'),
  ['卡片分镜', '视频结构', '素材策略', '一致性报告', '视频项目', '预览图', '最终视频', '质量报告', '交付包'],
)

assert.equal(
  canStartFinalRender([
    { ...artifacts[0], kind: 'VIDEO_COMPOSITION_SPEC', status: 'valid', humanApproved: true },
    { ...artifacts[0], kind: 'HYPERFRAMES_PROJECT', status: 'valid', humanApproved: false },
    { ...artifacts[0], kind: 'PREVIEW_SNAPSHOTS', status: 'valid', humanApproved: false },
  ], true).allowed,
  false,
)

assert.equal(
  canStartFinalRender([
    { ...artifacts[0], kind: 'VIDEO_COMPOSITION_SPEC', status: 'valid', humanApproved: true },
    { ...artifacts[0], kind: 'HYPERFRAMES_PROJECT', status: 'valid', humanApproved: false },
    { ...artifacts[0], kind: 'PREVIEW_SNAPSHOTS', status: 'valid', humanApproved: true },
  ], true).allowed,
  true,
)

console.log('directorStudioLogic tests passed')
