// Package vocabservice speaks the Pletka vocabulary-service contract: a JSON
// API serving one or more vocabularies. See docs/vocab-service.md in the
// kakugo repo, the contract's authority. kakugo (https://vocab.pletka.io) is
// its first implementation.
package vocabservice

import (
	"path"
	"strings"
)

// conceptID takes the last path segment of a concept URI, which is the
// identifier the service addresses concepts by (concept/{id}, children/{id}
// and under= all take a bare id, never a full IRI).
func conceptID(uri string) string {
	uri = strings.TrimRight(strings.TrimSpace(uri), "/")
	if uri == "" {
		return ""
	}
	return path.Base(uri)
}
