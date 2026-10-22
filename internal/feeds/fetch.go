package feeds

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Fetcher does polite conditional GETs: a descriptive User-Agent and
// If-None-Match / If-Modified-Since so unchanged feeds cost a 304.
type Fetcher struct {
	Client    *http.Client
	UserAgent func() string // read per request so an admin can change the contact URL
	MaxBytes  int64         // response size cap; 0 means 32 MiB

	mu    sync.Mutex
	cache map[string]validators
}

type validators struct{ etag, lastModified string }

// Response is a fetched body. NotModified means the upstream answered 304 and Body is nil.
type Response struct {
	Body         []byte
	NotModified  bool
	LastModified time.Time // from the Last-Modified header, zero when absent
	Latency      time.Duration
}

func NewFetcher(client *http.Client, userAgent string) *Fetcher {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Fetcher{Client: client, UserAgent: Const(userAgent)}
}

// Get fetches url. Conditional headers are only sent when conditional is true,
// so callers that need the body every time (on-demand caches) can opt out.
func (f *Fetcher) Get(ctx context.Context, url string, conditional bool) (Response, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("User-Agent", f.UserAgent())
	if conditional {
		f.mu.Lock()
		v := f.cache[url]
		f.mu.Unlock()
		if v.etag != "" {
			req.Header.Set("If-None-Match", v.etag)
		}
		if v.lastModified != "" {
			req.Header.Set("If-Modified-Since", v.lastModified)
		}
	}
	res, err := f.Client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()

	out := Response{Latency: time.Since(start)}
	if lm, err := http.ParseTime(res.Header.Get("Last-Modified")); err == nil {
		out.LastModified = lm
	}
	switch {
	case res.StatusCode == http.StatusNotModified && conditional:
		out.NotModified = true
		return out, nil
	case res.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64*1024))
		return out, fmt.Errorf("GET %s: %s", url, res.Status)
	}
	limit := f.MaxBytes
	if limit <= 0 {
		limit = 32 << 20
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return out, fmt.Errorf("GET %s: %w", url, err)
	}
	if int64(len(body)) > limit {
		return out, fmt.Errorf("GET %s: response exceeds %d bytes", url, limit)
	}
	out.Body = body
	out.Latency = time.Since(start)
	if conditional {
		f.mu.Lock()
		if f.cache == nil {
			f.cache = map[string]validators{}
		}
		f.cache[url] = validators{res.Header.Get("ETag"), res.Header.Get("Last-Modified")}
		f.mu.Unlock()
	}
	return out, nil
}

// PostJSON POSTs a JSON body and returns the response body (for GraphQL).
func (f *Fetcher) PostJSON(ctx context.Context, url string, body []byte) (Response, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("User-Agent", f.UserAgent())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := f.Client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	limit := f.MaxBytes
	if limit <= 0 {
		limit = 32 << 20
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return Response{}, fmt.Errorf("POST %s: %w", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("POST %s: %s", url, res.Status)
	}
	if int64(len(b)) > limit {
		return Response{}, fmt.Errorf("POST %s: response exceeds %d bytes", url, limit)
	}
	return Response{Body: b, Latency: time.Since(start)}, nil
}
