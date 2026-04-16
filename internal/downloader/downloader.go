package downloader

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ─── Retry and Backoff Parameters ──────────────────────────────────────────────

// fallbackPageSizes defines descending request-size fallback steps used after a failed attempt.
var fallbackPageSizes = []int{20000, 10000, 5000, 1000, 500, 100}

// timeoutMultipliers defines timeout backoff factors for retryable failures.
var timeoutMultipliers = []int{1, 2, 4}

// ErrIntegrityMismatch indicates that aggregated card count does not match totalResults.
var ErrIntegrityMismatch = errors.New("integrity mismatch")

// ─── Core Interfaces and Data Types ────────────────────────────────────────────

// Logger is the minimal logging contract used by downloader internals.
type Logger interface {
	Info(msg string, fields map[string]any)
	Warn(msg string, fields map[string]any)
	Debug(msg string, fields map[string]any)
	Trace(msg string, fields map[string]any)
}

// NopLogger is a no-op logger implementation for quiet/test paths.
type NopLogger struct{}

// Info ignores info logs.
func (NopLogger) Info(string, map[string]any) {}

// Warn ignores warning logs.
func (NopLogger) Warn(string, map[string]any) {}

// Debug ignores debug logs.
func (NopLogger) Debug(string, map[string]any) {}

// Trace ignores trace logs.
func (NopLogger) Trace(string, map[string]any) {}

// BrowserClient defines the browser operations required by the downloader.
type BrowserClient interface {
	Bootstrap(ctx context.Context, collectionURL, collectionID string, timeout time.Duration) (BootInfo, error)
	FetchPage(ctx context.Context, req FetchRequest) (FetchPageResult, error)
	Close() error
}

// BootInfo holds first-response bootstrap metadata for subsequent API fetches.
type BootInfo struct {
	TemplateURL  string
	TotalResults int
}

// FetchRequest describes one paginated API fetch attempt.
type FetchRequest struct {
	TemplateURL string
	PageNumber  int
	PageSize    int
	Timeout     time.Duration
}

// FetchPageResult carries HTTP-like details from an in-page fetch.
type FetchPageResult struct {
	StatusCode int
	StatusText string
	BodyText   string
}

// Options configures a full collection retrieval run.
type Options struct {
	CollectionID  string
	CollectionURL string
	BaseTimeout   time.Duration
	Logger        Logger
}

// Payload is the normalized collection JSON payload.
type Payload struct {
	TotalResults int              `json:"totalResults"`
	Data         []map[string]any `json:"data"`
	Raw          map[string]any   `json:"-"`
}

// RunStats contains counters summarizing retries/fallback behavior for a run.
type RunStats struct {
	Requests          int
	PagesFetched      int
	TimeoutBackoffs   int
	PageSizeBackoffs  int
	DuplicatesSkipped int
	FinalPageSize     int
}

// ─── Retrieval Orchestration ───────────────────────────────────────────────────

