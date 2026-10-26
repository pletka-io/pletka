package ontologyrelease

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// PublishCurrent explicitly establishes a guarantee from current stored bytes.
// It never certifies their historical meaning. Existing publications are rejected;
// portable identity-verified restore is a separate, not yet implemented boundary.
func PublishCurrent(ctx context.Context, pool *pgxpool.Pool, ref ontologybody.Reference, opts Options) (Artifact, error) {
	if ref.OntologyID() == "" || ref.VersionID() == "" {
		return Artifact{}, invalid("publication requires an ontology and version identity")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return Artifact{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := sqlcgen.New(tx)
	_, err = q.WeaveOntologyPublicationSource(ctx, sqlcgen.WeaveOntologyPublicationSourceParams{OntologyID: string(ref.OntologyID()), VersionID: string(ref.VersionID())})
	if err == nil {
		return Artifact{}, invalid("ontology version is already published")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, err
	}
	_, err = q.WeaveOntologyPublicationBody(ctx, sqlcgen.WeaveOntologyPublicationBodyParams{OntologyID: string(ref.OntologyID()), VersionID: string(ref.VersionID())})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ontologybody.Seal(ctx, q, ref)
	}
	if err != nil {
		return Artifact{}, err
	}
	if err := Capture(ctx, q, ref, opts); err != nil {
		return Artifact{}, err
	}
	row, err := q.WeaveOntologyPublicationSource(ctx, sqlcgen.WeaveOntologyPublicationSourceParams{OntologyID: string(ref.OntologyID()), VersionID: string(ref.VersionID())})
	if err != nil {
		return Artifact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Artifact{}, err
	}
	return Artifact{JSON: row.Artifact, Digest: row.Digest}, nil
}
