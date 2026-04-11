package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestResolveOutputPath verifies file and directory output path normalization.
func TestResolveOutputPath(t *testing.T) {
	t.Parallel()
	wd, _ := os.Getwd()
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{name: "empty", input: "", check: func(got string) bool { return got == filepath.Join(wd, "collection.json") }},
		{name: "file", input: "out.json", check: func(got string) bool { return got == "out.json" }},
		{name: "slash dir", input: "tmp/", check: func(got string) bool { return got == filepath.Join("tmp", "collection.json") }},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveOutputPath(tt.input); !tt.check(got) {
				t.Fatalf("ResolveOutputPath(%q) unexpected: %q", tt.input, got)
			}
		})
	}
}

// TestEnforceFreshness verifies freshness blocking and force override behavior.
func TestEnforceFreshness(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "collection.json")

	if err := EnforceFreshness(out, false); err != nil {
		t.Fatalf("missing file should pass: %v", err)
	}

	if err := os.WriteFile(out, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := EnforceFreshness(out, false); err == nil {
		t.Fatal("expected freshness error")
	}
	if err := EnforceFreshness(out, true); err != nil {
		t.Fatalf("force should pass: %v", err)
	}

	old := time.Now().Add(-80 * time.Hour)
	if err := os.Chtimes(out, old, old); err != nil {
		t.Fatalf("chtimes failed: %v", err)
	}
	if err := EnforceFreshness(out, false); err != nil {
		t.Fatalf("old file should pass: %v", err)
	}
}

// TestWriteJSONFile verifies pretty JSON writing and overwrite behavior.
func TestWriteJSONFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "collection.json")

	first := map[string]any{"value": 1}
	if err := WriteJSONFile(out, first); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	second := map[string]any{"value": 2}
	if err := WriteJSONFile(out, second); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	blob, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(blob, &parsed); err != nil {
		t.Fatalf("invalid json written: %v", err)
	}
	if parsed["value"] != float64(2) {
		t.Fatalf("unexpected value: %#v", parsed["value"])
	}
}