// Retrieve runs bootstrap, single-shot fetch, and adaptive fallback strategies.
func Retrieve(ctx context.Context, browser BrowserClient, opts Options) (map[string]any, RunStats, error) {
	stats := RunStats{}
	if opts.Logger == nil {
		opts.Logger = NopLogger{}
	}
	if opts.BaseTimeout <= 0 {
		opts.BaseTimeout = 60 * time.Second
	}

	timeouts := timeoutCandidates(opts.BaseTimeout)
	opts.Logger.Trace("timeout candidates built", map[string]any{
		"base_timeout": opts.BaseTimeout.String(),
		"candidates":   timeoutsToStrings(timeouts),
	})
	opts.Logger.Debug("bootstrap.start", map[string]any{
		"collection_id":  opts.CollectionID,
		"collection_url": opts.CollectionURL,
	})
	boot, err := bootstrapWithBackoff(ctx, browser, opts.CollectionURL, opts.CollectionID, timeouts, opts.Logger, &stats)
	if err != nil {
		return nil, stats, fmt.Errorf("bootstrap failed: %w", err)
	}
	if boot.TotalResults <= 0 {
		return nil, stats, fmt.Errorf("could not read totalResults from first response")
	}
	opts.Logger.Info("parsed total results", map[string]any{
		"total_results": boot.TotalResults,
	})

	initialPageSize := boot.TotalResults
	if initialPageSize > 20000 {
		opts.Logger.Warn("initial request size clamped", map[string]any{
			"requested_page_size": boot.TotalResults,
			"clamped_page_size":   20000,
		})
		initialPageSize = 20000
	}

	opts.Logger.Debug("bootstrap.source", map[string]any{
		"template_url":  boot.TemplateURL,
		"total_results": boot.TotalResults,
	})
	payload, err := fetchComplete(ctx, browser, boot.TemplateURL, boot.TotalResults, initialPageSize, timeouts, opts.Logger, &stats)
	if err == nil {
		stats.FinalPageSize = initialPageSize
		return payload.Raw, stats, nil
	}
	opts.Logger.Warn("initial fetch failed", map[string]any{
		"page_size": initialPageSize,
		"error":     err.Error(),
	})

	fallbacks := fallbackFrom(boot.TotalResults, initialPageSize)
	opts.Logger.Trace("fallback pagesizes derived", map[string]any{
		"total_results":      boot.TotalResults,
		"initial_page_size":  initialPageSize,
		"fallback_page_size": fallbacks,
	})
	prevPageSize := initialPageSize
	for _, pageSize := range fallbacks {
		stats.PageSizeBackoffs++
		opts.Logger.Warn("request size backoff", map[string]any{
			"previous_page_size": prevPageSize,
			"next_page_size":     pageSize,
			"reason":             "previous strategy failed",
		})
		payload, ferr := fetchComplete(ctx, browser, boot.TemplateURL, boot.TotalResults, pageSize, timeouts, opts.Logger, &stats)
		if ferr == nil {
			opts.Logger.Info("fallback succeeded", map[string]any{
				"page_size": pageSize,
			})
			stats.FinalPageSize = pageSize
			return payload.Raw, stats, nil
		}
		opts.Logger.Warn("fallback failed", map[string]any{
			"page_size": pageSize,
			"error":     ferr.Error(),
		})
		prevPageSize = pageSize
	}

	return nil, stats, fmt.Errorf("all strategies failed after initial pageSize=%d: %w", initialPageSize, err)
}

// bootstrapWithBackoff retries bootstrap using increasing timeouts for retryable failures.
func bootstrapWithBackoff(ctx context.Context, browser BrowserClient, collectionURL, collectionID string, timeouts []time.Duration, log Logger, stats *RunStats) (BootInfo, error) {
	var lastErr error
	for idx, timeout := range timeouts {
		log.Trace("bootstrap timeout attempt", map[string]any{
			"attempt_index": idx,
			"timeout":       timeout.String(),
		})
		boot, err := browser.Bootstrap(ctx, collectionURL, collectionID, timeout)
		if err == nil {
			return boot, nil
		}
		lastErr = classifyError(err)
		retryable, reason := retryDecision(lastErr)
		log.Trace("retry classification", map[string]any{
			"phase":     "bootstrap",
			"retryable": retryable,
			"reason":    reason,
			"error":     lastErr.Error(),
		})
		if retryable {
			stats.TimeoutBackoffs++
			nextTimeout := ""
			if idx+1 < len(timeouts) {
				nextTimeout = timeouts[idx+1].String()
			}
			log.Warn("bootstrap retry with increased timeout", map[string]any{
				"previous_timeout": timeout.String(),
				"next_timeout":     nextTimeout,
				"reason":           reason,
				"error":            lastErr.Error(),
			})
			continue
		}
		return BootInfo{}, lastErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("bootstrap failed")
	}
	return BootInfo{}, lastErr
}

// fetchComplete retries a full retrieval attempt for a fixed page size across timeout candidates.
func fetchComplete(ctx context.Context, browser BrowserClient, templateURL string, totalResults, pageSize int, timeouts []time.Duration, log Logger, stats *RunStats) (Payload, error) {
	if pageSize <= 0 {
		return Payload{}, fmt.Errorf("invalid pageSize: %d", pageSize)
	}

	var lastErr error
	for idx, timeout := range timeouts {
		log.Trace("fetch timeout attempt", map[string]any{
			"attempt_index": idx,
			"timeout":       timeout.String(),
			"page_size":     pageSize,
		})
		payload, err := fetchOnce(ctx, browser, templateURL, totalResults, pageSize, timeout, log, stats)
		if err == nil {
			return payload, nil
		}

		lastErr = err
		retryable, reason := retryDecision(err)
		log.Trace("retry classification", map[string]any{
			"phase":     "fetch",
			"retryable": retryable,
			"reason":    reason,
			"error":     err.Error(),
		})
		if retryable {
			stats.TimeoutBackoffs++
			nextTimeout := ""
			if idx+1 < len(timeouts) {
				nextTimeout = timeouts[idx+1].String()
			}
			log.Warn("retrying with increased timeout", map[string]any{
				"previous_timeout": timeout.String(),
				"next_timeout":     nextTimeout,
				"page_size":        pageSize,
				"reason":           reason,
				"error":            err.Error(),
			})
			continue
		}
		return Payload{}, err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("fetch failed")
	}
	return Payload{}, lastErr
}

