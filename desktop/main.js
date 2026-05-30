'use strict';

const { app, BrowserWindow, ipcMain, dialog, shell } = require('electron');
const path = require('path');
const fs = require('fs');
const readline = require('readline');
const { spawn } = require('child_process');

let mainWindow = null;
let currentChild = null;

// ---- 设置持久化（存到 userData，绝不进仓库；api_key 仅本地保存）----
function settingsPath() {
  return path.join(app.getPath('userData'), 'settings.json');
}

const defaultSettings = {
  baseUrl: 'http://127.0.0.1:8317/v1',
  apiKey: '',
  model: 'gpt-5.5',
  temperature: 0,
  kbRoot: '',
  outPath: '',
  intents: { info: 3, guidance: 1, draft: 1 },
  concurrency: 6,
  maxDocs: 0,
};

function loadSettings() {
  try {
    const raw = fs.readFileSync(settingsPath(), 'utf8');
    return Object.assign({}, defaultSettings, JSON.parse(raw));
  } catch {
    return Object.assign({}, defaultSettings);
  }
}

function saveSettings(s) {
  try {
    fs.writeFileSync(settingsPath(), JSON.stringify(s, null, 2), 'utf8');
    return true;
  } catch (e) {
    return false;
  }
}

// ---- 解析 qgen 二进制路径 ----
function agentBinaryPath() {
  const name = process.platform === 'win32' ? 'qgen.exe' : 'qgen';
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'bin', name);
  }
  return path.join(__dirname, 'bin', name);
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1080,
    height: 820,
    minWidth: 900,
    minHeight: 680,
    title: '知识库问题生成器',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });
  mainWindow.loadFile(path.join(__dirname, 'renderer', 'index.html'));
  // mainWindow.webContents.openDevTools();
}

app.whenReady().then(() => {
  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});

// ---- IPC：设置 ----
ipcMain.handle('settings:load', () => loadSettings());
ipcMain.handle('settings:save', (_e, s) => saveSettings(s));

// ---- IPC：对话框 ----
ipcMain.handle('dialog:pickFolder', async () => {
  const r = await dialog.showOpenDialog(mainWindow, { properties: ['openDirectory'] });
  return r.canceled ? '' : r.filePaths[0];
});

ipcMain.handle('dialog:pickSave', async (_e, defaultName) => {
  const r = await dialog.showSaveDialog(mainWindow, {
    defaultPath: defaultName || '知识库问题清单.xlsx',
    filters: [{ name: 'Excel', extensions: ['xlsx'] }],
  });
  return r.canceled ? '' : r.filePath;
});

// ---- IPC：打开/定位文件 ----
ipcMain.handle('file:open', (_e, p) => shell.openPath(p));
ipcMain.handle('file:reveal', (_e, p) => shell.showItemInFolder(p));

// ---- IPC：运行 agent ----
ipcMain.handle('run:start', (_e, opts) => {
  return new Promise((resolve) => {
    if (currentChild) {
      resolve({ ok: false, error: '已有任务在运行' });
      return;
    }
    const bin = agentBinaryPath();
    if (!fs.existsSync(bin)) {
      resolve({ ok: false, error: '未找到 qgen 可执行文件：' + bin + '\n请先在 desktop 目录执行 npm run build:agent' });
      return;
    }

    const args = ['-json'];
    if (opts.dryRun) args.push('-dry-run');
    if (opts.kbRoot) args.push('-root', opts.kbRoot);
    if (!opts.dryRun && opts.outPath) args.push('-out', opts.outPath);
    if (typeof opts.intents?.info === 'number') args.push('-n', String(opts.intents.info));
    if (typeof opts.intents?.guidance === 'number') args.push('-guidance', String(opts.intents.guidance));
    if (typeof opts.intents?.draft === 'number') args.push('-draft', String(opts.intents.draft));
    if (opts.concurrency) args.push('-c', String(opts.concurrency));
    if (typeof opts.maxDocs === 'number') args.push('-max', String(opts.maxDocs));

    const env = Object.assign({}, process.env, {
      QGEN_BASE_URL: opts.baseUrl || '',
      QGEN_API_KEY: opts.apiKey || '',
      QGEN_MODEL: opts.model || '',
      QGEN_TEMPERATURE: String(opts.temperature ?? 0),
    });

    const send = (ev) => {
      if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('run:event', ev);
    };

    let child;
    try {
      child = spawn(bin, args, { env });
    } catch (e) {
      resolve({ ok: false, error: String(e) });
      return;
    }
    currentChild = child;

    const rl = readline.createInterface({ input: child.stdout });
    rl.on('line', (line) => {
      const t = line.trim();
      if (!t) return;
      try {
        send(JSON.parse(t));
      } catch {
        send({ type: 'log', message: t });
      }
    });

    let stderrBuf = '';
    child.stderr.on('data', (d) => {
      stderrBuf += d.toString();
      send({ type: 'log', message: d.toString().trim() });
    });

    child.on('error', (err) => {
      send({ type: 'error', message: '启动失败: ' + String(err) });
    });

    child.on('close', (code) => {
      currentChild = null;
      send({ type: 'exit', code });
      resolve({ ok: code === 0, code, stderr: stderrBuf });
    });
  });
});

ipcMain.handle('run:cancel', () => {
  if (currentChild) {
    currentChild.kill('SIGTERM');
    currentChild = null;
    return true;
  }
  return false;
});
