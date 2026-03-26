'use strict';

const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');
const { sanitizeOutputFileName } = require('../../packages/core/files');
const {
  buildCollectionUrlFromId,
  isValidCollectionUrl,
  extractCollectionIdFromUrl
} = require('../../packages/core/moxfield');

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];

    if (token === '-q') {
      args.quiet = true;
      continue;
    }

    if (!token.startsWith('--')) continue;
    const key = token.slice(2);
    const next = argv[i + 1];
    if (!next || next.startsWith('-')) {
      args[key] = true;
      continue;
    }
    args[key] = next;
    i += 1;
  }
  return args;
}

function parseBoolean(value, fallback = false) {
  if (value === undefined || value === null || value === '') return fallback;
  const normalized = String(value).trim().toLowerCase();
  if (['1', 'true', 'yes', 'y', 'on'].includes(normalized)) return true;
  if (['0', 'false', 'no', 'n', 'off'].includes(normalized)) return false;
  return fallback;
}

function parsePositiveInt(value, fallback) {
  const parsed = Number.parseInt(String(value), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function formatAge(ageMs) {
  const ageHours = ageMs / (60 * 60 * 1000);
  if (ageHours < 24) return `${ageHours.toFixed(1)}h`;
  return `${(ageHours / 24).toFixed(1)}d`;
}

function resolveConfig(args, env) {
  const quiet = args.quiet === true
    ? true
    : parseBoolean(env.MCD_QUIET, false);
  const force = args.force === true
    ? true
    : parseBoolean(env.MCD_FORCE, false);

  const cliId = args.id ? String(args.id).trim() : '';
  const envId = env.MCD_COLLECTION_ID
    ? String(env.MCD_COLLECTION_ID).trim()
    : (env.MCD_ID ? String(env.MCD_ID).trim() : '');

  const cliUrl = args.url ? String(args.url).trim() : '';
  const envUrl = env.MCD_COLLECTION_URL
    ? String(env.MCD_COLLECTION_URL).trim()
    : (env.MCD_URL ? String(env.MCD_URL).trim() : '');

  const id = cliId || envId;
  const url = cliUrl || envUrl;

  const timeoutSec = parsePositiveInt(
    args.timeout !== undefined
      ? args.timeout
      : env.MCD_TIMEOUT,
    60
  );

  return {
    quiet,
    force,
    id,
    url,
    timeoutSec,
    outputDir: path.resolve(process.cwd(), 'output')
  };
}

function createLogger(quiet) {
  function log(line) {
    if (!quiet) console.log(line);
  }

  function warn(line) {
    if (!quiet) console.warn(line);
  }

  function error(line) {
    if (!quiet) console.error(line);
  }

  function fail(message, code = 1) {
    error(`[ERROR] ${message}`);
    process.exit(code);
  }

  return { log, warn, error, fail };
}

function normalizeBuildValue(value, fallback) {
  const raw = value === undefined || value === null ? '' : String(value).trim();
  if (!raw) return fallback;
  const unquoted = raw.replace(/^"+|"+$/g, '');
  return unquoted || fallback;
}

function resolveBuildInfo(env) {
  const version = normalizeBuildValue(env.MCD_BUILD_VERSION, 'dev');
  const revision = normalizeBuildValue(env.MCD_BUILD_REVISION, 'unknown');
  const buildTimestamp = normalizeBuildValue(env.MCD_BUILD_RFC3339, 'unknown');
  const shortRevision = revision === 'unknown' ? 'unknown' : revision.slice(0, 7);
  return { version, shortRevision, buildTimestamp };
}

async function enforceOutputFreshnessGuard({ outPath, force, warn, fail }) {
  const FRESHNESS_WINDOW_MS = 3 * 24 * 60 * 60 * 1000;
  let stat;
  try {
    stat = await fs.promises.stat(outPath);
  } catch (err) {
    if (err && err.code === 'ENOENT') return;
    fail(`Unable to read existing output file metadata: ${err.message || err}`);
  }

  const ageMs = Date.now() - stat.mtimeMs;
  if (ageMs < FRESHNESS_WINDOW_MS) {
    const ageText = formatAge(ageMs);
    const msg = `Output file is too recent (${ageText} old): ${outPath}. Use --force or MCD_FORCE=1 to override.`;
    if (force) {
      warn(`[WARNG] ${msg} Proceeding because force is enabled.`);
      return;
    }
    fail(msg);
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const config = resolveConfig(args, process.env);
  const buildInfo = resolveBuildInfo(process.env);
  const logger = createLogger(config.quiet);
  const { log, warn, fail } = logger;

  if (!config.id && !config.url) {
    fail('Missing input. Provide either --id <collectionId> or --url <collectionUrl> (or env vars).');
  }

  log(`[INFO ] Starting mcd ${buildInfo.version} (${buildInfo.shortRevision}) built ${buildInfo.buildTimestamp}`);

  await fs.promises.mkdir(config.outputDir, { recursive: true });
  const outFile = sanitizeOutputFileName('collection.json');
  const outPath = path.join(config.outputDir, outFile);
  await enforceOutputFreshnessGuard({ outPath, force: config.force, warn, fail });

  log(`[INFO ] Quiet mode: ${config.quiet ? 'ON' : 'OFF'}`);
  log(`[INFO ] Force mode: ${config.force ? 'ON' : 'OFF'}`);
  log(`[INFO ] Timeout: ${config.timeoutSec}s`);
  log(`[INFO ] Output directory: ${config.outputDir}`);

  if (config.id && config.url) {
    warn('[WARNG] Both id and url were supplied. Preferring collection ID and ignoring URL.');
  }

  let collectionUrl = '';
  let collectionId = '';

  if (config.id) {
    collectionId = String(config.id).trim();
    collectionUrl = buildCollectionUrlFromId(collectionId);
  } else {
    collectionUrl = String(config.url).trim();
    if (!isValidCollectionUrl(collectionUrl)) {
      fail('Invalid collection URL. Expected https://moxfield.com/collection/<id>');
    }
    collectionId = extractCollectionIdFromUrl(collectionUrl);
  }

  if (!collectionId) {
    fail('Unable to resolve collection ID.');
  }
  if (!isValidCollectionUrl(collectionUrl)) {
    fail(`Resolved collection URL is invalid: ${collectionUrl}`);
  }

  const timeoutMs = config.timeoutSec * 1000;
  log(`[INFO ] Collection ID: ${collectionId}`);
  log(`[INFO ] Collection URL: ${collectionUrl}`);

  log('[INFO ] Launching Chromium...');
  const browser = await chromium.launch({
    headless: true,
    args: [
      '--disable-blink-features=AutomationControlled',
      '--disable-dev-shm-usage',
      '--no-sandbox'
    ]
  });
  log('[INFO ] Chromium launched.');

  try {
    log('[INFO ] Creating browser context...');
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36',
      locale: 'en-US'
    });
    const page = await context.newPage();
    log('[INFO ] Browser context/page ready.');

    log('[INFO ] Waiting for first collection API response (pageNumber=1)...');
    const firstResponsePromise = page.waitForResponse((response) => {
      try {
        const u = new URL(response.url());
        return (
          u.hostname === 'api2.moxfield.com'
          && u.pathname === `/v1/collections/search/${collectionId}`
          && u.searchParams.get('pageNumber') === '1'
        );
      } catch {
        return false;
      }
    }, { timeout: timeoutMs });

    log(`[INFO ] Navigating to collection page: ${collectionUrl}`);
    await page.goto(collectionUrl, { waitUntil: 'domcontentloaded', timeout: timeoutMs });
    log('[INFO ] Navigation completed (domcontentloaded).');

    const firstResponse = await firstResponsePromise;
    const firstResponseUrl = firstResponse.url();
    log(`[INFO ] Intercepted first API response: ${firstResponseUrl}`);

    const firstResponseText = await firstResponse.text();
    log(`[INFO ] First response size: ${firstResponseText.length} bytes`);

    let firstJson;
    try {
      firstJson = JSON.parse(firstResponseText);
    } catch {
      fail('First collection API response was not valid JSON.');
    }

    const totalResults = parsePositiveInt(
      firstJson.totalResults || (firstJson.totals && firstJson.totals.totalResults),
      0
    );
    if (!totalResults) {
      fail('Could not read totalResults from first response.');
    }
    log(`[INFO ] Parsed totalResults=${totalResults}`);

    const fullUrl = new URL(firstResponseUrl);
    fullUrl.searchParams.set('pageNumber', '1');
    fullUrl.searchParams.set('pageSize', String(totalResults));
    log(`[INFO ] Single-shot URL: ${fullUrl.toString()}`);

    log('[INFO ] Fetching single-shot payload using in-page fetch (with cookies/session)...');
    const singlePageResult = await page.evaluate(async (url) => {
      const response = await fetch(url, {
        method: 'GET',
        credentials: 'include'
      });
      const text = await response.text();
      return {
        ok: response.ok,
        status: response.status,
        statusText: response.statusText,
        url: response.url,
        text
      };
    }, fullUrl.toString());

    if (!singlePageResult || !singlePageResult.ok) {
      const status = singlePageResult ? `${singlePageResult.status} ${singlePageResult.statusText}` : 'unknown';
      fail(`Single-shot fetch failed with status ${status}.`);
    }

    log(`[INFO ] Single-shot fetch succeeded: ${singlePageResult.status} ${singlePageResult.statusText}`);
    log(`[INFO ] Final payload size: ${singlePageResult.text.length} bytes`);

    let singlePageJson;
    try {
      singlePageJson = JSON.parse(singlePageResult.text);
    } catch {
      fail('Single-shot response was not valid JSON.');
    }

    const resultCount = Array.isArray(singlePageJson.data) ? singlePageJson.data.length : 0;
    log(`[INFO ] Parsed final JSON. data.length=${resultCount}`);

    await fs.promises.writeFile(outPath, JSON.stringify(singlePageJson, null, 2), 'utf8');
    log(`[INFO ] Saved JSON to ${outPath}`);
    log('[INFO ] Completed successfully.');
  } finally {
    log('[INFO ] Closing browser...');
    await browser.close();
    log('[INFO ] Browser closed.');
  }
}

main().catch((err) => {
  const args = parseArgs(process.argv.slice(2));
  const quiet = args.quiet === true
    ? true
    : parseBoolean(process.env.MCD_QUIET, false);
  const logger = createLogger(quiet);
  if (!quiet && err && err.stack) {
    logger.error(`[ERROR] Unhandled error stack:\\n${err.stack}`);
  }
  logger.fail(err && err.message ? err.message : String(err));
});
