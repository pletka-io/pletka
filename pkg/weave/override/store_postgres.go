package override

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pletka-io/pletka/pkg/database/advisorylock"
	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockTimeout bounds the TOTAL time a WithAdvisoryLock caller waits to
// acquire BOTH the project lock and the entity lock, not each one
// separately — see the remaining-budget comment inside WithAdvisoryLock for
// why a save that spends most of this budget on the project lock must not
// then get a second full lockTimeout for the entity lock. Set as a session
// GUC on the lock's own connection before each acquisition, so a caller
// with a background ctx (ops tooling, MCP, a future API writer) cannot
// queue forever behind someone else's save.
//
// A var, not a const, solely so an integration test can shorten it to
// exercise a real lock-busy wait in well under 10 seconds — see
// lock_busy_integration_test.go. Production code never assigns to it.
var lockTimeout = 10 * time.Second //nolint:gochecknoglobals // test-only override hook, see comment above

// minEntityLockTimeout floors the SECOND SET lock_timeout (for the entity
// lock) when the project lock ate most of the total budget. Two reasons it
// must never be allowed to reach zero: `SET lock_timeout = '0ms'` means
// DISABLED in Postgres — wait forever — which would silently reintroduce
// unbounded waiting exactly when the budget is nearly exhausted; and a
// vanishingly small positive value gives the entity lock no real chance to
// succeed even when it is, in fact, immediately available. 100ms is small
// relative to lockTimeout's production value (10s) and to the values tests
// shorten it to, so it does not meaningfully extend the total worst-case
// wait beyond lockTimeout.
const minEntityLockTimeout = 100 * time.Millisecond

// unlockGraceTimeout bounds the deferred pg_advisory_unlock call. It
// deliberately does not inherit the caller's ctx cancellation (see
// WithAdvisoryLock) — it needs its own bound so a truly wedged server can't
// hang the unlock forever either. Reuses advisorylock.GraceTimeout (the
// same value gitmaterializer's restore-side lock uses) rather than
// re-deriving it.
const unlockGraceTimeout = advisorylock.GraceTimeout

