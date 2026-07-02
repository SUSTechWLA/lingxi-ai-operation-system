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

  const profiles = logic.videoCreationProfiles()
  assert.equal(profiles.length, 2, 'director studio should expose two stable video profiles')
  assert.equal(
    logic.videoCreationProfileForId('aigc_shot').preflightPipeline,
    'wf-aigc-shot-video',
    'cinematic profile should use the AIGC shot preflight',
  )
  assert.equal(
    logic.videoCreationProfileForId('voice_visual').preflightPipeline,
    'wf-guided-image-text-video',
    'voice/knowledge profile should use the guided render preflight',
  )
  assert.ok(
    logic.videoCreationProfileForId('aigc_shot').requiredLocalCommands.includes('LOCAL_FILE_IMPORT'),
    'cinematic profile should tell users local import is required',
  )

  const healthItems = logic.buildEnvironmentChecklist({
    serviceStatus: 'unknown',
    selectedProfile: logic.videoCreationProfileForId('aigc_shot'),
    preflight: {
      pipeline: 'wf-aigc-shot-video',
      status: 'blocked',
      canStart: false,
      capabilityMenu: {
        localRunner: { available: true },
        compositionRuntime: { hyperframes: { available: true } },
        localTools: [
          { command: 'LOCAL_FILE_IMPORT', available: true },
          { command: 'FFMPEG_PROBE', available: false },
          { command: 'ARTIFACT_PACKAGE', available: true },
        ],
        warnings: [],
      },
      blockers: [{ code: 'FFMPEG_PROBE_NOT_AVAILABLE', message: '本地未检测到 FFMPEG_PROBE 执行能力' }],
    },
    modelProviderState: 'missing',
    missingModelCapabilities: ['text_to_video'],
  })
  assert.ok(
    healthItems.some((item) => item.id === 'local-runner' && item.status === 'passed'),
    'environment checklist should trust cloud runner availability when web service status is unknown',
  )
  assert.ok(
    healthItems.some((item) => item.id === 'model-provider' && item.status === 'warning' && item.actionLabel === '打开设置'),
    'manual-import environment checklist should not block when only external image/video providers are missing',
  )
  assert.ok(
    healthItems.some((item) => item.id === 'local-tool-FFMPEG_PROBE' && item.status === 'blocked'),
    'environment checklist should show exact missing local tool blockers',
  )

  const externalTaskPackage = logic.buildExternalGenerationTaskPackage({
    requestId: 'extgen_video_SHOT_01',
    kind: 'video',
    shotId: 'SHOT_01',
    prompt: 'A cinematic football underdog scene',
    negativePrompt: 'low quality, blurry',
    references: [
      { id: 'ref-player', label: '主角参考', role: 'character', storageRef: 'local://projects/vp-1/artifacts/ref-player/content.png' },
      { id: 'storyboard-1', label: '故事板 1', role: 'storyboard', storageRef: 'local://projects/vp-1/artifacts/storyboard-1/content.png' },
    ],
    target: { aspectRatio: '16:9', durationSec: 5, resolution: '1080p' },
    promptCharLimit: 2000,
    referenceImageLimit: 6,
  })
  assert.ok(externalTaskPackage.fullText.includes('完整任务包'), 'external package should be copyable as a full task package')
  assert.ok(externalTaskPackage.fullText.includes('Positive Prompt'), 'external package should include a positive prompt section')
  assert.ok(externalTaskPackage.fullText.includes('Negative Prompt'), 'external package should include a negative prompt section')
  assert.ok(externalTaskPackage.referenceManifest.includes('参考图 1'), 'external package should include ordered reference image manifest')
  assert.ok(externalTaskPackage.fullText.includes('上传回填'), 'external package should tell users how to upload the result back')

  const deliveryItems = logic.buildExportDeliveryItems([
    finalVideo,
    {
      id: 'probe-report',
      name: 'ffmpeg_probe.json',
      kind: 'FFMPEG_PROBE_REPORT',
      status: 'valid',
      owner: '质量审核',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/probe-report/report.json',
      metadata: {},
    },
    {
      id: 'publish-copy',
      name: 'publish-copy.md',
      kind: 'PUBLISH_COPY',
      status: 'valid',
      owner: '文案生成',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/publish-copy/publish.md',
      metadata: { artifactType: 'publish_copy' },
    },
  ])
  assert.deepEqual(
    deliveryItems.map((item) => item.id),
    ['final-video', 'project-package', 'publish-copy', 'quality-report', 'folder-entry'],
    'export delivery checklist should render stable required rows even when optional artifacts are missing',
  )
  assert.equal(
    deliveryItems.find((item) => item.id === 'project-package')?.status,
    'missing',
    'export delivery checklist should turn missing package into a friendly state instead of artifact not found',
  )
  assert.equal(
    logic.normalizeDirectorErrorMessage(new Error('artifact not found')),
    '产物文件还没有同步到本机，请重新生成或回到产物页确认该文件是否已上传。',
    'raw artifact not found errors should not be exposed to beta users',
  )

  console.log('director studio logic checks passed')
} finally {
  await rm(outDir, { recursive: true, force: true })
}
