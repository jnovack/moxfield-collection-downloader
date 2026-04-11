package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/rs/zerolog/log"
)

// PlaywrightBrowser implements BrowserClient using a real Chromium session.
type PlaywrightBrowser struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	context playwright.BrowserContext
	page    playwright.Page
}

// zerologSlogHandler forwards slog records to zerolog debug events.
type zerologSlogHandler struct {
	attrs []slog.Attr
	group string
}

// Enabled always returns true and relies on zerolog global level filtering.
func (h zerologSlogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle maps slog messages and attributes to zerolog debug fields.
func (h zerologSlogHandler) Handle(_ context.Context, record slog.Record) error {
	event := log.Debug().Str("source", "playwright")
	if h.group != "" {
		event = event.Str("group", h.group)
	}
	for _, attr := range h.attrs {
		event = event.Interface(attr.Key, attr.Value.Any())
	}
	record.Attrs(func(attr slog.Attr) bool {
		event = event.Interface(attr.Key, attr.Value.Any())
		return true
	})
	event.Msg(record.Message)
	return nil
}

// WithAttrs returns a handler with appended base attributes.
func (h zerologSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := zerologSlogHandler{
		attrs: make([]slog.Attr, 0, len(h.attrs)+len(attrs)),
		group: h.group,
	}
	next.attrs = append(next.attrs, h.attrs...)
	next.attrs = append(next.attrs, attrs...)
	return next
}

// WithGroup returns a handler bound to a slog group name.
func (h zerologSlogHandler) WithGroup(name string) slog.Handler {
	next := h
	if next.group == "" {
		next.group = name
	} else {
		next.group = next.group + "." + name
	}
	return next
}

// NewPlaywrightBrowser initializes Playwright, launches Chromium, and creates a page.
func NewPlaywrightBrowser() (*PlaywrightBrowser, error) {
	options := &playwright.RunOptions{
		Stderr: io.Discard,
		Stdout: io.Discard,
		Logger: slog.New(zerologSlogHandler{}),
	}

	if err := playwright.Install(options); err != nil {
		return nil, fmt.Errorf("playwright install failed: %w", err)
	}

	pw, err := playwright.Run(options)
	if err != nil {
		return nil, fmt.Errorf("playwright run failed: %w", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-dev-shm-usage",
			"--no-sandbox",
		},
	})
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("chromium launch failed: %w", err)
	}

	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:  &playwright.Size{Width: 1440, Height: 900},
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36"),
		Locale:    playwright.String("en-US"),
	})
	if err != nil {
		_ = browser.Close()
		_ = pw.Stop()
		return nil, fmt.Errorf("new context failed: %w", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		_ = ctx.Close()
		_ = browser.Close()
		_ = pw.Stop()
		return nil, fmt.Errorf("new page failed: %w", err)
	}

	return &PlaywrightBrowser{pw: pw, browser: browser, context: ctx, page: page}, nil
}

