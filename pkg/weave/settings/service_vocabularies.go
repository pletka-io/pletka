package settings

import "context"

// ServiceVocabulary is one vocabulary the configured vocabulary service
// serves, as the settings screen needs it: enough to choose between mounts
// and to see, before enabling one, which languages it can answer in.
type ServiceVocabulary struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Concepts int    `json:"concepts,omitempty"`
	// Scheme is the mount's base IRI, stored as the vocabulary row's
	// base_uri when a project adds it.
	Scheme    string   `json:"scheme,omitempty"`
	Languages []string `json:"languages,omitempty"`
}

// ServiceVocabularyLister reads the configured vocabulary service's own
// listing. The settings slice states what it needs rather than importing the
// connector; pkg/app supplies the adapter.
//
// A nil lister means no service is configured for this instance, and the
// settings screen offers no service-backed vocabularies — which is different
// from the service being unreachable, and must read differently to a curator.
type ServiceVocabularyLister interface {
	ServiceVocabularies(ctx context.Context) ([]ServiceVocabulary, error)
}

// ServiceMount is what a project records when it adds a vocabulary the
// service serves: the mount name it is keyed by, plus how the service
// describes it.
//
// Label and BaseURI come from the service's own listing rather than being
// invented here — a row added without them reads as "fish-monument-type"
// beside "Art & Architecture Thesaurus", and carries no base_uri for
// URI-to-vocabulary lookups to match on. Both fall back cleanly: an empty
// Label means the row is labeled by its mount name, which is the previous
// behavior rather than a broken one.
type ServiceMount struct {
	Name    string
	Label   string
	BaseURI string
	Lang    string
}
