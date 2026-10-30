package ontologysnapshot

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologysnapshot"
)

// Reader has only one captured root, for the callback lifetime. Returned values
// can escape, but this capability cannot retarget or consult current policy.
type Reader interface {
	Snapshot(context.Context) (Artifact, error)
}

type reader struct {
	queries readsql.Querier
	ref     Reference
	closed  bool
}

type missingError struct{}

func (*missingError) Error() string        { return "exact legacy ontology snapshot is not retained" }
func (*missingError) IsNotInRelease() bool { return true }

func (r *reader) Snapshot(ctx context.Context) (Artifact, error) {
	if r.closed {
		return Artifact{}, fmt.Errorf("legacy ontology snapshot reader is closed")
	}
	row, err := r.queries.OntologySnapshot(ctx, readsql.OntologySnapshotParams{ProjectID: r.ref.Origin().ProjectID(), Version: r.ref.Origin().Version(), Digest: r.ref.Digest()})
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, &missingError{}
	}
	if err != nil {
		return Artifact{}, err
	}
	a := Artifact{JSON: row.Document, Digest: row.Digest}
	d, err := a.Decode()
	if err != nil {
		return Artifact{}, err
	}
	if d.ProjectID != r.ref.Origin().ProjectID() || d.Version != r.ref.Origin().Version() || a.Digest != r.ref.Digest() {
		return Artifact{}, fmt.Errorf("legacy snapshot binding mismatch")
	}
	return a, nil
}
