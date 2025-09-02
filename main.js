// main.js
const { app, BrowserWindow, dialog, ipcMain, globalShortcut } = require('electron');
const path = require('path');
const fs = require('fs');
const crypto = require('crypto');

app.name = 'jnovack/moxfield-downloader';

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
    // get last path segment
    const parts = u.pathname.split('/').filter(Boolean);
    let base = parts.length ? parts[parts.length - 1] : 'index';
    base = base || 'index';
    base = safeFileName(base);

    if ((method || 'GET').toUpperCase() === 'GET' && u.search && u.search !== '') {
      const hash = crypto.createHash('sha1').update(u.search).digest('hex').slice(0, 7);
      return `${base}-${hash}.json`;
    } else {
      return `${base}.json`;
    }
  } catch (err) {
    // fallback
    const h = crypto.createHash('sha1').update(urlString).digest('hex').slice(0, 7);
    return `response-${h}.json`;
  }
}

async function saveResponseToDisk({ url, method, body }) {
  if (!saveDir) return;
  try {
    ensureDir(saveDir);
    const fname = computeFilenameFromUrl(url, method);
    let full = path.join(saveDir, fname);

    // If exists, append timestamp to avoid accidental overwrite
    if (fs.existsSync(full)) {
      const ext = path.extname(full);
      const base = path.basename(full, ext);
      const unique = `${base}-${Date.now()}${ext}`;
      full = path.join(saveDir, unique);
    }

    // Try to prettify JSON if possible
    let fileContent = body;
    try {
      const parsed = JSON.parse(body);
      fileContent = JSON.stringify(parsed, null, 2);
    } catch (e) {
      // not valid JSON; save raw
    }

    fs.writeFileSync(full, fileContent, 'utf8');
    console.log(`Saved API response to ${full}`);
  } catch (err) {
    console.error('Failed to save response:', err);
  }
}

function createWindow() {
  console.log("Preload path:", path.join(__dirname, 'preload.js'));
  const win = new BrowserWindow({
    width: 1200,
    height: 900,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false
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
  win.loadURL('https://www.moxfield.com', { userAgent: ua });
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