// Bootstrap opens the collection page and captures the first matching API response.
func (p *PlaywrightBrowser) Bootstrap(ctx context.Context, collectionURL, collectionID string, timeout time.Duration) (BootInfo, error) {
	firstResponseChan := make(chan playwright.Response, 1)

	handler := func(response playwright.Response) {
		responseURL := response.URL()
		parsed, err := url.Parse(responseURL)
		if err != nil {
			return
		}
		if parsed.Hostname() != "api2.moxfield.com" {
			return
		}
		if parsed.Path != fmt.Sprintf("/v1/collections/search/%s", collectionID) {
			return
		}
		if parsed.Query().Get("pageNumber") != "1" {
			return
		}

		select {
		case firstResponseChan <- response:
		default:
		}
	}
	p.page.On("response", handler)
	defer p.page.RemoveListener("response", handler)

	if _, err := p.page.Goto(collectionURL, playwright.PageGotoOptions{
		Timeout:   playwright.Float(float64(timeout.Milliseconds())),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		return BootInfo{}, fmt.Errorf("navigation failed: %w", err)
	}

	var result struct {
		BodyText    string
		TemplateURL string
	}

	apiURL := fmt.Sprintf("https://api2.moxfield.com/v1/collections/search/%s?pageNumber=1&pageSize=50", collectionID)

	select {
	case <-ctx.Done():
		return BootInfo{}, ctx.Err()
	case received := <-firstResponseChan:
		bodyText, err := received.Text()
		if err != nil {
			return BootInfo{}, fmt.Errorf("unable to read first response body: %w", err)
		}
		result = struct {
			BodyText    string
			TemplateURL string
		}{
			BodyText:    bodyText,
			TemplateURL: received.URL(),
		}
	case <-time.After(timeout):
		fetchResult, err := p.fetchJSON(apiURL, timeout)
		if err != nil {
			return BootInfo{}, err
		}
		result = struct {
			BodyText    string
			TemplateURL string
		}{
			BodyText:    fetchResult.BodyText,
			TemplateURL: fetchResult.TemplateURL,
		}
	}

	firstJSON := map[string]any{}
	if err := json.Unmarshal([]byte(result.BodyText), &firstJSON); err != nil {
		return BootInfo{}, fmt.Errorf("first collection API response was not valid JSON: %w", err)
	}

	totalResults := extractInt(firstJSON["totalResults"])
	if totalResults == 0 {
		if totals, ok := firstJSON["totals"].(map[string]any); ok {
			totalResults = extractInt(totals["totalResults"])
		}
	}
	if totalResults <= 0 {
		return BootInfo{}, fmt.Errorf("could not read totalResults from first response")
	}

	template := result.TemplateURL
	if template == "" {
		template = apiURL
	}

	return BootInfo{TemplateURL: template, TotalResults: totalResults}, nil
}

// FetchPage requests a collection API page with the requested page size and timeout.
func (p *PlaywrightBrowser) FetchPage(ctx context.Context, req FetchRequest) (FetchPageResult, error) {
	if err := ctx.Err(); err != nil {
		return FetchPageResult{}, err
	}
	pageURL, err := BuildPageURL(req.TemplateURL, req.PageNumber, req.PageSize)
	if err != nil {
		return FetchPageResult{}, err
	}
	res, err := p.fetchJSON(pageURL, req.Timeout)
	if err != nil {
		return FetchPageResult{}, err
	}
	return FetchPageResult{StatusCode: res.StatusCode, StatusText: res.StatusText, BodyText: res.BodyText}, nil
}

// fetchJSONResult carries the result of an in-page fetch operation.
type fetchJSONResult struct {
	StatusCode  int
	StatusText  string
	BodyText    string
	TemplateURL string
}

// fetchJSON executes an in-page fetch so cookies/session context are preserved.
func (p *PlaywrightBrowser) fetchJSON(rawURL string, timeout time.Duration) (fetchJSONResult, error) {
	payload := map[string]any{
		"url":       rawURL,
		"timeoutMs": timeout.Milliseconds(),
	}

	raw, err := p.page.Evaluate(`async ({url, timeoutMs}) => {
		const waitMs = Number.isFinite(Number(timeoutMs)) && Number(timeoutMs) > 0
			? Number(timeoutMs)
			: 120000;
		const timeoutPromise = new Promise((_, reject) => {
			setTimeout(() => reject(new Error("timeout exceeded")), waitMs);
		});
		try {
			const response = await Promise.race([fetch(url, {
				method: 'GET',
				credentials: 'include'
			}), timeoutPromise]);
			const text = await response.text();
			return {
				statusCode: response.status,
				statusText: response.statusText || '',
				bodyText: text,
				templateURL: response.url || url
			};
		} catch (error) {
			return {
				statusCode: 0,
				statusText: String(error),
				bodyText: '',
				templateURL: url
			};
		}
	}`, payload)
	if err != nil {
		return fetchJSONResult{}, err
	}

	jsonRaw, err := json.Marshal(raw)
	if err != nil {
		return fetchJSONResult{}, err
	}

	var parsed struct {
		StatusCode  int    `json:"statusCode"`
		StatusText  string `json:"statusText"`
		BodyText    string `json:"bodyText"`
		TemplateURL string `json:"templateURL"`
	}
	if err := json.Unmarshal(jsonRaw, &parsed); err != nil {
		return fetchJSONResult{}, err
	}

	if parsed.StatusCode == 0 && parsed.StatusText != "" {
		return fetchJSONResult{}, fmt.Errorf("fetch failed: %s", parsed.StatusText)
	}
	if strings.Contains(strings.ToLower(parsed.BodyText), "cloudflare") && strings.Contains(strings.ToLower(parsed.BodyText), "attention required") {
		return fetchJSONResult{}, fmt.Errorf("fetch failed: cloudflare challenge detected")
	}

	return fetchJSONResult{
		StatusCode:  parsed.StatusCode,
		StatusText:  parsed.StatusText,
		BodyText:    parsed.BodyText,
		TemplateURL: parsed.TemplateURL,
	}, nil
}

// Close releases Playwright browser/page resources.
func (p *PlaywrightBrowser) Close() error {
	var firstErr error
	if p.context != nil {
		if err := p.context.Close(); err != nil {
			firstErr = err
		}
	}
	if p.browser != nil {
		if err := p.browser.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if p.pw != nil {
		if err := p.pw.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// extractInt converts known JSON numeric variants into int.
func extractInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
