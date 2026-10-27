package namespace

import "strings"

// MatchesTerm compares a reference with an exact stored term identity. A QName
// may use the term's own prefix or a declared namespace alias. Effective (one
// namespace per prefix) bindings cannot represent aliases and must not be used
// here. Callers must reject multiple matching terms, not choose by iteration.
func MatchesTerm(reference, uri, prefix, local string, aliases []Binding) bool {
	if reference == "" || uri == "" {
		return false
	}
	if reference == uri {
		return true
	}
	p, name, ok := strings.Cut(reference, ":")
	if !ok || p == "" || name == "" || strings.HasPrefix(name, "//") {
		return false
	}
	if prefix == p && local == name {
		return true
	}
	for _, b := range aliases {
		if b.Prefix == p && b.Namespace+name == uri {
			return true
		}
	}
	return false
}
