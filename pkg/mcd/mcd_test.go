package mcd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jnovack/moxfield-collection-downloader/v2/internal/downloader"
)

type fakeBrowser struct {
	boot      downloader.BootInfo
	fetchFunc func(req downloader.FetchRequest) (downloader.FetchPageResult, error)
	closed    bool
}

func (f *fakeBrowser) Bootstrap(context.Context, string, string, time.Duration) (downloader.BootInfo, error) {
	return f.boot, nil
}

func (f *fakeBrowser) FetchPage(_ context.Context, req downloader.FetchRequest) (downloader.FetchPageResult, error) {
	return f.fetchFunc(req)
}

func (f *fakeBrowser) Close() error {
	f.closed = true
	return nil
}

func TestResolveInput(t *testing.T) {
	got, err := ResolveInput("abc123", "")
	if err != nil {
		t.Fatalf("ResolveInput by id returned error: %v", err)
	}
	if got.CollectionID != "abc123" {
		t.Fatalf("id mismatch: %q", got.CollectionID)
	}
	if got.CollectionURL != "https://moxfield.com/collection/abc123" {
		t.Fatalf("url mismatch: %q", got.CollectionURL)
	}

	got, err = ResolveInput("", "https://moxfield.com/collection/xyz")
	if err != nil {
		t.Fatalf("ResolveInput by url returned error: %v", err)
	}
	if got.CollectionID != "xyz" {
		t.Fatalf("id mismatch from url: %q", got.CollectionID)
	}
}

func TestResolveInputErrors(t *testing.T) {
	_, err := ResolveInput("", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty input, got: %v", err)
	}

	_, err = ResolveInput("bad id", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for bad id, got: %v", err)
	}

	_, err = ResolveInput("", "http://moxfield.com/collection/x")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for bad url, got: %v", err)
	}
}

func TestRetrieveWithMockBrowser(t *testing.T) {
	origFactory := newBrowserClient
	t.Cleanup(func() { newBrowserClient = origFactory })

	browser := &fakeBrowser{
		boot: downloader.BootInfo{TemplateURL: "https://api2.moxfield.com/v1/collections/search/x?pageNumber=1&pageSize=50", TotalResults: 2},
		fetchFunc: func(req downloader.FetchRequest) (downloader.FetchPageResult, error) {
			return fakeResponse(2, 2, 0), nil
		},
	}
	newBrowserClient = func() (downloader.BrowserClient, error) {
		return browser, nil
	}

	got, err := Retrieve(context.Background(), RetrieveOptions{
		CollectionID: "x",
		Timeout:      10 * time.Second,
		Logger:       NopLogger{},
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if got.CollectionID != "x" {
		t.Fatalf("collection id mismatch: %q", got.CollectionID)
	}
	if got.CollectionURL != "https://moxfield.com/collection/x" {
		t.Fatalf("collection url mismatch: %q", got.CollectionURL)
	}
	if got.Stats.Requests != 1 {
		t.Fatalf("expected 1 request, got %d", got.Stats.Requests)
	}
	if !browser.closed {
		t.Fatal("expected browser to be closed")
	}
}

func TestRetrieveMapsPayloadMismatch(t *testing.T) {
	origFactory := newBrowserClient
	t.Cleanup(func() { newBrowserClient = origFactory })

	browser := &fakeBrowser{
		boot: downloader.BootInfo{TemplateURL: "https://api2.moxfield.com/v1/collections/search/x?pageNumber=1&pageSize=50", TotalResults: 3},
		fetchFunc: func(req downloader.FetchRequest) (downloader.FetchPageResult, error) {
			return fakeResponse(3, 2, 0), nil
		},
	}
	newBrowserClient = func() (downloader.BrowserClient, error) {
		return browser, nil
	}

	_, err := Retrieve(context.Background(), RetrieveOptions{
		CollectionID: "x",
		Timeout:      10 * time.Second,
		Logger:       NopLogger{},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("expected ErrPayloadMismatch, got: %v", err)
	}
}

func TestRunFreshnessBlocked(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(outPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	_, err := Run(context.Background(), RunOptions{
		RetrieveOptions: RetrieveOptions{
			CollectionID: "x",
			Timeout:      10 * time.Second,
		},
		OutputPath: outPath,
		Force:      false,
	})
	if err == nil {
		t.Fatal("expected freshness error")
	}
	if !errors.Is(err, ErrFreshnessBlocked) {
		t.Fatalf("expected ErrFreshnessBlocked, got: %v", err)
	}
}

func TestRunWritesOutputWithForce(t *testing.T) {
	origFactory := newBrowserClient
	t.Cleanup(func() { newBrowserClient = origFactory })

	browser := &fakeBrowser{
		boot: downloader.BootInfo{TemplateURL: "https://api2.moxfield.com/v1/collections/search/x?pageNumber=1&pageSize=50", TotalResults: 1},
		fetchFunc: func(req downloader.FetchRequest) (downloader.FetchPageResult, error) {
			return fakeResponse(1, 1, 0), nil
		},
	}
	newBrowserClient = func() (downloader.BrowserClient, error) {
		return browser, nil
	}

	dir := t.TempDir()
	outPath := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(outPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	got, err := Run(context.Background(), RunOptions{
		RetrieveOptions: RetrieveOptions{
			CollectionID: "x",
			Timeout:      10 * time.Second,
			Logger:       NopLogger{},
		},
		OutputPath: outPath,
		Force:      true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.OutputPath != outPath {
		t.Fatalf("output path mismatch: got=%q want=%q", got.OutputPath, outPath)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
}

func fakeResponse(totalResults, count, start int) downloader.FetchPageResult {
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
	return downloader.FetchPageResult{StatusCode: 200, StatusText: "OK", BodyText: string(blob)}
}
