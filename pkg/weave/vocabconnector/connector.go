package vocabconnector

import (
	"context"
	"errors"

	"github.com/pletka-io/pletka/pkg/domain"
)

var ErrNotImplemented = errors.New("vocabulary connector not implemented")

type Entry struct {
	URI              string                      `json:"uri"`
	Label            domain.Translations         `json:"label"`
	ScopeNote        domain.Translations         `json:"scope_note,omitempty"`
	BroaderURI       string                      `json:"broader_uri,omitempty"`
	BroaderPath      []string                    `json:"broader_path,omitempty"`
	BroaderPathItems []domain.VocabularyEntryRef `json:"broader_path_items,omitempty"`
	ExternalID       string                      `json:"external_id,omitempty"`
}

type SearchOpts struct {
	Lang      string
	Limit     int
	ParentURI string
}

type Connector interface {
	Search(ctx context.Context, query string, opts SearchOpts) ([]Entry, error)
	Fetch(ctx context.Context, uri string, opts SearchOpts) (*Entry, error)
}
