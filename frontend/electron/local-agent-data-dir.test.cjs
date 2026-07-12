const assert = require('node:assert/strict')
const path = require('node:path')
const test = require('node:test')

const { resolveLocalAgentDataDir } = require('./local-agent-data-dir.cjs')

test('uses the workspace local-backend data directory during development', () => {
  const actual = resolveLocalAgentDataDir({
    appIsPackaged: false,
    workspaceRoot: path.join('workspace', 'tangying'),
    userDataDir: path.join('users', 'demo', 'app-data'),
    environment: {},
  })

  assert.equal(actual, path.join('workspace', 'tangying', 'local-backend', 'data'))
})

test('uses an explicit local data directory before development defaults', () => {
  const actual = resolveLocalAgentDataDir({
    appIsPackaged: false,
    workspaceRoot: path.join('workspace', 'tangying'),
    userDataDir: path.join('users', 'demo', 'app-data'),
    environment: { TANGYING_LOCAL_DATA_DIR: path.join('custom', 'history') },
  })

  assert.equal(actual, path.join('custom', 'history'))
})

test('uses the Electron user data directory in packaged applications', () => {
  const actual = resolveLocalAgentDataDir({
    appIsPackaged: true,
    workspaceRoot: path.join('workspace', 'tangying'),
    userDataDir: path.join('users', 'demo', 'app-data'),
    environment: {},
  })

  assert.equal(actual, path.join('users', 'demo', 'app-data', 'local-agent'))
})
