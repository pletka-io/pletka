package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	writesql "github.com/pletka-io/pletka/pkg/database/writesql/ontologydraft"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// DraftEditor saves only its bound draft. Publication is a separate capability.
type DraftEditor interface {
	DraftReader
	Save(context.Context, DraftDocument) (DraftSnapshot, error)
}
type draftEditor struct {
	*draftReader
	writes   writesql.Querier
	revision int64
}

func (e *draftEditor) Save(ctx context.Context, document DraftDocument) (DraftSnapshot, error) {
	if e.closed {
		return DraftSnapshot{}, fmt.Errorf("ontology draft editor is closed")
	}
	if err := document.validate(); err != nil {
		return DraftSnapshot{}, err
	}
	data, err := json.Marshal(document)
	if err != nil {
		return DraftSnapshot{}, err
	}
	revision, err := e.writes.SaveOntologyDraft(ctx, writesql.SaveOntologyDraftParams{OntologyID: string(e.ref.owner), DraftID: string(e.ref.id), Revision: e.revision, Document: data})
	if errors.Is(err, pgx.ErrNoRows) {
		return DraftSnapshot{}, apierror.Conflict("ontology draft revision changed; reload before editing")
	}
	if err != nil {
		return DraftSnapshot{}, err
	}
	e.revision = revision
	return e.Snapshot(ctx)
}
