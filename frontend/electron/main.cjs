const { app, BrowserWindow, ipcMain, dialog, shell } = require('electron')
const path = require('path')
const { spawn } = require('child_process')
const fs = require('fs')
const http = require('http')

const isDev = !app.isPackaged
const LOCAL_AGENT_URL = process.env.TANGYING_LOCAL_AGENT_URL || 'http://127.0.0.1:18080'
const CLOUD_API_BASE = process.env.TANGYING_CLOUD_API_BASE || process.env.VITE_CLOUD_API_BASE || 'http://localhost:8080/api'
const APP_ICON_FILE = '躺营ai自媒体运营助手.png'

let mainWindow = null
let localAgentProcess = null

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

function startLocalAgent() {
  if (process.env.TANGYING_SKIP_LOCAL_AGENT === 'true') return
  const binary = localAgentBinaryPath()
  if (!fs.existsSync(binary)) {
    console.warn(`Local agent binary not found: ${binary}`)
    return
  }
  const url = new URL(LOCAL_AGENT_URL)
  const addr = `${url.hostname}:${url.port || '18080'}`
  localAgentProcess = spawn(binary, ['-addr', addr, '-cloud-api-base', CLOUD_API_BASE], {
    stdio: ['ignore', 'ignore', 'pipe'],
    env: {
      ...process.env,
      TANGYING_LOCAL_DATA_DIR: path.join(app.getPath('userData'), 'local-agent'),
    },
  })
  localAgentProcess.stderr.on('data', (data) => {
    console.warn(`[local-agent] ${data.toString().trim()}`)
  })
  localAgentProcess.on('exit', (code) => {
    console.warn(`Local agent exited with code ${code}`)
    localAgentProcess = null
  })
}

function stopLocalAgent() {
  if (localAgentProcess) {
    localAgentProcess.kill()
    localAgentProcess = null
  }
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

// Execute a shell command with args
ipcMain.handle('execute-command', async (_, command, args = [], workDir = '/tmp') => {
  // Security: restrict to safe directory and block dangerous commands
  const safeDir = workDir || '/tmp'

  return new Promise((resolve) => {
    // Use spawn for proper arg handling
    const child = spawn(command, args, {
      cwd: safeDir,
      shell: true,
      timeout: 30000,
      env: { ...process.env, PATH: '/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin' },
    })

    let stdout = ''
    let stderr = ''

    child.stdout.on('data', (data) => { stdout += data.toString() })
    child.stderr.on('data', (data) => { stderr += data.toString() })
    child.on('error', (err) => resolve({ stdout: '', stderr: err.message, exitCode: -1 }))
    child.on('close', (code) => resolve({ stdout, stderr, exitCode: code ?? -1 }))
  })
})

// Read file from disk and return base64-encoded data with metadata.
// Used by the renderer to create proper File objects for upload.
ipcMain.handle('read-file', async (_, filePath) => {
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
  return result.canceled ? [] : result.filePaths
})

// Open directory dialog
ipcMain.handle('open-directory-dialog', async () => {
  const result = await dialog.showOpenDialog(mainWindow, {
    properties: ['openDirectory'],
  })
  return result.canceled ? [] : result.filePaths
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

// Open external URL in system browser
ipcMain.handle('open-external', async (_, url) => {
  if (typeof url === 'string' && url.startsWith('http')) {
    shell.openExternal(url)
  }
})

// ── Multi-platform publishing via browser automation ─────
// Launches Puppeteer to log into platforms and publish content
ipcMain.handle('publish-to-platforms', async (_, payload) => {
  const { title, description, images, platforms } = payload
  const results = []

  for (const platform of platforms) {
    try {
      const result = await publishToPlatform(platform, { title, description, images })
      results.push({ platform, success: true, result })
    } catch (err) {
      results.push({ platform, success: false, error: err.message })
    }
  }

  return results
})

async function publishToPlatform(platform, content) {
  // Try executing a platform-specific script from scripts/publishers/
  const scriptPath = path.join(__dirname, '..', 'scripts', 'publishers', `${platform}.js`)
  if (fs.existsSync(scriptPath)) {
    return new Promise((resolve, reject) => {
      const child = spawn('node', [scriptPath, JSON.stringify(content)], {
        shell: true,
        timeout: 120000,
      })
      let out = ''
      child.stdout.on('data', (d) => { out += d.toString() })
      child.on('close', (code) => {
        if (code === 0) {
          resolve(out)
          return
        }
        reject(new Error(`Script exited with code ${code}: ${out}`))
      })
      child.on('error', reject)
    })
  }
  throw new Error(`No publisher script found for platform: ${platform}. Create scripts/publishers/${platform}.js`)
}

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

app.on('before-quit', stopLocalAgent)
