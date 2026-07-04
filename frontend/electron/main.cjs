const { app, BrowserWindow, ipcMain, dialog, shell } = require('electron')
const path = require('path')
const { spawn } = require('child_process')
const fs = require('fs')
const http = require('http')
const {
  buildLocalAgentLaunchOptions,
  normalizeRunnerSession,
  readOrCreateDeviceID,
} = require('./local-agent-runtime.cjs')
const { createFileAccessController } = require('./file-access-runtime.cjs')

const isDev = !app.isPackaged
const LOCAL_AGENT_URL = process.env.TANGYING_LOCAL_AGENT_URL || 'http://127.0.0.1:18080'
const CLOUD_API_BASE = process.env.TANGYING_CLOUD_API_BASE || process.env.VITE_CLOUD_API_BASE || 'http://localhost:8080/api'
const APP_ICON_FILE = '躺营ai自媒体运营助手.png'

let mainWindow = null
let localAgentProcess = null
const fileAccess = createFileAccessController()
let localRunnerSession = normalizeRunnerSession({
  userToken: process.env.TANGYING_USER_TOKEN,
  deviceID: process.env.TANGYING_DEVICE_ID,
})

function localAgentBinaryPath() {
  const binaryName = process.platform === 'win32' ? 'tangying-local-agent.exe' : 'tangying-local-agent'
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'bin', binaryName)
  }
  return path.join(__dirname, '..', 'resources', 'bin', binaryName)
}

function appIconPath() {
  if (app.isPackaged) {
    return path.join(__dirname, '..', 'dist', APP_ICON_FILE)
  }
  return path.join(__dirname, '..', 'public', APP_ICON_FILE)
}

function localAgentDataDir() {
  return path.join(app.getPath('userData'), 'local-agent')
}

function startLocalAgent() {
  if (process.env.TANGYING_SKIP_LOCAL_AGENT === 'true') return
  if (localAgentProcess) return
  const binary = localAgentBinaryPath()
  if (!fs.existsSync(binary)) {
    console.warn(`Local agent binary not found: ${binary}`)
    return
  }
  const launch = buildLocalAgentLaunchOptions({
    localAgentUrl: LOCAL_AGENT_URL,
    cloudApiBase: CLOUD_API_BASE,
    dataDir: localAgentDataDir(),
    session: localRunnerSession,
  })
  localAgentProcess = spawn(binary, launch.args, {
    stdio: ['ignore', 'ignore', 'pipe'],
    env: launch.env,
  })
  localAgentProcess.stderr.on('data', (data) => {
    console.warn(`[local-agent] ${data.toString().trim()}`)
  })
  localAgentProcess.on('exit', (code) => {
    console.warn(`Local agent exited with code ${code}`)
    localAgentProcess = null
  })
}

function requestLocalAgentJSON(pathname) {
  return new Promise((resolve, reject) => {
    const req = http.get(`${LOCAL_AGENT_URL}${pathname}`, (res) => {
      let data = ''
      res.on('data', (chunk) => { data += chunk })
      res.on('end', () => {
        if (res.statusCode < 200 || res.statusCode >= 300) {
          reject(new Error(`local agent returned ${res.statusCode}: ${data}`))
          return
        }
        try {
          resolve(JSON.parse(data))
        } catch (err) {
          reject(err)
        }
      })
    })
    req.on('error', reject)
    req.setTimeout(3000, () => {
      req.destroy()
      reject(new Error('local agent request timeout'))
    })
  })
}

function stopLocalAgent() {
  if (localAgentProcess) {
    const proc = localAgentProcess
    localAgentProcess = null
    return new Promise((resolve) => {
      let resolved = false
      const finish = () => {
        if (resolved) return
        resolved = true
        resolve()
      }
      proc.once('exit', finish)
      proc.kill()
      setTimeout(finish, 3000).unref?.()
    })
  }
  return Promise.resolve()
}

async function restartLocalAgent() {
  await stopLocalAgent()
  startLocalAgent()
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1400,
    height: 960,
    minWidth: 1024,
    minHeight: 700,
    title: '躺营AI自媒体运营助手',
    icon: appIconPath(),
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  })

  if (isDev) {
    mainWindow.loadURL('http://localhost:3000')
    mainWindow.webContents.openDevTools()
  } else {
    mainWindow.loadFile(path.join(__dirname, '..', 'dist', 'index.html'))
  }
}

// ── IPC Handlers ──────────────────────────────────────────

