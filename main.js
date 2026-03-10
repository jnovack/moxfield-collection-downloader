// main.js
const { app, BrowserWindow, dialog, ipcMain, globalShortcut, clipboard } = require('electron');
const path = require('path');
const fs = require('fs');
const crypto = require('crypto');

app.name = 'jnovack/moxfield-downloader';
app.commandLine.appendSwitch('enable-gpu');
app.commandLine.appendSwitch('enable-webgl');
app.commandLine.appendSwitch('ignore-gpu-blocklist');
app.commandLine.appendSwitch('disable-blink-features', 'AutomationControlled');

let saveDir = null;
let launchUrl = 'https://www.moxfield.com';

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

function createWindow(initialUrl = launchUrl) {
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
  win.loadURL(initialUrl);
}

function isValidCollectionUrl(urlString) {
  try {
    const u = new URL(urlString);
    const isHttps = u.protocol === 'https:';
    const isMoxfieldHost = u.hostname === 'moxfield.com' || u.hostname === 'www.moxfield.com';
    const isCollectionPath = /^\/collection\/[A-Za-z0-9_-]+\/?$/.test(u.pathname);
    return isHttps && isMoxfieldHost && isCollectionPath;
  } catch {
    return false;
  }
}

async function promptForLaunchMode() {
  const { response } = await dialog.showMessageBox({
    type: 'question',
    title: 'Choose Launch Mode',
    message: 'How would you like to start?',
    detail: 'Browse mode opens the Moxfield front page. Collection mode opens a specific collection URL.',
    buttons: ['Browse Mode', 'Collection Mode', 'Cancel'],
    defaultId: 0,
    cancelId: 2,
    noLink: true
  });

  if (response === 0) return 'browse';
  if (response === 1) return 'collection';
  return null;
}

function getValidCollectionUrlFromClipboard() {
  const text = (clipboard.readText() || '').trim();
  if (!text) return null;
  return isValidCollectionUrl(text) ? text : null;
}

async function promptForCollectionUrl(initialValue = '') {
  const promptWin = new BrowserWindow({
    width: 560,
    height: 220,
    resizable: false,
    minimizable: false,
    maximizable: false,
    autoHideMenuBar: true,
    modal: true,
    show: true,
    webPreferences: {
      nodeIntegration: true,
      contextIsolation: false
    }
  });

  const html = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>Collection URL</title>
    <style>
      body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 18px; }
      h2 { margin: 0 0 8px; font-size: 18px; }
      p { margin: 0 0 12px; color: #333; font-size: 13px; }
      input { width: 100%; box-sizing: border-box; font-size: 14px; padding: 8px; margin-bottom: 12px; }
      .actions { display: flex; justify-content: flex-end; gap: 8px; }
      button { padding: 6px 12px; font-size: 13px; }
    </style>
  </head>
  <body>
    <h2>Collection Mode</h2>
    <p>Enter a Moxfield collection URL:</p>
    <input id="urlInput" type="text" placeholder="https://moxfield.com/collection/abcdef123456" autofocus />
    <div class="actions">
      <button id="cancelBtn" type="button">Cancel</button>
      <button id="openBtn" type="button">Open</button>
    </div>
    <script>
      const { ipcRenderer } = require('electron');
      const input = document.getElementById('urlInput');
      const openBtn = document.getElementById('openBtn');
      const cancelBtn = document.getElementById('cancelBtn');
      const initialValue = ${JSON.stringify(initialValue)};

      if (initialValue) {
        input.value = initialValue;
        input.select();
      }

      function submit() {
        ipcRenderer.send('collection-url-prompt-result', input.value || '');
      }

      function cancel() {
        ipcRenderer.send('collection-url-prompt-result', null);
      }

      openBtn.addEventListener('click', submit);
      cancelBtn.addEventListener('click', cancel);
      input.addEventListener('keydown', (ev) => {
        if (ev.key === 'Enter') submit();
        if (ev.key === 'Escape') cancel();
      });
      window.addEventListener('beforeunload', cancel);
    </script>
  </body>
</html>`;

  await promptWin.loadURL(`data:text/html;charset=UTF-8,${encodeURIComponent(html)}`);

  return await new Promise((resolve) => {
    let settled = false;

    const finish = (value) => {
      if (settled) return;
      settled = true;
      ipcMain.removeListener('collection-url-prompt-result', onResult);
      if (!promptWin.isDestroyed()) promptWin.close();
      resolve(value);
    };

    const onResult = (_event, value) => finish(value);
    ipcMain.once('collection-url-prompt-result', onResult);
    promptWin.on('closed', () => finish(null));
  });
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

  const mode = await promptForLaunchMode();
  if (!mode) {
    app.quit();
    return;
  }

  if (mode === 'collection') {
    const clipboardCollectionUrl = getValidCollectionUrlFromClipboard();
    let firstPrompt = true;
    while (true) {
      const enteredUrl = await promptForCollectionUrl(firstPrompt ? (clipboardCollectionUrl || '') : '');
      firstPrompt = false;
      if (!enteredUrl) {
        app.quit();
        return;
      }

      const trimmedUrl = enteredUrl.trim();
      if (isValidCollectionUrl(trimmedUrl)) {
        launchUrl = trimmedUrl;
        break;
      }

      const invalid = await dialog.showMessageBox({
        type: 'warning',
        title: 'Invalid Collection URL',
        message: 'Please enter a valid Moxfield collection URL.',
        detail: 'Expected format: https://moxfield.com/collection/<id>',
        buttons: ['Try Again', 'Cancel'],
        defaultId: 0,
        cancelId: 1,
        noLink: true
      });

      if (invalid.response === 1) {
        app.quit();
        return;
      }
    }
  } else {
    launchUrl = 'https://www.moxfield.com';
  }

  // IPC handler to receive responses from preload/page
  ipcMain.on('save-api-response', (event, payload) => {
    // payload: { url, method, body }
    saveResponseToDisk(payload);
  });

  createWindow(launchUrl);

  app.on('activate', function () {
    if (BrowserWindow.getAllWindows().length === 0) createWindow(launchUrl);
  });
});

app.on('will-quit', () => {
  globalShortcut.unregisterAll();
});

app.on('window-all-closed', function () {
  if (process.platform !== 'darwin') app.quit();
});
