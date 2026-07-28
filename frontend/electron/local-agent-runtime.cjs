const crypto = require('crypto')
const fs = require('fs')
const path = require('path')

function normalizeRunnerSession(value) {
  if (!value || typeof value !== 'object') return null
  const userToken = typeof value.userToken === 'string' ? value.userToken.trim() : ''
  const deviceID = typeof value.deviceID === 'string' ? value.deviceID.trim() : ''
  if (!userToken || !deviceID) return null
  return { userToken, deviceID }
}

function readOrCreateDeviceID(dataDir, randomUUID = () => crypto.randomUUID()) {
  fs.mkdirSync(dataDir, { recursive: true })
  const file = path.join(dataDir, 'device-id')
  try {
    const existing = fs.readFileSync(file, 'utf8').trim()
    if (existing) return existing
  } catch {
    // Create below.
  }
  const next = `desktop-${randomUUID()}`
  fs.writeFileSync(file, `${next}\n`, { mode: 0o600 })
  return next
}

function localAgentAddress(localAgentUrl) {
  const url = new URL(localAgentUrl)
  return `${url.hostname}:${url.port || '18080'}`
}

function buildLocalAgentLaunchOptions({
  localAgentUrl,
  cloudApiBase,
  dataDir,
  ipAvatarMcpScript,
  ipAvatarProfile,
  videoQAMcpScript,
  session,
  baseEnv = process.env,
}) {
  const env = {
    ...baseEnv,
    // Python MCP servers live inside the signed desktop bundle. Writing .pyc
    // files there after launch invalidates the macOS code signature.
    PYTHONDONTWRITEBYTECODE: '1',
    TANGYING_LOCAL_DATA_DIR: dataDir,
  }
  delete env.TANGYING_USER_TOKEN
  delete env.TANGYING_DEVICE_ID
  if (ipAvatarMcpScript) env.TANGYING_IP_AVATAR_MCP_SCRIPT = ipAvatarMcpScript
  if (ipAvatarProfile) env.TANGYING_IP_AVATAR_PROFILE = ipAvatarProfile
  if (videoQAMcpScript) env.VIDEO_QA_MCP_STDIO_SCRIPT = videoQAMcpScript

  const runnerSession = normalizeRunnerSession(session)
  if (runnerSession) {
    env.TANGYING_USER_TOKEN = runnerSession.userToken
    env.TANGYING_DEVICE_ID = runnerSession.deviceID
  }

  return {
    args: ['-addr', localAgentAddress(localAgentUrl), '-cloud-api-base', cloudApiBase],
    env,
  }
}

module.exports = {
  buildLocalAgentLaunchOptions,
  normalizeRunnerSession,
  readOrCreateDeviceID,
}
