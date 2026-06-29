import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { createServer as createNetServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'

const require = createRequire(import.meta.url)
const frontendDir = fileURLToPath(new URL('..', import.meta.url))
const defaultTopic = '帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹'
const topic = process.env.DIRECTOR_SMOKE_TOPIC || defaultTopic
const apiPort = Number(process.env.DIRECTOR_SMOKE_API_PORT || await freePort())
const webPort = Number(process.env.DIRECTOR_SMOKE_WEB_PORT || await freePort())
const apiBase = `http://127.0.0.1:${apiPort}/api`
const appURL = `http://127.0.0.1:${webPort}`
const calls = []

const mockServer = createServer(async (req, res) => {
  res.setHeader('Access-Control-Allow-Origin', '*')
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, DeviceID')
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS')
  if (req.method === 'OPTIONS') {
    res.writeHead(204)
    res.end()
    return
  }

  const url = new URL(req.url || '/', `http://${req.headers.host}`)
  const body = await readJSON(req)
  calls.push({ method: req.method, path: url.pathname, body })

  if (req.method === 'GET' && url.pathname === '/api/auth/me') {
    return sendJSON(res, 200, ok({
      id: 'smoke-user',
      email: 'smoke@example.com',
      nickname: 'Smoke Tester',
      status: 'active',
    }))
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
        localRunner: { available: true },
        compositionRuntime: { hyperframes: { available: true } },
        localTools: [
          { command: 'node', available: true },
          { command: 'ffmpeg', available: true },
        ],
        warnings: [],
      },
      blockers: [],
    }))
  }

  if (req.method === 'POST' && url.pathname === '/api/video-projects') {
    return sendJSON(res, 200, ok({
      project: {
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
      },
    }))
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
    return sendJSON(res, 200, ok({ runId: 'run-smoke', reviews: [] }))
  }

  if (req.method === 'GET' && url.pathname === '/api/agent/runs/run-smoke/trace') {
    return sendJSON(res, 200, ok({
      nodes: [
        {
          id: 'script_generation_exec',
          name: 'external',
          type: 'TOOL',
          status: 'RUNNING',
          input: { tool: 'external', capabilityTool: 'video_script_generator' },
          output: {},
        },
      ],
    }))
  }

  if (req.method === 'GET' && url.pathname === '/api/video-projects/project-smoke/artifacts') {
    return sendJSON(res, 200, ok({ artifacts: [] }))
  }

  sendJSON(res, 404, { code: 404, message: `No mock route for ${req.method} ${url.pathname}`, data: null })
})

let vite
let tempDir

try {
  await listen(mockServer, apiPort)
  vite = startVite()
  await waitForHTTP(appURL, 20_000)
  tempDir = await mkdtemp(join(tmpdir(), 'director-start-smoke-'))
  const mainPath = join(tempDir, 'electron-main.cjs')
  await writeFile(mainPath, electronMainSource(), 'utf8')
  await runElectron(mainPath)

  const projectCall = calls.find((call) => call.method === 'POST' && call.path === '/api/video-projects')
  const runCall = calls.find((call) => call.method === 'POST' && call.path === '/api/agent/runs')

  assert.ok(projectCall, 'expected UI to create a video project')
  assert.ok(runCall, 'expected UI to start a dynamic agent run')
  assert.equal(runCall.body?.domain, 'video_creation')
  assert.equal(runCall.body?.mode, 'dynamic_agent')
  assert.match(runCall.body?.message || '', /佛得角/)
  assert.equal(runCall.body?.context?.projectId, 'project-smoke')

  console.log('director start smoke passed')
  console.log(`topic: ${topic}`)
  console.log(`agent run payload: ${JSON.stringify(runCall.body)}`)
} finally {
  if (vite) vite.kill('SIGTERM')
  await closeServer(mockServer)
  if (tempDir) await rm(tempDir, { recursive: true, force: true })
}

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
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', (chunk) => process.stdout.write(prefixLines('vite', chunk)))
  child.stderr.on('data', (chunk) => process.stderr.write(prefixLines('vite', chunk)))
  return child
}

