package settings

import "context"

// ServiceVocabulary is one vocabulary the configured vocabulary service
// serves, as the settings screen needs it: enough to choose between mounts
// and to see, before enabling one, which languages it can answer in.
type ServiceVocabulary struct {
	Name      string   `json:"name"`
	Label     string   `json:"label,omitempty"`
	Concepts  int      `json:"concepts,omitempty"`
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
