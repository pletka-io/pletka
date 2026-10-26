package ontologybody

import (
	"context"
	"fmt"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologybody"
)

// Reader is bound to one ontology/version for the callback lifetime. It cannot
// select a draft, another owner or another release; returned values may escape.
type Reader interface {
	Body(context.Context) (Body, error)
}

type reader struct {
	queries readsql.Querier
	ref     Reference
	closed  bool
}

func (r *reader) Body(ctx context.Context) (Body, error) {
	if r.closed {
		return Body{}, fmt.Errorf("ontology body reader is closed")
	}
	row, err := r.queries.OntologyBody(ctx, readsql.OntologyBodyParams{OntologyID: string(r.ref.ontology), VersionID: string(r.ref.version)})
	if err != nil {
		return Body{}, readError(err, r.ref)
	}
	body := Body{Reference: r.ref, FormatVersion: row.FormatVersion, CanonicalJSON: row.CanonicalBody, Digest: row.Digest, SealedAt: row.SealedAt}
	if err := body.Verify(); err != nil {
		return Body{}, err
	}
	return body, nil
}
