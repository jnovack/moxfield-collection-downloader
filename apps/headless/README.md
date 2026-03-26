# Headless One-Shot Downloader

This directory contains a Dockerized, headless downloader that:

1. Opens a collection URL in a real browser context (JS enabled).
2. Intercepts the first collection API response (`pageNumber=1`).
3. Reads `totalResults` from that response.
4. Re-fetches collection page 1 with `pageSize=totalResults` in a single request.
5. Saves the final JSON to `./output` (inside container: `/app/output`).

## Build

```bash
docker build -t mcd -f build/package/Dockerfile .
```

## Run

Using collection ID:

```bash
docker run --rm \
  -v "$PWD/apps/headless/output:/app/output" \
  mcd \
  --id "cpfxIAEPH0aGHI-3r9F_xg"
```

Using full URL:

```bash
docker run --rm \
  -v "$PWD/apps/headless/output:/app/output" \
  mcd \
  --url "https://moxfield.com/collection/cpfxIAEPH0aGHI-3r9F_xg"
```

## CLI Arguments

- `--id <collectionId>`: Collection ID only. Auto-builds URL as `https://moxfield.com/collection/<id>`.
- `--url <collectionUrl>`: Full collection URL.
- `--timeout <seconds>`: Timeout in seconds (optional, default `60`).
- `-q` or `--quiet`: Suppress all output.

### `--id` and `--url` together

If both are supplied, the tool prints a warning and **prefers `--id`**.

## Environment Variables

All runtime values can be provided through environment variables:

- `MCD_COLLECTION_ID` (or `MCD_ID`)
- `MCD_COLLECTION_URL` (or `MCD_URL`)
- `MCD_TIMEOUT` (seconds)
- `MCD_QUIET` (`1/true/yes/on` or `0/false/no/off`)

Command-line args always take precedence over environment variables.

Example:

```bash
docker run --rm \
  -e MCD_COLLECTION_ID="cpfxIAEPH0aGHI-3r9F_xg" \
  -e MCD_TIMEOUT=90 \
  -v "$PWD/apps/headless/output:/app/output" \
  mcd
```

## Disclaimer

MCD stands for **Moxfield Collection Downloader**.
This project is an independent tool and is not affiliated with, endorsed by, or sponsored by Moxfield.
All trademarks are the property of their respective owners.
