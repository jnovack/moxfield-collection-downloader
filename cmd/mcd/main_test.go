package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestParseBool verifies bool parsing behavior across supported inputs.
func TestParseBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in       string
		fallback bool
		want     bool
	}{
		{"1", false, true},
		{"true", false, true},
		{"no", true, false},
		{"", true, true},
		{"weird", true, true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := parseBool(tt.in, tt.fallback); got != tt.want {
				t.Fatalf("parseBool(%q)=%v want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseConfig verifies flag parsing and derived config values.
func TestParseConfig(t *testing.T) {
	cfg, showVersion, err := parseConfig([]string{"--id", "abc", "--timeout", "90", "--output", "./out.json"}, nil)
	if err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}
	if showVersion {
		t.Fatal("showVersion should be false")
	}
	if cfg.CollectionID != "abc" {
		t.Fatalf("id mismatch: %q", cfg.CollectionID)
	}
	if cfg.CollectionURL != "https://moxfield.com/collection/abc" {
		t.Fatalf("url mismatch: %q", cfg.CollectionURL)
	}
	if cfg.Timeout != 90*time.Second {
		t.Fatalf("timeout mismatch: %s", cfg.Timeout)
	}
	if cfg.OutputPath != "out.json" && cfg.OutputPath != "./out.json" {
		t.Fatalf("output mismatch: %q", cfg.OutputPath)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("log level mismatch: %q", cfg.LogLevel)
	}
}

// TestParseConfigDefaultTimeout verifies the default timeout is 10 seconds.
func TestParseConfigDefaultTimeout(t *testing.T) {
	cfg, showVersion, err := parseConfig([]string{"--id", "abc"}, nil)
	if err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}
	if showVersion {
		t.Fatal("showVersion should be false")
	}
	if cfg.Timeout != 10*time.Second {
		t.Fatalf("timeout mismatch: got=%s want=%s", cfg.Timeout, 10*time.Second)
	}
}

// TestEnvMap verifies conversion of environment key/value pairs to a map.
func TestEnvMap(t *testing.T) {
	t.Parallel()
	got := envMap([]string{"A=1", "B=2"})
	want := map[string]string{"A": "1", "B": "2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("envMap mismatch: got=%v want=%v", got, want)
	}
}

// TestParseConfigLogLevelFromEnv verifies log level can be sourced from env.
func TestParseConfigLogLevelFromEnv(t *testing.T) {
	cfg, showVersion, err := parseConfig([]string{"--id", "abc"}, []string{"MCD_LOG_LEVEL=debug"})
	if err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}
	if showVersion {
		t.Fatal("showVersion should be false")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("log level mismatch: %q", cfg.LogLevel)
	}
}

// TestParseLogLevel verifies accepted and rejected log level values.
func TestParseLogLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    zerolog.Level
		wantErr bool
	}{
		{name: "none", input: "none", want: zerolog.Disabled},
		{name: "trace", input: "trace", want: zerolog.TraceLevel},
		{name: "debug uppercase", input: "DEBUG", want: zerolog.DebugLevel},
		{name: "info default", input: "", want: zerolog.InfoLevel},
		{name: "warn alias", input: "warning", want: zerolog.WarnLevel},
		{name: "error", input: "error", want: zerolog.ErrorLevel},
		{name: "invalid", input: "verbose", wantErr: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseLogLevel(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLogLevel error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseLogLevel mismatch: got=%v want=%v", got, tt.want)
			}
		})
	}
}

// TestConfigureLoggingLevels verifies global zerolog levels for none/info/debug/trace.
func TestConfigureLoggingLevels(t *testing.T) {
	original := zerolog.GlobalLevel()
	t.Cleanup(func() {
		zerolog.SetGlobalLevel(original)
	})

	tests := []struct {
		name  string
		level string
		want  zerolog.Level
	}{
		{name: "none", level: "none", want: zerolog.Disabled},
		{name: "info", level: "info", want: zerolog.InfoLevel},
		{name: "debug", level: "debug", want: zerolog.DebugLevel},
		{name: "trace", level: "trace", want: zerolog.TraceLevel},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := configureLogging(tt.level); err != nil {
				t.Fatalf("configureLogging returned error: %v", err)
			}
			if got := zerolog.GlobalLevel(); got != tt.want {
				t.Fatalf("global level mismatch: got=%v want=%v", got, tt.want)
			}
		})
	}
}
