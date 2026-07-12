const path = require('path')

function resolveLocalAgentDataDir({
  appIsPackaged,
  workspaceRoot,
  userDataDir,
  environment = process.env,
}) {
  const configured = String(environment.TANGYING_LOCAL_DATA_DIR || '').trim()
  if (configured) return configured

  if (!appIsPackaged) {
    return path.join(workspaceRoot, 'local-backend', 'data')
  }

  return path.join(userDataDir, 'local-agent')
}

module.exports = { resolveLocalAgentDataDir }
