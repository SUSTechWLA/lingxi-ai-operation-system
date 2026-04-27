# Skill：React 项目 → Electron 桌面应用 → .exe 安装包

## 概述
将用户已有的 React 项目（使用 Create React App 或 Vite）自动包装为 Electron 桌面应用，并打包成 Windows 安装包（.exe），让小白用户双击即可安装使用。

**前置条件**：
- React 项目已存在，且能在浏览器正常运行。
- 项目根目录有 `package.json`。
- 用户系统已安装 Node.js 且 `npm` 可用。

## 执行流程
1. 自动检测 React 项目的构建方式（CRA 还是 Vite），调整少量配置。
2. 创建 `electron` 目录，写入 `main.js` 和 `preload.js`。
3. 修改 `package.json`，添加 Electron 依赖、入口文件、打包脚本和打包配置。
4. 安装 Electron 依赖。
5. 构建 React 项目。
6. 运行 `electron-builder` 打包，输出 `.exe`（Windows）或 `.dmg`（Mac）。
7. 提示用户打包产物的位置。

## 文件模板（可直接写入）

### `electron/main.js`
```javascript
const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('path');
const { exec } = require('child_process');

let mainWindow;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      nodeIntegration: false,
      contextIsolation: true,
    },
  });

  // 生产模式加载构建后的 index.html
  mainWindow.loadFile(path.join(__dirname, '../build/index.html'));

  // 可选：开发模式加载 dev server，需要环境变量控制
  // if (process.env.NODE_ENV === 'development') {
  //   mainWindow.loadURL('http://localhost:3000');
  // } else {
  //   mainWindow.loadFile(path.join(__dirname, '../build/index.html'));
  // }
}

// 安全暴露：执行系统命令
ipcMain.handle('run-command', async (event, command) => {
  return new Promise((resolve) => {
    exec(command, { timeout: 30000 }, (error, stdout, stderr) => {
      if (error) resolve({ error: error.message });
      else resolve({ result: stdout, error: stderr });
    });
  });
});

app.whenReady().then(createWindow);

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow();
});
```

### `electron/preload.js`
```javascript
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('agent', {
  runCommand: (cmd) => ipcRenderer.invoke('run-command', cmd),
});
```

## `package.json` 修改模板（合并到原 package.json）
注意：不要覆盖原有的 `scripts` 和 `dependencies`，仅添加/修改以下字段。

```json
{
  "main": "electron/main.js",
  "scripts": {
    "electron-dev": "concurrently \"npm start\" \"wait-on http://localhost:3000 && electron .\"",
    "electron-build": "npm run build && electron-builder"
  },
  "devDependencies": {
    "electron": "^28.0.0",
    "electron-builder": "^24.0.0",
    "concurrently": "^8.2.0",
    "wait-on": "^7.0.0"
  },
  "build": {
    "appId": "com.yourcompany.yourapp",
    "productName": "你的应用名",
    "directories": {
      "output": "dist-electron"
    },
    "files": [
      "build/**/*",
      "electron/**/*"
    ],
    "win": {
      "target": "nsis",
      "icon": "public/favicon.ico"
    },
    "mac": {
      "target": "dmg"
    },
    "nsis": {
      "oneClick": false,
      "allowToChangeInstallationDirectory": true
    }
  }
}
```
> **注意**：如果 React 项目使用 Vite，构建产物目录可能为 `dist`，请将 `build` 全部替换为 `dist`，并调整 `main.js` 中的 `loadFile` 路径。

## 操作步骤（Claude Code 自动执行）

1. **询问用户**：
   - “你的 React 项目构建工具是 Create React App 还是 Vite？（默认 CRA）”
   - “应用的英文名称（用于 appId）？”
   - “应用的中文产品名（显示在安装界面和桌面图标）？”
   - “是否需要调用系统命令的能力？若需要，我们将在渲染进程暴露 `window.agent.runCommand(cmd)`。”

2. **检测构建路径**：
   - 若项目下有 `public/index.html` 且无 `vite.config.js`，判定为 CRA，构建目录 `build`。
   - 若有 `vite.config.js`，判定为 Vite，构建目录 `dist`。

3. **写入文件**：
   - 创建 `electron/main.js`，根据上一步的目录调整 `loadFile` 路径。
   - 创建 `electron/preload.js`。
   - 修改 `package.json`：加入 `main`、`scripts`、`devDependencies`（保留原有依赖）、`build` 配置。**注意**：如果原有 `homepage` 为 `"."`，保留；否则可能需处理静态资源路径。

4. **安装依赖**：
   ```bash
   npm install
   ```

5. **构建前端**：
   - CRA：`npm run build`
   - Vite：`npm run build`（确保 vite.config 中 `base` 为 `'./'`）

6. **打包 Electron**：
   ```bash
   npx electron-builder --win
   ```
   （如需生成 Mac 版本，去掉 `--win` 并在 Mac 上运行）

7. **输出结果**：
   - 提示用户： “✅ 打包完成！安装包位于 `dist-electron/你的应用名 Setup x.x.x.exe`，将其发送给小白用户，双击即可安装。”

## 异常处理
- 如果 `npm install` 失败，检查 Node 版本（建议 18+）。
- 如果 `npm run build` 失败，检查 React 项目本身是否正确。
- 如果 `electron-builder` 报错（如缺少 wine、Mono），提示用户按错误信息解决环境问题，或仅生成文件夹版本（Green）。