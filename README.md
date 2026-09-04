# icann-tlds

Go package for detecting ICANN-registered TLDs and determining whether a
domain name belongs to the ICANN namespace, backed by the IANA
[tlds-alpha-by-domain](https://data.iana.org/TLD/tlds-alpha-by-domain.txt)
root zone list.

## Usage

```go
import "go.lumeweb.com/icann-tlds"
```

The list is fetched from IANA lazily on first use and cached for the process
lifetime. Queries are case-insensitive:

```go
ok, err := icann.IsICANN(ctx, "example.com")
ok, err := icann.IsICANNTld(ctx, "com")
```

Independent instances with custom options:

```go
reg, err := icann.New(icann.WithURL("https://mirror.example/tlds.txt"))
ok, err := reg.IsICANN(ctx, "example.com")
```

Failed fetches return `icann.ErrNotLoaded`; a failed `Refresh` keeps serving
the previously loaded list.

## API

| Member | Description |
|--------|-------------|
| `Default()` | Shared package-level `Registry` |
| `New(opts...)` | Instance with custom options |
| `(Registry).IsICANN(ctx, domain)` | Is the domain's final label an ICANN TLD? |
| `(Registry).IsICANNTld(ctx, tld)` | Is the single label an ICANN TLD? |
| `(Registry).TLDs(ctx)` | Sorted list of registered TLDs |
| `(Registry).Refresh(ctx)` | Force a re-fetch (conditional GET when supported) |
| `(Registry).LastUpdated()` | When the snapshot was fetched |
| `(Registry).Source()` | URL the list is fetched from |

Options: `WithURL`, `WithHTTPClient`, `WithRetryConfig`, `WithLogger`.

## Development

```sh
go build ./...
go test -race ./...
mockery
```

## License

MIT — see [LICENSE](LICENSE).
