# moxfield-collection-downloader

A headless Go CLI that downloads a Moxfield collection as JSON for local use or CI.
It uses Playwright for session-aware requests and applies guarded pagination with integrity checks.

## Quickstart

```bash
mcd --id "cpfxIAEPH0aGHI-3r9F_xg"

# or

docker run --rm jnovack/mcd:2 --id "cpfxIAEPH0aGHI-3r9F_xg"
```

## Configuration

| Purpose | Flag(s) | Environment Variable(s) | Default | Notes |
| --- | --- | --- | --- | --- |
| Collection ID | `--id <collectionId>` | `MCD_COLLECTION_ID`, `MCD_ID` | none | Use either `--id` or `--url`. |
| Collection URL | `--url <collectionUrl>` | `MCD_COLLECTION_URL`, `MCD_URL` | none | Must be `https://moxfield.com/collection/<id>`. |
| Output path | `--output <path>` | `MCD_OUTPUT` | `./collection.json` | Directory values resolve to `collection.json`. |
| Timeout (seconds) | `--timeout <seconds>` | `MCD_TIMEOUT` | `10` | Used as the base timeout for retries/backoff. |
| Log level | `--log-level <none\|trace\|debug\|info\|warn\|error>` | `MCD_LOG_LEVEL` | `info` | Use `none` to suppress all logs. |
| Force overwrite | `--force` | `MCD_FORCE` | `false` | Bypasses freshness guard. (Don't do this...) |
| Version only | `--version` | n/a | n/a | Prints version/build metadata and exits. |

## Behavior

All good netizens need to be nice to APIs so we've implemented some guardrails:

1. If output already exists and is newer than 72 hours, by default, retrieval is blocked.
2. Retrieval starts with single-shot `pageSize=totalResults` and clamps only if `totalResults > 20000`.
3. On timeout/block/mismatch, page size backs off: `20000 -> 10000 -> 5000 -> 1000 -> 500 -> 100`.
4. Pages are reassembled, deduplicated, and validated with `totalResults == len(data)`.

## Library Usage

Programmatic consumers can use `pkg/mcd` directly without spawning `mcd`.

### Fetch only

```go
result, err := mcd.Retrieve(ctx, mcd.RetrieveOptions{
   CollectionID: "cpfxIAEPH0aGHI-3r9F_xg",
   Timeout:      10 * time.Second,
})
if err != nil {
 return err
}
fmt.Println(result.CollectionID, len(result.Payload))
```

### Fetch and write JSON

```go
result, err := mcd.Run(ctx, mcd.RunOptions{
    RetrieveOptions: mcd.RetrieveOptions{
        CollectionID: "cpfxIAEPH0aGHI-3r9F_xg",
        Timeout:      10 * time.Second,
    },
    OutputPath: "./collection.json",
    Force:      false,
})
if err != nil {
    return err
}
fmt.Println(result.OutputPath, result.Stats.Requests)
```

### Validation-only input normalization

```go
resolved, err := mcd.ResolveInput("", "https://moxfield.com/collection/cpfxIAEPH0aGHI-3r9F_xg")
if err != nil {
    return err
}
fmt.Println(resolved.CollectionID, resolved.CollectionURL)
```

Package API behavior:

- `mcd.Retrieve` resolves/validates input and fetches payload only.
- `mcd.Run` also enforces freshness + writes output.

CLI-only behavior:

- Flag/env parsing and process exit codes are handled only by `cmd/mcd`.

## Disclaimer

This project is not affiliated with, endorsed by, or sponsored by Moxfield.  But I do hope they secretly like this project.
