const assert = require('node:assert/strict')
const { mkdirSync, readFileSync, rmSync } = require('node:fs')
const { tmpdir } = require('node:os')
const path = require('node:path')
const test = require('node:test')

const {
  buildLocalAgentLaunchOptions,
  normalizeRunnerSession,
  readOrCreateDeviceID,
} = require('./local-agent-runtime.cjs')

test('buildLocalAgentLaunchOptions enables runner with cloud token and device id', () => {
  const options = buildLocalAgentLaunchOptions({
    binary: '/opt/Tangying/bin/tangying-local-agent',
    localAgentUrl: 'http://127.0.0.1:18082',
    cloudApiBase: 'https://cloud.example.com/api',
    dataDir: '/tmp/tangying-local-agent',
    ipAvatarMcpScript: '/opt/Tangying/mcp/ip_avatar_3d/server.py',
    ipAvatarProfile: '/opt/Tangying/ip-assets/main-ip/character-profile.json',
    videoQAMcpScript: '/opt/Tangying/mcp/video_qa/server.py',
    session: {
      userToken: 'access-token-123',
      deviceID: 'desktop-device-123',
    },
    baseEnv: { PATH: '/usr/bin' },
  })

  assert.deepEqual(options.args, ['-addr', '127.0.0.1:18082', '-cloud-api-base', 'https://cloud.example.com/api'])
  assert.equal(options.env.TANGYING_USER_TOKEN, 'access-token-123')
  assert.equal(options.env.TANGYING_DEVICE_ID, 'desktop-device-123')
  assert.equal(options.env.TANGYING_LOCAL_DATA_DIR, '/tmp/tangying-local-agent')
  assert.equal(options.env.TANGYING_IP_AVATAR_MCP_SCRIPT, '/opt/Tangying/mcp/ip_avatar_3d/server.py')
  assert.equal(options.env.TANGYING_IP_AVATAR_PROFILE, '/opt/Tangying/ip-assets/main-ip/character-profile.json')
  assert.equal(options.env.VIDEO_QA_MCP_STDIO_SCRIPT, '/opt/Tangying/mcp/video_qa/server.py')
})

test('desktop package includes both local MCP servers', () => {
  const packageJSON = JSON.parse(readFileSync(path.join(__dirname, '..', 'package.json'), 'utf8'))
  const resources = packageJSON.build.extraResources
  assert.ok(resources.some((entry) => entry.to === 'mcp/ip_avatar_3d'))
  assert.ok(resources.some((entry) => entry.to === 'mcp/video_qa'))
})

test('normalizeRunnerSession rejects incomplete runner credentials', () => {
  assert.equal(normalizeRunnerSession({ userToken: 'token-only' }), null)
  assert.equal(normalizeRunnerSession({ deviceID: 'device-only' }), null)
  assert.equal(normalizeRunnerSession(null), null)
})

test('readOrCreateDeviceID returns a stable desktop device id', () => {
  const root = path.join(tmpdir(), `tangying-device-${Date.now()}`)
  mkdirSync(root, { recursive: true })
  try {
    const first = readOrCreateDeviceID(root, () => 'uuid-1')
    const second = readOrCreateDeviceID(root, () => 'uuid-2')
    assert.equal(first, 'desktop-uuid-1')
    assert.equal(second, first)
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})
