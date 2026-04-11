package mcd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jnovack/moxfield-collection-downloader/internal/downloader"
	"github.com/jnovack/moxfield-collection-downloader/internal/moxfield"
	"github.com/jnovack/moxfield-collection-downloader/internal/output"
)

var (
	// ErrInvalidInput indicates invalid or missing collection input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrFreshnessBlocked indicates output freshness guard prevented writing.
	ErrFreshnessBlocked = errors.New("freshness blocked")
	// ErrRetrievalFailed indicates data retrieval failed.
	ErrRetrievalFailed = errors.New("retrieval failed")
	// ErrPayloadMismatch indicates payload integrity validation failed.
	ErrPayloadMismatch = errors.New("payload validation mismatch")
	// ErrOutputWrite indicates writing output payload failed.
	ErrOutputWrite = errors.New("output write failed")
)

const (
	// DefaultTimeout is the default request timeout used by the package API.
	DefaultTimeout = 10 * time.Second
)

// Logger defines the logging contract accepted by the public API.
type Logger interface {
	Info(msg string, fields map[string]any)
	Warn(msg string, fields map[string]any)
	Debug(msg string, fields map[string]any)
	Trace(msg string, fields map[string]any)
}

// NopLogger discards all logs.
type NopLogger struct{}

// Info ignores info logs.
func (NopLogger) Info(string, map[string]any) {}

// Warn ignores warning logs.
func (NopLogger) Warn(string, map[string]any) {}

// Debug ignores debug logs.
func (NopLogger) Debug(string, map[string]any) {}

// Trace ignores trace logs.
func (NopLogger) Trace(string, map[string]any) {}

// ResolvedInput contains normalized collection identifiers.
type ResolvedInput struct {
	CollectionID  string
	CollectionURL string
}

// RetrieveOptions configures retrieval-only execution.
type RetrieveOptions struct {
	CollectionID  string
	CollectionURL string
	Timeout       time.Duration
	Logger        Logger
}

// RunOptions configures fetch+write execution.
type RunOptions struct {
	RetrieveOptions
	OutputPath string
	Force      bool
}

// RunStats exposes retrieval counters.
type RunStats struct {
	Requests          int
	PagesFetched      int
	TimeoutBackoffs   int
	PageSizeBackoffs  int
	DuplicatesSkipped int
	FinalPageSize     int
}

// RetrieveResult is the result of a successful retrieval.
type RetrieveResult struct {
	CollectionID  string
	CollectionURL string
	Payload       map[string]any
	Stats         RunStats
}

// RunResult is the result of a successful fetch+write call.
type RunResult struct {
	RetrieveResult
	OutputPath string
}

var newBrowserClient = func() (downloader.BrowserClient, error) {
	return downloader.NewPlaywrightBrowser()
}

// ResolveInput validates and normalizes collection input.
func ResolveInput(collectionID, collectionURL string) (ResolvedInput, error) {
	if collectionID == "" && collectionURL == "" {
		return ResolvedInput{}, fmt.Errorf("%w: provide collection id or url", ErrInvalidInput)
	}

	if collectionID != "" {
		if !moxfield.IsValidCollectionID(collectionID) {
			return ResolvedInput{}, fmt.Errorf("%w: invalid collection ID %q: must be 1-32 alphanumeric, underscore, or hyphen characters", ErrInvalidInput, collectionID)
		}
		return ResolvedInput{
			CollectionID:  collectionID,
			CollectionURL: moxfield.BuildCollectionURLFromID(collectionID),
		}, nil
	}

	if !moxfield.IsValidCollectionURL(collectionURL) {
		return ResolvedInput{}, fmt.Errorf("%w: invalid collection URL: expected https://moxfield.com/collection/<id>", ErrInvalidInput)
	}
	id := moxfield.ExtractCollectionIDFromURL(collectionURL)
	if id == "" {
		return ResolvedInput{}, fmt.Errorf("%w: unable to resolve collection ID", ErrInvalidInput)
	}
	return ResolvedInput{
		CollectionID:  id,
		CollectionURL: moxfield.BuildCollectionURLFromID(id),
	}, nil
}

// ResolveOutputPath normalizes output path semantics used by CLI and package callers.
func ResolveOutputPath(path string) string {
	return output.ResolveOutputPath(path)
}

// Retrieve fetches a collection payload without writing files.
func Retrieve(ctx context.Context, opts RetrieveOptions) (RetrieveResult, error) {
	resolved, err := ResolveInput(opts.CollectionID, opts.CollectionURL)
	if err != nil {
		return RetrieveResult{}, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	logger := opts.Logger
	if logger == nil {
		logger = NopLogger{}
	}

	browser, err := newBrowserClient()
	if err != nil {
		return RetrieveResult{}, fmt.Errorf("%w: %w", ErrRetrievalFailed, err)
	}
	defer func() {
		if closeErr := browser.Close(); closeErr != nil {
			logger.Warn("browser close failed", map[string]any{"error": closeErr.Error()})
		}
	}()

	payload, stats, err := downloader.Retrieve(ctx, browser, downloader.Options{
		CollectionID:  resolved.CollectionID,
		CollectionURL: resolved.CollectionURL,
		BaseTimeout:   timeout,
		Logger:        loggerAdapter{logger: logger},
	})
	if err != nil {
		if errors.Is(err, downloader.ErrIntegrityMismatch) {
			return RetrieveResult{}, fmt.Errorf("%w: %w", ErrPayloadMismatch, err)
		}
		return RetrieveResult{}, fmt.Errorf("%w: %w", ErrRetrievalFailed, err)
	}

	return RetrieveResult{
		CollectionID:  resolved.CollectionID,
		CollectionURL: resolved.CollectionURL,
		Payload:       payload,
		Stats:         statsFromDownloader(stats),
	}, nil
}

// Run enforces freshness, fetches the payload, and writes JSON output.
func Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	outPath := ResolveOutputPath(opts.OutputPath)
	if err := output.EnforceFreshness(outPath, opts.Force); err != nil {
		return RunResult{}, fmt.Errorf("%w: %w", ErrFreshnessBlocked, err)
	}

	retrieved, err := Retrieve(ctx, opts.RetrieveOptions)
	if err != nil {
		return RunResult{}, err
	}

	if err := output.WriteJSONFile(outPath, retrieved.Payload); err != nil {
		return RunResult{}, fmt.Errorf("%w: %w", ErrOutputWrite, err)
	}

	return RunResult{
		RetrieveResult: retrieved,
		OutputPath:     outPath,
	}, nil
}

type loggerAdapter struct {
	logger Logger
}

func (l loggerAdapter) Info(msg string, fields map[string]any) {
	l.logger.Info(msg, fields)
}

func (l loggerAdapter) Warn(msg string, fields map[string]any) {
	l.logger.Warn(msg, fields)
}

func (l loggerAdapter) Debug(msg string, fields map[string]any) {
	l.logger.Debug(msg, fields)
}

func (l loggerAdapter) Trace(msg string, fields map[string]any) {
	l.logger.Trace(msg, fields)
}

func statsFromDownloader(stats downloader.RunStats) RunStats {
	return RunStats{
		Requests:          stats.Requests,
		PagesFetched:      stats.PagesFetched,
		TimeoutBackoffs:   stats.TimeoutBackoffs,
		PageSizeBackoffs:  stats.PageSizeBackoffs,
		DuplicatesSkipped: stats.DuplicatesSkipped,
		FinalPageSize:     stats.FinalPageSize,
	}
}
