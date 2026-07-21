//go:build integration

package release

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

func releaseTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:pw123@localhost:5433/pletka_weave?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestCreate_BlocksDraftParentDependencies(t *testing.T) {
	ctx := context.Background()
	pool := releaseTestPool(t)
	queries := sqlcgen.New(pool)

	ownerID := "TEST_RELEASE_OWNER"
	projectID := "TEST_RELEASE_CHILD"
	parentID := "TEST_RELEASE_PARENT"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_releases WHERE project_id LIKE 'TEST_RELEASE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_categories WHERE project_id LIKE 'TEST_RELEASE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_RELEASE_%' OR parent_project_id LIKE 'TEST_RELEASE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_RELEASE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "Release Org",
		Slug:        "release-org",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create actor: %v", err)
	}

	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:         parentID,
		UiName:     []byte(`{"en":"Parent"}`),
		Status:     "draft",
		OwnerID:    ownerID,
		Visibility: "private",
	}); err != nil {
		t.Fatalf("create parent project: %v", err)
	}
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:         projectID,
		UiName:     []byte(`{"en":"Child"}`),
		Status:     "draft",
		OwnerID:    ownerID,
		Visibility: "private",
	}); err != nil {
		t.Fatalf("create child project: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, canonical_order, source_mode, source_version)
		VALUES ($1, $2, true, 0, 'draft', NULL)
	`, projectID, parentID); err != nil {
		t.Fatalf("create inheritance: %v", err)
	}
	if _, err := queries.WeaveCreateCategory(ctx, sqlcgen.WeaveCreateCategoryParams{
		ID:             "TEST_RELEASE_CAT",
		SemanticID:     strPtr("TEST_RELEASE.CAT.1"),
		SystemName:     strPtr("release-cat"),
		UiName:         []byte(`{"en":"Release Cat"}`),
		Description:    []byte(`{"en":"Release category"}`),
		Status:         "draft",
		ProjectID:      projectID,
		CanonicalOrder: 0,
	}); err != nil {
		t.Fatalf("create category: %v", err)
	}

	authCtx := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	authCtx = weaveauth.WithPrincipal(authCtx, &weaveauth.Principal{ActorID: ownerID})

	svc := NewService(nil, pool, nil)
	_, err := svc.Create(authCtx, projectID, CreateInput{Version: "1.0.0"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verr *ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("expected ErrValidation, got %T: %v", err, err)
	}
	msgs := verr.Fields["parent_dependencies"]
	if len(msgs) != 1 {
		t.Fatalf("expected parent_dependencies validation, got %#v", verr.Fields)
	}
	if got := msgs[0]; got == "" || !containsAll(got, "follow draft", parentID) {
		t.Fatalf("unexpected validation message: %q", got)
	}
}

func strPtr(v string) *string { return &v }

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
