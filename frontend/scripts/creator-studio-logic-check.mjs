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

  const creationRequest = logic.buildCreationRequest({
    prompt: '为夏日咖啡新品拍一支轻快的竖版短片',
    durationSec: 30,
    aspectRatio: '9:16',
    platform: '抖音',
    materialCount: 2,
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
    },
  })
  const creationCopy = `${creationRequest.project.description} ${creationRequest.agentRun.message}`
  assert.doesNotMatch(creationCopy, /provider|run|trace|artifact/i, 'creator copy must not expose developer vocabulary')

  const defaultCreationRequest = logic.buildCreationRequest({
    prompt: '做一个品牌故事',
    aspectRatio: '16:9',
    materialCount: 0,
  })
  assert.equal(defaultCreationRequest.project.targetDurationSec, undefined)
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
  for (const functionName of [
    'getCreationView', 'getStepVersions', 'previewStepRevision', 'reviseStep', 'confirmStep',
    'restoreStepVersion', 'registerProjectMaterial', 'listShots', 'getShotSummary',
    'getShotWorkspace', 'getShotHistory', 'previewShotRegeneration', 'regenerateShot',
    'acceptShotCandidate', 'restoreShotCandidate',
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
  assert.match(workspaceSource, /\[currentArtifactId, currentVersion, projectId, stepId\]/)
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

  console.log('creator studio logic and client contract checks passed')
} finally {
  await rm(temp, { recursive: true, force: true })
}
