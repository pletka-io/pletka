package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// ArchiveInput carries the required message explaining why a release is
// being archived.
type ArchiveInput struct {
	Message string
}

// Archive marks a release as archived: it stamps archived_at/archived_message
// on the immutable weave_releases row and enqueues a release_archived
// change-set row for the git materializer. Releases are otherwise
// immutable — archiving is the only lifecycle change (design:
// docs/plans/2026-07-23-releases-git-design.md §1).
func (s *Service) Archive(ctx context.Context, projectID, version string, in ArchiveInput) (*Release, error) {
	if err := s.requireEdit(ctx); err != nil {
		return nil, err
	}

	message := strings.TrimSpace(in.Message)
	if message == "" {
		return nil, &ErrValidation{Fields: map[string][]string{
			"message": {"an archive message is required"},
		}}
	}

	principal := weaveauth.PrincipalFromContext(ctx)
	if principal == nil || principal.ActorID == "" {
		return nil, errors.New("release: missing principal")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin release archive tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var archivedAtCheck *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT archived_at
		FROM weave_releases
		WHERE project_id = $1 AND version = $2
		FOR UPDATE
	`, projectID, version).Scan(&archivedAtCheck); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock release row: %w", err)
	}
	if archivedAtCheck != nil {
		return nil, &ErrValidation{Fields: map[string][]string{
			"version": {"release is already archived"},
		}}
	}

	var updated Release
	if err := tx.QueryRow(ctx, `
		UPDATE weave_releases
		SET archived_at = NOW(), archived_message = $3
		WHERE project_id = $1 AND version = $2
		RETURNING project_id, version, title, description, created_at, created_by_id, archived_at, archived_message
	`, projectID, version, message).Scan(
		&updated.ProjectID,
		&updated.Version,
		&updated.Title,
		&updated.Description,
		&updated.CreatedAt,
		&updated.CreatedByID,
		&updated.ArchivedAt,
		&updated.ArchivedMessage,
	); err != nil {
		return nil, fmt.Errorf("archive release: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO weave_change_set (project_id, actor_id, actor_name, actor_email,
			commit_message, started_at, closed_at, kind, release_version)
		VALUES ($1, $2, 'pletka-system', 'system@pletka.local', $3, NOW(), NOW(), 'release_archived', $4)
	`, projectID, principal.ActorID,
		fmt.Sprintf("Archive release v%s: %s", version, message), version); err != nil {
		return nil, fmt.Errorf("enqueue release archive change set: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit release archive tx: %w", err)
	}
	return &updated, nil
}
