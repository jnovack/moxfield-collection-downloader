const fs = require('fs');
const path = require('path');

function fail(message) {
  console.error(`[check:electron] ${message}`);
  process.exit(1);
}

function warn(message) {
  console.warn(`[check:electron] ${message}`);
}

const rootDir = path.resolve(__dirname, '..');
const binElectronPath = path.join(rootDir, 'node_modules', '.bin', 'electron');
const packageCliPath = path.join(rootDir, 'node_modules', 'electron', 'cli.js');
const desktopEntryPath = path.join(rootDir, 'apps', 'desktop', 'main.js');

if (!fs.existsSync(packageCliPath)) {
  fail('Missing node_modules/electron/cli.js. Run npm install.');
}

if (!fs.existsSync(desktopEntryPath)) {
  fail('Missing apps/desktop/main.js desktop entrypoint.');
}

let electronBinaryPath;
try {
  electronBinaryPath = require('electron');
} catch (err) {
  fail(`Unable to require("electron"): ${err.message}`);
}

if (!electronBinaryPath || !fs.existsSync(electronBinaryPath)) {
  fail(`Electron binary not found at: ${electronBinaryPath || '(empty path)'}`);
}

if (process.env.ELECTRON_RUN_AS_NODE) {
  warn('ELECTRON_RUN_AS_NODE is set in this shell; start script will unset it before launch.');
}

if (!fs.existsSync(binElectronPath)) {
  fail('Missing node_modules/.bin/electron shim. Reinstall dependencies.');
}

const stat = fs.lstatSync(binElectronPath);
if (stat.isSymbolicLink()) {
  console.log('[check:electron] OK');
  process.exit(0);
}

const shimContents = fs.readFileSync(binElectronPath, 'utf8');
const pointsToPackageCli = shimContents.includes('../electron/cli.js') || shimContents.includes('node_modules/electron/cli.js');
if (!pointsToPackageCli) {
  warn('node_modules/.bin/electron does not point to electron/cli.js. start is safe because it bypasses .bin.');
  console.log('[check:electron] OK');
  process.exit(0);
}

warn('node_modules/.bin/electron is not a symlink, but wrapper content looks valid.');
console.log('[check:electron] OK');
