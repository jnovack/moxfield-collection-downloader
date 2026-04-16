// Package downloader retrieves collection payloads through a browser-backed API flow.
//
// Responsibilities:
//   - bootstrap API discovery from a collection page,
//   - fetch and merge paged API responses,
//   - apply timeout and page-size backoff strategies,
//   - enforce totalResults/data-length integrity before returning payloads.
//
// File layout:
//   - downloader.go: strategy orchestration, retry classification, merge logic.
//   - playwright.go: concrete BrowserClient implemented with Playwright.
//   - downloader_test.go: fallback/retry/integrity behavior tests.
//   - doc.go: package contract and failure semantics.
//
// Failure modes:
//   - non-retryable HTTP/API failures propagate immediately,
//   - retryable timeout/challenge failures back off until candidates are exhausted,
//   - integrity mismatch returns ErrIntegrityMismatch.
package downloader
