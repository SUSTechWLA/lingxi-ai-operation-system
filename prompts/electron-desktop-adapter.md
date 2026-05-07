# Skill: Electron 桌面端安全通信层与本地能力集成

## 目标
在现有浏览器版前端基础上，引入 Electron 桌面端支持，通过安全通信层给前端开放受限的本地系统权限。实现「命令/脚本调用、本地文件浏览、自定义下载路径」等核心桌面能力，同时保持原有 Web 端上传下载功能完全兼容。

## 前置条件
- 已有可跑在浏览器中的前端项目（React / Vue / 原生等均可）。
- 后端服务已运行在本地 `127.0.0.1:9966`（或可配置）。
- 开发环境中已安装 Node.js 和 Electron。

## 实施原则
- **非侵入式**：原有 Web 功能一行不改，仅通过环境判断增量添加桌面端 UI 和逻辑。
- **最小权限**：预加载脚本只暴露经过审计的安全 API，不开放 `nodeIntegration`。
- **权限校验**：所有危险操作（如执行命令、写文件）必须通过白名单目录 + 二次确认。
- **渐进增强**：先打通通信链路和基础命令执行，再扩展文件管理等高级能力。

## 约束
- 保持原有浏览器的开发、构建、部署流程不变。
- Electron 仅作为壳，加载原有前端的开发服务器或构建产物（以开发模式优先）。
- 所有改动需编译通过，无破坏性副作用。

---

## 第一步：初始化 Electron 项目结构

### 1.1 在主项目根目录新建 `electron/` 目录
```
electron/
  main.js            # 主进程入口
  preload.js         # 预加载脚本（安全桥）
  package.json       # Electron 依赖与脚本
```

### 1.2 编写 `electron/package.json`
```json
{
  "name": "tangying-aios-desktop",
  "version": "0.1.0",
  "main": "main.js",
  "scripts": {
    "start": "electron .",
    "build": "electron-builder"
  },
  "devDependencies": {
    "electron": "^28.0.0",
    "electron-builder": "^24.0.0"
  }
}
```

### 1.3 修改根 `package.json`，增加桌面启动脚本
```json
"scripts": {
  "electron:dev": "cd electron && npm start"
}
```

---

## 第二步：实现 Electron 主进程 (`main.js`)

### 2.1 窗口创建与生命周期
```javascript
const { app, BrowserWindow, ipcMain, dialog } = require('electron');
const path = require('path');

let mainWindow;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1400,
    height: 900,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,   // 必须开启，配合 contextBridge
      nodeIntegration: false,   // 禁止，安全底线
      sandbox: true
    }
  });

  // 开发模式加载前端开发服务器，生产模式加载打包后文件
  const isDev = process.env.NODE_ENV === 'development';
  if (isDev) {
    mainWindow.loadURL('http://localhost:3000'); // 假设前端 dev server
    mainWindow.webContents.openDevTools();
  } else {
    mainWindow.loadFile(path.join(__dirname, '../web/dist/index.html'));
  }

  mainWindow.on('closed', () => { mainWindow = null; });
}

app.whenReady().then(createWindow);
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit(); });
```

### 2.2 实现受控的 IPC 处理函数

主进程只监听通过 `preload.js` 暴露的频道，所有请求均附带权限检查。

```javascript
// 命令执行：白名单 + 路径限制
ipcMain.handle('system:execute-command', async (event, { command, args, workDir }) => {
  // 0. 权限检查：仅允许在预定义的工作目录内执行
  const allowedDirs = [app.getPath('userData'), '/tmp/tangying-sandbox'];
  const resolved = path.resolve(workDir || process.cwd());
  if (!allowedDirs.some(allowed => resolved.startsWith(allowed))) {
    throw new Error('工作目录不在白名单内');
  }

  // 1. 危险命令拦截
  const dangerousPatterns = ['rm -rf', 'mkfs', 'dd if=', 'chmod 777'];
  const fullCmd = command + ' ' + (args || []).join(' ');
  if (dangerousPatterns.some(p => fullCmd.includes(p))) {
    throw new Error('命令包含危险操作，已拦截');
  }

  // 2. 二次确认（在 preload 中调用 dialog.showMessageBox）
  // 这里主进程执行命令，但确认发生在渲染进程通过 preload 调用 dialog

  const { execFile } = require('child_process');
  return new Promise((resolve, reject) => {
    execFile(command, args || [], {
      cwd: resolved,
      timeout: 30000,
      maxBuffer: 1024 * 1024
    }, (error, stdout, stderr) => {
      resolve({ stdout, stderr, exitCode: error ? error.code : 0 });
    });
  });
});

// 文件选择对话框
ipcMain.handle('dialog:open-file', async (event, options) => {
  const result = await dialog.showOpenDialog(mainWindow, options);
  return result.filePaths;
});

// 文件夹选择对话框
ipcMain.handle('dialog:open-directory', async (event, options) => {
  const result = await dialog.showOpenDialog(mainWindow, { ...options, properties: ['openDirectory'] });
  return result.filePaths;
});

// 保存文件对话框（下载到自定义路径）
ipcMain.handle('dialog:save-file', async (event, options) => {
  const result = await dialog.showSaveDialog(mainWindow, options);
  return result.filePath;
});

// 本地服务健康检查（后台自动检测）
ipcMain.handle('service:health', async () => {
  const http = require('http');
  return new Promise((resolve) => {
    const req = http.get('http://127.0.0.1:9966/api/health', (res) => {
      resolve(res.statusCode === 200 ? 'ok' : 'unhealthy');
    });
    req.on('error', () => resolve('unreachable'));
    req.setTimeout(3000, () => { req.destroy(); resolve('timeout'); });
  });
});
```

