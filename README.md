# moxfield-collection-downloader

Moxfield collection downloader with two interfaces:

- Desktop Electron app (`apps/desktop`)
- Headless runner for CI/Docker (`apps/headless`)

Shared helpers are in `packages/core`.

## Desktop

```bash
npm install
npm start
```

## Headless (local Node)

```bash
node apps/headless/run.js --id "cpfxIAEPH0aGHI-3r9F_xg"
```

or

```bash
npm run headless -- --id "cpfxIAEPH0aGHI-3r9F_xg"
```

## Headless (Docker)

```bash
make headless-build
make headless-run ID="cpfxIAEPH0aGHI-3r9F_xg"
```

Dockerfile location is fixed at:

- `build/package/Dockerfile`
