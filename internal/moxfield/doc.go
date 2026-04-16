// Package moxfield centralizes collection URL and identifier normalization rules.
//
// Responsibilities:
//   - build canonical collection URLs from IDs,
//   - validate supported URL and ID formats,
//   - extract collection IDs from validated URLs.
//
// File layout:
//   - moxfield.go: public parsing and normalization helpers.
//   - regex.go: compiled validation expressions shared by helpers.
//   - moxfield_test.go: URL/ID validation and extraction tests.
//
// Non-obvious decisions:
//   - validation intentionally allows both moxfield.com and www.moxfield.com
//     hosts while enforcing HTTPS-only URLs.
package moxfield
