package ontologysnapshot

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

// Restore installs exact sealed bodies and retains the original imported
// artifacts in one transaction. Imported evidence is separate from locally
// captured snapshots: it creates no project release, completion certificate,
// ontology publication or active selection. No RDF is reparsed.
//
// Existing seals and imports are reusable only with identical identities and
// bytes. An unsealed version is never overwritten or implicitly certified.
func Restore(ctx context.Context, pool *pgxpool.Pool, artifacts []Artifact) (err error) {
	bundle, err := ontologyevidence.PrepareBundle(artifacts)
	if err != nil {
		return err
	}
	bodies, evidence := bundle.Bodies(), bundle.Artifacts()
	if len(evidence) == 0 {
		return nil
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	defer func() {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && (pgerr.ConstraintName == "ontology_artifact_conflict" || pgerr.ConstraintName == "legacy_ontology_import_conflict") {
			err = &restoreConflict{cause: err}
		}
	}()
	q := sqlcgen.New(tx)
	documents := make([]string, len(evidence))
	for i, a := range evidence {
		documents[i] = a.JSON
	}
	if err := q.WeaveLockImportedLegacyOntologyContexts(ctx, documents); err != nil {
		return err
	}
	for _, b := range bodies {
		if err := q.WeaveRestoreOntologyBody(ctx, sqlcgen.WeaveRestoreOntologyBodyParams{
			OntologyID: b.OntologyID, VersionID: b.VersionID, BodyText: b.BodyJSON, Digest: b.Digest,
		}); err != nil {
			return err
		}
	}
	for _, a := range evidence {
		if err := q.WeaveRetainImportedLegacyOntologySnapshot(ctx, sqlcgen.WeaveRetainImportedLegacyOntologySnapshotParams{
			Document: a.JSON, Digest: a.Digest,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type restoreConflict struct{ cause error }

func (e *restoreConflict) Error() string {
	return "legacy ontology restore conflicts with retained content: " + e.cause.Error()
}
func (e *restoreConflict) Unwrap() error           { return e.cause }
func (e *restoreConflict) ConflictMessage() string { return e.Error() }
