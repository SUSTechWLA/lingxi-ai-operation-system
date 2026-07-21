import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const temp = await mkdtemp(join(tmpdir(), 'creator-studio-'))
const bundle = join(temp, 'logic.mjs')
const focusBundle = join(temp, 'focus-cycle.mjs')

try {
  execFileSync(process.execPath, [
    new URL('../node_modules/typescript/bin/tsc', import.meta.url).pathname,
    '--noEmit', '--strict', '--skipLibCheck', '--module', 'ESNext', '--moduleResolution', 'bundler',
    '--target', 'ES2020', new URL('./fixtures/creator-contract-types.ts', import.meta.url).pathname,
  ], { stdio: 'inherit' })

  await build({
    entryPoints: [new URL('../src/features/creator-studio/logic.ts', import.meta.url).pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: bundle,
  })
  const logic = await import(pathToFileURL(bundle))
  await build({
    entryPoints: [new URL('../src/features/creator-studio/focusCycle.ts', import.meta.url).pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: focusBundle,
  })
  const focus = await import(pathToFileURL(focusBundle))

  const emptyView = { steps: [] }
  assert.deepEqual(logic.nextCreatorAction(emptyView), {
    kind: 'start', stepId: 'requirements', label: '开始创作',
  })
  const steps = [
    { id: 'requirements', label: '需求', state: 'confirmed' },
    { id: 'direction', label: '创作方向', state: 'generating' },
    { id: 'script', label: '口播稿', state: 'needs_review' },
  ]
  assert.deepEqual(logic.nextCreatorAction({ steps }), {
    kind: 'wait', stepId: 'direction', label: '创作方向生成中',
  })
  assert.deepEqual(logic.nextCreatorAction({ steps: [{ id: 'script', label: '口播稿', state: 'needs_review' }] }), {
    kind: 'review', stepId: 'script', label: '审核口播稿',
  })
  assert.deepEqual(logic.nextCreatorAction({ steps: [{ id: 'shots', label: '分镜与素材', state: 'failed' }] }), {
    kind: 'fix', stepId: 'shots', label: '处理分镜与素材',
  })

  assert.deepEqual(logic.CREATOR_WORKSPACE_STEP_IDS, [
    'requirements', 'direction', 'script', 'shots', 'preview', 'delivery',
  ], 'workspace must keep the six creation steps in their real order')
  assert.deepEqual(
    logic.creatorProjectProgress('COMPLETED', { steps: logic.CREATOR_WORKSPACE_STEP_IDS.map(id => ({ id, state: 'idle' })) }),
    { label: '已完成 6/6 个步骤', percent: 100 },
    'a terminal project must not be shown as 0/6 after the agent run completes',
  )
  assert.deepEqual(
    logic.creatorProjectProgress('RUNNING', { steps: [{ id: 'requirements', state: 'confirmed' }, { id: 'script', state: 'needs_review' }] }),
    { label: '已完成 1/2 个步骤', percent: 50 },
  )
  const localRenderContent = {
    artifact: {
      projectId: 'vp-1',
      metadata: { localOnly: true, localPath: '/tmp/tangying/projects/vp-1/renders/final.mp4' },
    },
  }
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', localRenderContent, 'http://127.0.0.1:18080/'),
    'http://127.0.0.1:18080/api/local/media?projectId=vp-1&path=%2Ftmp%2Ftangying%2Fprojects%2Fvp-1%2Frenders%2Ffinal.mp4',
  )
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: 'https://media.example/final.mp4' }, 'http://127.0.0.1:18080'),
    'https://media.example/final.mp4',
  )
  assert.equal(logic.resolveCreatorArtifactMediaUrl('other-project', localRenderContent, 'http://127.0.0.1:18080'), undefined)
  assert.equal(logic.isCreatorStepReadable({ state: 'confirmed' }), true)
  assert.equal(logic.isCreatorStepReadable({ state: 'needs_attention' }), true)
  assert.equal(logic.canConfirmCreatorStep({ state: 'confirmed', allowedActions: ['confirm'] }), false)
  assert.equal(logic.canConfirmCreatorStep({ state: 'needs_attention', allowedActions: ['confirm'] }), false)
  assert.equal(logic.canConfirmCreatorStep({ state: 'needs_review', allowedActions: [] }), false)
  assert.equal(logic.canConfirmCreatorStep({ state: 'needs_review', allowedActions: ['confirm'] }), true)
  assert.equal(
    logic.formatStepImpact({ affectedStepIds: ['shots', 'preview', 'delivery'], requiresConfirmation: true }),
    '分镜与素材、成片预览、交付需要更新',
  )
  assert.deepEqual(
    logic.normalizeRectSelection({ x: 80, y: 60, width: -30, height: -20 }, { width: 100, height: 100 }),
    { kind: 'rect', x: 0.5, y: 0.4, width: 0.3, height: 0.2 },
  )
  assert.deepEqual(
    logic.createTimeSelection(2800, -1200),
    { kind: 'time', startMs: 0, endMs: 2800 },
  )
  const instructionMutation = {
    artifactId: 'artifact-1', baseVersion: 2, mode: 'instruction', instruction: '语气更轻快',
    selection: { kind: 'time', startMs: 1200, endMs: 2800 }, confirmedAffectedShotIds: ['shot-02'],
  }
  assert.equal(
    logic.creatorMutationIdempotencyKey('project-1', 'script', instructionMutation),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { ...instructionMutation, confirmedAffectedShotIds: ['shot-02'] }),
    'retries of the same requested revision need a stable idempotency key',
  )
  assert.notEqual(
    logic.creatorMutationIdempotencyKey('project-1', 'script', instructionMutation),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { ...instructionMutation, instruction: '改成沉稳语气' }),
  )
  assert.notEqual(
    logic.creatorMutationIdempotencyKey('project-1', 'script', instructionMutation),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { ...instructionMutation, confirmedAffectedShotIds: ['shot-03'] }),
  )
  assert.notEqual(
    logic.creatorMutationIdempotencyKey('project-1', 'script', { artifactId: 'artifact-1', baseVersion: 2, mode: 'restore', version: 1, confirmedAffectedShotIds: [] }),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { artifactId: 'artifact-1', baseVersion: 2, mode: 'restore', version: 2, confirmedAffectedShotIds: [] }),
  )
  assert.equal(logic.creatorPollDelay(0), 500)
  assert.equal(logic.creatorPollDelay(1), 1000)
  assert.equal(logic.creatorPollDelay(20), 5000)
  const scriptKey = logic.workspaceArtifactKey({ stepId: 'script', artifactId: 'script-v1', version: 1 })
  const directionKey = logic.workspaceArtifactKey({ stepId: 'direction', artifactId: 'direction-v1', version: 1 })
  const scriptResult = { key: scriptKey, value: { artifact: { id: 'script-v1' }, versions: ['v1'] } }
  assert.equal(logic.isCurrentWorkspaceArtifact(scriptResult, { stepId: 'direction', artifactId: 'direction-v1', version: 1 }), false, 'step B must not render step A while B loads')
  const directionResult = { key: directionKey, value: { artifact: { id: 'direction-v1' }, versions: ['v1'] } }
  assert.equal(logic.isCurrentWorkspaceArtifact(directionResult, { stepId: 'direction', artifactId: 'direction-v1', version: 1 }), true)
  assert.equal(logic.isCurrentWorkspaceArtifact(scriptResult, { stepId: 'direction', artifactId: 'direction-v1', version: 1 }), false, 'late A response must not overwrite B')
  assert.equal(logic.isLatestWorkspaceRequest(2, 2), true)
  assert.equal(logic.isLatestWorkspaceRequest(1, 2), false)
  const previousTasks = [{ id: 'task-a', shotId: 'shot-a', status: 'running' }, { id: 'task-b', shotId: 'shot-b', status: 'running' }]
  assert.equal(logic.didSelectedShotTaskChange(previousTasks, [{ id: 'task-b', shotId: 'shot-b', status: 'running' }], 'shot-a'), true, 'selected shot task disappearing must refetch')
  assert.equal(logic.didSelectedShotTaskChange(previousTasks, [{ id: 'task-a', shotId: 'shot-a', status: 'running' }, { id: 'task-b', shotId: 'shot-b', status: 'processing' }], 'shot-a'), false, 'other shot changes must not refetch selected shot')
  assert.equal(logic.didSelectedShotTaskChange(previousTasks, [], undefined), false, 'no selected shot must not fan out callbacks')

  const pendingReview = logic.nextPendingCreatorReview([
    { id: 'approved-script', nodeId: 'script-review', status: 'APPROVED', stage: 'script_generation' },
    { id: 'pending-preview', nodeId: 'preview-review', status: 'PENDING', stage: 'preview', reviewContent: '检查文字层' },
  ])
  assert.equal(pendingReview.id, 'pending-preview', 'creator workspace must surface the next dynamic review gate')
  assert.equal(logic.creatorStepForAgentReview(pendingReview), 'preview')
  assert.equal(logic.creatorStepForAgentReview({ stage: 'script_generation', tool: 'video_script_generator' }), 'script')
  assert.equal(logic.creatorStepForAgentReview({ stage: 'visual_alignment', tool: 'visual_alignment_planner' }), 'shots')
  assert.equal(logic.creatorStepForAgentReview({ stage: 'publish_copy', tool: 'publish_copy_generator' }), 'delivery')

  const hundredShotWindow = logic.shotQueueWindow({
    total: 100, scrollTop: 0, viewportHeight: 480,
  })
  assert.equal(logic.DEFAULT_SHOT_QUEUE_FILTER.status, 'needs_attention', 'the review queue starts with actionable Shots')
  assert.ok(hundredShotWindow.items.length <= 12, '480px / 64px rows with overscan must render at most 12 queue rows')
  assert.deepEqual(hundredShotWindow.items, Array.from({ length: hundredShotWindow.items.length }, (_, index) => index))
  assert.equal(logic.selectedShotAfterAppend('shot-024', [{ id: 'shot-001' }], [{ id: 'shot-025' }]), 'shot-024', 'page append never clears the active Shot')
  assert.equal(logic.selectedShotAfterAppend(undefined, [{ id: 'shot-001' }], [{ id: 'shot-025' }]), 'shot-001', 'initial page picks its first Shot')
  assert.equal(logic.selectedShotAfterReplacement('shot-024', [{ id: 'shot-024' }, { id: 'shot-025' }]), 'shot-024', 'a queue refresh preserves the selected Shot when it still matches')
  assert.equal(logic.selectedShotAfterReplacement('shot-024', [{ id: 'shot-025' }]), 'shot-025', 'a queue refresh moves only when the selected Shot no longer matches')
  assert.equal(logic.isShotRetryEligible({ qaStatus: '', candidates: [{ status: 'SHOT_QA_FAILED' }] }), true, 'candidate QA failures offer a scoped retry')
  assert.equal(logic.isShotRetryEligible({ qaStatus: '', candidates: [{ status: 'SHOT_QA_PASSED', qaReport: { status: 'SHOT_QA_FAILED' } }] }), true, 'candidate report failures offer a scoped retry')
  assert.equal(logic.isShotRetryEligible({ qaStatus: 'SHOT_QA_PASSED', candidates: [] }), false)
  const adoptedTasks = logic.adoptCreatorShotTask(
    [{ id: 'task-older', scope: 'shots', shotId: 'shot-001', status: 'running', label: '旧任务' }],
    { id: 'task-new', scope: 'shots', shotId: 'shot-002', status: 'queued', label: '正在重新生成镜头' },
  )
  assert.deepEqual(adoptedTasks.map(task => task.id), ['task-older', 'task-new'], 'adopting a new Shot task keeps other durable Shot work active')
  assert.equal(focus.cycleFocusIndex(1, 3, false), 2)
  assert.equal(focus.cycleFocusIndex(0, 3, true), 2)
  assert.equal(focus.cycleFocusIndex(-1, 3, true), 2)

  const creationRequest = logic.buildCreationRequest({
    prompt: '为夏日咖啡新品拍一支轻快的竖版短片',
    durationSec: 30,
    aspectRatio: '9:16',
    platform: '抖音',
    materialCount: 2,
    productionRoute: 'cinematic_story',
    modelProviders: {
      text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-private' },
    },
  })
  assert.deepEqual(creationRequest.project, {
    name: '为夏日咖啡新品拍一支轻快的竖版短片',
    description: '为夏日咖啡新品拍一支轻快的竖版短片',
    mode: 'aigc_shot',
    skillName: 'video-creator',
    skillVersion: 'v4.0',
    workflowName: 'dynamic-agent-video-creation',
    workflowVersion: 'v4.0',
    generationMode: 'provider_api',
    aspectRatio: '9:16',
    targetDurationSec: 30,
    language: 'zh-CN',
    config: {
      entry: 'creator_studio',
      topic: '为夏日咖啡新品拍一支轻快的竖版短片',
      durationSec: 30,
      targetDurationSec: 30,
      aspectRatio: '9:16',
      platform: '抖音',
      materialCount: 2,
      productionRoute: 'cinematic_story',
      canonicalProfileId: 'cinematic_story',
      aigcEnabled: true,
      aigcProvider: 'auto',
      aigcPolicy: 'auto',
      visualLayerContract: 'shot_visual_layers_v1',
      designedLayers: ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'],
      layerExecutionPolicy: {
        ip_aroll: 'optional', hyperframes_text: 'required', aigc_enrichment: 'auto',
      },
      requiredLayers: ['aigc_main', 'hyperframes_text'],
      modelProviderRefs: {
        text_to_text: { source: 'local_agent', baseUrl: 'https://model.test/v1', model: 'writer' },
      },
    },
  })
  assert.deepEqual(creationRequest.agentRun, {
    message: '创作一支 30 秒、适合抖音发布的视频：为夏日咖啡新品拍一支轻快的竖版短片',
    domain: 'video_creation',
    mode: 'dynamic_agent',
    context: {
      topic: '为夏日咖啡新品拍一支轻快的竖版短片',
      durationSec: 30,
      targetDurationSec: 30,
      aspectRatio: '9:16',
      platform: '抖音',
      materialCount: 2,
      productionRoute: 'cinematic_story',
      canonicalProfileId: 'cinematic_story',
      videoType: 'aigc_shot',
      aigcEnabled: true,
      aigcProvider: 'auto',
      aigcPolicy: 'auto',
      visualLayerContract: 'shot_visual_layers_v1',
      designedLayers: ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'],
      layerExecutionPolicy: {
        ip_aroll: 'optional', hyperframes_text: 'required', aigc_enrichment: 'auto',
      },
      requiredLayers: ['aigc_main', 'hyperframes_text'],
      modelProviders: {
        text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-private' },
      },
    },
  })
  assert.doesNotMatch(JSON.stringify(creationRequest.project), /sk-private/, 'project config must never persist API keys')
  const creationCopy = `${creationRequest.project.description} ${creationRequest.agentRun.message}`
  assert.doesNotMatch(creationCopy, /provider|run|trace|artifact/i, 'creator copy must not expose developer vocabulary')

  const talkingHeadRequest = logic.buildCreationRequest({
    prompt: '树懒阿洛分享三个提高专注力的小技巧，配合简洁标题卡片',
    durationSec: 15,
    aspectRatio: '16:9',
    materialCount: 0,
    productionRoute: 'talking_head',
    modelProviders: {
      text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-text' },
      text_to_image: { baseUrl: 'https://model.test/v1', model: 'image', apiKey: 'sk-image' },
      text_to_video: { baseUrl: 'https://model.test/v1', model: 'video', apiKey: 'sk-video' },
    },
  })
  assert.equal(talkingHeadRequest.project.mode, 'voice_visual')
  assert.equal(talkingHeadRequest.project.generationMode, 'provider_api')
  assert.equal(talkingHeadRequest.project.config.productionRoute, 'talking_head')
  assert.equal(talkingHeadRequest.project.config.canonicalProfileId, 'talking_head')
  assert.equal(talkingHeadRequest.project.config.aigcEnabled, true)
  assert.equal(talkingHeadRequest.project.config.aigcPolicy, 'auto')
  assert.equal(talkingHeadRequest.project.config.visualLayerContract, 'shot_visual_layers_v1')
  assert.deepEqual(talkingHeadRequest.project.config.designedLayers, ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'])
  assert.deepEqual(talkingHeadRequest.project.config.layerExecutionPolicy, {
    ip_aroll: 'required', hyperframes_text: 'required', aigc_enrichment: 'auto',
  })
  assert.deepEqual(talkingHeadRequest.project.config.requiredLayers, ['ip_aroll', 'hyperframes_text'])
  assert.equal(talkingHeadRequest.project.config.ipRenderMode, 'preview')
  assert.equal(talkingHeadRequest.agentRun.context.videoType, 'voice_visual')
  assert.equal(talkingHeadRequest.agentRun.context.aigcEnabled, true)
  assert.equal(talkingHeadRequest.agentRun.context.aigcProvider, 'auto')
  assert.equal(talkingHeadRequest.agentRun.context.ipRenderMode, 'preview')
  assert.deepEqual(Object.keys(talkingHeadRequest.agentRun.context.modelProviders), ['text_to_text', 'text_to_image', 'text_to_video'])

  const localTalkingHeadRequest = logic.buildCreationRequest({
    prompt: '纯本地树懒口播，但保留未来 AIGC 丰富层的设计',
    durationSec: 15,
    aspectRatio: '16:9',
    materialCount: 0,
    productionRoute: 'talking_head',
    aigcPolicy: 'disabled',
    modelProviders: {
      text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-text' },
      text_to_video: { baseUrl: 'https://model.test/v1', model: 'video', apiKey: 'sk-video' },
    },
  })
  assert.equal(localTalkingHeadRequest.project.config.aigcEnabled, false)
  assert.equal(localTalkingHeadRequest.project.config.aigcPolicy, 'disabled')
  assert.equal(localTalkingHeadRequest.agentRun.context.aigcProvider, 'disabled')
  assert.deepEqual(localTalkingHeadRequest.agentRun.context.designedLayers, ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'])
  assert.deepEqual(localTalkingHeadRequest.agentRun.context.layerExecutionPolicy, {
    ip_aroll: 'required', hyperframes_text: 'required', aigc_enrichment: 'disabled',
  })
  assert.deepEqual(Object.keys(localTalkingHeadRequest.agentRun.context.modelProviders), ['text_to_text'])
  assert.doesNotMatch(JSON.stringify(localTalkingHeadRequest), /sk-video/)

  const defaultCreationRequest = logic.buildCreationRequest({
    prompt: '做一个品牌故事',
    aspectRatio: '16:9',
    materialCount: 0,
  })
  assert.equal(defaultCreationRequest.project.targetDurationSec, undefined)
  assert.equal(defaultCreationRequest.project.mode, 'voice_visual', 'creator defaults to deterministic talking-head production')
  assert.equal(defaultCreationRequest.project.config.aigcEnabled, true)
  assert.equal(defaultCreationRequest.agentRun.context.durationSec, undefined)
  assert.equal(defaultCreationRequest.agentRun.message, '创作视频：做一个品牌故事')

  let activeWorkers = 0
  let maxActiveWorkers = 0
  const concurrentResults = await logic.mapWithConcurrency([1, 2, 3, 4, 5], 2, async value => {
    activeWorkers += 1
    maxActiveWorkers = Math.max(maxActiveWorkers, activeWorkers)
    await new Promise(resolve => setTimeout(resolve, 1))
    activeWorkers -= 1
    return value * 2
  })
  assert.deepEqual(concurrentResults, [2, 4, 6, 8, 10])
  assert.equal(maxActiveWorkers, 2, 'creation view loading must have a bounded concurrency')

  const settledIndexes = []
  await logic.mapWithConcurrency([1, 2], 2, async value => {
    await new Promise(resolve => setTimeout(resolve, value === 1 ? 3 : 1))
    return value
  }, (_value, index) => settledIndexes.push(index))
  assert.deepEqual(settledIndexes, [1, 0], 'each creation view must update as it settles')

  assert.equal(
    logic.buildProjectMaterialStorageRef('project-1', 'material-1'),
    'local://projects/project-1/materials/material-1',
  )
  for (const unsafe of ['material/1', 'material%1', 'material\\1', 'material?1', 'material#1']) {
    assert.throws(() => logic.buildProjectMaterialStorageRef('project-1', unsafe), /safe path segment/)
  }
  assert.equal(logic.creatorStartIdempotencyKey('project-1'), 'creator-start:project-1')

  const viewOrder = logic.prioritizeCreationViewProjects([
    { id: 'completed-new', status: 'COMPLETED', updatedAt: '2026-07-20T12:00:00Z' },
    { id: 'active-old', status: 'RUNNING', updatedAt: '2026-07-20T09:00:00Z' },
    { id: 'active-new', status: 'PAUSED', updatedAt: '2026-07-20T11:00:00Z' },
    { id: 'archived', status: 'ARCHIVED', updatedAt: '2026-07-20T13:00:00Z' },
  ])
  assert.deepEqual(viewOrder.map(project => project.id), ['active-new', 'active-old', 'archived', 'completed-new'])

  for (const accepted of [0.001, 1, 14.999]) assert.equal(logic.canSubmitShotDuration(accepted), true)
  for (const rejected of [-1, 0, 15, 16, Number.NaN, Number.POSITIVE_INFINITY]) {
    assert.equal(logic.canSubmitShotDuration(rejected), false)
  }
  assert.equal(logic.deliveryArtifactPassesFinalReview({ finalQaStatus: 'passed' }), true)
  assert.equal(logic.deliveryArtifactPassesFinalReview({ finalQualityCheck: { passed: true } }), true)
  assert.equal(logic.deliveryArtifactPassesFinalReview({ finalQaStatus: 'failed' }), false)
	assert.equal(logic.deliveryArtifactPassesFinalReview({ status: 'passed' }), false, 'generic artifact status is not final QA')
	assert.equal(logic.deliveryArtifactPassesFinalReview({ qaStatus: 'passed' }), false, 'Shot QA is not final QA')
  assert.equal(logic.deliveryArtifactPassesFinalReview({ artifact: { metadata: { finalQaStatus: 'passed' } } }), true)
	assert.equal(logic.deliveryArtifactPassesFinalReview({ artifact: { metadata: { qaStatus: 'passed' } } }), false)
  assert.equal(logic.deliveryArtifactPassesFinalReview({ mediaUrl: '/video.mp4' }), false, 'a media URL alone must never unlock delivery')

  const shots = Array.from({ length: 100 }, (_, index) => ({
    id: `shot-${String(100 - index).padStart(3, '0')}`,
    sequenceIndex: index % 10,
    title: index % 2 ? `Night ${index}` : `Morning ${index}`,
    chapter: index % 3 ? 'middle' : 'opening',
    reviewStatus: index % 4 ? 'pending' : 'approved',
    generationStatus: index % 5 ? 'idle' : 'generating',
  }))
  const snapshot = JSON.stringify(shots)
  const selected = logic.selectShotListItems(shots, {
    status: 'pending', chapter: 'middle', query: 'night',
  })
  assert.equal(JSON.stringify(shots), snapshot, 'selector must not mutate input')
  assert.ok(selected.length > 0 && selected.length < 100)
  for (const shot of selected) {
    assert.equal(shot.reviewStatus, 'pending')
    assert.equal(shot.chapter, 'middle')
    assert.match(shot.title.toLowerCase(), /night/)
  }
  assert.deepEqual(
    selected.map(({ sequenceIndex, id }) => [sequenceIndex, id]),
    [...selected].map(({ sequenceIndex, id }) => [sequenceIndex, id]).sort(([aSequence, aId], [bSequence, bId]) =>
      aSequence - bSequence || aId.localeCompare(bId)),
    'Shot ordering is deterministic by sequenceIndex then id',
  )
  const ties = [
    { id: 'same', sequenceIndex: 1, title: 'first', reviewStatus: 'pending', generationStatus: 'PLANNED' },
    { id: 'same', sequenceIndex: 1, title: 'second', reviewStatus: 'pending', generationStatus: 'PLANNED' },
  ]
  assert.deepEqual(logic.selectShotListItems(ties, {}).map(item => item.title), ['first', 'second'])

  assert.equal(logic.CREATOR_CONFLICT_COPY, '内容已更新，请刷新后重试')
  assert.equal(logic.isTargetOnlyShotImpact({
    shotId: 'shot-042', affectedShotIds: ['shot-042'], regeneratesOtherShots: false,
  }, 'shot-042'), true)
  assert.equal(logic.isTargetOnlyShotImpact({
    shotId: 'shot-042', affectedShotIds: ['shot-042', 'shot-043'], regeneratesOtherShots: true,
  }, 'shot-042'), false)

  const apiSource = readFileSync(new URL('../src/services/creatorApi.ts', import.meta.url), 'utf8')
  const runtimeApiSource = readFileSync(new URL('../src/services/api.ts', import.meta.url), 'utf8')
  const typeSource = readFileSync(new URL('../src/features/creator-studio/types.ts', import.meta.url), 'utf8')
  const generatedSource = readFileSync(new URL('../src/utils/api-types.generated.ts', import.meta.url), 'utf8')
  const startPageSource = readFileSync(new URL('../src/features/creator-studio/StartCreationPage.tsx', import.meta.url), 'utf8')
  const librarySource = readFileSync(new URL('../src/features/creator-studio/VideoLibraryPage.tsx', import.meta.url), 'utf8')
  const workspaceSource = readFileSync(new URL('../src/features/creator-studio/ProjectWorkspacePage.tsx', import.meta.url), 'utf8')
  const stripSource = readFileSync(new URL('../src/features/creator-studio/components/CreationStrip.tsx', import.meta.url), 'utf8')
  const reviewSource = readFileSync(new URL('../src/features/creator-studio/components/ArtifactReviewPanel.tsx', import.meta.url), 'utf8')
  const recoverySource = readFileSync(new URL('../src/features/creator-studio/components/TaskRecoveryBanner.tsx', import.meta.url), 'utf8')
  const queueSource = readFileSync(new URL('../src/features/creator-studio/components/ShotReviewQueue.tsx', import.meta.url), 'utf8')
  const inspectorSource = readFileSync(new URL('../src/features/creator-studio/components/ShotInspector.tsx', import.meta.url), 'utf8')
  const improveSource = readFileSync(new URL('../src/features/creator-studio/components/ShotImprovePanel.tsx', import.meta.url), 'utf8')
  const previewSource = readFileSync(new URL('../src/features/creator-studio/components/PreviewDeliveryPanel.tsx', import.meta.url), 'utf8')
  for (const functionName of [
    'getCreationView', 'getStepVersions', 'previewStepRevision', 'reviseStep', 'confirmStep',
    'restoreStepVersion', 'registerProjectMaterial', 'listShots', 'getShotSummary',
    'getShotWorkspace', 'getShotHistory', 'previewShotRegeneration', 'regenerateShot',
    'acceptShotCandidate', 'restoreShotCandidate', 'rebuildFinalAssembly',
  ]) {
    assert.match(apiSource, new RegExp(`export (?:async )?function ${functionName}\\b`), `${functionName} must be exported`)
  }
  assert.doesNotMatch(apiSource, /\bany\b/, 'creator API boundaries must not use any')
  assert.doesNotMatch(typeSource, /Omit<Generated/, 'honest generated contracts must not be masked by curated Omit types')
  assert.match(apiSource, /'Idempotency-Key': idempotencyKey/g)
  assert.match(apiSource, /request: StepRevisionPreviewRequest/)
  assert.match(apiSource, /request: StepRevisionMutationRequest/)
  assert.match(typeSource, /StepRevisionPreviewRequest = GeneratedStepRevisionPreviewRequest/)
  assert.match(typeSource, /StepRevisionMutationRequest = GeneratedStepRevisionMutationRequest/)
  assert.match(generatedSource, /scope: 'candidate_accept'/)
  assert.match(generatedSource, /scope: 'candidate_restore'/)
  assert.match(generatedSource, /export interface StepRevisionPreviewRequest \{\s+artifactId: string;\s+baseVersion: number;\s+\}/)
  assert.match(generatedSource, /export type StepRevisionMutationRequest = .*mode: 'direct'.* \| .*mode: 'instruction'/)
  assert.match(apiSource, /assertPositiveVersion\(request\.baseVersion\)/)
  assert.match(apiSource, /signal/g, 'creator requests must support AbortSignal')
  assert.match(startPageSource, /storageRef: buildProjectMaterialStorageRef\(nextProjectId, item\.id\)/)
  assert.match(startPageSource, /creatorStartIdempotencyKey\(nextProjectId\)/)
  assert.match(startPageSource, /productionRoute/)
  assert.match(startPageSource, /三层口播（IP \+ 文字特效 \+ AIGC）/)
  assert.match(startPageSource, /纯本地（保留 AIGC 层设计但不执行）/)
  assert.match(startPageSource, /const modelProviders = await buildClientModelProvidersForRun\(\)/, 'creator start resolves runtime providers')
  assert.match(startPageSource, /buildCreationRequest\(\{[\s\S]*modelProviders,[\s\S]*\}\)/, 'creator start sends provider refs and transient runtime providers through the request builder')
  assert.match(reviewSource, /request\.mode === 'instruction'\s*\? await buildClientModelProvidersForRun\(\)\s*:\s*undefined/, 'instruction revisions resolve runtime providers while direct edits do not')
  assert.ok((startPageSource.match(/disabled=\{starting\}/g) || []).length >= 6, 'creation inputs must lock while starting')
  assert.match(startPageSource, /AbortController/)
  assert.match(startPageSource, /useEffect\(\(\) => \{\s+activeRef\.current = true/)
  assert.match(startPageSource, /signal: controller\.signal/g)
  assert.match(startPageSource, /operationControllerRef\.current\?\.abort\(\)/)
  assert.match(startPageSource, /item\.upload \|\| await uploadLocalArtifactFile/)
  assert.ok(
    startPageSource.indexOf('const upload = item.upload') < startPageSource.indexOf('await registerProjectMaterial') &&
    startPageSource.indexOf('await registerProjectMaterial') < startPageSource.indexOf('await startAgentRun'),
    'materials must upload, register, then start in that order',
  )
  assert.match(runtimeApiSource, /'Idempotency-Key': options\.idempotencyKey/)
  assert.match(runtimeApiSource, /signal: options\.signal/)
  assert.match(librarySource, /prioritizeCreationViewProjects/)
  assert.match(workspaceSource, /getCreationView\(projectId/)
  assert.match(workspaceSource, /document\.visibilityState !== 'visible'/)
  assert.match(workspaceSource, /controller\.abort\(\)/)
  assert.match(workspaceSource, /workspaceArtifactKey\(requestSelection\)/)
  assert.match(workspaceSource, /isLatestWorkspaceRequest/)
  assert.match(workspaceSource, /selectedShotId/)
  assert.doesNotMatch(workspaceSource, /\[projectId, stepId, view\]/)
  assert.match(recoverySource, /生成仍在后台继续/)
  assert.match(stripSource, /aria-current=\{isCurrent \? 'step' : undefined\}/)
  assert.match(stripSource, /canSelect/)
  assert.match(reviewSource, /确认并继续/)
  assert.match(reviewSource, /previewStepRevision/)
  assert.match(reviewSource, /confirmedAffectedShotIds/)
  assert.match(reviewSource, /normalizeRectSelection/)
  assert.match(reviewSource, /createTimeSelection/)
  assert.match(reviewSource, /CREATOR_CONFLICT_COPY/)
  assert.match(reviewSource, /contentLoadedRef/)
  assert.match(reviewSource, /artifact-selection-overlay/)
  assert.match(reviewSource, /onPointerCancel/)
  assert.match(reviewSource, /开始时间（秒）/)
  assert.match(reviewSource, /aria-modal="true"/)
  assert.match(reviewSource, /cycleFocusIndex/)
  assert.match(reviewSource, /operationControllerRef\.current === controller/)
  assert.match(queueSource, /height: 480/)
  assert.match(queueSource, /SHOT_QUEUE_ROW_HEIGHT/)
  assert.match(queueSource, /Math\.min\(3, unresolved\.length\)/, 'visible thumbnails must use bounded concurrency')
  assert.match(queueSource, /getCreatorArtifactContent/, 'queue thumbnails use real artifact metadata')
  assert.match(queueSource, /scrollRef\.current\.scrollTop = 0/, 'filter changes reset the virtual scroll origin')
  assert.doesNotMatch(queueSource, /<video\b/, 'the queue never mounts a video player')
  assert.equal((inspectorSource.match(/<video\b/g) || []).length, 1, 'only the selected candidate mounts one video player')
  assert.match(inspectorSource, /getCreatorArtifactContent/)
  assert.match(inspectorSource, /candidateId/)
  assert.match(inspectorSource, /baseVersion: shot\.version/)
  assert.match(inspectorSource, /SHOT_QUEUE_CONFLICT_COPY/)
  assert.match(inspectorSource, /canSubmitShotDuration/)
  assert.match(inspectorSource, /artifactId of candidateRefs/, 'media source falls back through candidate artifacts')
  assert.match(improveSource, /isTargetOnlyShotImpact/)
  assert.match(improveSource, /只会新增 Shot \{sequence\} 的候选，不影响其他 \{Math\.max\(0, totalShots - 1\)\} 个 Shot/)
  assert.match(improveSource, /crypto\.randomUUID/)
  assert.match(improveSource, /aria-modal="true"/)
  assert.match(improveSource, /重试这个 Shot/)
  assert.match(improveSource, /onRegenerationStarted\(result\)/, 'a queued task is handed to the workspace immediately')
  assert.match(workspaceSource, /ShotReviewQueue/)
  assert.match(workspaceSource, /shotWorkspace\?\.shot\.id !== activeShotId/)
  assert.match(workspaceSource, /selectedShotAfterReplacement/)
  assert.match(workspaceSource, /adoptCreatorShotTask/)
  assert.match(workspaceSource, /activeTaskSignature/)
  assert.match(workspaceSource, /viewRequestTokenRef\.current \+= 1[\s\S]*adoptCreatorShotTask/, 'adopting a newer Shot task invalidates an older in-flight creation view before it can overwrite the task set')
  assert.match(workspaceSource, /SHOT_QUEUE_CONFLICT_COPY/)
  assert.match(workspaceSource, /PreviewDeliveryPanel/)
  assert.match(previewSource, /getCreatorArtifactContent/, 'preview and delivery use current server artifacts')
  assert.match(previewSource, /rebuildFinalAssembly/, 'assembly retry is a dedicated action')
  assert.doesNotMatch(previewSource, /regenerateShot\(/, 'assembly retry must never regenerate a Shot')
  assert.match(previewSource, /assemblyDirty/)

  assert.match(previewSource, /deliveryArtifactPassesFinalReview/)
  assert.match(previewSource, /result\.status === 'queued'/)
  assert.match(previewSource, /result\.status === 'dispatching'/)
  assert.match(previewSource, /crypto\.randomUUID/)
  assert.match(previewSource, /assemblyKeyRef\.current = null/)
  assert.match(previewSource, /'response' in caught[\s\S]*assemblyKeyRef\.current = null/, 'a definite server failure starts a new assembly attempt')
  assert.match(previewSource, /network failure keeps the UUID|response-less network failure keeps the UUID/, 'an ambiguous network failure must retain the assembly attempt key')
  assert.match(previewSource, /成片检查通过后，才会显示最终视频和交付文件/)
  assert.equal((previewSource.match(/<video\b/g) || []).length, 1, 'preview mounts at most one assembled video player')

  console.log('creator studio logic and client contract checks passed')
} finally {
  await rm(temp, { recursive: true, force: true })
}
