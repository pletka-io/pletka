package ontologysnapshot

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

type captureQueries interface {
	WeaveCaptureLegacyOntologySnapshot(context.Context, sqlcgen.WeaveCaptureLegacyOntologySnapshotParams) (sqlcgen.WeaveCaptureLegacyOntologySnapshotRow, error)
}

// Capture is the explicit legacy-preservation entry point for transaction-owning
// infrastructure. Supply the cut's repeatable-read transaction and roll it back
// on any error. Storage derives the evidence and seals selected local bodies;
// this operation does not finalize or certify a whole project release.
func Capture(ctx context.Context, q captureQueries, origin Origin) (Artifact, error) {
	if origin.ProjectID() == "" || origin.Version() == "" || q == nil {
		return Artifact{}, fmt.Errorf("legacy capture requires a validated origin and transaction")
	}
	row, err := q.WeaveCaptureLegacyOntologySnapshot(ctx, sqlcgen.WeaveCaptureLegacyOntologySnapshotParams{ProjectID: origin.ProjectID(), Version: origin.Version()})
	if err != nil {
		return Artifact{}, err
	}
	a := Artifact{JSON: row.Document, Digest: row.Digest}
	if _, err := a.Decode(); err != nil {
		return Artifact{}, err
	}
	return a, nil
}
