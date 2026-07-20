import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const temp = await mkdtemp(join(tmpdir(), 'creator-studio-'))
const bundle = join(temp, 'logic.mjs')

try {
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
  for (const functionName of [
    'getCreationView', 'getStepVersions', 'previewStepRevision', 'reviseStep', 'confirmStep',
    'restoreStepVersion', 'registerProjectMaterial', 'listShots', 'getShotSummary',
    'getShotWorkspace', 'getShotHistory', 'previewShotRegeneration', 'regenerateShot',
    'acceptShotCandidate', 'restoreShotCandidate',
  ]) {
    assert.match(apiSource, new RegExp(`export (?:async )?function ${functionName}\\b`), `${functionName} must be exported`)
  }
  assert.doesNotMatch(apiSource, /\bany\b/, 'creator API boundaries must not use any')
  assert.match(apiSource, /'Idempotency-Key': idempotencyKey/g)
  assert.match(typeSource, /scope: 'candidate_accept'/)
  assert.match(typeSource, /scope: 'candidate_restore'/)
  assert.match(typeSource, /locks: ShotLock\[\]/g)
  assert.match(apiSource, /assertPositiveVersion\(request\.baseVersion\)/)
  assert.match(apiSource, /signal/g, 'creator requests must support AbortSignal')

  console.log('creator studio logic and client contract checks passed')
} finally {
  await rm(temp, { recursive: true, force: true })
}
