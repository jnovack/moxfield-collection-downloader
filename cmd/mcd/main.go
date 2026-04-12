package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jnovack/flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/jnovack/moxfield-collection-downloader/v2/internal/buildinfo"
	"github.com/jnovack/moxfield-collection-downloader/v2/pkg/mcd"
)

var (
	version      = buildinfo.DefaultVersion
	buildRFC3339 = buildinfo.DefaultBuildRFC3339
	revision     = buildinfo.DefaultRevision
)

// config contains the resolved runtime settings for a downloader execution.
type config struct {
	Force         bool
	LogLevel      string
	CollectionID  string
	CollectionURL string
	Timeout       time.Duration
	OutputPath    string
}

// zLogger adapts zerolog to the downloader.Logger interface.
type zLogger struct{}

// Info writes informational messages via zerolog.
func (zLogger) Info(msg string, fields map[string]any) { log.Info().Fields(fields).Msg(msg) }

// Warn writes warning messages via zerolog.
func (zLogger) Warn(msg string, fields map[string]any) { log.Warn().Fields(fields).Msg(msg) }

// Debug writes debug messages via zerolog.
func (zLogger) Debug(msg string, fields map[string]any) { log.Debug().Fields(fields).Msg(msg) }

// Trace writes trace messages via zerolog.
func (zLogger) Trace(msg string, fields map[string]any) { log.Trace().Fields(fields).Msg(msg) }

// main is the CLI entrypoint for the mcd binary.
func main() {
	version, buildRFC3339, revision = buildinfo.Populate(version, buildRFC3339, revision)
	if err := configureLogging("info"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure logging: %v\n", err)
		os.Exit(1)
	}
	log.Info().
		Str("version", version).
		Str("build_rfc3339", buildRFC3339).
		Str("revision", revision).
		Msg("jnovack/moxfield-collection-downloader")

	cfg, showVersion, err := parseConfig(os.Args[1:], os.Environ())
	if showVersion {
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("invalid configuration")
		flag.Usage()
		os.Exit(1)
	}

	if err := configureLogging(cfg.LogLevel); err != nil {
		log.Error().Err(err).Msg("invalid logging configuration")
		flag.Usage()
		os.Exit(1)
	}

	log.Info().
		Bool("force", cfg.Force).
		Str("log_level", strings.ToLower(strings.TrimSpace(cfg.LogLevel))).
		Dur("timeout", cfg.Timeout).
		Str("output_path", cfg.OutputPath).
		Msg("configuration loaded")
	log.Debug().
		Str("collection_id", cfg.CollectionID).
		Str("collection_url", cfg.CollectionURL).
		Msg("run.start")
	runStart := time.Now()

	runResult, err := mcd.Run(context.Background(), mcd.RunOptions{
		RetrieveOptions: mcd.RetrieveOptions{
			CollectionID:  cfg.CollectionID,
			CollectionURL: cfg.CollectionURL,
			Timeout:       cfg.Timeout,
			Logger:        zLogger{},
		},
		OutputPath: cfg.OutputPath,
		Force:      cfg.Force,
	})
	if err != nil {
		switch {
		case errors.Is(err, mcd.ErrFreshnessBlocked):
			log.Error().Err(err).Str("output_path", cfg.OutputPath).Msg("freshness guard blocked run")
		case errors.Is(err, mcd.ErrOutputWrite):
			log.Error().Err(err).Msg("Failed writing output")
		default:
			log.Error().Err(err).Msg("Download failed")
		}
		os.Exit(1)
	}
	log.Info().Str("output_path", runResult.OutputPath).Msg("saved collection JSON")
	log.Debug().
		Dur("duration", time.Since(runStart)).
		Int("requests", runResult.Stats.Requests).
		Int("pages_fetched", runResult.Stats.PagesFetched).
		Int("timeout_backoffs", runResult.Stats.TimeoutBackoffs).
		Int("request_size_backoffs", runResult.Stats.PageSizeBackoffs).
		Int("duplicates_skipped", runResult.Stats.DuplicatesSkipped).
		Int("final_page_size", runResult.Stats.FinalPageSize).
		Msg("run.complete")
	log.Info().Msg("Completed successfully")
}

