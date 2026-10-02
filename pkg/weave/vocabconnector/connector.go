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
	// NarrowerTotal is how many terms sit directly under this one. It is what
	// lets a parent-term picker say "this cannot be a parent": a concept with
	// none is a leaf, and a controlled list scoped to it browses empty.
	//
	// A pointer, because nil and zero say different things and a plain int
	// cannot hold the difference — nil is "this source does not count", zero
	// is "counted, and there are none". Only the vocabulary service reports
	// it; the local, csv, sparql and aat connectors leave it nil, and a
	// picker must not mark their terms as leaves for it.
	NarrowerTotal *int `json:"narrower_total,omitempty"`
	// DescendantsTotal is the size of the whole subtree under this term, not
	// just its direct children (NarrowerTotal). It is the real size of a
	// controlled list scoped to this term, so a parent-term picker shows it as
	// "N terms". Only the vocabulary service reports it, and only when asked
	// (descendants=1, contract v2.13); nil everywhere else. A pointer for the
	// same nil-vs-zero reason as NarrowerTotal.
	DescendantsTotal *int `json:"descendants_total,omitempty"`
	// HasChildren is set on a children listing: whether this child itself has
	// children (so a tree picker shows an expander). nil when not reported.
	HasChildren *bool `json:"has_children,omitempty"`
	// Browse is a curated root's hint — "children" (step one level) or
	// "descendants" (the node scopes everything below). Empty off the roots
	// listing.
	Browse string `json:"browse,omitempty"`
	// Note is a curated root's optional operator note. Empty off the roots
	// listing.
	Note string `json:"note,omitempty"`
}

type SearchOpts struct {
	Lang      string
	Limit     int
	ParentURI string
}

type Connector interface {
	Search(ctx context.Context, query string, opts SearchOpts) ([]Entry, error)
	Fetch(ctx context.Context, uri string, opts SearchOpts) (*Entry, error)
	// Roots returns a vocabulary's curated browse roots (contract v2.6), each
	// with its Browse hint, Note and DescendantsTotal. Empty (not an error)
	// when the source has none — most mounts, and every non-service connector.
	Roots(ctx context.Context, lang string) ([]Entry, error)
	// Children returns the direct children of one concept (depth=1), paginated
	// by limit/offset, each with HasChildren and DescendantsTotal. Empty when
	// the source has no hierarchy.
	Children(ctx context.Context, conceptID, lang string, limit, offset int) ([]Entry, error)
}
