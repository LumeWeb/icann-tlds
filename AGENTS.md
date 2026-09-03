# AGENTS.md

This file provides development guidelines and architectural documentation for
the icann-tlds project.

## Common Commands

### Building
```bash
# Build all packages
go build -v ./...

# Build a specific package
go build -v ./tlds
```

### Testing
```bash
# Run all tests with race detection and coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Run a specific package's tests
go test -v -race ./tlds

# View coverage report
go tool cover -func=coverage.out
```

### Templated Code Generation

```bash
# Generate mocks for interfaces (uses .mockery.yaml; mockery is
# pre-installed at $HOME/go/bin/mockery)
mockery
```

### Dependency Management
```bash
# Download dependencies
go mod download

# Verify dependencies
go mod verify

# Tidy dependencies
go mod tidy
```

## Project Overview

This project answers one question: does a domain name belong to the ICANN
namespace — i.e., is its final label a TLD registered in the IANA root zone —
versus an alternate root such as HNS.

## Architecture

### Package Structure
- **Module path**: `go.lumeweb.com/icann-tlds`
- **Root package** (`icann`): umbrella docs only via `doc.go`
- **`tlds` subpackage**: the actual functionality

### tlds Subpackage Design
The root zone list is fetched from IANA
(`https://data.iana.org/TLD/tlds-alpha-by-domain.txt`) lazily on first use —
no network call happens at import time or in `New`.

- **`Registry` interface**: query surface over a fetched list snapshot
  (`IsICANN`, `IsICANNTld`, `TLDs`, `Refresh`, `LastUpdated`, `Source`)
- **`registry` struct**: unexported implementation, safe for concurrent use
  (`sync.RWMutex`); the list is replaced atomically on refresh
- **`Default()`**: shared, lazily-created package-level instance
- **Options pattern**: `WithURL`, `WithHTTPClient`, `WithRetryConfig`,
  `WithLogger` passed to `New`

### Fetch Behavior
- Retries with exponential back-off (`retry-go/v5`); retry config is
  customizable via `RetryConfig`
- Conditional requests (ETag / If-Modified-Since) make re-fetches cheap;
  a 304 confirms the loaded list is still current
- Responses are capped at 4 MiB to reject non-authority payloads
- Format validation: single DNS labels (A-labels) only; comments and blank
  lines per the IANA format; an empty list is an error

### Error Model
- Queries return `ErrNotLoaded` (wrapped) when no list is available yet
- A failed refresh keeps the previous list queryable and wraps
  `ErrInvalidList` for malformed payloads
- There is no embedded fallback list by design: failures must be explicit,
  never silently return stale verdicts

### Testing Conventions
- All fetch tests use `httptest` servers or closed servers; tests never hit
  the real IANA endpoint
- `testdata/tlds-alpha-by-domain.txt` is a real IANA list snapshot used as a
  fixture only — it is not embedded at runtime
- Generated mocks live in `mocks/` (mockery, testify templates)