// postgresStore is the pgx + sqlc implementation of Store, backed by the
// weave_field_overrides + weave_override_refs tables.
type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{
		queries: sqlcgen.New(pool),
		pool:    pool,
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// Create inserts a new override. ID/timestamps are written back on success.
func (s *postgresStore) Create(ctx context.Context, o *domain.FieldOverride) error {
	row, err := s.queries.WeaveCreateOverride(ctx, toCreateParams(o))
	if err != nil {
		return fmt.Errorf("create override: %w", err)
	}
	o.ID = row.ID
	o.CreatedAt = row.CreatedAt
	o.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID returns the override or (nil, nil) when not found.
func (s *postgresStore) GetByID(ctx context.Context, id int64) (*domain.FieldOverride, error) {
	row, err := s.queries.WeaveGetOverrideByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get override by id: %w", err)
	}
	return rowToOverride(row), nil
}

// GetBase returns the base override for (fieldID, projectID), or (nil,
// nil) when no base row exists.
func (s *postgresStore) GetBase(ctx context.Context, fieldID, projectID string) (*domain.FieldOverride, error) {
	row, err := s.queries.WeaveGetBaseOverride(ctx, sqlcgen.WeaveGetBaseOverrideParams{
		FieldID:   fieldID,
		ProjectID: projectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get base override: %w", err)
	}
	return rowToOverride(row), nil
}

// GetBaseVersion returns the archived base override for (fieldID,
// projectID) at version, or (nil, nil) when no base row was archived at
// that version.
func (s *postgresStore) GetBaseVersion(ctx context.Context, fieldID, projectID, version string) (*domain.FieldOverride, error) {
	row, err := s.queries.WeaveGetBaseOverrideVersion(ctx, sqlcgen.WeaveGetBaseOverrideVersionParams{
		FieldID:       fieldID,
		ProjectID:     projectID,
		VersionNumber: version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get archived base override: %w", err)
	}
	return rowToArchivedOverride(row), nil
}

// Update writes the full row. Returns "not found" if the row has been
// deleted between Get and Update.
func (s *postgresStore) Update(ctx context.Context, o *domain.FieldOverride) error {
	row, err := s.queries.WeaveUpdateOverride(ctx, toUpdateParams(o))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update override %d: not found", o.ID)
		}
		return fmt.Errorf("update override: %w", err)
	}
	o.UpdatedAt = row.UpdatedAt
	return nil
}

// Delete removes the override. weave_override_refs rows cascade.
func (s *postgresStore) Delete(ctx context.Context, id int64) error {
	if err := s.queries.WeaveDeleteOverride(ctx, id); err != nil {
		return fmt.Errorf("delete override: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

// ListForEntity returns overrides for (entityType, entityID), ordered by position.
func (s *postgresStore) ListForEntity(ctx context.Context, entityType, entityID string) ([]domain.FieldOverride, error) {
	rows, err := s.queries.WeaveListOverridesForEntity(ctx, sqlcgen.WeaveListOverridesForEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("list overrides for entity: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToOverride(row))
	}
	return out, nil
}

// ListForField returns all override rows across all entity types that
// reference fieldID, ordered by entity-type priority then position.
func (s *postgresStore) ListForField(ctx context.Context, fieldID string) ([]domain.FieldOverride, error) {
	rows, err := s.queries.WeaveListOverridesForField(ctx, fieldID)
	if err != nil {
		return nil, fmt.Errorf("list overrides for field: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToOverride(row))
	}
	return out, nil
}

// ListByProjectAndType returns overrides scoped to (projectID, entityType).
// Sorted by entity_id then position.
func (s *postgresStore) ListByProjectAndType(ctx context.Context, projectID, entityType string) ([]domain.FieldOverride, error) {
	rows, err := s.queries.WeaveListOverridesByProjectAndType(ctx, sqlcgen.WeaveListOverridesByProjectAndTypeParams{
		ProjectID:  projectID,
		EntityType: entityType,
	})
	if err != nil {
		return nil, fmt.Errorf("list overrides by project and type: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToOverride(row))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Atomic bulk mutations
// ---------------------------------------------------------------------------

// ReplaceForEntity makes the entity's override rows equal to overrides in
// one transaction, keeping the ids of rows it can match (see matchOverrides)
// so example values anchored to them survive the save.
func (s *postgresStore) ReplaceForEntity(ctx context.Context, entityType, entityID string, overrides []domain.FieldOverride) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin replace overrides tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)

	existingRows, err := q.WeaveListOverridesForEntity(ctx, sqlcgen.WeaveListOverridesForEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return fmt.Errorf("load existing overrides: %w", err)
	}
	existing := make([]domain.FieldOverride, 0, len(existingRows))
	for _, row := range existingRows {
		existing = append(existing, *rowToOverride(row))
	}

	plan := matchOverrides(existing, overrides)
	for _, id := range plan.remove {
		if err := q.WeaveDeleteOverride(ctx, id); err != nil {
			return fmt.Errorf("delete override %d: %w", id, err)
		}
	}

	for i := range overrides {
		o := &overrides[i]
		o.EntityType = entityType
		o.EntityID = entityID

		if id := plan.update[i]; id != 0 {
			o.ID = id
			row, err := q.WeaveUpdateOverride(ctx, toUpdateParams(o))
			if err != nil {
				return fmt.Errorf("update override %d: %w", id, err)
			}
			o.CreatedAt, o.UpdatedAt = row.CreatedAt, row.UpdatedAt
			continue
		}

		row, err := q.WeaveCreateOverride(ctx, toCreateParams(o))
		if err != nil {
			return fmt.Errorf("create override %d: %w", i, err)
		}
		o.ID, o.CreatedAt, o.UpdatedAt = row.ID, row.CreatedAt, row.UpdatedAt
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit replace overrides tx: %w", err)
	}
	return nil
}

// SetRefs atomically replaces the override-ref rows for overrideID.
func (s *postgresStore) SetRefs(ctx context.Context, overrideID int64, refs []domain.OverrideRef) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set refs tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)

	if err := q.WeaveDeleteOverrideRefs(ctx, overrideID); err != nil {
		return fmt.Errorf("delete existing override refs: %w", err)
	}

	for i, ref := range refs {
		if err := q.WeaveCreateOverrideRef(ctx, sqlcgen.WeaveCreateOverrideRefParams{
			OverrideID: overrideID,
			RefType:    ref.RefType,
			TargetID:   ref.TargetID,
			SemanticID: ref.SemanticID,
			Position:   int32(ref.Position),
		}); err != nil {
			return fmt.Errorf("create override ref %d: %w", i, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set refs tx: %w", err)
	}
	return nil
}

// GetRefs returns all refs for overrideID, ordered by ref_type then position.
func (s *postgresStore) GetRefs(ctx context.Context, overrideID int64) ([]domain.OverrideRef, error) {
	rows, err := s.queries.WeaveListOverrideRefs(ctx, overrideID)
	if err != nil {
		return nil, fmt.Errorf("get override refs: %w", err)
	}
	out := make([]domain.OverrideRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToRef(row))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Fingerprint + locking support
// ---------------------------------------------------------------------------

// RefsForOverrides bulk-loads refs for a set of override ids, grouped by
// override id. An empty ids returns an empty map without a round trip.
func (s *postgresStore) RefsForOverrides(ctx context.Context, ids []int64) (map[int64][]domain.OverrideRef, error) {
	out := map[int64][]domain.OverrideRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.queries.WeaveListRefsForOverrides(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list refs for overrides: %w", err)
	}
	for _, row := range rows {
		out[row.OverrideID] = append(out[row.OverrideID], rowToRef(row))
	}
	return out, nil
}

// setSessionLockTimeout sets the `lock_timeout` GUC on conn's session to d.
// SET does not accept a bind parameter, so d is formatted into the
// statement text; every caller passes a package var or a computed
// time.Duration, never unsanitized input.
func setSessionLockTimeout(ctx context.Context, conn *pgxpool.Conn, d time.Duration) error {
	stmt := fmt.Sprintf(`SET lock_timeout = '%dms'`, d.Milliseconds())
	_, err := conn.Exec(ctx, stmt)
	return err
}

// WithAdvisoryLock runs fn while holding, on one pooled connection, a
// session-level Postgres advisory lock SHARED on ProjectLockKey(projectID)
// and then a session-level advisory lock EXCLUSIVE on key, in that order —
// so two saves of one entity queue instead of racing, while two saves of
// different entities in the same project still run concurrently (both only
// take the project lock SHARED). The save spans several service calls, so
// the locks live on their own pooled connection rather than inside a
// transaction, not `pg_advisory_xact_lock`.
//
// lockTimeout bounds the TOTAL wait across BOTH acquisitions, not each one
// separately: the project lock is bounded by the full lockTimeout, but the
// entity lock is then bounded by whatever of that budget remains (floored
// at minEntityLockTimeout), computed from how long the project lock
// actually took. Without this, a save that spent nearly all of lockTimeout
// waiting for the project lock would get a FRESH full lockTimeout for the
// entity lock too, so a curator's worst-case wait for a 409 would be up to
// 2x the documented bound instead of lockTimeout itself.
//
// Both keys are hashed server-side with hashtext (so the same key hashes
// identically across processes and Go versions), an undocumented internal
// function returning int4: the keyspace is only 2^32, so an unrelated pair
// of saves — or a save and an unrelated restore — can in principle collide
// and queue behind each other — a slowdown, never a correctness bug, since
// the lock is only ever a serialization aid, not an identity check.
// Session-level advisory locks also require a direct, session-pinned
// connection: they do not work behind a transaction-pooling pgbouncer,
// which would hand the "session" to a different backend between statements.
//
// The lock is NOT re-entrant, and not just for the same (projectID, key)
// pair: no call may be nested inside another ANYWHERE within the same
// project, even for a different entity. Once a caller holds the project
// lock SHARED and something waiting for it EXCLUSIVE (a restore) has
// queued, Postgres's own fairness rule — confirmed empirically, not just
// documented — makes a SECOND SHARED request that arrives after that
// queue behind the exclusive waiter too, even though shared+shared would
// ordinarily be compatible. A nested call (different connection, same
// project) issued from inside fn would be exactly that second SHARED
// request: it queues behind the restore, which is itself blocked waiting
// for the OUTER call's project lock to release — which never happens,
// because the outer call is blocked waiting for the nested call (inside
// fn) to return. Three-way deadlock, entirely created by adding the
// project lock; the old "don't nest the same entity" rule alone no longer
// covers it.
func (s *postgresStore) WithAdvisoryLock(ctx context.Context, projectID, key string, fn func(context.Context) error) (err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire lock conn: %w", err)
	}
	defer conn.Release()

	deadline := time.Now().Add(lockTimeout)
	if setErr := setSessionLockTimeout(ctx, conn, lockTimeout); setErr != nil {
		return fmt.Errorf("set advisory lock_timeout: %w", setErr)
	}

	// connClosed is set by an unlock defer below when it closes conn
	// itself. Every defer registered after this point checks connClosed
	// first (LIFO means they run in reverse of this registration order),
	// so once one teardown step has destroyed the connection, the rest
	// skip their own guaranteed-to-fail-again attempt instead of piling a
	// second copy of the same underlying failure onto err.
	var connClosed bool

	// lock_timeout is session state and the pool does not scrub a connection
	// on release, so leaving it set would arm a short timeout on whatever
	// unrelated work draws this connection next: a release tag or a git
	// restore waiting on a row lock would abort instead of waiting. Reset it
	// on every path out, including the lock-busy one, on a context the
	// caller's cancellation cannot cut short. This defer is registered
	// before either unlock defer, so it runs last (LIFO), after both locks
	// have been released.
	defer func() {
		if connClosed {
			return
		}
		resetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockGraceTimeout)
		defer cancel()
		if _, resetErr := conn.Exec(resetCtx, `RESET lock_timeout`); resetErr != nil {
			// A session left with a stale lock_timeout must not be reused.
			_ = conn.Conn().Close(resetCtx) //nolint:errcheck // best-effort; Release then destroys the resource
			err = errors.Join(err, fmt.Errorf("reset lock_timeout: %w", resetErr))
		}
	}()

	projectKey := advisorylock.ProjectLockKey(projectID)
	if lockErr := advisorylock.Acquire(ctx, conn, advisorylock.Shared, projectKey); lockErr != nil {
		return fmt.Errorf("take project advisory lock %q: %w", projectKey, lockErr)
	}
	// Registered right after the project lock is taken, so on LIFO
	// teardown it runs AFTER the entity-unlock defer below (registered
	// later, once the entity lock is taken): the entity lock releases
	// first, then the project lock — the mirror image of the
	// project-then-entity acquisition order above.
	defer func() {
		if connClosed {
			return
		}
		if unlockErr := advisorylock.Release(ctx, conn, advisorylock.Shared, projectKey, unlockGraceTimeout); unlockErr != nil {
			connClosed = true
			err = errors.Join(err, fmt.Errorf("release project advisory lock: %w", unlockErr))
		}
	}()

	// Remaining budget for the entity lock: whatever of lockTimeout the
	// project lock did not already spend, floored at minEntityLockTimeout
	// (never zero or negative — see that const's doc comment for why).
	remaining := time.Until(deadline)
	if remaining < minEntityLockTimeout {
		remaining = minEntityLockTimeout
	}
	if setErr := setSessionLockTimeout(ctx, conn, remaining); setErr != nil {
		return fmt.Errorf("set advisory lock_timeout for entity lock: %w", setErr)
	}

	if lockErr := advisorylock.Acquire(ctx, conn, advisorylock.Exclusive, key); lockErr != nil {
		return fmt.Errorf("take advisory lock %q: %w", key, lockErr)
	}
	defer func() {
		if connClosed {
			return
		}
		if unlockErr := advisorylock.Release(ctx, conn, advisorylock.Exclusive, key, unlockGraceTimeout); unlockErr != nil {
			connClosed = true
			err = errors.Join(err, fmt.Errorf("release advisory lock: %w", unlockErr))
		}
	}()

	return fn(ctx)
}

// ---------------------------------------------------------------------------
// Row converters + sqlc param builders
// ---------------------------------------------------------------------------

// rowToOverride converts a sqlcgen row to a *domain.FieldOverride. The
// nullable / pointer-typed sqlc columns are dereferenced through small
// helpers below; MaxOccurs is widened from *int32 to *int.
func rowToOverride(row sqlcgen.WeaveFieldOverride) *domain.FieldOverride {
	o := &domain.FieldOverride{
		ID:                 row.ID,
		FieldID:            row.FieldID,
		ProjectID:          row.ProjectID,
		EntityType:         row.EntityType,
		EntityID:           row.EntityID,
		Position:           int(row.Position),
		CollectionOrder:    int(row.CollectionOrder),
		DisplayName:        unmarshalTranslations(row.DisplayName),
		Description:        unmarshalTranslations(row.Description),
		CollectionName:     unmarshalTranslations(row.CollectionName),
		CategoryID:         derefStr(row.CategoryID),
		PartOfCollectionID: derefStr(row.PartOfCollectionID),
		SetValue:           derefStr(row.SetValue),
		IsRequired:         derefBool(row.IsRequired),
		MinOccurs:          derefInt32(row.MinOccurs),
		IsHidden:           derefBool(row.IsHidden),
		Visibility:         derefStr(row.Visibility),
		StagingID:          row.StagingID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.MaxOccurs != nil {
		v := int(*row.MaxOccurs)
		o.MaxOccurs = &v
	}
	return o
}

// rowToArchivedOverride converts an archived (weave_field_overrides_archive)
// sqlc row to a domain.FieldOverride. Mirrors rowToOverride; the archive
// table lacks set_value_entry_id, otherwise same columns.
func rowToArchivedOverride(row sqlcgen.WeaveFieldOverridesArchive) *domain.FieldOverride {
	o := &domain.FieldOverride{
		ID:                 row.ID,
		FieldID:            row.FieldID,
		ProjectID:          row.ProjectID,
		EntityType:         row.EntityType,
		EntityID:           row.EntityID,
		Position:           int(row.Position),
		CollectionOrder:    int(row.CollectionOrder),
		DisplayName:        unmarshalTranslations(row.DisplayName),
		Description:        unmarshalTranslations(row.Description),
		CollectionName:     unmarshalTranslations(row.CollectionName),
		CategoryID:         derefStr(row.CategoryID),
		PartOfCollectionID: derefStr(row.PartOfCollectionID),
		SetValue:           derefStr(row.SetValue),
		IsRequired:         derefBool(row.IsRequired),
		MinOccurs:          derefInt32(row.MinOccurs),
		IsHidden:           derefBool(row.IsHidden),
		Visibility:         derefStr(row.Visibility),
		StagingID:          row.StagingID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.MaxOccurs != nil {
		v := int(*row.MaxOccurs)
		o.MaxOccurs = &v
	}
	return o
}

// rowToRef converts a sqlcgen ref row to a domain.OverrideRef.
func rowToRef(row sqlcgen.WeaveOverrideRef) domain.OverrideRef {
	return domain.OverrideRef{
		OverrideID: row.OverrideID,
		RefType:    row.RefType,
		TargetID:   row.TargetID,
		SemanticID: row.SemanticID,
		Position:   int(row.Position),
	}
}

func toCreateParams(o *domain.FieldOverride) sqlcgen.WeaveCreateOverrideParams {
	p := sqlcgen.WeaveCreateOverrideParams{
		FieldID:            o.FieldID,
		ProjectID:          o.ProjectID,
		EntityType:         o.EntityType,
		EntityID:           o.EntityID,
		Position:           int32(o.Position),
		CollectionOrder:    int32(o.CollectionOrder),
		DisplayName:        marshalTranslations(o.DisplayName),
		Description:        marshalTranslations(o.Description),
		CollectionName:     marshalTranslations(o.CollectionName),
		CategoryID:         dbutil.EmptyToNil(o.CategoryID),
		PartOfCollectionID: dbutil.EmptyToNil(o.PartOfCollectionID),
		ExpectedValueType:  nil,
		SetValue:           dbutil.EmptyToNil(o.SetValue),
		IsRequired:         dbutil.Ptr(o.IsRequired),
		MinOccurs:          int32Ptr(o.MinOccurs),
		IsHidden:           dbutil.Ptr(o.IsHidden),
		Visibility:         dbutil.EmptyToNil(o.Visibility),
		StagingID:          o.StagingID,
	}
	if o.MaxOccurs != nil {
		v := int32(*o.MaxOccurs)
		p.MaxOccurs = &v
	}
	return p
}

func toUpdateParams(o *domain.FieldOverride) sqlcgen.WeaveUpdateOverrideParams {
	p := sqlcgen.WeaveUpdateOverrideParams{
		ID:                 o.ID,
		Position:           int32(o.Position),
		CollectionOrder:    int32(o.CollectionOrder),
		DisplayName:        marshalTranslations(o.DisplayName),
		Description:        marshalTranslations(o.Description),
		CollectionName:     marshalTranslations(o.CollectionName),
		CategoryID:         dbutil.EmptyToNil(o.CategoryID),
		PartOfCollectionID: dbutil.EmptyToNil(o.PartOfCollectionID),
		ExpectedValueType:  nil,
		SetValue:           dbutil.EmptyToNil(o.SetValue),
		IsRequired:         dbutil.Ptr(o.IsRequired),
		MinOccurs:          int32Ptr(o.MinOccurs),
		IsHidden:           dbutil.Ptr(o.IsHidden),
		Visibility:         dbutil.EmptyToNil(o.Visibility),
	}
	if o.MaxOccurs != nil {
		v := int32(*o.MaxOccurs)
		p.MaxOccurs = &v
	}
	return p
}

// ---------------------------------------------------------------------------
// Local helpers (slice-private; mirrors the Category slice's helpers)
// ---------------------------------------------------------------------------

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}


func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func int32Ptr(v int) *int32 {
	i := int32(v)
	return &i
}

func derefInt32(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func marshalTranslations(t domain.Translations) []byte {
	if t == nil {
		return nil
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil
	}
	return b
}

func unmarshalTranslations(b []byte) domain.Translations {
	if len(b) == 0 {
		return nil
	}
	var t domain.Translations
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	return t
}
