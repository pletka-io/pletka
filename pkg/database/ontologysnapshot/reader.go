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
	row, err := r.queries.OntologySnapshot(ctx, readsql.OntologySnapshotParams{ProjectID: r.ref.origin.project, Version: r.ref.origin.version, Digest: r.ref.digest})
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
	if d.ProjectID != r.ref.origin.project || d.Version != r.ref.origin.version || a.Digest != r.ref.digest {
		return Artifact{}, fmt.Errorf("legacy snapshot binding mismatch")
	}
	return a, nil
}