// parseConfig resolves CLI flags and environment variables into runtime config.
func parseConfig(args []string, environ []string) (config, bool, error) {
	flag.CommandLine = flag.NewFlagSet("mcd", flag.ContinueOnError)

	idFlag := flag.String("id", "", "Collection ID")
	urlFlag := flag.String("url", "", "Collection URL")
	timeoutFlag := flag.Int("timeout", 0, "Timeout in seconds")
	forceFlag := flag.Bool("force", false, "Ignore freshness guard")
	logLevelFlag := flag.String("log-level", "", "Log level (none, trace, debug, info, warn, error)")
	outputFlag := flag.String("output", "", "Output file path")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.String(flag.DefaultConfigFlagname, "", "path to config file")

	if err := flag.CommandLine.Parse(args); err != nil {
		return config{}, false, err
	}

	env := envMap(environ)

	cfg := config{}
	cfg.Force = *forceFlag || parseBool(env["MCD_FORCE"], false)
	cfg.LogLevel = firstNonEmpty(strings.TrimSpace(*logLevelFlag), strings.TrimSpace(env["MCD_LOG_LEVEL"]), "info")

	cfg.CollectionID = firstNonEmpty(strings.TrimSpace(*idFlag), strings.TrimSpace(env["MCD_COLLECTION_ID"]), strings.TrimSpace(env["MCD_ID"]))
	cfg.CollectionURL = firstNonEmpty(strings.TrimSpace(*urlFlag), strings.TrimSpace(env["MCD_COLLECTION_URL"]), strings.TrimSpace(env["MCD_URL"]))

	timeoutSec := 10
	if *timeoutFlag > 0 {
		timeoutSec = *timeoutFlag
	} else if parsed := parsePositiveInt(env["MCD_TIMEOUT"]); parsed > 0 {
		timeoutSec = parsed
	}
	cfg.Timeout = time.Duration(timeoutSec) * time.Second

	outputValue := strings.TrimSpace(*outputFlag)
	if outputValue == "" {
		outputValue = strings.TrimSpace(env["MCD_OUTPUT"])
	}
	cfg.OutputPath = mcd.ResolveOutputPath(outputValue)

	if *showVersion {
		return cfg, true, nil
	}

	resolved, err := mcd.ResolveInput(cfg.CollectionID, cfg.CollectionURL)
	if err != nil {
		return config{}, false, err
	}
	cfg.CollectionID = resolved.CollectionID
	cfg.CollectionURL = resolved.CollectionURL
	return cfg, *showVersion, nil
}

// configureLogging sets zerolog defaults according to chosen log level.
func configureLogging(logLevel string) error {
	parsedLevel, err := parseLogLevel(logLevel)
	if err != nil {
		return fmt.Errorf("invalid -log-level value %q: %w", logLevel, err)
	}
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.SetGlobalLevel(parsedLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	return nil
}

// envMap converts KEY=VALUE environment entries into a map.
func envMap(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, pair := range environ {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		m[parts[0]] = parts[1]
	}
	return m
}

// parseBool parses common truthy and falsey string values.
func parseBool(value string, fallback bool) bool {
	if value == "" {
		return fallback
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

// parsePositiveInt parses positive integer values and returns 0 for invalid input.
func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

// firstNonEmpty returns the first non-empty value from inputs.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// parseLogLevel parses a textual log level into zerolog.Level.
func parseLogLevel(level string) (zerolog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "none":
		return zerolog.Disabled, nil
	case "trace":
		return zerolog.TraceLevel, nil
	case "debug":
		return zerolog.DebugLevel, nil
	case "", "info":
		return zerolog.InfoLevel, nil
	case "warn", "warning":
		return zerolog.WarnLevel, nil
	case "error":
		return zerolog.ErrorLevel, nil
	default:
		return zerolog.NoLevel, fmt.Errorf("expected one of none, trace, debug, info, warn, error")
	}
}
