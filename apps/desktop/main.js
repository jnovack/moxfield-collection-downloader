// main.js
const { app, BrowserWindow, dialog, ipcMain, clipboard } = require('electron');
const path = require('path');
const fs = require('fs');
const crypto = require('crypto');
const { computeApiResponseRelativePath } = require('../../packages/core/files');
const { isValidCollectionUrl, extractCollectionIdFromUrl } = require('../../packages/core/moxfield');

app.name = 'jnovack/moxfield-downloader';
app.commandLine.appendSwitch('enable-gpu');
app.commandLine.appendSwitch('enable-webgl');
app.commandLine.appendSwitch('ignore-gpu-blocklist');
app.commandLine.appendSwitch('disable-blink-features', 'AutomationControlled');

let saveDir = null;
let mainWindow = null;
let collectionWorkerWindow = null;
let collectionProgressWindow = null;
const savedResponseHashes = new Set();

const COLLECTION_PAGE_DELAY_MS = 3000;
const COLLECTION_GUARDRAIL_MAX_PAGES = 10;
const DEFAULT_COLLECTION_PAGE_SIZE = 50;
const COLLECTION_CACHE_MAX_AGE_DAYS = Math.max(
  1,
  Number.parseInt(process.env.COLLECTION_CACHE_MAX_AGE_DAYS || '7', 10) || 7
);

async function saveResponseToDisk({ url, method, body }) {
  if (!saveDir) return;
  try {
    const responseHash = crypto
      .createHash('sha1')
      .update(`${method || 'GET'}|${url || ''}|${body || ''}`)
      .digest('hex');
    if (savedResponseHashes.has(responseHash)) {
      return;
    }
    savedResponseHashes.add(responseHash);

    const relPath = computeApiResponseRelativePath(url, method);
    const fullPath = path.join(saveDir, relPath);

    // Ensure the directory exists (recursively)
    await fs.promises.mkdir(path.dirname(fullPath), { recursive: true });

    // Try to prettify JSON if possible
    let fileContent = body;
    try {
      const parsed = JSON.parse(body);
      fileContent = JSON.stringify(parsed, null, 2);
    } catch {
      // not valid JSON; save raw
    }

    fs.writeFileSync(fullPath, fileContent, 'utf8');
    console.log(`Saved API response to ${fullPath}`);
  } catch (err) {
    console.error('Failed to save response:', err);
  }
}

function getExpectedResponsePath(url, method = 'GET') {
  if (!saveDir) return null;
  return path.join(saveDir, computeApiResponseRelativePath(url, method));
}

async function readFreshCachedResponse(url, method = 'GET') {
  const fullPath = getExpectedResponsePath(url, method);
  if (!fullPath) return null;

  try {
    const stat = await fs.promises.stat(fullPath);
    const maxAgeMs = COLLECTION_CACHE_MAX_AGE_DAYS * 24 * 60 * 60 * 1000;
    const ageMs = Date.now() - stat.mtimeMs;
    if (ageMs > maxAgeMs) {
      return null;
    }
    const body = await fs.promises.readFile(fullPath, 'utf8');
    return { url, method, body, fromCache: true, cachePath: fullPath };
  } catch (err) {
    if (err && err.code === 'ENOENT') return null;
    console.warn(`[collection] Failed reading cache file for ${url}:`, err.message || err);
    return null;
  }
}

function createCollectionWorkerWindow(initialUrl) {
  collectionWorkerWindow = new BrowserWindow({
    width: 1200,
    height: 900,
    show: false,
    skipTaskbar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      partition: 'persist:mox',
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true
    }
  });

  collectionWorkerWindow.webContents.setUserAgent(
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36'
  );
  collectionWorkerWindow.loadURL(initialUrl);
  return collectionWorkerWindow;
}