async function runElectron(mainPath) {
  const electronPath = require('electron')
  const child = spawn(electronPath, [mainPath, appURL, topic], {
    cwd: frontendDir,
    env: {
      ...process.env,
      ELECTRON_DISABLE_SECURITY_WARNINGS: 'true',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', (chunk) => process.stdout.write(prefixLines('electron', chunk)))
  child.stderr.on('data', (chunk) => process.stderr.write(prefixLines('electron', chunk)))
  const code = await new Promise((resolve) => child.on('close', resolve))
  assert.equal(code, 0, `electron smoke runner exited with ${code}`)
}

function electronMainSource() {
  return `
const { app, BrowserWindow } = require('electron')

const appURL = process.argv[2]
const topic = process.argv[3]

app.whenReady().then(async () => {
  const win = new BrowserWindow({
    show: false,
    width: 1280,
    height: 900,
    webPreferences: { contextIsolation: true, sandbox: true },
  })
  try {
    await load(win, appURL)
    await win.webContents.executeJavaScript(\`
      localStorage.setItem('tangying.auth.session', JSON.stringify({
        user: { id: 'smoke-user', email: 'smoke@example.com', nickname: 'Smoke Tester', status: 'active' },
        accessToken: 'smoke-access-token',
        refreshToken: 'smoke-refresh-token',
        expiresIn: 3600
      }))
    \`)
    await load(win, appURL)
    await waitForDOM(win, \`document.querySelector('textarea')\`)
    await win.webContents.executeJavaScript(\`
      const textarea = document.querySelector('textarea')
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set
      setter.call(textarea, \${JSON.stringify(topic)})
      textarea.dispatchEvent(new Event('input', { bubbles: true }))
    \`)
    await waitForDOM(win, \`
      Array.from(document.querySelectorAll('button')).some((button) =>
        button.textContent.includes('开始项目') && !button.disabled
      )
    \`)
    await win.webContents.executeJavaScript(\`
      const button = Array.from(document.querySelectorAll('button')).find((item) =>
        item.textContent.includes('开始项目')
      )
      button.click()
    \`)
    await waitForDOM(win, \`document.body.innerText.includes('Run run-smok')\`, 15000)
    const bodyText = await win.webContents.executeJavaScript('document.body.innerText')
    if (bodyText.includes('guard agent plan')) {
      throw new Error(bodyText)
    }
    console.log('clicked start project successfully')
    app.exit(0)
  } catch (error) {
    console.error(error && error.stack ? error.stack : error)
    app.exit(1)
  }
})

function load(win, url) {
  return new Promise((resolve, reject) => {
    const done = () => {
      cleanup()
      resolve()
    }
    const failed = (_event, _code, description) => {
      cleanup()
      reject(new Error(description || 'load failed'))
    }
    const cleanup = () => {
      win.webContents.off('did-finish-load', done)
      win.webContents.off('did-fail-load', failed)
    }
    win.webContents.once('did-finish-load', done)
    win.webContents.once('did-fail-load', failed)
    win.loadURL(url)
  })
}

async function waitForDOM(win, expression, timeoutMs = 10000) {
  const started = Date.now()
  while (Date.now() - started < timeoutMs) {
    const ok = await win.webContents.executeJavaScript(\`Boolean(\${expression})\`)
    if (ok) return
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error('Timed out waiting for DOM expression: ' + expression)
}
`
}

function ok(data) {
  return { code: 0, message: 'ok', data }
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
    {
      id: 'render_producer',
      name: 'Render Producer',
      displayName: '渲染制片',
      stage: 'render',
      goal: '确认并执行最终渲染。',
      allowedTools: ['render_dependency_guard', 'hyperframes_renderer', 'local_job_status_tracker'],
      requiredOutputs: ['VIDEO', 'RENDER_REPORT'],
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
        arguments: { topic },
        expectedOutput: ['script'],
        produceArtifact: true,
      },
      {
        id: 'render',
        intent: '渲染视频',
        tool: 'hyperframes_renderer',
        dependsOn: ['script_generation'],
        arguments: { projectDir: '{{preview.output.projectDir}}', previewApproved: true },
        expectedOutput: ['outputPath'],
        produceArtifact: true,
      },
    ],
    budget: {
      maxSteps: 6,
      maxToolCalls: 6,
      maxReplans: 1,
      maxCostLevel: 'high',
    },
    stopPolicy: { stopWhenEnough: true },
  }
}

async function readJSON(req) {
  const chunks = []
  for await (const chunk of req) chunks.push(chunk)
  if (!chunks.length) return undefined
  const raw = Buffer.concat(chunks).toString('utf8')
  if (!raw.trim()) return undefined
  return JSON.parse(raw)
}

function sendJSON(res, status, payload) {
  res.writeHead(status, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify(payload))
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

function closeServer(server) {
  return new Promise((resolve) => server.close(() => resolve()))
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