// fetchOnce fetches and merges all pages for a chosen page size.
func fetchOnce(ctx context.Context, browser BrowserClient, templateURL string, totalResults, pageSize int, timeout time.Duration, log Logger, stats *RunStats) (Payload, error) {
	totalPages := int(math.Ceil(float64(totalResults) / float64(pageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	merged := make([]map[string]any, 0, totalResults)
	seen := make(map[string]struct{}, totalResults)
	var canonical map[string]any

	for page := 1; page <= totalPages; page++ {
		requestURL, urlErr := BuildPageURL(templateURL, page, pageSize)
		if urlErr == nil {
			log.Debug("request.send", map[string]any{
				"url":         requestURL,
				"page_number": page,
				"page_size":   pageSize,
				"timeout":     timeout.String(),
			})
		}
		result, err := browser.FetchPage(ctx, FetchRequest{
			TemplateURL: templateURL,
			PageNumber:  page,
			PageSize:    pageSize,
			Timeout:     timeout,
		})
		stats.Requests++
		if err != nil {
			return Payload{}, classifyError(err)
		}
		log.Debug("request.response", map[string]any{
			"url":          requestURL,
			"page_number":  page,
			"page_size":    pageSize,
			"status_code":  result.StatusCode,
			"status_text":  strings.TrimSpace(result.StatusText),
			"body_bytes":   len(result.BodyText),
			"total_pages":  totalPages,
			"total_result": totalResults,
		})
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			return Payload{}, classifyError(fmt.Errorf("status %d %s", result.StatusCode, strings.TrimSpace(result.StatusText)))
		}

		jsonBody, err := decodeBody(result.BodyText)
		if err != nil {
			return Payload{}, classifyError(err)
		}
		if canonical == nil {
			canonical = jsonBody
		}

		data, err := extractDataSlice(jsonBody)
		if err != nil {
			return Payload{}, err
		}
		duplicatesBefore := stats.DuplicatesSkipped

		for _, row := range data {
			k, keySource := dedupeKey(row)
			if _, ok := seen[k]; ok {
				stats.DuplicatesSkipped++
				log.Trace("merge.dedupe.skip", map[string]any{
					"page_number": page,
					"dedupe_key":  k,
					"key_source":  keySource,
				})
				continue
			}
			seen[k] = struct{}{}
			merged = append(merged, row)
		}
		stats.PagesFetched++
		log.Debug("merge.progress", map[string]any{
			"page_number":            page,
			"total_pages":            totalPages,
			"page_items":             len(data),
			"merged_unique_total":    len(merged),
			"duplicates_skipped":     stats.DuplicatesSkipped - duplicatesBefore,
			"duplicates_accumulated": stats.DuplicatesSkipped,
		})
	}

	log.Debug("integrity.check", map[string]any{
		"expected_total_results": totalResults,
		"actual_data_length":     len(merged),
		"page_size":              pageSize,
	})
	if len(merged) != totalResults {
		return Payload{}, fmt.Errorf("%w: totalResults=%d len(data)=%d", ErrIntegrityMismatch, totalResults, len(merged))
	}

	canonical["totalResults"] = totalResults
	canonical["data"] = merged

	return Payload{TotalResults: totalResults, Data: merged, Raw: canonical}, nil
}

// ─── Retry and Merge Helpers ───────────────────────────────────────────────────

// timeoutCandidates returns bounded timeout values based on the configured base timeout.
func timeoutCandidates(base time.Duration) []time.Duration {
	out := make([]time.Duration, 0, len(timeoutMultipliers))
	for _, mult := range timeoutMultipliers {
		candidate := time.Duration(mult) * base
		if candidate > 3*time.Minute {
			candidate = 3 * time.Minute
		}
		out = append(out, candidate)
	}
	return out
}

// floorPow10 returns the largest power of ten less than or equal to n.
func floorPow10(n int) int {
	if n <= 0 {
		return 0
	}
	value := 1
	for n >= 10 {
		n /= 10
		value *= 10
	}
	return value
}

// fallbackFrom builds the descending fallback page-size list after the initial attempt.
func fallbackFrom(totalResults, initialPageSize int) []int {
	start := floorPow10(totalResults)
	if start > 20000 {
		start = 20000
	}
	if start == 0 {
		start = 100
	}

	seen := map[int]struct{}{initialPageSize: {}}
	values := make([]int, 0, len(fallbackPageSizes))

	for _, pageSize := range fallbackPageSizes {
		if pageSize > start {
			continue
		}
		if pageSize >= initialPageSize {
			continue
		}
		if _, ok := seen[pageSize]; ok {
			continue
		}
		seen[pageSize] = struct{}{}
		values = append(values, pageSize)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(values)))
	return values
}

// decodeBody parses a JSON API payload and rejects Cloudflare challenge pages.
func decodeBody(body string) (map[string]any, error) {
	if isCloudflarePayload(body) {
		return nil, fmt.Errorf("cloudflare challenge detected")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, fmt.Errorf("response was not valid JSON: %w", err)
	}
	return parsed, nil
}

// extractDataSlice extracts and type-checks the payload data array.
func extractDataSlice(body map[string]any) ([]map[string]any, error) {
	raw, ok := body["data"]
	if !ok {
		return nil, fmt.Errorf("response missing data array")
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("response data was not an array")
	}
	out := make([]map[string]any, 0, len(arr))
	for _, row := range arr {
		obj, ok := row.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("response data row was not an object")
		}
		out = append(out, obj)
	}
	return out, nil
}