function createCollectionProgressWindow(initialCollectionUrl = '') {
  const progressWin = new BrowserWindow({
    width: 940,
    height: 700,
    minWidth: 900,
    minHeight: 680,
    autoHideMenuBar: true,
    backgroundColor: '#101418',
    webPreferences: {
      nodeIntegration: true,
      contextIsolation: false
    }
  });

  const html = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width,initial-scale=1" />
    <meta name="color-scheme" content="dark light" />
    <title>Collection Download</title>
    <style>
      :root {
        color-scheme: dark light;
        --bg: #101418;
        --panel: #171d22;
        --text: #e7edf2;
        --muted: #96a3af;
        --border: #2b3843;
        --accent: #2f9e75;
        --accent-track: #1e2730;
      }

      @media (prefers-color-scheme: light) {
        :root {
          --bg: #f2f5f8;
          --panel: #ffffff;
          --text: #0f1720;
          --muted: #4e5d6a;
          --border: #c7d3dc;
          --accent: #1f7a57;
          --accent-track: #dbe4ec;
        }
      }

      * { box-sizing: border-box; }
      html, body {
        margin: 0;
        padding: 0;
        width: 100%;
        height: 100%;
        background: radial-gradient(circle at 15% 20%, #1a232a 0%, var(--bg) 45%);
        color: var(--text);
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        overflow: hidden;
      }
      .wrap {
        width: 100%;
        height: 100%;
        max-width: 900px;
        margin: 0 auto;
        padding: 22px;
        display: flex;
      }
      .panel {
        width: 100%;
        background: color-mix(in srgb, var(--panel) 90%, transparent);
        border: 1px solid var(--border);
        border-radius: 12px;
        padding: 18px;
        box-shadow: 0 12px 30px rgba(0, 0, 0, 0.18);
        display: flex;
        flex-direction: column;
        justify-content: center;
      }
      .input-section { display: block; }
      .progress-section { display: none; }
      h1 {
        margin: 0 0 10px;
        font-size: 24px;
        line-height: 1.2;
      }
      .meta {
        color: var(--muted);
        font-size: 13px;
        margin-bottom: 18px;
        overflow-wrap: anywhere;
      }
      .stat-row {
        display: flex;
        justify-content: space-between;
        align-items: baseline;
        margin-bottom: 8px;
      }
      .status {
        font-size: 14px;
        color: var(--muted);
      }
      .percent {
        font-size: 26px;
        font-weight: 600;
      }
      .track {
        width: 100%;
        height: 18px;
        border-radius: 999px;
        background: var(--accent-track);
        border: 1px solid var(--border);
        overflow: hidden;
        position: relative;
      }
      .fill {
        width: 0%;
        height: 100%;
        background: linear-gradient(90deg, #2f9e75 0%, #2ab685 100%);
        transition: width 3.0s linear;
        position: relative;
        overflow: hidden;
      }
      .fill::after {
        content: "";
        position: absolute;
        inset: 0;
        background-image: linear-gradient(
          110deg,
          rgba(255, 255, 255, 0) 0%,
          rgba(255, 255, 255, 0.26) 38%,
          rgba(255, 255, 255, 0.06) 55%,
          rgba(255, 255, 255, 0) 78%
        );
        background-size: 140px 100%;
        animation: sweep 2s linear infinite;
      }
      .footer {
        margin-top: 14px;
        font-size: 14px;
        color: var(--muted);
        display: flex;
        justify-content: space-between;
        gap: 16px;
      }
      .kv {
        font-variant-numeric: tabular-nums;
      }
      .stats-grid {
        margin-top: 16px;
        display: grid;
        grid-template-columns: repeat(5, minmax(96px, 1fr));
        gap: 10px;
      }
      .stat-card {
        border: 1px solid var(--border);
        border-radius: 10px;
        padding: 10px 12px;
        background: color-mix(in srgb, var(--panel) 86%, #1d262d);
      }
      .stat-label {
        font-size: 11px;
        color: var(--muted);
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }
      .stat-value {
        margin-top: 4px;
        font-size: 18px;
        font-weight: 650;
        font-variant-numeric: tabular-nums;
      }
      .done {
        color: #3bc389;
      }
      .error {
        color: #ff8c7d;
      }
      .url-form {
        margin-top: 12px;
      }
      .url-label {
        font-size: 13px;
        color: var(--muted);
        display: block;
        margin-bottom: 8px;
      }
      .url-input-row {
        display: grid;
        grid-template-columns: 1fr auto;
        gap: 10px;
      }
      .url-input {
        width: 100%;
        min-width: 0;
        padding: 11px 12px;
        border-radius: 9px;
        border: 1px solid var(--border);
        background: color-mix(in srgb, var(--panel) 86%, #0f1419);
        color: var(--text);
        font-size: 14px;
        outline: none;
      }
      .url-input:focus {
        border-color: #48b386;
        box-shadow: 0 0 0 2px color-mix(in srgb, #48b386 35%, transparent);
      }
      .start-btn {
        border: 1px solid #2f9e75;
        background: linear-gradient(180deg, #34b283 0%, #2b906d 100%);
        color: #ecfff6;
        border-radius: 9px;
        padding: 0 15px;
        font-size: 14px;
        font-weight: 600;
        cursor: pointer;
      }
      .start-btn:disabled {
        opacity: 0.6;
        cursor: not-allowed;
      }
      .input-error {
        margin-top: 10px;
        min-height: 18px;
        color: #ff8c7d;
        font-size: 13px;
      }
      @keyframes sweep {
        0% { transform: translateX(-140px); }
        100% { transform: translateX(560px); }
      }
      @media (max-width: 760px) {
        .wrap { padding: 12px; }
        .url-input-row { grid-template-columns: 1fr; }
        .stats-grid { grid-template-columns: repeat(2, minmax(120px, 1fr)); }
      }
    </style>
  </head>
  <body>
    <main class="wrap">
      <section class="panel" aria-live="polite">
        <div class="input-section" id="inputSection">
          <h1>Collection Mode</h1>
          <div class="meta">Enter a Moxfield collection URL to start background download. Fresh cache max age: ${COLLECTION_CACHE_MAX_AGE_DAYS} day(s).</div>
          <form class="url-form" id="urlForm">
            <label class="url-label" for="collectionInput">Collection URL</label>
            <div class="url-input-row">
              <input
                class="url-input"
                id="collectionInput"
                type="url"
                required
                placeholder="https://moxfield.com/collection/cpfxIAEPH0aGHI-3r9F_xg"
                value=${JSON.stringify(initialCollectionUrl)}
              />
              <button class="start-btn" id="startBtn" type="submit">Start Download</button>
            </div>
          </form>
          <div class="input-error" id="inputError"></div>
        </div>

        <div class="progress-section" id="progressSection">
          <h1>Downloading Collection JSON</h1>
          <div class="meta" id="collectionUrl"></div>
          <div class="stat-row">
            <div class="status" id="statusText">Waiting for first response...</div>
            <div class="percent" id="percentText">0%</div>
          </div>
          <div class="track" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0">
            <div class="fill" id="fillBar"></div>
          </div>
          <div class="footer">
            <div class="kv" id="pageText">Page 0 / 0</div>
            <div class="kv" id="etaText">ETA --:--</div>
          </div>
          <div class="stats-grid">
            <div class="stat-card">
              <div class="stat-label">Total Cards</div>
              <div class="stat-value" id="totalResults">-</div>
            </div>
            <div class="stat-card">
              <div class="stat-label">Common</div>
              <div class="stat-value" id="totalCommon">-</div>
            </div>
            <div class="stat-card">
              <div class="stat-label">Uncommon</div>
              <div class="stat-value" id="totalUncommon">-</div>
            </div>
            <div class="stat-card">
              <div class="stat-label">Rare</div>
              <div class="stat-value" id="totalRare">-</div>
            </div>
            <div class="stat-card">
              <div class="stat-label">Mythic</div>
              <div class="stat-value" id="totalMythic">-</div>
            </div>
          </div>
        </div>
      </section>
    </main>
    <script>
      const { ipcRenderer } = require('electron');
      const inputSection = document.getElementById('inputSection');
      const progressSection = document.getElementById('progressSection');
      const urlForm = document.getElementById('urlForm');
      const collectionInput = document.getElementById('collectionInput');
      const startBtn = document.getElementById('startBtn');
      const inputError = document.getElementById('inputError');
      const statusText = document.getElementById('statusText');
      const percentText = document.getElementById('percentText');
      const fillBar = document.getElementById('fillBar');
      const pageText = document.getElementById('pageText');
      const etaText = document.getElementById('etaText');
      const trackEl = document.querySelector('.track');
      const collectionUrlEl = document.getElementById('collectionUrl');
      const totalResultsEl = document.getElementById('totalResults');
      const totalCommonEl = document.getElementById('totalCommon');
      const totalUncommonEl = document.getElementById('totalUncommon');
      const totalRareEl = document.getElementById('totalRare');
      const totalMythicEl = document.getElementById('totalMythic');
      collectionUrlEl.textContent = '';

      function formatInt(value) {
        if (typeof value !== 'number' || !Number.isFinite(value)) return '-';
        return value.toLocaleString();
      }

      function formatEta(seconds) {
        if (typeof seconds !== 'number' || !Number.isFinite(seconds) || seconds < 0) return '--:--';
        const rounded = Math.round(seconds);
        const mm = String(Math.floor(rounded / 60)).padStart(2, '0');
        const ss = String(rounded % 60).padStart(2, '0');
        return mm + ':' + ss;
      }

      urlForm.addEventListener('submit', async (event) => {
        event.preventDefault();
        const enteredUrl = (collectionInput.value || '').trim();
        inputError.textContent = '';
        startBtn.disabled = true;
        try {
          const result = await ipcRenderer.invoke('start-collection-download', enteredUrl);
          if (!result || !result.ok) {
            inputError.textContent = (result && result.error) ? result.error : 'Unable to start collection mode.';
            return;
          }
          collectionUrlEl.textContent = result.collectionUrl;
          inputSection.style.display = 'none';
          progressSection.style.display = 'block';
        } finally {
          startBtn.disabled = false;
        }
      });

      if (collectionInput.value) {
        collectionInput.select();
      }

      ipcRenderer.on('collection-progress', (_event, progress) => {
        const ratio = typeof progress.ratio === 'number' ? progress.ratio : 0;
        const percent = Math.max(0, Math.min(100, Math.round(ratio * 100)));
        const currentPage = progress.currentPage || 0;
        const totalPages = progress.totalPages || 0;

        percentText.textContent = percent + '%';
        fillBar.style.width = percent + '%';
        trackEl.setAttribute('aria-valuenow', String(percent));
        pageText.textContent = 'Page ' + currentPage + ' / ' + totalPages;
        statusText.textContent = progress.statusText || 'Working...';
        etaText.textContent = 'ETA ' + formatEta(progress.etaSeconds);

        statusText.classList.remove('done', 'error');
        if (progress.status === 'done') statusText.classList.add('done');
        if (progress.status === 'error') statusText.classList.add('error');

        const totals = progress.collectionTotals || {};
        totalResultsEl.textContent = formatInt(totals.totalResults);
        totalCommonEl.textContent = formatInt(totals.totalCommon);
        totalUncommonEl.textContent = formatInt(totals.totalUncommon);
        totalRareEl.textContent = formatInt(totals.totalRare);
        totalMythicEl.textContent = formatInt(totals.totalMythic);
      });
    </script>
  </body>
</html>`;

  progressWin.loadURL(`data:text/html;charset=UTF-8,${encodeURIComponent(html)}`);
  return progressWin;
}

function getValidCollectionUrlFromClipboard() {
  const text = (clipboard.readText() || '').trim();
  if (!text) return null;
  return isValidCollectionUrl(text) ? text : null;
}

function buildDefaultCollectionSearchUrl(collectionId, pageNumber, pageSize = DEFAULT_COLLECTION_PAGE_SIZE) {
  const u = new URL(`https://api2.moxfield.com/v1/collections/search/${collectionId}`);
  u.searchParams.set('sortType', 'cardName');
  u.searchParams.set('sortDirection', 'ascending');
  u.searchParams.set('pageNumber', String(pageNumber));
  u.searchParams.set('pageSize', String(pageSize));
  u.searchParams.set('playStyle', 'paperDollars');
  u.searchParams.set('pricingProvider', 'cardkingdom');
  return u.toString();
}

function buildCollectionSearchUrlFromTemplate(templateUrl, pageNumber, pageSize) {
  const u = new URL(templateUrl);
  u.searchParams.set('pageNumber', String(pageNumber));
  u.searchParams.set('pageSize', String(pageSize));
  return u.toString();
}

function parsePositiveInt(value, fallback) {
  const parsed = Number.parseInt(String(value), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function parseNonNegativeInt(value, fallback = 0) {
  const parsed = Number.parseInt(String(value), 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}

function extractCollectionTotals(collectionJson) {
  const source = collectionJson && typeof collectionJson.totals === 'object'
    ? collectionJson.totals
    : collectionJson;
  return {
    totalResults: parseNonNegativeInt(source && source.totalResults, 0),
    totalCommon: parseNonNegativeInt(source && source.totalCommon, 0),
    totalUncommon: parseNonNegativeInt(source && source.totalUncommon, 0),
    totalRare: parseNonNegativeInt(source && source.totalRare, 0),
    totalMythic: parseNonNegativeInt(source && source.totalMythic, 0)
  };
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function updateCollectionProgressUI(progress) {
  const ratio = typeof progress.ratio === 'number' ? progress.ratio : 0;
  if (collectionProgressWindow && !collectionProgressWindow.isDestroyed()) {
    collectionProgressWindow.webContents.send('collection-progress', progress);
    collectionProgressWindow.setProgressBar(progress.status === 'done' ? -1 : ratio);
  }
}

function logCollectionProgress(currentPage, totalPages, statusText = 'Downloading pages...', status = 'running', extra = {}) {
  const width = 24;
  const ratio = totalPages > 0 ? Math.min(currentPage / totalPages, 1) : 0;
  const filled = Math.round(width * ratio);
  const bar = `${'#'.repeat(filled)}${'-'.repeat(width - filled)}`;
  console.log(`[collection] Progress [${bar}] ${currentPage}/${totalPages}`);
  updateCollectionProgressUI({ ratio, currentPage, totalPages, statusText, status, ...extra });
}

async function fetchCollectionPage(win, urlString) {
  const script = `
    (async () => {
      const response = await fetch(${JSON.stringify(urlString)}, {
        method: 'GET',
        credentials: 'include'
      });
      const body = await response.text();
      return {
        ok: response.ok,
        status: response.status,
        url: response.url,
        body
      };
    })();
  `;
  return win.webContents.executeJavaScript(script, true);
}

function pluralizePages(count) {
  return count === 1 ? 'page' : 'pages';
}

function computeEtaSeconds({ startedAt, fetchedCount, currentPage, totalPages }) {
  const remainingPages = Math.max(0, totalPages - currentPage);
  if (remainingPages === 0) return 0;

  const fetchBudgetRemaining = Math.max(0, COLLECTION_GUARDRAIL_MAX_PAGES - fetchedCount);
  const possibleFetchesRemaining = Math.min(remainingPages, fetchBudgetRemaining);
  if (possibleFetchesRemaining === 0) return 0;

  if (fetchedCount === 0) {
    return Math.round((possibleFetchesRemaining * COLLECTION_PAGE_DELAY_MS) / 1000);
  }

  const elapsedSeconds = Math.max(1, Math.round((Date.now() - startedAt) / 1000));
  const secondsPerFetch = Math.max(COLLECTION_PAGE_DELAY_MS / 1000, elapsedSeconds / fetchedCount);
  return Math.round(possibleFetchesRemaining * secondsPerFetch);
}

async function runCollectionPagination({ win, collectionId }) {
  if (!win || win.isDestroyed()) return;

  try {
    const startedAt = Date.now();
    const templateUrl = buildDefaultCollectionSearchUrl(collectionId, 1, DEFAULT_COLLECTION_PAGE_SIZE);
    let fetchedPages = 0;
    let skippedPages = 0;
    let guardrailStopped = false;

    async function getPagePayload(pageNumber) {
      const pageUrl = buildCollectionSearchUrlFromTemplate(templateUrl, pageNumber, DEFAULT_COLLECTION_PAGE_SIZE);
      const cached = await readFreshCachedResponse(pageUrl, 'GET');
      if (cached) {
        skippedPages += 1;
        console.log(`[collection] Skipping cached page ${pageNumber}: ${cached.cachePath}`);
        return { ...cached, pageNumber };
      }

      if (fetchedPages >= COLLECTION_GUARDRAIL_MAX_PAGES) {
        guardrailStopped = true;
        return null;
      }

      if (fetchedPages > 0) {
        await sleep(COLLECTION_PAGE_DELAY_MS);
      }

      const response = await fetchCollectionPage(win, pageUrl);
      if (!response || typeof response.body !== 'string') {
        throw new Error(`Failed to fetch page ${pageNumber}.`);
      }
      fetchedPages += 1;

      await saveResponseToDisk({
        url: response.url || pageUrl,
        method: 'GET',
        body: response.body
      });

      return {
        url: response.url || pageUrl,
        method: 'GET',
        body: response.body,
        fromCache: false,
        pageNumber
      };
    }

    updateCollectionProgressUI({
      ratio: 0,
      currentPage: 0,
      totalPages: 0,
      statusText: 'Loading collection page in background...',
      status: 'loading',
      etaSeconds: null,
      collectionTotals: null
    });

    const firstPayload = await getPagePayload(1);
    if (!firstPayload) {
      throw new Error(`Guardrail reached before page 1 could be fetched.`);
    }
    const firstUrl = new URL(firstPayload.url);
    const firstPage = parsePositiveInt(firstUrl.searchParams.get('pageNumber'), 1);

    let firstJson;
    try {
      firstJson = JSON.parse(firstPayload.body);
    } catch {
      throw new Error('Initial collection response was not valid JSON.');
    }

    const reportedTotalPages = parsePositiveInt(firstJson.totalPages, firstPage);
    const collectionTotals = extractCollectionTotals(firstJson);
    const initialEtaSeconds = computeEtaSeconds({
      startedAt,
      fetchedCount: fetchedPages,
      currentPage: firstPage,
      totalPages: reportedTotalPages
    });

    console.log(`[collection] totalPages=${reportedTotalPages}, guardrailCap=${COLLECTION_GUARDRAIL_MAX_PAGES}, cacheMaxAgeDays=${COLLECTION_CACHE_MAX_AGE_DAYS}.`);
    logCollectionProgress(
      Math.min(firstPage, reportedTotalPages),
      reportedTotalPages,
      `Fetched ${fetchedPages} ${pluralizePages(fetchedPages)} | Skipped ${skippedPages} cached`,
      'running',
      {
        etaSeconds: initialEtaSeconds,
        collectionTotals
      }
    );

    for (let pageNumber = firstPage + 1; pageNumber <= reportedTotalPages; pageNumber += 1) {
      const payload = await getPagePayload(pageNumber);
      if (!payload && guardrailStopped) {
        console.warn(`[collection] Guardrail stopped pagination after ${fetchedPages} fetched pages.`);
        break;
      }

      const etaSeconds = computeEtaSeconds({
        startedAt,
        fetchedCount: fetchedPages,
        currentPage: pageNumber,
        totalPages: reportedTotalPages
      });
      logCollectionProgress(pageNumber, reportedTotalPages, `Fetched ${fetchedPages} ${pluralizePages(fetchedPages)} | Skipped ${skippedPages} cached`, 'running', {
        etaSeconds,
        collectionTotals
      });
    }

    updateCollectionProgressUI({
      ratio: reportedTotalPages > 0 ? Math.min((fetchedPages + skippedPages) / reportedTotalPages, 1) : 0,
      currentPage: Math.min(fetchedPages + skippedPages, reportedTotalPages),
      totalPages: reportedTotalPages,
      statusText: guardrailStopped
        ? `Stopped after ${fetchedPages} fetched pages (guardrail). Skipped ${skippedPages} cached.`
        : `Complete. Fetched ${fetchedPages} ${pluralizePages(fetchedPages)}, skipped ${skippedPages} cached.`,
      status: 'done',
      etaSeconds: 0,
      collectionTotals
    });
  } catch (err) {
    console.error('[collection] Pagination failed:', err);
    updateCollectionProgressUI({
      ratio: 0,
      currentPage: 0,
      totalPages: 0,
      statusText: `Error: ${err.message}`,
      status: 'error',
      etaSeconds: null,
      collectionTotals: null
    });
  } finally {
    if (collectionWorkerWindow && !collectionWorkerWindow.isDestroyed()) {
      collectionWorkerWindow.close();
      collectionWorkerWindow = null;
    }
  }
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
  const clipboardCollectionUrl = getValidCollectionUrlFromClipboard() || '';

  // IPC handler to receive responses from preload/page
  ipcMain.on('save-api-response', (event, payload) => {
    // payload: { url, method, body }
    saveResponseToDisk(payload);
  });

  function teardownCollectionWorker() {
    if (collectionWorkerWindow && !collectionWorkerWindow.isDestroyed()) {
      collectionWorkerWindow.close();
    }
    collectionWorkerWindow = null;
  }

  function openCollectionWindow(initialUrlValue = '') {
    mainWindow = createCollectionProgressWindow(initialUrlValue);
    collectionProgressWindow = mainWindow;
    collectionProgressWindow.on('closed', () => {
      collectionProgressWindow = null;
      mainWindow = null;
      teardownCollectionWorker();
    });
  }

  openCollectionWindow(clipboardCollectionUrl);

  ipcMain.handle('start-collection-download', async (_event, enteredUrl) => {
    const trimmedUrl = (enteredUrl || '').trim();
    if (!isValidCollectionUrl(trimmedUrl)) {
      return {
        ok: false,
        error: 'Please enter a valid Moxfield collection URL (https://moxfield.com/collection/<id>).'
      };
    }

    if (collectionWorkerWindow && !collectionWorkerWindow.isDestroyed()) {
      return {
        ok: false,
        error: 'A collection download is already in progress.'
      };
    }

    const collectionId = extractCollectionIdFromUrl(trimmedUrl);
    if (!collectionId) {
      return {
        ok: false,
        error: 'Unable to read collection ID from URL.'
      };
    }

    collectionWorkerWindow = createCollectionWorkerWindow(trimmedUrl);
    collectionWorkerWindow.webContents.once('did-finish-load', () => {
      runCollectionPagination({
        win: collectionWorkerWindow,
        collectionId
      });
    });

    return { ok: true, collectionUrl: trimmedUrl };
  });

  app.on('activate', function () {
    if (BrowserWindow.getAllWindows().length === 0) {
      openCollectionWindow('');
    }
  });
});

app.on('window-all-closed', function () {
  if (process.platform !== 'darwin') app.quit();
});
