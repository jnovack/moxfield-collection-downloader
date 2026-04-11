package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type logEntry struct {
	level  string
	msg    string
	fields map[string]any
}

type recordingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (l *recordingLogger) add(level, msg string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cloned := map[string]any{}
	for k, v := range fields {
		cloned[k] = v
	}
	l.entries = append(l.entries, logEntry{level: level, msg: msg, fields: cloned})
}

func (l *recordingLogger) Info(msg string, fields map[string]any)  { l.add("info", msg, fields) }
func (l *recordingLogger) Warn(msg string, fields map[string]any)  { l.add("warn", msg, fields) }
func (l *recordingLogger) Debug(msg string, fields map[string]any) { l.add("debug", msg, fields) }
func (l *recordingLogger) Trace(msg string, fields map[string]any) { l.add("trace", msg, fields) }

func (l *recordingLogger) has(level, msg string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, entry := range l.entries {
		if entry.level == level && entry.msg == msg {
			return true
		}
	}
	return false
}

// mockBrowser provides controllable BrowserClient behavior for tests.
type mockBrowser struct {
	boot      BootInfo
	fetchFunc func(FetchRequest) (FetchPageResult, error)
}

// Bootstrap returns predefined bootstrap metadata.
func (m *mockBrowser) Bootstrap(context.Context, string, string, time.Duration) (BootInfo, error) {
	return m.boot, nil
}

// FetchPage delegates to the configured test fetch function.
func (m *mockBrowser) FetchPage(_ context.Context, req FetchRequest) (FetchPageResult, error) {
	return m.fetchFunc(req)
}

// Close is a no-op for mockBrowser.
func (m *mockBrowser) Close() error { return nil }

// TestFloorPow10 verifies power-of-ten rounding behavior.
func TestFloorPow10(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want int
	}{{0, 0}, {9, 1}, {99, 10}, {1234, 1000}, {12345, 10000}}
	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("%d", tt.in), func(t *testing.T) {
			t.Parallel()
			if got := floorPow10(tt.in); got != tt.want {
				t.Fatalf("floorPow10(%d)=%d want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestFallbackFrom verifies generated fallback page-size ordering.
func TestFallbackFrom(t *testing.T) {
	t.Parallel()
	got := fallbackFrom(12345, 12345)
	want := []int{10000, 5000, 1000, 500, 100}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fallbackFrom mismatch: got=%v want=%v", got, want)
	}
}

// TestRetrieveFallbacksOnMismatch verifies fallback behavior after integrity mismatch.
func TestRetrieveFallbacksOnMismatch(t *testing.T) {
	t.Parallel()

	browser := &mockBrowser{
		boot: BootInfo{TemplateURL: "https://api2.moxfield.com/v1/collections/search/x?pageNumber=1&pageSize=50", TotalResults: 1234},
		fetchFunc: func(req FetchRequest) (FetchPageResult, error) {
			if req.PageSize == 1234 {
				return responseWith(1234, 1200, 0), nil
			}
			if req.PageSize == 1000 {
				if req.PageNumber == 1 {
					return responseWith(1234, 1000, 0), nil
				}
				return responseWith(1234, 234, 1000), nil
			}
			return FetchPageResult{}, errors.New("unexpected page size")
		},
	}

	logger := &recordingLogger{}
	got, stats, err := Retrieve(context.Background(), browser, Options{
		CollectionID:  "x",
		CollectionURL: "https://moxfield.com/collection/x",
		BaseTimeout:   5 * time.Second,
		Logger:        logger,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if stats.PageSizeBackoffs == 0 {
		t.Fatal("expected page-size backoff stats")
	}
	if !logger.has("warn", "request size backoff") {
		t.Fatal("expected request size backoff warning log")
	}
	if !logger.has("debug", "request.send") || !logger.has("debug", "request.response") {
		t.Fatal("expected debug request send/response logs")
	}
	if !logger.has("trace", "fallback pagesizes derived") {
		t.Fatal("expected trace fallback derivation log")
	}

	dataRaw, ok := got["data"].([]map[string]any)
	if !ok {
		arr, ok := got["data"].([]any)
		if !ok {
			t.Fatalf("unexpected data type: %T", got["data"])
		}
		dataRaw = make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			obj := item.(map[string]any)
			dataRaw = append(dataRaw, obj)
		}
	}
	if len(dataRaw) != 1234 {
		t.Fatalf("len(data)=%d want 1234", len(dataRaw))
	}
}

// TestRetrieveClassifiesCloudflareAsRetryable verifies timeout/backoff retries on Cloudflare responses.
func TestRetrieveClassifiesCloudflareAsRetryable(t *testing.T) {
	t.Parallel()
	browser := &mockBrowser{
		boot: BootInfo{TemplateURL: "https://api2.moxfield.com/v1/collections/search/x?pageNumber=1&pageSize=50", TotalResults: 100},
		fetchFunc: func(req FetchRequest) (FetchPageResult, error) {
			if req.Timeout < 2*time.Second {
				return FetchPageResult{StatusCode: 403, StatusText: "Forbidden", BodyText: "Attention Required! | Cloudflare"}, nil
			}
			return responseWith(100, 100, 0), nil
		},
	}

	logger := &recordingLogger{}
	_, stats, err := Retrieve(context.Background(), browser, Options{
		CollectionID:  "x",
		CollectionURL: "https://moxfield.com/collection/x",
		BaseTimeout:   1 * time.Second,
		Logger:        logger,
	})
	if err != nil {
		t.Fatalf("expected success after timeout retry, got %v", err)
	}
	if stats.TimeoutBackoffs == 0 {
		t.Fatal("expected timeout backoff stats")
	}
	if !logger.has("warn", "retrying with increased timeout") {
		t.Fatal("expected warn timeout backoff log")
	}
	if !logger.has("trace", "retry classification") {
		t.Fatal("expected trace retry classification log")
	}
}

// TestTimeoutCandidates verifies timeout scaling logic.
func TestTimeoutCandidates(t *testing.T) {
	t.Parallel()
	got := timeoutCandidates(15 * time.Second)
	want := []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timeout candidates got=%v want=%v", got, want)
	}
}

// responseWith builds a JSON response payload with synthetic data rows.
func responseWith(totalResults, count, start int) FetchPageResult {
	data := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		idx := start + i
		data = append(data, map[string]any{
			"id":   fmt.Sprintf("id-%d", idx),
			"name": fmt.Sprintf("name-%d", idx),
		})
	}
	payload := map[string]any{
		"totalResults": totalResults,
		"data":         data,
	}
	blob, _ := json.Marshal(payload)
	return FetchPageResult{StatusCode: 200, StatusText: "OK", BodyText: string(blob)}
}
