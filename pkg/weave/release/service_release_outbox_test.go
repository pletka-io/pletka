//go:build integration

package release

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// seedZREL creates a synthetic project ("ZREL") mirroring the ZDEL seed
// block in pkg/weave/project/delete_integration_test.go: one owner actor,
// one project, one category, one field, one model, and one base override.
// Registers a cleanup that removes both the live rows and the archive rows
// Create's snapshot step writes.
func seedZREL(t *testing.T, pool *pgxpool.Pool) (ctx context.Context, ownerID, projectID string) {
	t.Helper()
	ctx = context.Background()
	ownerID = "ZREL_OWNER"
	projectID = "ZREL"

	cleanup := func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_change_set WHERE project_id='ZREL'`,
			`DELETE FROM weave_releases WHERE project_id='ZREL'`,
			`DELETE FROM weave_field_overrides WHERE entity_id LIKE 'ZREL%' OR field_id IN (SELECT id FROM weave_fields WHERE project_id='ZREL')`,
			`DELETE FROM weave_fields WHERE project_id='ZREL'`,
			`DELETE FROM weave_models WHERE project_id='ZREL'`,
			`DELETE FROM weave_categories WHERE project_id='ZREL'`,
			`DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id='ZREL'`,
			`DELETE FROM weave_projects WHERE id='ZREL'`,
			`DELETE FROM weave_actors WHERE id='ZREL_OWNER'`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id='ZREL'`,
			`DELETE FROM weave_fields_archive WHERE project_id='ZREL'`,
			`DELETE FROM weave_models_archive WHERE project_id='ZREL'`,
			`DELETE FROM weave_categories_archive WHERE project_id='ZREL'`,
			`DELETE FROM weave_projects_archive WHERE id='ZREL'`,
		} {
			_, _ = pool.Exec(bg, stmt)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed %q: %v", sql, err)
		}
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Release Probe"})
	mustExec(`INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
	          VALUES ('ZREL_OWNER','organization','Release Probe Org','zrel-owner','private',NOW(),NOW())`)
	mustExec(`INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
	          VALUES ($1,$2,$2,'draft','ZREL_OWNER','private',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_categories (id, semantic_id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZREL.CAT.1','ZREL.CAT.1',$1,$2,'draft',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_fields (id, semantic_id, system_name, project_id, ui_name, status, path_elements, created_at, updated_at)
	          VALUES ('ZRELF1','ZRELF.1','release_probe',$1,$2,'draft','[]',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_models (id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZRELM.1',$1,$2,'draft',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_field_overrides (field_id, entity_type, entity_id, project_id, created_at, updated_at)
	          VALUES ('ZRELF1','model','ZRELM.1',$1,NOW(),NOW())`, projectID)

	return ctx, ownerID, projectID
}

func releaseAuthCtx(ctx context.Context, ownerID string) context.Context {
	authCtx := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	return weaveauth.WithPrincipal(authCtx, &weaveauth.Principal{ActorID: ownerID})
}

func TestCreateEnqueuesReleaseChangeSet(t *testing.T) {
	pool := releaseTestPool(t)
	ctx, ownerID, projectID := seedZREL(t, pool)
	authCtx := releaseAuthCtx(ctx, ownerID)

	svc := NewService(NewPostgresStore(pool), pool, nil)
	if _, err := svc.Create(authCtx, projectID, CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT project_id, kind, release_version, closed_at, processed_at, actor_name
		FROM weave_change_set
		WHERE project_id = $1
	`, projectID)
	if err != nil {
		t.Fatalf("query change set: %v", err)
	}
	defer rows.Close()

	type row struct {
		projectID      string
		kind           string
		releaseVersion string
		closedAt       any
		processedAt    any
		actorName      string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.projectID, &r.kind, &r.releaseVersion, &r.closedAt, &r.processedAt, &r.actorName); err != nil {
			t.Fatalf("scan change set row: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected exactly one change set row, got %d: %+v", len(got), got)
	}
	r := got[0]
	if r.projectID != "ZREL" || r.kind != "release" || r.releaseVersion != "1.0.0" {
		t.Fatalf("unexpected change set row: %+v", r)
	}
	if r.closedAt == nil {
		t.Fatalf("expected closed_at to be set, got nil")
	}
	if r.processedAt != nil {
		t.Fatalf("expected processed_at NULL, got %v", r.processedAt)
	}
	if r.actorName != "pletka-system" {
		t.Fatalf("expected actor_name 'pletka-system', got %q", r.actorName)
	}
}

func TestArchiveSetsFlagsAndEnqueues(t *testing.T) {
	pool := releaseTestPool(t)
	ctx, ownerID, projectID := seedZREL(t, pool)
	authCtx := releaseAuthCtx(ctx, ownerID)

	svc := NewService(NewPostgresStore(pool), pool, nil)
	if _, err := svc.Create(authCtx, projectID, CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	archived, err := svc.Archive(authCtx, projectID, "1.0.0", ArchiveInput{Message: "superseded by 2.x"})
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("expected ArchivedAt to be set")
	}
	if archived.ArchivedMessage != "superseded by 2.x" {
		t.Fatalf("expected ArchivedMessage to be set, got %q", archived.ArchivedMessage)
	}

	got, err := svc.Get(authCtx, projectID, "1.0.0")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ArchivedAt == nil || got.ArchivedMessage != "superseded by 2.x" {
		t.Fatalf("Get disagrees with Archive result: %+v", got)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM weave_change_set
		WHERE project_id = $1 AND kind = 'release_archived' AND release_version = '1.0.0'
	`, projectID).Scan(&count); err != nil {
		t.Fatalf("count change set: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one release_archived change set row, got %d", count)
	}

	// Second archive call: already archived -> ErrValidation.
	_, err = svc.Archive(authCtx, projectID, "1.0.0", ArchiveInput{Message: "again"})
	var verr *ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ErrValidation on re-archive, got %T: %v", err, err)
	}

	// Archive of a missing version -> ErrNotFound.
	_, err = svc.Archive(authCtx, projectID, "9.9.9", ArchiveInput{Message: "does not matter"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing version, got %T: %v", err, err)
	}

	// Archive with empty message -> ErrValidation.
	if _, err := svc.Create(authCtx, projectID, CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatalf("Create 2.0.0: %v", err)
	}
	_, err = svc.Archive(authCtx, projectID, "2.0.0", ArchiveInput{Message: "  "})
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ErrValidation for empty message, got %T: %v", err, err)
	}
}
