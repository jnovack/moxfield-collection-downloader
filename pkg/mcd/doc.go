// Package mcd exposes the public API for collection retrieval and file output.
//
// Responsibilities:
//   - validate and normalize collection identifiers/URLs,
//   - orchestrate retrieval through internal/downloader,
//   - apply freshness guardrails and atomic output writes.
//
// File layout:
//   - mcd.go: exported options/results plus orchestration entrypoints.
//   - mcd_test.go: package-level behavior and error-mapping tests.
//   - doc.go: public package contract.
//
// Non-obvious decisions:
//   - downloader errors are intentionally re-mapped to stable package sentinels
//     so callers can match behavior without depending on internal packages.
//
// Failure modes:
//   - invalid input returns ErrInvalidInput,
//   - integrity mismatch returns ErrPayloadMismatch,
//   - freshness and output failures return dedicated sentinels.
package mcd
