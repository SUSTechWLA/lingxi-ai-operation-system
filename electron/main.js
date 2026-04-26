const { app, BrowserWindow, ipcMain, dialog } = require('electron')
const path = require('path')
const { execFile } = require('child_process')
const http = require('http')

let mainWindow

const BACKEND_URL = process.env.LINGXI_BACKEND_URL || 'http://localhost:8080'
const FRONTEND_DEV_URL = 'http://localhost:3000'

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1400,
    height: 900,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })

  const isDev = process.env.NODE_ENV === 'development'
  if (isDev) {
    mainWindow.loadURL(FRONTEND_DEV_URL)
    mainWindow.webContents.openDevTools()
  } else {
    mainWindow.loadFile(path.join(__dirname, '../frontend/dist/index.html'))
  }

  mainWindow.on('closed', () => {
    mainWindow = null
  })
}

app.whenReady().then(createWindow)
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
app.on('activate', () => {
  if (mainWindow === null) createWindow()
})

// --- IPC Handlers ---

// Confirm dialog
ipcMain.handle('dialog:confirm', async (_event, { message }) => {
  const result = await dialog.showMessageBox(mainWindow, {
    type: 'question',
    buttons: ['取消', '确认'],
    defaultId: 1,
    title: '操作确认',
    message: message || '确认执行此操作？',
  })
  return result.response === 1
})

// Command execution: whitelist dirs + dangerous pattern filtering + execFile
ipcMain.handle('system:execute-command', async (_event, { command, args, workDir }) => {
  const allowedDirs = [app.getPath('userData'), '/tmp/lingxi-sandbox', '/tmp/ai-sandbox']
  const resolved = path.resolve(workDir || process.cwd())
  if (!allowedDirs.some((d) => resolved.startsWith(d))) {
    throw new Error('工作目录不在白名单内')
  }

  const dangerousPatterns = ['rm -rf', 'mkfs', 'dd if=', 'chmod 777', 'format ', 'del /']
  const fullCmd = command + ' ' + (args || []).join(' ')
  if (dangerousPatterns.some((p) => fullCmd.includes(p))) {
    throw new Error('命令包含危险操作，已拦截')
  }

  return new Promise((resolve, reject) => {
    execFile(command, args || [], {
      cwd: resolved,
      timeout: 30000,
      maxBuffer: 1024 * 1024,
    }, (error, stdout, stderr) => {
      resolve({
        stdout: stdout || '',
        stderr: stderr || '',
        exitCode: error ? (error.code || 1) : 0,
      })
    })
  })
})

// File open dialog
ipcMain.handle('dialog:open-file', async (_event, options) => {
  const result = await dialog.showOpenDialog(mainWindow, options)
  return result.filePaths
})

// Directory open dialog
ipcMain.handle('dialog:open-directory', async (_event, options) => {
  const result = await dialog.showOpenDialog(mainWindow, {
    ...options,
    properties: ['openDirectory'],
  })
  return result.filePaths
})

// Save file dialog
ipcMain.handle('dialog:save-file', async (_event, options) => {
  const result = await dialog.showSaveDialog(mainWindow, options)
  return result.filePath
})

// Backend health check
ipcMain.handle('service:health', async () => {
  return new Promise((resolve) => {
    const url = new URL('/api/health', BACKEND_URL)
    const req = http.get(url.toString(), (res) => {
      resolve(res.statusCode === 200 ? 'ok' : 'unhealthy')
    })
    req.on('error', () => resolve('unreachable'))
    req.setTimeout(3000, () => {
      req.destroy()
      resolve('timeout')
    })
  })
})
