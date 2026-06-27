import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
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
    buildDirectorArtifacts,
    buildPublishCopies,
    formatDirectorErrorMessage,
    normalizeDirectorErrorMessage,
    publishCopiesToJSON,
    publishCopiesToMarkdown,
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
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
