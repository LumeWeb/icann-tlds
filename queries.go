package icann

import (
	"context"
	"sort"
	"strings"
)

// IsICANN reports whether domain's final label is an IANA-registered TLD,
// fetching the list on first use. Matching is case-insensitive. The domain
// is not otherwise validated; callers are expected to have normalized it
// already.
func (r *registry) IsICANN(ctx context.Context, domain string) (bool, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return false, err
	}
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	idx := strings.LastIndex(domain, ".")
	if idx < 0 || idx == len(domain)-1 {
		return false, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tlds[domain[idx+1:]]
	return ok, nil
}

// IsICANNTld reports whether tld is a single label registered as an ICANN
// TLD, fetching the list on first use. Matching is case-insensitive.
func (r *registry) IsICANNTld(ctx context.Context, tld string) (bool, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tlds[strings.ToLower(strings.TrimSpace(tld))]
	return ok, nil
}

// TLDs returns the sorted list of registered TLDs (lower-cased), fetching
// the list on first use. The returned slice is a copy.
func (r *registry) TLDs(ctx context.Context) ([]string, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	tlds := make([]string, 0, len(r.tlds))
	for tld := range r.tlds {
		tlds = append(tlds, tld)
	}
	sort.Strings(tlds)
	return tlds, nil
}
