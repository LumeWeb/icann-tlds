package icann

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mustTestList(t testing.TB) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "tlds-alpha-by-domain.txt"))
	if err != nil {
		t.Fatalf("read test list: %v", err)
	}
	return string(data)
}

// newServerRegistry builds a Registry backed by a test server with the
// default retry config (retries would skew request counts) unless overridden.
func newServerRegistry(t *testing.T, handler http.HandlerFunc, opts ...Option) Registry {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newTestRegistry(t, append([]Option{WithURL(srv.URL)}, opts...)...)
}

func newTestRegistry(t *testing.T, opts ...Option) Registry {
	t.Helper()
	reg, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return reg
}

func fastRetryOpts() []Option {
	return []Option{WithRetryConfig(RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond})}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(WithURL("")); err == nil {
		t.Error("New() with empty URL should fail")
	}
	if _, err := New(WithURL("::::")); err == nil {
		t.Error("New() with invalid URL should fail")
	}
	if _, err := New(WithURL("not-a-url")); err == nil {
		t.Error("New() with scheme-less URL should fail")
	}
	if _, err := New(); err != nil {
		t.Errorf("New() with defaults: %v", err)
	}
}

func TestIsICANNRealList(t *testing.T) {
	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mustTestList(t)))
	})
	ctx := context.Background()

	icannDomains := []string{"example.com", "www.example.COM", "starter.pinned.site"}
	for _, domain := range icannDomains {
		got, err := reg.IsICANN(ctx, domain)
		if err != nil {
			t.Fatalf("IsICANN(%q): %v", domain, err)
		}
		if !got {
			t.Errorf("IsICANN(%q) = false, want true", domain)
		}
	}

	nonICANNDomains := []string{"lumeweb", "blog.altroot", "", "example."}
	for _, domain := range nonICANNDomains {
		got, err := reg.IsICANN(ctx, domain)
		if err != nil {
			t.Fatalf("IsICANN(%q): %v", domain, err)
		}
		if got {
			t.Errorf("IsICANN(%q) = true, want false", domain)
		}
	}
}

func TestIsICANNTldRealList(t *testing.T) {
	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mustTestList(t)))
	})
	ctx := context.Background()

	icannTlds := []string{"com", "COM", "io", "dev", "xn--p1ai"}
	for _, tld := range icannTlds {
		got, err := reg.IsICANNTld(ctx, tld)
		if err != nil {
			t.Fatalf("IsICANNTld(%q): %v", tld, err)
		}
		if !got {
			t.Errorf("IsICANNTld(%q) = false, want true", tld)
		}
	}

	nonTlds := []string{"altroot", "", "not a tld", "a b", "-bad-", "bad.", "under_score"}
	for _, tld := range nonTlds {
		got, err := reg.IsICANNTld(ctx, tld)
		if err != nil {
			t.Fatalf("IsICANNTld(%q): %v", tld, err)
		}
		if got {
			t.Errorf("IsICANNTld(%q) = true, want false", tld)
		}
	}
}

func TestQueriesBeforeFetch(t *testing.T) {
	// Point at a server that is closed so first use fails deterministically
	// without reaching the real IANA endpoint.
	srv := httptest.NewServer(http.NewServeMux())
	srv.Close()
	reg := newTestRegistry(t, append(fastRetryOpts(), WithURL(srv.URL))...)
	ctx := context.Background()

	if _, err := reg.IsICANN(ctx, "example.com"); err == nil {
		t.Error("IsICANN before load should return an error")
	}
	if _, err := reg.IsICANNTld(ctx, "com"); err == nil {
		t.Error("IsICANNTld before load should return an error")
	}
	if _, err := reg.TLDs(ctx); err == nil {
		t.Error("TLDs before load should return an error")
	}
	if _, ok := reg.LastUpdated(); ok {
		t.Error("LastUpdated before load should report false")
	}
}

func TestRefreshRetries(t *testing.T) {
	var calls atomic.Int32
	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(mustTestList(t)))
	}, WithRetryConfig(RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}))

	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh with one transient 500: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("fetch attempts = %d, want 2", calls.Load())
	}
	got, err := reg.IsICANNTld(context.Background(), "com")
	if err != nil || !got {
		t.Errorf("IsICANNTld after retry = (%v, %v), want (true, nil)", got, err)
	}
}

func TestFetchFailureKeepsPreviousList(t *testing.T) {
	var fail atomic.Bool
	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(mustTestList(t)))
	}, fastRetryOpts()...)

	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("initial Refresh: %v", err)
	}

	fail.Store(true)
	if err := reg.Refresh(context.Background()); err == nil {
		t.Error("Refresh against failing authority should return an error")
	}

	got, err := reg.IsICANNTld(context.Background(), "com")
	if err != nil || !got {
		t.Errorf("previous list should remain queryable after failed refresh: got (%v, %v)", got, err)
	}
}

func TestConditionalGet304(t *testing.T) {
	etag := `"v1"`
	list := mustTestList(t)
	var requests atomic.Int32

	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write([]byte(list))
	}, WithRetryConfig(RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}))

	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("initial Refresh: %v", err)
	}
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("re-Refresh after 304: %v", err)
	}
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
	got, err := reg.IsICANNTld(context.Background(), "net")
	if err != nil || !got {
		t.Errorf("IsICANNTld after 304 = (%v, %v), want (true, nil)", got, err)
	}
	if _, ok := reg.LastUpdated(); !ok {
		t.Error("LastUpdated after 304 should report true")
	}
	if reg.Source() == "" {
		t.Error("Source should report the fetch URL")
	}
}

func TestInvalidListRejected(t *testing.T) {
	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# only a header\n\nBAD LABEL WITH SPACES\n"))
	}, fastRetryOpts()...)

	if _, err := reg.TLDs(context.Background()); err == nil {
		t.Error("invalid list payload should be rejected")
	}
}

func TestTLDsSortedAndNormalized(t *testing.T) {
	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# header\n\nCOM\nnet\nORG\nio\n"))
	})

	got, err := reg.TLDs(context.Background())
	if err != nil {
		t.Fatalf("TLDs: %v", err)
	}
	if want := "com io net org"; strings.Join(got, " ") != want {
		t.Errorf("TLDs = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestConcurrentAccess(t *testing.T) {
	reg := newServerRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte(mustTestList(t)))
	}, fastRetryOpts()...)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			if _, err := reg.IsICANN(ctx, "example.com"); err != nil {
				t.Errorf("concurrent IsICANN: %v", err)
			}
			if err := reg.Refresh(ctx); err != nil {
				t.Errorf("concurrent Refresh: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestDefaultShared(t *testing.T) {
	// The real Default points at IANA; only verify initialization and
	// identity without making a network call.
	if Default() == nil {
		t.Fatal("Default() returned nil")
	}
	if Default() != Default() {
		t.Error("Default() should return the same instance")
	}
	if Default().Source() != DefaultURL {
		t.Errorf("Default().Source() = %q, want %q", Default().Source(), DefaultURL)
	}
}
