package ontologysnapshot

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/importedontologysnapshot"
)

type importedReader struct {
	queries readsql.Querier
	ref     Reference
	closed  bool
}

func (r *importedReader) Snapshot(ctx context.Context) (Artifact, error) {
	if r.closed {
		return Artifact{}, fmt.Errorf("imported legacy ontology snapshot reader is closed")
	}
	row, err := r.queries.ImportedOntologySnapshot(ctx, readsql.ImportedOntologySnapshotParams{
		ProjectID: r.ref.origin.project, Version: r.ref.origin.version, Digest: r.ref.digest,
	})
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
		return Artifact{}, fmt.Errorf("imported legacy snapshot binding mismatch")
	}
	return a, nil
}
