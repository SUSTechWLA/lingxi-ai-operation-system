import assert from 'node:assert/strict'
import { mkdir, rm } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import * as esbuild from 'esbuild'

const __dirname = dirname(fileURLToPath(import.meta.url))
const root = resolve(__dirname, '..')
const outDir = resolve(root, '.tmp-tests')
const outFile = resolve(outDir, 'directorStudioLogic.mjs')

await rm(outDir, { recursive: true, force: true })
await mkdir(outDir, { recursive: true })

try {
  await esbuild.build({
    entryPoints: [resolve(root, 'src/pages/directorStudioLogic.ts')],
    outfile: outFile,
    bundle: true,
    platform: 'node',
    format: 'esm',
    target: 'node20',
    external: ['react', 'react-dom'],
    logLevel: 'silent',
  })

  const logic = await import(pathToFileURL(outFile).href)

  const validShotPrompt = {
    id: 'prompt-shot-01',
    name: 'external_generation_request.json',
    kind: 'EXTERNAL_GENERATION_REQUEST',
    status: 'valid',
    owner: '视频提示词',
    updatedAt: '-',
    humanApproved: true,
    storageRef: 'local://projects/vp-1/artifacts/extgen_video_SHOT_01/request.json',
    metadata: {
      shotId: 'SHOT_01',
      artifactType: 'external_generation_request',
      generationKind: 'video',
      requestId: 'extgen_video_SHOT_01',
    },
  }

  const validShotVideo = {
    id: 'shot-video-01',
    name: 'external_generation_result.mp4',
    kind: 'VIDEO',
    status: 'valid',
    owner: '用户上传',
    updatedAt: '-',
    humanApproved: true,
    storageRef: 'local://projects/vp-1/artifacts/shot-video-01/result.mp4',
    metadata: {
      shotId: 'SHOT_01',
      artifactType: 'external_generation_result',
      generationKind: 'video',
      externalGenerationRequestId: 'extgen_video_SHOT_01',
    },
  }

  const finalVideo = {
    id: 'final-video',
    name: 'final.mp4',
    kind: 'VIDEO',
    status: 'valid',
    owner: '渲染制片',
    updatedAt: '-',
    humanApproved: true,
    storageRef: 'local://projects/vp-1/artifacts/final-video/final.mp4',
    metadata: {
      stageName: 'render',
      unitId: 'final-video',
      tags: ['final'],
    },
  }

  const shotGroups = logic.buildShotReviewGroups([validShotPrompt, validShotVideo, finalVideo])
  assert.equal(shotGroups.length, 1, 'final video must not create a shot group')
  assert.equal(shotGroups[0].shotId, 'SHOT_01')
  assert.equal(logic.unresolvedMaterialDependencyCount(shotGroups), 0, 'fulfilled external generation requests should not remain unresolved')

  const selectedFinalVideo = logic.findFinalVideoArtifact([validShotVideo, finalVideo])
  assert.equal(selectedFinalVideo?.id, 'final-video', 'export view should prefer the final render over per-shot uploaded videos')

  assert.deepEqual(
    logic.localServiceStatusDisplay('unknown', true),
    { label: '本地在线', tone: 'ok' },
    'cloud preflight runner availability should make the local status actionable in web mode',
  )

  assert.equal(
    logic.projectPrimaryAction({
      preflightCanStart: false,
      loading: false,
      topic: '佛得角世界杯奇迹',
      stages: [],
    }).disabled,
    true,
    'blocked preflight must disable project start',
  )

  console.log('director studio logic checks passed')
} finally {
  await rm(outDir, { recursive: true, force: true })
}
