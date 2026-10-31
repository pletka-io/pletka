package ontologyrelease

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Restore installs an exact artifact bundle, dependencies first, in one snapshot
// transaction. Existing IDs must have the same published bytes; an unpublished
// collision is never overwritten or implicitly certified. No RDF is reparsed.
func Restore(ctx context.Context, pool *pgxpool.Pool, artifacts []Artifact) (err error) {
	ordered, err := orderArtifacts(artifacts)
	if err != nil {
		return err
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
		if errors.As(err, &pgerr) && pgerr.ConstraintName == "ontology_artifact_conflict" {
			err = &restoreConflict{cause: err}
		}
	}()
	if err := restoreOrdered(ctx, tx, ordered); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type restoreConflict struct{ cause error }

func (e *restoreConflict) Error() string {
	return "ontology artifact conflicts with existing or invalid content: " + e.cause.Error()
}
func (e *restoreConflict) Unwrap() error           { return e.cause }
func (e *restoreConflict) ConflictMessage() string { return e.Error() }

func orderArtifacts(artifacts []Artifact) ([]Artifact, error) {
	// A bundle may contain independent roots with different versions of an
	// owner. Each artifact's Decode still validates its own closure pins.
	values := map[string]Artifact{}
	docs := map[string]Document{}
	for _, a := range artifacts {
		d, err := a.Decode()
		if err != nil {
			return nil, err
		}
		if old, ok := values[d.VersionID]; ok && old != a {
			return nil, invalid("conflicting artifact bytes for %s", d.VersionID)
		}
		values[d.VersionID] = a
		docs[d.VersionID] = d
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	visiting := map[string]bool{}
	done := map[string]bool{}
	out := make([]Artifact, 0, len(values))
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("cyclic ontology artifact bundle")
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, pin := range docs[id].Pins {
			if source, ok := values[pin.VersionID]; ok {
				if source.Digest != pin.Digest || docs[pin.VersionID].OntologyID != pin.OntologyID {
					return invalid("artifact dependency identity mismatch")
				}
				if err := visit(pin.VersionID); err != nil {
					return err
				}
			}
		}
		visiting[id] = false
		done[id] = true
		out = append(out, values[id])
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RestoreTx restores exact publications inside the caller's repeatable-read
// transaction. The caller owns commit/rollback; no active policy is selected.
func RestoreTx(ctx context.Context, tx pgx.Tx, artifacts []Artifact) error {
	ordered, err := orderArtifacts(artifacts)
	if err != nil {
		return err
	}
	return restoreOrdered(ctx, tx, ordered)
}
func restoreOrdered(ctx context.Context, tx pgx.Tx, ordered []Artifact) error {
	q := sqlcgen.New(tx)
	for _, a := range ordered {
		if err := q.WeaveRestoreOntologyArtifact(ctx, sqlcgen.WeaveRestoreOntologyArtifactParams{Artifact: a.JSON, Digest: a.Digest}); err != nil {
			return err
		}
	}
	return nil
}
