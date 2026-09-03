// Package tlds provides detection of ICANN-gTLD/ccTLD suffixes using the
// IANA "tlds-alpha-by-domain" root zone list, fetched from the IANA
// authority at first use.
//
// The package answers a single question: does a domain name belong to the
// ICANN namespace (its final label is an IANA-registered TLD) versus an
// alternate root such as HNS.
//
// The package-level Default registry is shared and fetched lazily:
//
//	ok, err := tlds.IsICANN(context.Background(), "example.com")
//
// Independent instances with custom options:
//
//	reg, err := tlds.New(tlds.WithURL("https://mirror.example/tlds.txt"))
//	ok, err := reg.IsICANN(ctx, "example.com")
package icann

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DefaultURL is the IANA root zone list URL used unless overridden with
// WithURL.
const DefaultURL = "https://data.iana.org/TLD/tlds-alpha-by-domain.txt"

// ErrNotLoaded is returned by queries when the list has not been fetched
// successfully yet, for example when the authority is unreachable.
var ErrNotLoaded = errors.New("icann: registry list not loaded")

// Registry is the query surface over an IANA root zone list snapshot.
// Implementations must be safe for concurrent use.
type Registry interface {
	// IsICANN reports whether domain's final label is an IANA-registered
	// TLD. The domain is not otherwise validated; callers are expected to
	// have normalized it already. Matching on the TLD is case-insensitive.
	IsICANN(ctx context.Context, domain string) (bool, error)
	// IsICANNTld reports whether tld is a single label registered as an
	// ICANN TLD. Matching is case-insensitive.
	IsICANNTld(ctx context.Context, tld string) (bool, error)
	// TLDs returns the sorted list of registered TLDs (lower-cased).
	TLDs(ctx context.Context) ([]string, error)
	// Refresh re-fetches the list from the source. Conditional requests
	// are used when the authority supports them, so an unchanged list is
	// cheap to re-confirm. The previously loaded list stays intact when a
	// fetch fails.
	Refresh(ctx context.Context) error
	// LastUpdated reports when the current list snapshot was fetched. The
	// bool result is false until the first successful fetch.
	LastUpdated() (time.Time, bool)
	// Source reports the URL the list is fetched from.
	Source() string
}

// RetryConfig controls the retry behavior of list fetches.
type RetryConfig struct {
	// MaxAttempts is the total number of fetch attempts. Defaults to 3.
	MaxAttempts uint
	// BaseDelay is the exponential back-off base delay. Defaults to 200ms.
	BaseDelay time.Duration
}

type config struct {
	url        string
	httpClient *http.Client
	retry      RetryConfig
	logger     *zap.Logger
}

func defaultRetryConfig() RetryConfig {
	return RetryConfig{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond}
}

// Option configures a Registry.
type Option func(*config)

// WithURL overrides the URL the list is fetched from.
func WithURL(rawURL string) Option {
	return func(c *config) {
		c.url = rawURL
	}
}

// WithHTTPClient sets the HTTP client used for fetches.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) {
		c.httpClient = client
	}
}

// WithRetryConfig overrides the default retry behavior of fetches. Zero
// fields keep their defaults.
func WithRetryConfig(cfg RetryConfig) Option {
	return func(c *config) {
		if cfg.MaxAttempts > 0 {
			c.retry.MaxAttempts = cfg.MaxAttempts
		}
		if cfg.BaseDelay > 0 {
			c.retry.BaseDelay = cfg.BaseDelay
		}
	}
}

// WithLogger sets the logger used for fetch diagnostics. Fetch failures are
// logged but only surface to callers as errors from the query methods.
func WithLogger(logger *zap.Logger) Option {
	return func(c *config) {
		c.logger = logger
	}
}

// New creates a Registry that fetches the IANA root zone list on first use.
// The returned instance is ready immediately; no network call is made until
// the first query or Refresh.
func New(opts ...Option) (Registry, error) {
	cfg := &config{
		url:        DefaultURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		retry:      defaultRetryConfig(),
		logger:     zap.NewNop(),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if strings.TrimSpace(cfg.url) == "" {
		return nil, errors.New("icann: fetch URL must not be empty")
	}
	parsed, err := url.Parse(cfg.url)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("icann: invalid fetch URL: " + cfg.url)
	}
	return &registry{cfg: cfg}, nil
}

var (
	defaultRegistry Registry
	defaultOnce     sync.Once
)

// Default returns the shared package-level registry, created with the
// default options on first use. Like all registries, the list is fetched
// lazily on the first query, so callers may prefer an explicit Refresh with
// their own context at startup to control the initial fetch.
func Default() Registry {
	defaultOnce.Do(func() {
		defaultRegistry = &registry{
			cfg: &config{
				url:        DefaultURL,
				httpClient: &http.Client{Timeout: 30 * time.Second},
				retry:      defaultRetryConfig(),
				logger:     zap.NewNop(),
			},
		}
	})
	return defaultRegistry
}

// IsICANN delegates to the package-level Default registry. See
// [Registry.IsICANN].
func IsICANN(ctx context.Context, domain string) (bool, error) {
	return Default().IsICANN(ctx, domain)
}

// IsICANNTld delegates to the package-level Default registry. See
// [Registry.IsICANNTld].
func IsICANNTld(ctx context.Context, tld string) (bool, error) {
	return Default().IsICANNTld(ctx, tld)
}

// TLDs delegates to the package-level Default registry. See
// [Registry.TLDs].
func TLDs(ctx context.Context) ([]string, error) {
	return Default().TLDs(ctx)
}
