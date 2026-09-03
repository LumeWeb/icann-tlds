package icann

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v5"
)

// registry is the default [Registry] implementation. The list is fetched
// lazily on first use and replaced atomically on refresh; concurrent readers
// never lock the network path for longer than a single load.
type registry struct {
	cfg *config

	mu           sync.RWMutex
	tlds         map[string]struct{}
	etag         string
	lastModified string
	updatedAt    time.Time
	fetchedFrom  string
}

var errNotModified = errors.New("icann: list not modified")

// ensureLoaded fetches the list if it has not been loaded yet. Subsequent
// calls after a failed load retry the fetch, so callers recover as soon as
// the authority becomes reachable.
func (r *registry) ensureLoaded(ctx context.Context) error {
	r.mu.RLock()
	loaded := r.tlds != nil
	r.mu.RUnlock()
	if loaded {
		return nil
	}
	return r.Refresh(ctx)
}

// Refresh fetches the list from the source. Conditional headers (ETag /
// If-Modified-Since) are sent when a previous snapshot exists; a 304 keeps
// the current list and confirms it is still current. On failure the existing
// list, if any, is left intact.
func (r *registry) Refresh(ctx context.Context) error {
	data, notModified, fetchErr := r.fetch(ctx)

	r.mu.Lock()
	defer r.mu.Unlock()

	if fetchErr != nil {
		if r.tlds != nil {
			// Keep serving the stale list; report why it was not updated.
			return fmt.Errorf("icann: refresh failed, serving previous list: %w", fetchErr)
		}
		return fmt.Errorf("%w: %v", ErrNotLoaded, fetchErr)
	}

	if notModified && r.tlds != nil {
		r.updatedAt = time.Now()
		return nil
	}

	if notModified {
		return fmt.Errorf("%w: authority reported 304 but no list is loaded", ErrNotLoaded)
	}

	parsed, err := parseList(data)
	if err != nil {
		if r.tlds != nil {
			return fmt.Errorf("icann: refresh failed, serving previous list: %w", err)
		}
		return err
	}
	r.tlds = parsed
	r.updatedAt = time.Now()
	r.fetchedFrom = r.cfg.url
	return nil
}

// fetch performs the HTTP GET with retries and conditional headers. The
// returned data is only meaningful when notModified is false.
func (r *registry) fetch(ctx context.Context) (data []byte, notModified bool, err error) {
	r.mu.RLock()
	etag := r.etag
	lastModified := r.lastModified
	loaded := r.tlds != nil
	r.mu.RUnlock()

	retryErr := retry.New(
		retry.Context(ctx),
		retry.Attempts(r.cfg.retry.MaxAttempts),
		retry.Delay(r.cfg.retry.BaseDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	).Do(
		func() error {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.url, nil)
			if reqErr != nil {
				return retry.Unrecoverable(reqErr)
			}
			if etag != "" {
				req.Header.Set("If-None-Match", etag)
			}
			if lastModified != "" {
				req.Header.Set("If-Modified-Since", lastModified)
			}
			resp, reqErr := r.cfg.httpClient.Do(req)
			if reqErr != nil {
				return reqErr
			}
			defer resp.Body.Close()

			switch {
			case resp.StatusCode == http.StatusNotModified:
				notModified = true
				return nil
			case resp.StatusCode == http.StatusNotFound:
				return retry.Unrecoverable(fmt.Errorf("list not found at %s", r.cfg.url))
			case resp.StatusCode == http.StatusOK:
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxListSize))
				if readErr != nil {
					return readErr
				}
				data = body
				etag = strings.TrimSpace(resp.Header.Get("ETag"))
				lastModified = resp.Header.Get("Last-Modified")
				return nil
			default:
				return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, r.cfg.url)
			}
		},
	)
	if retryErr != nil {
		return nil, false, retryErr
	}
	if !loaded && notModified {
		return nil, false, fmt.Errorf("%w: 304 without a cached snapshot", ErrNotLoaded)
	}
	return data, notModified, nil
}

// maxListSize caps the accepted payload. The real list is well under 1 MiB;
// anything larger indicates a non-authority response (interstitials,
// captive portals, redirects to HTML error pages).
const maxListSize = 4 << 20

func (r *registry) LastUpdated() (time.Time, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.updatedAt.IsZero() {
		return time.Time{}, false
	}
	return r.updatedAt, true
}

func (r *registry) Source() string {
	return r.cfg.url
}
