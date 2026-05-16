package weave

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domainweave "github.com/pletka-io/pletka/domain/weave"
)

type projectInheritanceStore struct {
	pool *pgxpool.Pool
}

var _ domainweave.ProjectInheritanceStore = (*projectInheritanceStore)(nil)

// ProjectInheritances returns the store slice for parent project reuse links.
func (s *Store) ProjectInheritances() domainweave.ProjectInheritanceStore {
	if s == nil {
		return &projectInheritanceStore{}
	}
	return &projectInheritanceStore{pool: s.pool}
}

func (s *projectInheritanceStore) List(ctx context.Context, projectID string) ([]domainweave.ProjectInheritance, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT project_id, parent_project_id, is_primary, canonical_order, adopted_at, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project inheritances: %w", err)
	}
	defer rows.Close()
	return scanProjectInheritances(rows)
}

func (s *projectInheritanceStore) ListVersion(ctx context.Context, projectID, version string) ([]domainweave.ProjectInheritance, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT project_id, parent_project_id, is_primary, canonical_order, adopted_at, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived project inheritances: %w", err)
	}
	defer rows.Close()
	return scanProjectInheritances(rows)
}

func (s *projectInheritanceStore) ListByParent(ctx context.Context, parentID string) ([]domainweave.ProjectInheritance, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT project_id, parent_project_id, is_primary, canonical_order, adopted_at, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance
		WHERE parent_project_id = $1
		ORDER BY project_id ASC
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list project inheritances by parent: %w", err)
	}
	defer rows.Close()
	return scanProjectInheritances(rows)
}

func (s *projectInheritanceStore) Add(ctx context.Context, link domainweave.ProjectInheritance) error {
	pool, err := s.connectionPool()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin add project inheritance transaction: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO weave_project_inheritance (
			project_id, parent_project_id, is_primary, canonical_order, adopted_at, source_mode, source_version
		) VALUES ($1, $2, $3, $4, COALESCE($5, NOW()), $6, $7)
		ON CONFLICT (project_id, parent_project_id)
		DO UPDATE SET
			is_primary = EXCLUDED.is_primary,
			canonical_order = EXCLUDED.canonical_order,
			source_mode = EXCLUDED.source_mode,
			source_version = EXCLUDED.source_version
	`, link.ProjectID, link.ParentProjectID, link.IsPrimary, link.CanonicalOrder, nullableTime(link.AdoptedAt), normalizeInheritanceSourceMode(link.SourceMode), nullableTrimmedString(link.SourceVersion)); err != nil {
		return fmt.Errorf("add project inheritance: %w", err)
	}
	if link.IsPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE weave_project_inheritance
			SET is_primary = false
			WHERE project_id = $1 AND parent_project_id <> $2
		`, link.ProjectID, link.ParentProjectID); err != nil {
			return fmt.Errorf("clear competing primary inheritances: %w", err)
		}
	}
	if err := normalizeInheritanceOrderTx(ctx, tx, link.ProjectID); err != nil {
		return err
	}
	if err := ensureSinglePrimaryInheritanceTx(ctx, tx, link.ProjectID); err != nil {
		return err
	}
	if err := syncPrimaryParentColumnTx(ctx, tx, link.ProjectID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit add project inheritance transaction: %w", err)
	}
	return nil
}

func (s *projectInheritanceStore) Remove(ctx context.Context, projectID, parentID string) error {
	pool, err := s.connectionPool()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin remove project inheritance transaction: %w", err)
	}
	defer rollback(ctx, tx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM weave_project_inheritance
		WHERE project_id = $1 AND parent_project_id = $2
	`, projectID, parentID); err != nil {
		return fmt.Errorf("remove project inheritance: %w", err)
	}
	if err := normalizeInheritanceOrderTx(ctx, tx, projectID); err != nil {
		return err
	}
	if err := ensureSinglePrimaryInheritanceTx(ctx, tx, projectID); err != nil {
		return err
	}
	if err := syncPrimaryParentColumnTx(ctx, tx, projectID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit remove project inheritance transaction: %w", err)
	}
	return nil
}

func (s *projectInheritanceStore) SetPrimary(ctx context.Context, projectID, parentID string) error {
	pool, err := s.connectionPool()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set primary inheritance transaction: %w", err)
	}
	defer rollback(ctx, tx)

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM weave_project_inheritance
			WHERE project_id = $1 AND parent_project_id = $2
		)
	`, projectID, parentID).Scan(&exists); err != nil {
		return fmt.Errorf("lookup project inheritance for primary set: %w", err)
	}
	if !exists {
		return errors.New("project inheritance not found")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE weave_project_inheritance
		SET is_primary = false
		WHERE project_id = $1
	`, projectID); err != nil {
		return fmt.Errorf("clear primary inheritance: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE weave_project_inheritance
		SET is_primary = true
		WHERE project_id = $1 AND parent_project_id = $2
	`, projectID, parentID); err != nil {
		return fmt.Errorf("set primary inheritance: %w", err)
	}
	if err := syncPrimaryParentColumnTx(ctx, tx, projectID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set primary inheritance transaction: %w", err)
	}
	return nil
}

