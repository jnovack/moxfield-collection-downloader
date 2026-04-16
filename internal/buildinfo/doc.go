// Package buildinfo resolves runtime-visible version metadata.
//
// Responsibilities:
//   - expose default metadata values for local/dev builds,
//   - populate missing version, revision, and build time fields from Go build info.
//
// File layout:
//   - buildinfo.go: defaults and Populate helper.
//   - doc.go: package contract and metadata source behavior.
//
// Non-obvious decisions:
//   - Populate only overwrites known default placeholders, preserving explicit
//     ldflags-injected values when present.
package buildinfo
