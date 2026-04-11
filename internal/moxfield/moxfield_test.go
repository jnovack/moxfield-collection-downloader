package moxfield

import "testing"

// TestBuildCollectionURLFromID verifies canonical URL composition from a collection ID.
func TestBuildCollectionURLFromID(t *testing.T) {
	t.Parallel()
	got := BuildCollectionURLFromID("abc123")
	if got != "https://moxfield.com/collection/abc123" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

// TestIsValidCollectionURL verifies accepted and rejected collection URL formats.
func TestIsValidCollectionURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "valid", url: "https://moxfield.com/collection/abc_123-XYZ", want: true},
		{name: "valid with slash", url: "https://www.moxfield.com/collection/abc_123-XYZ/", want: true},
		{name: "invalid scheme", url: "http://moxfield.com/collection/abc", want: false},
		{name: "invalid host", url: "https://example.com/collection/abc", want: false},
		{name: "invalid path", url: "https://moxfield.com/collections/abc", want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsValidCollectionURL(tt.url); got != tt.want {
				t.Fatalf("IsValidCollectionURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// TestExtractCollectionIDFromURL verifies ID extraction for valid and invalid paths.
func TestExtractCollectionIDFromURL(t *testing.T) {
	t.Parallel()
	if got := ExtractCollectionIDFromURL("https://moxfield.com/collection/abc_123-"); got != "abc_123-" {
		t.Fatalf("unexpected id: %q", got)
	}
	if got := ExtractCollectionIDFromURL("https://moxfield.com/decks/abc_123-"); got != "" {
		t.Fatalf("expected empty id, got %q", got)
	}
}
