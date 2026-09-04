// Package icann provides detection of ICANN-gTLD/ccTLD suffixes using the
// IANA "tlds-alpha-by-domain" root zone list, fetched from the IANA
// authority at first use.
//
// The package answers a single question: does a domain name belong to the
// ICANN namespace (its final label is an IANA-registered TLD) versus an
// alternate root such as HNS.
//
// The list is fetched lazily on first use and cached for the process
// lifetime. Queries are case-insensitive:
//
//	ok, err := icann.IsICANN(ctx, "example.com")
//
// Independent instances with custom options:
//
//	reg, err := icann.New(icann.WithURL("https://mirror.example/tlds.txt"))
//	ok, err := reg.IsICANN(ctx, "example.com")
package icann
