import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { createServer as createNetServer } from 'node:net'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

const frontendDir = fileURLToPath(new URL('..', import.meta.url))
const defaultTopic = '帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹'
const topic = process.env.DIRECTOR_SMOKE_TOPIC || defaultTopic
const apiPort = Number(process.env.DIRECTOR_SMOKE_API_PORT || await freePort())
const webPort = Number(process.env.DIRECTOR_SMOKE_WEB_PORT || await freePort())
const apiBase = `http://127.0.0.1:${apiPort}/api`
const localAgentBase = `http://127.0.0.1:${apiPort}`
const appURL = `http://127.0.0.1:${webPort}`
const calls = []
const registeredExternalResults = []

const mockServer = createServer(async (req, res) => {
  res.setHeader('Access-Control-Allow-Origin', '*')
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, DeviceID')
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, PATCH, OPTIONS')
  if (req.method === 'OPTIONS') {
    res.writeHead(204)
    res.end()
    return
  }

  const url = new URL(req.url || '/', `http://${req.headers.host}`)
  const body = await readBody(req)
  calls.push({ method: req.method, path: url.pathname, body })

  if (req.method === 'POST' && (url.pathname === '/api/auth/login' || url.pathname === '/api/auth/register')) {
    return sendJSON(res, 200, ok({
      user: smokeUser(),
      access_token: 'smoke-access-token',
      refresh_token: 'smoke-refresh-token',
      expires_in: 3600,
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/auth/me') {
    return sendJSON(res, 200, ok(smokeUser()))
  }

  if (req.method === 'GET' && url.pathname === '/api/video/role-agents') {
    return sendJSON(res, 200, ok({ roleAgents: roleAgents() }))
  }

  if (req.method === 'GET' && url.pathname === '/api/video/preflight') {
    return sendJSON(res, 200, ok({
      pipeline: url.searchParams.get('pipeline') || 'wf-guided-image-text-video',
      status: 'passed',
      canStart: true,
      capabilityMenu: {
        localRunner: { available: true, runnerId: 'runner-smoke' },
        compositionRuntime: { hyperframes: { available: true } },
        localTools: [
          { command: 'HYPERFRAMES_PROJECT_GENERATE', available: true },
          { command: 'HYPERFRAMES_RENDER', available: true },
          { command: 'FFMPEG_PROBE', available: true },
        ],
        warnings: [],
      },
      blockers: [],
    }))
  }

  if (req.method === 'POST' && url.pathname === '/api/video-projects') {
    return sendJSON(res, 200, ok({ project: smokeProject(body) }))
  }

  if (req.method === 'POST' && url.pathname === '/api/agent/runs') {
    return sendJSON(res, 200, ok({
      runId: 'run-smoke',
      taskId: 'task-smoke',
      status: 'RUNNING',
      plan: smokePlan(body?.message || topic),
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/agent/runs/run-smoke') {
    return sendJSON(res, 200, ok({
      run: {
        id: 'run-smoke',
        taskId: 'task-smoke',
        userId: 'smoke-user',
        domain: 'video_creation',
        message: topic,
        plan: smokePlan(topic),
        status: 'RUNNING',
        createdAt: new Date(0).toISOString(),
        updatedAt: new Date(0).toISOString(),
      },
      task: {},
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/agent/runs/run-smoke/reviews') {
    return sendJSON(res, 200, ok({
      runId: 'run-smoke',
      reviews: [
        {
          id: 'review-script',
          runId: 'run-smoke',
          taskId: 'task-smoke',
          nodeId: 'script_review',
          stage: 'script',
          roleAgentId: 'script_writer',
          tool: 'video_script_generator',
          status: 'PENDING',
          requiredInputs: ['VIDEO_PROPOSAL'],
          requiredOutputs: ['VIDEO_SCRIPT'],
          reviewReason: '脚本产物需要人工确认后进入发布包生成。',
          reviewContent: '# 口播脚本\n\n佛得角足球的逆袭故事。',
          reviewOutput: { content: '# 口播脚本\n\n佛得角足球的逆袭故事。' },
          createdAt: new Date(0).toISOString(),
        },
      ],
    }))
  }

  if (req.method === 'POST' && url.pathname === '/api/agent/runs/run-smoke/reviews/review-script/approve') {
    return sendJSON(res, 200, ok({ reviewId: 'review-script', status: 'APPROVED' }))
  }

  if (req.method === 'POST' && url.pathname === '/api/agent/runs/run-smoke/reviews/review-script/submit-edited') {
    return sendJSON(res, 200, ok({ reviewId: 'review-script', status: 'APPROVED' }))
  }

  if (req.method === 'POST' && url.pathname === '/api/agent/runs/run-smoke/reviews/review-script/regenerate') {
    return sendJSON(res, 200, ok({ reviewId: 'review-script', status: 'REGENERATING' }))
  }

  if (req.method === 'GET' && url.pathname === '/api/agent/runs/run-smoke/trace') {
    return sendJSON(res, 200, ok({
      nodes: [
        {
          id: 'script_generation_exec',
          name: 'video_script_generator',
          type: 'TOOL',
          status: 'COMPLETED',
          input: { tool: 'video_script_generator' },
          output: { artifactId: 'artifact-script' },
        },
      ],
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/video-projects/project-smoke/artifacts') {
    return sendJSON(res, 200, ok({
      artifacts: [
        {
          id: 'artifact-script',
          projectId: 'project-smoke',
          name: 'voiceover_script.md',
          kind: 'MARKDOWN',
          status: 'valid',
          version: 1,
          createdAt: new Date(0).toISOString(),
          updatedAt: new Date(0).toISOString(),
        },
        {
          id: 'artifact-external-request',
          projectId: 'project-smoke',
          name: 'external_generation_request.json',
          kind: 'JSON',
          status: 'valid',
          version: 1,
          storageType: 'inline',
          mimeType: 'application/json',
          metadata: {
            artifactType: 'external_generation_request',
            generationKind: 'video',
            relatedShotId: 'SHOT_01',
            status: 'pending_upload',
          },
          createdAt: new Date(0).toISOString(),
          updatedAt: new Date(0).toISOString(),
        },
        ...registeredExternalResults,
      ],
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/artifacts/artifact-external-request/content') {
    return sendJSON(res, 200, ok({
      content: {
        requestId: 'extgen_video_SHOT_01',
        kind: 'video',
        shotId: 'SHOT_01',
        prompt: '非真人风格化动画，佛得角群岛地图升起，足球和出线时间线形成 6 秒动态画面。',
        negativePrompt: '禁止真人写实，禁止新闻照片质感。',
        references: [
          {
            id: 'ref_SHOT_01_01',
            label: '佛得角群岛地图参考',
            role: 'scene',
            storageRef: 'manual://references/SHOT_01/01',
          },
        ],
        target: { aspectRatio: '16:9', durationSec: 6, resolution: '1920x1080' },
        promptCharLimit: 2000,
        referenceImageLimit: 6,
        status: 'pending_upload',
      },
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/artifacts/artifact-external-request/history') {
    return sendJSON(res, 200, ok({ history: [] }))
  }

  if (req.method === 'POST' && url.pathname === '/api/local/artifacts') {
    return sendJSON(res, 200, {
      id: 'local-extgen-video',
      projectId: 'project-smoke',
      storageRef: 'local://projects/project-smoke/artifacts/local-extgen-video.mp4',
      mimeType: 'video/mp4',
      contentHash: 'sha256:smoke-video',
      sizeBytes: 17,
      path: '/tmp/local-extgen-video.mp4',
      metadataPath: '/tmp/local-extgen-video.json',
      metadata: { smoke: true },
    })
  }

  if (req.method === 'POST' && url.pathname === '/api/video-projects/project-smoke/external-generation-results') {
    registeredExternalResults.push({
      id: 'artifact-uploaded-video',
      projectId: 'project-smoke',
      name: 'external_generation_result.mp4',
      kind: 'VIDEO',
      status: 'valid',
      version: 1,
      storageType: 'local',
      storageRef: body?.storageRef,
      mimeType: body?.mimeType || 'video/mp4',
      metadata: {
        artifactType: 'external_generation_result',
        externalGenerationRequestId: body?.generationRequestId,
        relatedShotId: body?.relatedShotId,
        source: body?.source,
      },
      createdAt: new Date(0).toISOString(),
      updatedAt: new Date(0).toISOString(),
    })
    return sendJSON(res, 200, ok({
      manifest: {
        assetId: 'asset-smoke-video',
        type: body?.kind,
        storageType: body?.storageType,
        storageRef: body?.storageRef,
        source: body?.source,
        generationRequestId: body?.generationRequestId,
        relatedShotId: body?.relatedShotId,
      },
      artifact: registeredExternalResults.at(-1),
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/video-projects/project-smoke/workflow-runs/run-smoke/checkpoints') {
    return sendJSON(res, 200, ok({ checkpoints: [] }))
  }

  sendJSON(res, 404, { code: 404, message: `No mock route for ${req.method} ${url.pathname}`, data: null })
})

let shuttingDown = false

await listen(mockServer, apiPort)
const vite = startVite()
await waitForHTTP(appURL, 20_000)
console.log(JSON.stringify({ appURL, apiBase, topic }))

process.on('SIGTERM', shutdown)
process.on('SIGINT', shutdown)

setInterval(() => {}, 1_000)

function startVite() {
  const viteBin = join(frontendDir, 'node_modules', 'vite', 'bin', 'vite.js')
  const child = spawn(process.execPath, [
    viteBin,
    '--host',
    '127.0.0.1',
    '--port',
    String(webPort),
    '--strictPort',
  ], {
    cwd: frontendDir,
    env: {
      ...process.env,
      VITE_CLOUD_API_BASE: apiBase,
      VITE_API_BASE: apiBase,
      VITE_LOCAL_AGENT_URL: localAgentBase,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', (chunk) => process.stdout.write(prefixLines('vite', chunk)))
  child.stderr.on('data', (chunk) => process.stderr.write(prefixLines('vite', chunk)))
  return child
}

function smokeUser() {
  return {
    id: 'smoke-user',
    email: 'smoke@example.com',
    nickname: 'Smoke Tester',
    status: 'active',
  }
}

function smokeProject(body) {
  return {
    id: 'project-smoke',
    userId: 'smoke-user',
    name: body?.name || '视频创作项目',
    description: body?.description,
    mode: body?.mode || 'voice_visual',
    status: 'RUNNING',
    skillName: body?.skillName || 'video-creator',
    skillVersion: body?.skillVersion || 'v4.0',
    workflowName: body?.workflowName || 'dynamic-agent-video-creation',
    workflowVersion: body?.workflowVersion || 'v4.0',
    generationMode: body?.generationMode || 'provider_api',
    aspectRatio: body?.aspectRatio || '16:9',
    targetDurationSec: body?.targetDurationSec || 60,
    language: body?.language || 'zh-CN',
    config: body?.config || {},
    currentRunId: 'run-smoke',
    createdAt: new Date(0).toISOString(),
    updatedAt: new Date(0).toISOString(),
  }
}

function roleAgents() {
  return [
    {
      id: 'creative_director',
      name: 'Creative Director',
      displayName: '创意总监',
      stage: 'proposal',
      goal: '理解需求，确定创作方向。',
      allowedTools: ['proposal_generator', 'capability_preflight'],
      requiredOutputs: ['VIDEO_PROPOSAL'],
      humanReview: { required: true },
    },
    {
      id: 'script_writer',
      name: 'Script Writer',
      displayName: '脚本编剧',
      stage: 'script',
      goal: '生成口播脚本。',
      allowedTools: ['video_script_generator'],
      requiredOutputs: ['VIDEO_SCRIPT'],
      humanReview: { required: true },
    },
  ]
}

function smokePlan(message) {
  return {
    goal: message,
    domain: 'video_creation',
    mode: 'dynamic_agent',
    steps: [
      {
        id: 'script_generation',
        intent: '生成口播脚本',
        tool: 'video_script_generator',
        arguments: { topic: message },
        expectedOutput: ['script'],
        produceArtifact: true,
      },
      {
        id: 'publish_package',
        intent: '生成发布包',
        tool: 'publish_copy_generator',
        dependsOn: ['script_generation'],
        arguments: { platform: 'xiaohongshu' },
        expectedOutput: ['publishCopy'],
        produceArtifact: true,
      },
    ],
    budget: { maxSteps: 6, maxToolCalls: 6, maxReplans: 1, maxCostLevel: 'medium' },
    stopPolicy: { stopWhenEnough: true },
  }
}

async function readBody(req) {
  const chunks = []
  for await (const chunk of req) chunks.push(chunk)
  if (!chunks.length) return undefined
  const contentType = req.headers['content-type'] || ''
  if (String(contentType).includes('multipart/form-data')) {
    return { multipart: true, sizeBytes: Buffer.concat(chunks).length }
  }
  const raw = Buffer.concat(chunks).toString('utf8')
  if (!raw.trim()) return undefined
  return JSON.parse(raw)
}

function sendJSON(res, status, payload) {
  res.writeHead(status, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify(payload))
}

function ok(data) {
  return { code: 0, message: 'ok', data }
}

function listen(server, port) {
  return new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(port, '127.0.0.1', () => {
      server.off('error', reject)
      resolve()
    })
  })
}

function freePort() {
  return new Promise((resolve, reject) => {
    const server = createNetServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      server.close(() => {
        if (!address || typeof address === 'string') {
          reject(new Error('Could not allocate a free TCP port'))
          return
        }
        resolve(address.port)
      })
    })
  })
}

async function waitForHTTP(url, timeoutMs) {
  const started = Date.now()
  while (Date.now() - started < timeoutMs) {
    try {
      const response = await fetch(url)
      if (response.ok) return
    } catch {
      // keep waiting
    }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`Timed out waiting for ${url}`)
}

function prefixLines(label, chunk) {
  return String(chunk).split(/(?<=\n)/).map((line) => line ? `[${label}] ${line}` : '').join('')
}

function shutdown() {
  if (shuttingDown) return
  shuttingDown = true
  if (vite) vite.kill('SIGTERM')
  mockServer.close(() => process.exit(0))
  setTimeout(() => process.exit(0), 1_000).unref()
}
