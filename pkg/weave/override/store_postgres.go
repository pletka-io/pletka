package override

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

// WithAdvisoryLock runs fn while holding a session-level Postgres advisory
// lock on key, so two saves of one entity queue instead of racing. The save
// spans several service calls, so the lock lives on its own pooled
// connection rather than inside a transaction.
func (s *postgresStore) WithAdvisoryLock(ctx context.Context, key string, fn func(context.Context) error) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire lock conn: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, key); err != nil {
		return fmt.Errorf("take advisory lock: %w", err)
	}
	// The connection is released back to the pool right after, which ends
	// the session and therefore the lock even if the unlock call itself
	// fails — there is no logger on this store to report that error to.
	defer func() {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock(hashtext($1))`, key)
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
