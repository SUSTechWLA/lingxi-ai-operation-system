# Electron Local History Directory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Electron development launches use the same local-agent data directory as the repository startup script so historical projects remain visible after restart.

**Architecture:** Extract data-directory resolution from Electron's main process into a small CommonJS module. The resolver accepts explicit runtime inputs and applies a fixed priority: explicit environment override, development workspace data directory, then packaged-app user data. `main.cjs` passes its resolved directory into the spawned local-agent environment.

**Tech Stack:** Node.js CommonJS, `node:test`, Electron main process.

## Global Constraints

- Do not migrate, delete, or rewrite existing local project history.
- Preserve packaged Electron storage at `app.getPath('userData')/local-agent`.
- Preserve `TANGYING_LOCAL_DATA_DIR` as the highest-priority explicit override.

---

### Task 1: Add the data-directory resolver and its regression tests

**Files:**
- Create: `frontend/electron/local-agent-data-dir.cjs`
- Create: `frontend/electron/local-agent-data-dir.test.cjs`
- Modify: `frontend/electron/main.cjs:1-55`

**Interfaces:**
- Produces: `resolveLocalAgentDataDir({ appIsPackaged, workspaceRoot, userDataDir, environment }) => string`.
- Consumes: Electron's `app.isPackaged`, `app.getPath('userData')`, repository root, and `process.env`.

- [ ] **Step 1: Write the failing test**

```js
test('uses the workspace local-backend data directory during development', () => {
  const actual = resolveLocalAgentDataDir({
    appIsPackaged: false,
    workspaceRoot: '/workspace/tangying',
    userDataDir: '/users/demo/app-data',
    environment: {},
  })

  assert.equal(actual, path.join('/workspace/tangying', 'local-backend', 'data'))
})
```

Add equivalent tests that assert a nonblank `TANGYING_LOCAL_DATA_DIR` wins over every default, and that a packaged app resolves to `path.join(userDataDir, 'local-agent')`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `node --test electron/local-agent-data-dir.test.cjs`

Expected: FAIL because `local-agent-data-dir.cjs` does not exist yet.

- [ ] **Step 3: Write the minimal implementation**

```js
function resolveLocalAgentDataDir({ appIsPackaged, workspaceRoot, userDataDir, environment = process.env }) {
  const configured = String(environment.TANGYING_LOCAL_DATA_DIR || '').trim()
  if (configured) return configured
  if (!appIsPackaged) return path.join(workspaceRoot, 'local-backend', 'data')
  return path.join(userDataDir, 'local-agent')
}
```

Import this resolver from `main.cjs` and replace the hard-coded `path.join(app.getPath('userData'), 'local-agent')` assignment in `startLocalAgent` with a call that supplies `app.isPackaged`, `path.join(__dirname, '..', '..')`, `app.getPath('userData')`, and `process.env`.

- [ ] **Step 4: Run the resolver tests to verify they pass**

Run: `node --test electron/local-agent-data-dir.test.cjs`

Expected: PASS with three tests.

- [ ] **Step 5: Verify the frontend build**

Run: `npm run build`

Expected: exit code 0.