---

## 第三步：编写安全预加载脚本 (`preload.js`)

```javascript
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('electronAPI', {
  // 受控的命令执行（带用户确认）
  executeCommand: async (command, args, workDir) => {
    // 二次确认
    const choice = await ipcRenderer.invoke('dialog:confirm', {
      message: `即将执行命令：\n${command} ${args?.join(' ') || ''}\n\n是否继续？`
    });
    if (!choice) throw new Error('用户取消执行');
    return ipcRenderer.invoke('system:execute-command', { command, args, workDir });
  },

  // 文件选择
  openFileDialog: (options) => ipcRenderer.invoke('dialog:open-file', options),

  // 文件夹选择
  openDirectoryDialog: (options) => ipcRenderer.invoke('dialog:open-directory', options),

  // 保存文件
  saveFileDialog: (options) => ipcRenderer.invoke('dialog:save-file', options),

  // 服务连通性
  checkServiceHealth: () => ipcRenderer.invoke('service:health'),

  // 判断当前是否为桌面环境
  isElectron: true
});
```

> **说明**：`dialog:confirm` 需在主进程用 `dialog.showMessageBox` 实现，并在 `preload` 中暴露。此处省略具体代码，可参考 Electron 文档。

---

## 第四步：前端增量改造（非侵入式）

### 4.1 环境检测与模式标记
在原有前端入口文件中添加：

```javascript
// 假设使用 React，其他框架同理
const isElectron = window.electronAPI?.isElectron || false;

// 可通过 Context 或全局状态传递
export const AppModeContext = React.createContext({ isElectron });
```

### 4.2 新增桌面端专属入口（仅在 Electron 中展示）

在主界面顶部或侧边栏增加一个“桌面工具”入口，当 `isElectron` 为 `true` 时显示。

示例组件结构（伪代码）：
```jsx
{isElectron && (
  <DesktopToolbar>
    <CommandPanel />         {/* 命令执行面板 */}
    <ScriptManager />        {/* 本地脚本管理 */}
    <FileBrowser />          {/* 本地文件浏览器 */}
    <CustomDownloadPath />   {/* 下载路径设置 */}
  </DesktopToolbar>
)}
```

### 4.3 命令执行面板组件（核心）

组件状态：输入命令、参数、工作目录，显示执行结果（stdout/stderr）。
```jsx
function CommandPanel() {
  const [command, setCommand] = useState('');
  const [output, setOutput] = useState('');

  const runCommand = async () => {
    try {
      const result = await window.electronAPI.executeCommand(command, [], '/tmp');
      setOutput(result.stdout + '\n' + result.stderr);
    } catch (e) {
      setOutput('错误：' + e.message);
    }
  };

  return (
    <div>
      <input value={command} onChange={e => setCommand(e.target.value)} placeholder="输入命令" />
      <button onClick={runCommand}>执行</button>
      <pre>{output}</pre>
    </div>
  );
}
```

### 4.4 文件操作改造

原有上传/下载按钮增加 Electron 环境下的本地文件选择逻辑：
```javascript
// 原浏览器上传（保持不变）
<input type="file" onChange={uploadFile} />

// 桌面端增加一个按钮
{isElectron && (
  <button onClick={async () => {
    const files = await window.electronAPI.openFileDialog({ properties: ['openFile', 'multiSelections'] });
    // 将选择的本地文件路径发送给后端处理，或者读取后手动上传
  }}>
    导入本地文件
  </button>
)}
```

下载功能：在 Electron 中调用 `saveFileDialog` 指定保存路径，再通知后端将文件内容保存至该路径。

### 4.5 服务连通性检测

在 Electron 环境启动时自动检测后端服务：
```javascript
// 在 App 组件挂载时
if (isElectron) {
  window.electronAPI.checkServiceHealth().then(status => {
    if (status !== 'ok') {
      showErrorNotification('后端服务未启动，请检查');
    }
  });
  // 每30秒检测一次
  setInterval(async () => { /* 同上 */ }, 30000);
}
```

---

## 第五步：确保原有浏览器功能零破坏

- 所有 `window.electronAPI` 调用都必须加可选链或 `isElectron` 判断，避免在普通浏览器中报错。
- 构建流程保持独立：Web 前端代码正常打包，Electron 通过加载 `http://localhost:3000` 开发或加载打包后的 `dist` 目录。
- 原有上传下载逻辑中的 `fetch` / `axios` 完全不变，仅在外层增加文件路径选择逻辑。

---

## 第六步：测试验证

1. **纯浏览器模式**：打开原有 `localhost:3000`，确认所有功能正常，无 Electron 相关 UI 或报错。
2. **Electron 开发模式**：启动后端 + 前端 dev server，运行 `npm run electron:dev`，检查桌面端专属入口出现，命令执行成功，文件选择对话框工作。
3. **服务断连测试**：关闭后端，重启 Electron，应看到错误提示。
4. **安全测试**：尝试执行 `rm -rf /` 或指定白名单外目录，应被拦截。
5. **打包测试**：使用 `electron-builder` 打包后，在干净环境中运行，确保前端静态资源能够离线工作。