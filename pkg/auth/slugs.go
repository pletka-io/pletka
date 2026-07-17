package auth

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrSlugReserved is returned by ValidateSlug when the slug matches a
// reserved word (top-level URL routes).
var ErrSlugReserved = errors.New("slug is reserved")

var reservedSlugs = map[string]struct{}{
	"admin": {}, "api": {}, "auth": {}, "login": {}, "logout": {},
	"settings": {}, "help": {}, "about": {}, "contact": {}, "docs": {},
	"blog": {}, "legal": {}, "privacy": {}, "terms": {}, "static": {},
	"assets": {}, "well-known": {}, "robots.txt": {}, "favicon.ico": {},
	"users": {}, "orgs": {}, "projects": {}, "project": {}, "profile": {},
	"register": {}, "ontologies": {}, "health": {}, "healthz": {}, "readyz": {},
}

// slugPattern matches lowercase alphanumeric, hyphens, underscores. 2-50
// chars. Cannot start or end with a separator, no consecutive separators.
var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9]|[-_][a-z0-9]){1,49}$`)

// ValidateSlug returns nil when s is a syntactically valid, non-reserved slug.
func ValidateSlug(s string) error {
	if _, ok := reservedSlugs[s]; ok {
		return fmt.Errorf("%q: %w", s, ErrSlugReserved)
	}
	if !slugPattern.MatchString(s) {
		return fmt.Errorf("slug %q: must be 2-50 chars, lowercase alphanumerics with single - or _ separators", s)
	}
	return nil
}
