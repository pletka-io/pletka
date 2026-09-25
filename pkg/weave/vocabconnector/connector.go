package vocabconnector

import (
	"context"
	"errors"

	"github.com/pletka-io/pletka/pkg/domain"
)

var ErrNotImplemented = errors.New("vocabulary connector not implemented")

// ErrDegraded is returned with zero entries when a lookup could not be
// performed: a timeout, a transport failure, a non-2xx status, or a body that
// would not decode. It is NOT "found nothing".
//
// A connector returns it instead of swallowing the failure, so that the policy
// decision — autocomplete never errors — is made once, in the vocabulary
// service, rather than separately inside every connector. A connector that
// swallows its own failures cannot be told apart from one that genuinely
// matched nothing, which is what made a service outage invisible.
var ErrDegraded = errors.New("vocabulary lookup degraded")

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
