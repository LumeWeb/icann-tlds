package tlds

import "errors"

// ErrInvalidList is returned when a fetched root zone list payload does not
// conform to the expected IANA "tlds-alpha-by-domain" format.
var ErrInvalidList = errors.New("tlds: invalid root zone list")
