package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologydraft"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// DraftReader can read only its bound ontology draft in one snapshot.
type DraftReader interface {
	Snapshot(context.Context) (DraftSnapshot, error)
}

type draftReader struct {
	queries readsql.Querier
	ref     DraftReference
	closed  bool
}

func (r *draftReader) Snapshot(ctx context.Context) (DraftSnapshot, error) {
	if r.closed {
		return DraftSnapshot{}, fmt.Errorf("ontology draft reader is closed")
	}
	row, err := r.queries.OntologyDraft(ctx, readsql.OntologyDraftParams{OntologyID: string(r.ref.owner), DraftID: string(r.ref.id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return DraftSnapshot{}, apierror.NotFound("ontology draft not found")
	}
	if err != nil {
		return DraftSnapshot{}, err
	}
	var document DraftDocument
	if err := json.Unmarshal(row.Document, &document); err != nil {
		return DraftSnapshot{}, err
	}
	if err := document.validate(); err != nil {
		return DraftSnapshot{}, err
	}
	return DraftSnapshot{OntologyID: r.ref.owner, ID: r.ref.id, BaseVersionID: row.BaseVersionID, Revision: row.Revision, Document: document}, nil
}
