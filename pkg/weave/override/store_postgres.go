package override

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockTimeout bounds how long a WithAdvisoryLock caller waits to acquire the
// per-entity lock. Set as a session GUC on the lock's own connection before
// taking the lock, so a caller with a background ctx (ops tooling, MCP, a
// future API writer) cannot queue forever behind someone else's save.
//
// A var, not a const, solely so an integration test can shorten it to
// exercise a real lock-busy wait in well under 10 seconds — see
// lock_busy_integration_test.go. Production code never assigns to it.
var lockTimeout = 10 * time.Second //nolint:gochecknoglobals // test-only override hook, see comment above

// unlockGraceTimeout bounds the deferred pg_advisory_unlock call. It
// deliberately does not inherit the caller's ctx cancellation (see
// WithAdvisoryLock) — it needs its own bound so a truly wedged server can't
// hang the unlock forever either.
const unlockGraceTimeout = 5 * time.Second

// lockNotAvailableSQLState is Postgres's SQLSTATE for "lock_timeout
// exceeded while waiting for a lock" (55P03), returned when
// pg_advisory_lock aborts because lock_timeout fired.
const lockNotAvailableSQLState = "55P03"

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

// projectLockSQL, projectUnlockSQL, entityLockSQL, and entityUnlockSQL are
// the advisory-lock statements WithAdvisoryLock takes in order (project
// SHARED, then entity EXCLUSIVE) and releases in reverse order. All four
// take hashtext($1) rather than $1 directly (see WithAdvisoryLock's doc
// comment on the 2^32 keyspace), and all four share the same lock table
// regardless of which pair a given key is taken with — the flavor
// (shared/exclusive, session/xact) comes from which function is called, not
// from a different keyspace.
const (
	projectLockSQL   = `SELECT pg_advisory_lock_shared(hashtext($1))`
	projectUnlockSQL = `SELECT pg_advisory_unlock_shared(hashtext($1))`
	entityLockSQL    = `SELECT pg_advisory_lock(hashtext($1))`
	entityUnlockSQL  = `SELECT pg_advisory_unlock(hashtext($1))`
)

// WithAdvisoryLock runs fn while holding, on one pooled connection, a
// session-level Postgres advisory lock SHARED on ProjectLockKey(projectID)
// and then a session-level advisory lock EXCLUSIVE on key, in that order —
// so two saves of one entity queue instead of racing, while two saves of
// different entities in the same project still run concurrently (both only
// take the project lock SHARED). The save spans several service calls, so
// the locks live on their own pooled connection rather than inside a
// transaction, not `pg_advisory_xact_lock`.
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
func (s *postgresStore) WithAdvisoryLock(ctx context.Context, projectID, key string, fn func(context.Context) error) (err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire lock conn: %w", err)
	}
	defer conn.Release()

	// SET does not accept a bind parameter, so the bound duration is
	// formatted into the statement text. lockTimeout is a package variable
	// only so a test can shorten the wait; it is never caller input.
	setLockTimeout := fmt.Sprintf(`SET lock_timeout = '%dms'`, lockTimeout.Milliseconds())
	if _, execErr := conn.Exec(ctx, setLockTimeout); execErr != nil {
		return fmt.Errorf("set advisory lock_timeout: %w", execErr)
	}

	// connClosed is set by an unlock defer below when it closes conn
	// itself. Every defer registered after this point checks connClosed
	// first (LIFO means they run in reverse of this registration order),
	// so once one teardown step has destroyed the connection, the rest
	// skip their own guaranteed-to-fail-again attempt instead of piling a
	// second copy of the same underlying failure onto err.
	var connClosed bool

	// lock_timeout is session state and the pool does not scrub a connection
	// on release, so leaving it set would arm a 10s timeout on whatever
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

	projectKey := ProjectLockKey(projectID)
	if lockErr := acquireAdvisoryLock(ctx, conn, projectLockSQL, projectKey); lockErr != nil {
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
		if unlockErr := releaseAdvisoryLock(ctx, conn, projectUnlockSQL, projectKey); unlockErr != nil {
			connClosed = true
			err = errors.Join(err, fmt.Errorf("release project advisory lock: %w", unlockErr))
		}
	}()

	if lockErr := acquireAdvisoryLock(ctx, conn, entityLockSQL, key); lockErr != nil {
		return fmt.Errorf("take advisory lock %q: %w", key, lockErr)
	}
	defer func() {
		if connClosed {
			return
		}
		if unlockErr := releaseAdvisoryLock(ctx, conn, entityUnlockSQL, key); unlockErr != nil {
			connClosed = true
			err = errors.Join(err, fmt.Errorf("release advisory lock: %w", unlockErr))
		}
	}()

	return fn(ctx)
}

// acquireAdvisoryLock runs lockSQL (always a package constant, never caller
// input — one of projectLockSQL/entityLockSQL) on conn with key, translating
// a lock_timeout expiry (SQLSTATE 55P03) into ErrLockBusy.
func acquireAdvisoryLock(ctx context.Context, conn *pgxpool.Conn, lockSQL, key string) error {
	if _, lockErr := conn.Exec(ctx, lockSQL, key); lockErr != nil {
		if isLockNotAvailable(lockErr) {
			// Join rather than wrap the sentinel alone: errors.Is still
			// matches ErrLockBusy, and an operator staring at a 409 keeps
			// the key and the server's own message.
			return errors.Join(ErrLockBusy, lockErr)
		}
		return lockErr
	}
	return nil
}

// releaseAdvisoryLock runs unlockSQL (always a package constant, never
// caller input — one of projectUnlockSQL/entityUnlockSQL) on conn with key.
//
// The unlock must not run on ctx once ctx may already be done: pgx
// short-circuits an already-canceled ctx (newContextAlreadyDoneError)
// without ever touching the wire, so the UNLOCK never reaches Postgres,
// the session survives, and Release below would hand a connection that
// is still holding this lock straight back to the pool. Run the unlock
// on a context that ignores the caller's cancellation, bounded by its
// own short timeout instead, so it always gets a real chance to reach
// the server; if it still fails (or reports the lock wasn't held), the
// connection must not go back to the pool holding the lock, so close it
// — the caller's Release then destroys the resource instead of recycling
// it. A non-nil return means the caller must treat conn as already closed.
func releaseAdvisoryLock(ctx context.Context, conn *pgxpool.Conn, unlockSQL, key string) error {
	unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockGraceTimeout)
	defer cancel()

	var released bool
	unlockErr := conn.QueryRow(unlockCtx, unlockSQL, key).Scan(&released)
	if unlockErr == nil && released {
		return nil
	}
	if unlockErr == nil {
		unlockErr = fmt.Errorf("advisory lock %q was not held by this session at unlock", key)
	}
	_ = conn.Conn().Close(unlockCtx) //nolint:errcheck // best-effort; Release below destroys the resource regardless
	return unlockErr
}

// isLockNotAvailable reports whether err is Postgres aborting a lock wait
// because lock_timeout fired (SQLSTATE 55P03) — the signal WithAdvisoryLock
// translates into ErrLockBusy.
func isLockNotAvailable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == lockNotAvailableSQLState
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
