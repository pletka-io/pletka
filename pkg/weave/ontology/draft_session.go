package ontology

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologydraft"
	writesql "github.com/pletka-io/pletka/pkg/database/writesql/ontologydraft"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// DraftService owns draft sessions and publication transactions. Authorization
// belongs to the caller; the editor capability itself cannot publish or retarget.
type DraftService struct {
	pool *pgxpool.Pool
	bus  domain.EventBus
}

// NewDraftService constructs the editor lifecycle independently of the live Store.
func NewDraftService(pool *pgxpool.Pool, bus domain.EventBus) *DraftService {
	return &DraftService{pool: pool, bus: bus}
}

// Read binds a draft-only reader to a read-only snapshot and callback lifetime.
func (s *DraftService) Read(ctx context.Context, ref DraftReference, fn func(DraftReader) error) (err error) {
	if err := s.validateRef(ref); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("draft callback required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	r := &draftReader{queries: readsql.New(tx), ref: ref}
	defer func() { r.closed = true; err = closeDraftTx(ctx, tx, err) }()
	if _, err := r.Snapshot(ctx); err != nil {
		return err
	}
	return fn(r)
}

// Edit locks one expected revision. Callback failure rolls back all saves;
// returned values are tentative until Edit itself returns successfully.
func (s *DraftService) Edit(ctx context.Context, ref DraftReference, revision int64, fn func(DraftEditor) error) (err error) {
	defer func() { err = mapDraftError(err) }()
	if err := s.validateRef(ref); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("draft callback required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	r := &draftReader{queries: readsql.New(tx), ref: ref}
	defer func() { r.closed = true; err = closeDraftTx(ctx, tx, err) }()
	writes := writesql.New(tx)
	if err := lockDraft(ctx, writes, ref, revision); err != nil {
		return err
	}
	if err := fn(&draftEditor{draftReader: r, writes: writes, revision: revision}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *DraftService) validateRef(ref DraftReference) error {
	if s == nil || s.pool == nil || ref.owner == "" || ref.id == "" {
		return draftInvalid("draft", "a validated ontology draft reference is required")
	}
	return nil
}
func lockDraft(ctx context.Context, q writesql.Querier, ref DraftReference, expected int64) error {
	if expected < 1 {
		return draftInvalid("revision", "a positive expected revision is required")
	}
	revision, err := q.LockOntologyDraft(ctx, writesql.LockOntologyDraftParams{OntologyID: string(ref.owner), DraftID: string(ref.id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.NotFound("ontology draft not found")
	}
	if err != nil {
		return err
	}
	if revision != expected {
		return apierror.Conflict("ontology draft revision changed; reload before editing or publishing")
	}
	return nil
}
func closeDraftTx(ctx context.Context, tx pgx.Tx, err error) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if closeErr := tx.Rollback(cleanup); closeErr != nil && !errors.Is(closeErr, pgx.ErrTxClosed) {
		return errors.Join(err, closeErr)
	}
	return err
}
func mapDraftError(err error) error {
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && (pgerr.Code == "23505" || pgerr.Code == "40001") {
		return &draftConflict{cause: err}
	}
	return err
}

type draftConflict struct{ cause error }

func (e *draftConflict) Error() string {
	return "ontology draft or release identity changed; reload and choose a new revision or release label"
}
func (e *draftConflict) ConflictMessage() string { return e.Error() }
func (e *draftConflict) Unwrap() error           { return e.cause }