// dedupeKey returns a stable key for page-merge deduplication.
func dedupeKey(row map[string]any) (string, string) {
	if v, ok := row["id"].(string); ok && v != "" {
		return "id:" + v, "id"
	}
	if card, ok := row["card"].(map[string]any); ok {
		if v, ok := card["id"].(string); ok && v != "" {
			return "card.id:" + v, "card.id"
		}
	}

	blob, _ := json.Marshal(row)
	h := sha1.Sum(blob)
	return "sha1:" + hex.EncodeToString(h[:]), "sha1"
}

// isCloudflarePayload detects common Cloudflare challenge payload markers.
func isCloudflarePayload(body string) bool {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "cloudflare") && strings.Contains(lower, "attention required") {
		return true
	}
	if strings.Contains(lower, "cf-ray") && strings.Contains(lower, "enable cookies") {
		return true
	}
	return false
}

// retryDecision returns retryability and classifier reason for trace diagnostics.
func retryDecision(err error) (bool, string) {
	if err == nil {
		return false, "nil error"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true, "deadline exceeded"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "cloudflare") {
		return true, "timeout/cloudflare marker"
	}
	if strings.Contains(msg, "status 403") || strings.Contains(msg, "status 429") {
		return true, "blocked/rate-limited status"
	}
	return false, "non-retryable"
}

// classifyError normalizes retry-related error messages for caller handling.
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "cloudflare") {
		return fmt.Errorf("cloudflare block/challenge detected: %w", err)
	}
	if strings.Contains(msg, "status 403") || strings.Contains(msg, "status 429") {
		return fmt.Errorf("request blocked/rate-limited: %w", err)
	}
	return err
}

// timeoutsToStrings renders timeout values for structured logging fields.
func timeoutsToStrings(timeouts []time.Duration) []string {
	out := make([]string, 0, len(timeouts))
	for _, timeout := range timeouts {
		out = append(out, timeout.String())
	}
	return out
}

// BuildPageURL applies pageNumber/pageSize parameters to an API URL template.
func BuildPageURL(template string, pageNumber, pageSize int) (string, error) {
	u, err := url.Parse(template)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("pageNumber", fmt.Sprintf("%d", pageNumber))
	q.Set("pageSize", fmt.Sprintf("%d", pageSize))
	u.RawQuery = q.Encode()
	return u.String(), nil
}
