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
  const typeSource = readFileSync(new URL('../src/features/creator-studio/types.ts', import.meta.url), 'utf8')
  const generatedSource = readFileSync(new URL('../src/utils/api-types.generated.ts', import.meta.url), 'utf8')
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

  console.log('creator studio logic and client contract checks passed')
} finally {
  await rm(temp, { recursive: true, force: true })
}