func (s *projectInheritanceStore) Reorder(ctx context.Context, projectID string, parentIDsInOrder []string) error {
	if len(parentIDsInOrder) == 0 {
		return nil
	}
	pool, err := s.connectionPool()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reorder inheritance transaction: %w", err)
	}
	defer rollback(ctx, tx)

	for i, parentID := range parentIDsInOrder {
		if strings.TrimSpace(parentID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE weave_project_inheritance
			SET canonical_order = $3
			WHERE project_id = $1 AND parent_project_id = $2
		`, projectID, parentID, i); err != nil {
			return fmt.Errorf("reorder project inheritance: %w", err)
		}
	}
	if err := normalizeInheritanceOrderTx(ctx, tx, projectID); err != nil {
		return err
	}
	if err := ensureSinglePrimaryInheritanceTx(ctx, tx, projectID); err != nil {
		return err
	}
	if err := syncPrimaryParentColumnTx(ctx, tx, projectID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reorder inheritance transaction: %w", err)
	}
	return nil
}

func (s *projectInheritanceStore) connectionPool() (*pgxpool.Pool, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("project inheritance store is not initialized")
	}
	return s.pool, nil
}

func syncPrimaryParentColumnTx(ctx context.Context, tx pgx.Tx, projectID string) error {
	var parentID *string
	if err := tx.QueryRow(ctx, `
		SELECT parent_project_id
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
		LIMIT 1
	`, projectID).Scan(&parentID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("select primary inheritance for sync: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE weave_projects
		SET parent_project_id = $2
		WHERE id = $1
	`, projectID, parentID); err != nil {
		return fmt.Errorf("sync project parent_project_id: %w", err)
	}
	return nil
}

func ensureSinglePrimaryInheritanceTx(ctx context.Context, tx pgx.Tx, projectID string) error {
	rows, err := tx.Query(ctx, `
		SELECT parent_project_id
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		return fmt.Errorf("list project inheritances for primary normalization: %w", err)
	}
	defer rows.Close()

	ordered := []string{}
	for rows.Next() {
		var parentID string
		if err := rows.Scan(&parentID); err != nil {
			return fmt.Errorf("scan project inheritance primary normalization: %w", err)
		}
		ordered = append(ordered, parentID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project inheritances for primary normalization: %w", err)
	}
	if len(ordered) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE weave_project_inheritance
		SET is_primary = CASE WHEN parent_project_id = $2 THEN true ELSE false END
		WHERE project_id = $1
	`, projectID, ordered[0]); err != nil {
		return fmt.Errorf("normalize primary inheritance: %w", err)
	}
	return nil
}

func normalizeInheritanceOrderTx(ctx context.Context, tx pgx.Tx, projectID string) error {
	rows, err := tx.Query(ctx, `
		SELECT parent_project_id
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		return fmt.Errorf("list project inheritances for order normalization: %w", err)
	}
	defer rows.Close()

	ordered := []string{}
	for rows.Next() {
		var parentID string
		if err := rows.Scan(&parentID); err != nil {
			return fmt.Errorf("scan project inheritance order normalization: %w", err)
		}
		ordered = append(ordered, parentID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project inheritances for order normalization: %w", err)
	}
	for i, parentID := range ordered {
		if _, err := tx.Exec(ctx, `
			UPDATE weave_project_inheritance
			SET canonical_order = $3
			WHERE project_id = $1 AND parent_project_id = $2
		`, projectID, parentID, i); err != nil {
			return fmt.Errorf("normalize project inheritance order: %w", err)
		}
	}
	return nil
}

func scanProjectInheritances(rows pgx.Rows) ([]domainweave.ProjectInheritance, error) {
	out := []domainweave.ProjectInheritance{}
	for rows.Next() {
		var row domainweave.ProjectInheritance
		var sourceMode string
		var sourceVersion string
		if err := rows.Scan(
			&row.ProjectID,
			&row.ParentProjectID,
			&row.IsPrimary,
			&row.CanonicalOrder,
			&row.AdoptedAt,
			&sourceMode,
			&sourceVersion,
		); err != nil {
			return nil, fmt.Errorf("scan project inheritance: %w", err)
		}
		row.SourceMode = normalizeInheritanceSourceMode(domainweave.DependencySourceMode(sourceMode))
		row.SourceVersion = strings.TrimSpace(sourceVersion)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project inheritances: %w", err)
	}
	return out, nil
}

func normalizeInheritanceSourceMode(mode domainweave.DependencySourceMode) domainweave.DependencySourceMode {
	switch mode {
	case domainweave.DependencySourceRelease:
		return domainweave.DependencySourceRelease
	default:
		return domainweave.DependencySourceDraft
	}
}

func nullableTrimmedString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