// Read file from disk and return base64-encoded data with metadata.
// Used by the renderer to create proper File objects for upload.
ipcMain.handle('read-file', async (_, filePath) => {
  if (!fileAccess.canRead(filePath)) {
    throw new Error('file path has not been granted by the file picker')
  }
  const data = fs.readFileSync(filePath)
  const ext = path.extname(filePath).toLowerCase()
  const mimeTypes = {
    '.png': 'image/png',
    '.jpg': 'image/jpeg',
    '.jpeg': 'image/jpeg',
    '.gif': 'image/gif',
    '.webp': 'image/webp',
    '.mp4': 'video/mp4',
    '.mov': 'video/quicktime',
    '.avi': 'video/x-msvideo',
  }
  return {
    data: data.toString('base64'),
    mimeType: mimeTypes[ext] || 'application/octet-stream',
    name: path.basename(filePath),
    size: data.length,
  }
})

// Open file dialog
ipcMain.handle('open-file-dialog', async (_, options = {}) => {
  const result = await dialog.showOpenDialog(mainWindow, {
    properties: ['openFile', 'multiSelections'],
    filters: options.filters || [
      { name: 'All Files', extensions: ['*'] },
    ],
    ...options,
  })
  if (result.canceled) return []
  fileAccess.grantFiles(result.filePaths)
  return result.filePaths
})

// Open directory dialog
ipcMain.handle('open-directory-dialog', async () => {
  const result = await dialog.showOpenDialog(mainWindow, {
    properties: ['openDirectory'],
  })
  if (result.canceled) return []
  fileAccess.grantDirectories(result.filePaths)
  return result.filePaths
})

// Save file dialog
ipcMain.handle('save-file-dialog', async (_, options = {}) => {
  const result = await dialog.showSaveDialog(mainWindow, options)
  return result.canceled ? undefined : result.filePath
})

// Check if backend is healthy
ipcMain.handle('check-service-health', async () => {
  return new Promise((resolve) => {
    const req = http.get(`${LOCAL_AGENT_URL}/api/local/health`, (res) => {
      let data = ''
      res.on('data', (chunk) => { data += chunk })
      res.on('end', () => {
        try {
          const parsed = JSON.parse(data)
          resolve(parsed.status === 'ok' ? 'ok' : 'unhealthy')
        } catch {
          resolve(data)
        }
      })
    })
    req.on('error', () => resolve('unreachable'))
    req.setTimeout(3000, () => { req.destroy(); resolve('timeout') })
  })
})

ipcMain.handle('get-runtime-config', async () => ({
  localAgentUrl: LOCAL_AGENT_URL,
  cloudApiBase: CLOUD_API_BASE,
}))

ipcMain.handle('get-or-create-device-id', async () =>
  readOrCreateDeviceID(localAgentDataDir())
)

ipcMain.handle('configure-local-runner-session', async (_, session) => {
  const userToken = typeof session?.userToken === 'string' ? session.userToken.trim() : ''
  const deviceID = typeof session?.deviceID === 'string' && session.deviceID.trim()
    ? session.deviceID.trim()
    : readOrCreateDeviceID(localAgentDataDir())
  const next = normalizeRunnerSession({ userToken, deviceID })
  if (!next) return { enabled: false }
  if (localRunnerSession?.userToken === next.userToken && localRunnerSession?.deviceID === next.deviceID) {
    return { enabled: true, deviceID: next.deviceID }
  }
  localRunnerSession = next
  await restartLocalAgent()
  return { enabled: true, deviceID: next.deviceID }
})

ipcMain.handle('clear-local-runner-session', async () => {
  if (!localRunnerSession) return { enabled: false }
  localRunnerSession = null
  await restartLocalAgent()
  return { enabled: false }
})

ipcMain.handle('get-model-provider-settings-with-keys', async () =>
  requestLocalAgentJSON('/api/local/model-providers?include_key=true')
)

// Open external URL in system browser
ipcMain.handle('open-external', async (_, url) => {
  if (typeof url === 'string' && url.startsWith('http')) {
    shell.openExternal(url)
  }
})

// Open a local file or directory from the renderer. Used for generated video/package handoff.
ipcMain.handle('open-path', async (_, targetPath) => {
  if (typeof targetPath !== 'string' || !targetPath.trim()) return false
  const cleanPath = targetPath.trim()
  if (fs.existsSync(cleanPath) && fs.statSync(cleanPath).isFile()) {
    shell.showItemInFolder(cleanPath)
    return true
  }
  const result = await shell.openPath(cleanPath)
  return result === ''
})

// ── App lifecycle ─────────────────────────────────────────

app.whenReady().then(() => {
  startLocalAgent()
  createWindow()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow()
})

app.on('before-quit', () => {
  void stopLocalAgent()
})
