// Package output handles output-path resolution and safe JSON persistence.
//
// Responsibilities:
//   - normalize caller output paths (directory vs file semantics),
//   - enforce freshness guardrails unless force mode is enabled,
//   - write payloads atomically to avoid partial file corruption.
//
// File layout:
//   - output.go: path resolution, freshness checks, atomic file writes.
//   - output_test.go: path/freshness/write behavior tests.
//
// Failure modes:
//   - metadata lookup and write failures are returned directly,
//   - freshness violations return actionable errors for callers and CLI output.
package output
