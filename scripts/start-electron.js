const { spawn } = require('child_process');
const path = require('path');

let electronPath;
try {
  electronPath = require('electron');
} catch (err) {
  console.error(`[start] Unable to resolve electron: ${err.message}`);
  process.exit(1);
}

const env = { ...process.env };
delete env.ELECTRON_RUN_AS_NODE;

const extraArgs = process.argv.slice(2);
const desktopEntry = path.resolve(__dirname, '..', 'apps', 'desktop', 'main.js');
const child = spawn(electronPath, [desktopEntry, ...extraArgs], {
  stdio: 'inherit',
  env,
  windowsHide: false
});

child.on('exit', (code, signal) => {
  if (code === null) {
    console.error(`[start] Electron exited with signal ${signal}`);
    process.exit(1);
  }
  process.exit(code);
});
