// Package main provides the mcd CLI entrypoint and runtime wiring.
//
// Responsibilities:
//   - resolve flags and environment defaults into one run config,
//   - initialize zerolog formatting and level behavior,
//   - emit build metadata at startup,
//   - invoke pkg/mcd for retrieval and output handling.
//
// File layout:
//   - main.go: flag parsing, logger adapter, process exit behavior.
//   - doc.go: package contract and boot sequence summary.
//
// Failure modes:
//   - invalid input/flags terminate with non-zero status,
//   - retrieval/output failures are surfaced as structured log errors.
package main
