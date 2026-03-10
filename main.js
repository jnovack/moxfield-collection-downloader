// main.js
const { app, BrowserWindow, dialog, ipcMain, globalShortcut } = require('electron');
const path = require('path');
const fs = require('fs');
const crypto = require('crypto');

app.name = 'jnovack/moxfield-downloader';
app.commandLine.appendSwitch('enable-gpu');
app.commandLine.appendSwitch('enable-webgl');
app.commandLine.appendSwitch('ignore-gpu-blocklist');
app.commandLine.appendSwitch('disable-blink-features', 'AutomationControlled');

let saveDir = null;

let win = BrowserWindow;

function ensureDir(dir) {
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
}

function safeFileName(name) {
  // Very conservative sanitize: remove directory separators and control chars
  return name.replace(/[/\\?%*:|"<>]/g, '-').replace(/\s+/g, '-');
}

function computeFilenameFromUrl(urlString, method) {
  try {
    const u = new URL(urlString);

    // Break the path into safe directory segments
    const parts = u.pathname.split('/').filter(Boolean).map(safeFileName);

    // If no path parts, default to ['index']
    if (parts.length === 0) parts.push('index');

    // Last segment becomes the base filename
    let base = parts.pop() || 'index';

    // Append hash if GET with query params
    if ((method || 'GET').toUpperCase() === 'GET' && u.search && u.search !== '') {
      const hash = crypto.createHash('sha1').update(u.search).digest('hex').slice(0, 7);
      base = `${base}-${hash}`;
    }

    // Add .json extension
    const filename = `${base}.json`;

    // Return a relative path like "v1/cards/search-abc1234.json"
    return path.join(...parts, filename);

  } catch (err) {
    // Fallback: flat hashed filename
    const h = crypto.createHash('sha1').update(urlString).digest('hex').slice(0, 7);
    return `response-${h}.json`;
  }
}

async function saveResponseToDisk({ url, method, body }) {
  if (!saveDir) return;
  try {
    const relPath = computeFilenameFromUrl(url, method);
    const fullPath = path.join(saveDir, relPath);

    // Ensure the directory exists (recursively)
    await fs.promises.mkdir(path.dirname(fullPath), { recursive: true });

    let finalPath = fullPath;

    // If exists, append timestamp to avoid accidental overwrite
    if (fs.existsSync(finalPath)) {
      const ext = path.extname(finalPath);
      const base = path.basename(finalPath, ext);
      const unique = `${base}-${Date.now()}${ext}`;
      finalPath = path.join(path.dirname(finalPath), unique);
    }

    // Try to prettify JSON if possible
    let fileContent = body;
    try {
      const parsed = JSON.parse(body);
      fileContent = JSON.stringify(parsed, null, 2);
    } catch {
      // not valid JSON; save raw
    }

    fs.writeFileSync(finalPath, fileContent, 'utf8');
    console.log(`Saved API response to ${finalPath}`);
  } catch (err) {
    console.error('Failed to save response:', err);
  }
}

function createWindow() {
  const win = new BrowserWindow({
    width: 1200,
    height: 900,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      partition: 'persist:mox',
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true, // helps mimic Chrome security model
    }
  });

  globalShortcut.register('F12', () => {
    if (win) {
      if (win.webContents.isDevToolsOpened()) {
        win.webContents.closeDevTools();
      } else {
        win.webContents.openDevTools({ mode: 'detach' });
      }
    }
  });

  // present itself as an electron app in user agent
  const ua = `jnovack/moxfield-downloader (${process.platform}) ${app.getVersion()}`;
  win.webContents.setUserAgent(
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36'
  );
  // win.loadURL('https://www.moxfield.com', { userAgent: ua });
  win.loadURL('https://www.moxfield.com');
}

app.whenReady().then(async () => {
  // Ask where to save files
  const res = await dialog.showOpenDialog({
    title: 'Choose a folder to save moxfield api2.moxfield.com responses',
    properties: ['openDirectory', 'createDirectory']
  });

  if (res.canceled || !res.filePaths || !res.filePaths[0]) {
    // no directory selected: quit
    app.quit();
    return;
  }

  saveDir = res.filePaths[0];
  console.log('Saving API responses to:', saveDir);

  // IPC handler to receive responses from preload/page
  ipcMain.on('save-api-response', (event, payload) => {
    // payload: { url, method, body }
    saveResponseToDisk(payload);
  });

  createWindow();

  app.on('activate', function () {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('will-quit', () => {
  globalShortcut.unregisterAll();
});

app.on('window-all-closed', function () {
  if (process.platform !== 'darwin') app.quit();
});
