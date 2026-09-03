package tlds

import (
	"fmt"
	"strings"
)

// parseList converts the raw IANA "tlds-alpha-by-domain" payload into a
// lower-cased TLD set. The format is one TLD per line (A-labels only);
// blank lines and lines starting with "#" are skipped.
func parseList(data []byte) (map[string]struct{}, error) {
	tlds := make(map[string]struct{}, 1500)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !validTLDLabel(line) {
			return nil, fmt.Errorf("%w: invalid TLD %q at line %d", ErrInvalidList, line, i+1)
		}
		tlds[strings.ToLower(line)] = struct{}{}
	}
	if len(tlds) == 0 {
		return nil, fmt.Errorf("%w: list is empty", ErrInvalidList)
	}
	return tlds, nil
}

// validTLDLabel accepts only single DNS labels consisting of letters,
// digits, and hyphens, with no leading or trailing hyphen.
func validTLDLabel(label string) bool {
	if label == "" || strings.ContainsAny(label, " \t./") {
		return false
	}
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}
