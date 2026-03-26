# Architecture

This repository has two runtime interfaces:

- `apps/desktop`: Electron desktop app (interactive workflow).
- `apps/headless`: headless one-shot runner (Docker/CI workflow).

Shared, runtime-neutral helpers live in:

- `packages/core`: URL and filename/path utilities used by both interfaces.

## Boundary Rules

- Desktop-specific logic stays in `apps/desktop` (Electron main/preload, window orchestration, IPC).
- Headless-specific logic stays in `apps/headless` (CLI parsing, Playwright runtime, one-shot export).
- Shared behavior should be extracted only when stable and used by both interfaces.

## Packaging and Release

- Canonical Dockerfile path: `build/package/Dockerfile`.
- Local build: `make headless-build` or `npm run headless:docker:build`.
- GitHub Actions Docker publish workflow: `.github/workflows/docker-image.yml`.
