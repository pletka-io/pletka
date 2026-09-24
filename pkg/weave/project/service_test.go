//go:build integration

package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// testPool returns a pgxpool connected to TEST_DATABASE_URL (default:
// pletka_weave on localhost:5433). Skips the test if the database isn't
// reachable. Pool is closed at test end.
func testPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
}

// seedActor inserts a minimal weave_actors row required for owner_id FK.
// Caller is responsible for cleanup.
func seedActor(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	id := ids.GenerateULID()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1, 'person', $2, $2, $2, $3)
	`, id, name, name+"@test.local")
	if err != nil {
		t.Fatalf("seed actor %s: %v", name, err)
	}
	return id
}

// cleanupProject deletes a project and any FK fanout rows that the
// service may have created. Best-effort.
func cleanupProject(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_project_actors WHERE project_id = $1`, id)
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_projects WHERE id = $1`, id)
}

// newService returns a project.Service wired to the given pool. The
// hierarchy reader is nil — Create doesn't need it.
func newService(pool *pgxpool.Pool) *project.Service {
	return project.NewService(project.NewPostgresStore(pool), nil, nil, nil, nil)
}

func TestService_Create_Success(t *testing.T) {
	pool := testPool(t)
	svc := newService(pool)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "projsvc_owner_a")
	t.Cleanup(func() {
		cleanupProject(t, pool, "PSVCA")
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	created, err := svc.Create(ctx, project.CreateInput{
		UIName:   domain.Translations{"en": "Project SVCA"},
		IDPrefix: "PSVCA",
		OwnerID:  ownerID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "PSVCA" {
		t.Errorf("ID = %q, want PSVCA", created.ID)
	}
	if created.Visibility != "private" {
		t.Errorf("Visibility = %q, want private (default)", created.Visibility)
	}
	stored, err := svc.Get(ctx, "PSVCA")
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored project")
	}
	if stored.Visibility != "private" {
		t.Errorf("stored.Visibility = %q, want private", stored.Visibility)
	}
	if created.SystemName != "project-svca" {
		t.Errorf("SystemName = %q, want project-svca (slug from UIName)", created.SystemName)
	}
	if created.Status != domain.StatusDraft {
		t.Errorf("Status = %q, want draft", created.Status)
	}
}

func TestService_Create_NormalisesIDPrefix(t *testing.T) {
	pool := testPool(t)
	svc := newService(pool)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "projsvc_owner_b")
	t.Cleanup(func() {
		cleanupProject(t, pool, "PSVCB")
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	created, err := svc.Create(ctx, project.CreateInput{
		UIName:   domain.Translations{"en": "Project B"},
		IDPrefix: "  psvcb  ", // lowercase + whitespace; should upper-case + trim
		OwnerID:  ownerID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "PSVCB" {
		t.Errorf("ID = %q, want PSVCB (normalised)", created.ID)
	}
}

func TestService_Create_MissingUIName(t *testing.T) {
	pool := testPool(t)
	svc := newService(pool)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "projsvc_owner_c")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	_, err := svc.Create(ctx, project.CreateInput{
		IDPrefix: "PSVCC",
		OwnerID:  ownerID,
	})
	var verr *project.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(verr.Fields["ui_name"]) == 0 {
		t.Errorf("expected ui_name validation error, got %v", verr.Fields)
	}
}

func TestService_Create_BadIDPrefix(t *testing.T) {
	pool := testPool(t)
	svc := newService(pool)
	ctx := context.Background()

	cases := []struct {
		name   string
		prefix string
	}{
		{"empty", ""},
		{"too short", "A"},
		{"too long", "ABCDEFGHIJK"}, // 11 chars
		{"contains digit", "AB1"},
		{"contains punctuation", "AB-CD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, project.CreateInput{
				UIName:   domain.Translations{"en": "x"},
				IDPrefix: tc.prefix,
				OwnerID:  "ignored", // no FK touched — fails before insert
			})
			var verr *project.ErrValidation
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
			if len(verr.Fields["id_prefix"]) == 0 {
				t.Errorf("expected id_prefix error, got %v", verr.Fields)
			}
		})
	}
}

func TestService_Create_DuplicatePrefix(t *testing.T) {
	pool := testPool(t)
	svc := newService(pool)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "projsvc_owner_d")
	t.Cleanup(func() {
		cleanupProject(t, pool, "PSVCD")
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	in := project.CreateInput{
		UIName:   domain.Translations{"en": "first"},
		IDPrefix: "PSVCD",
		OwnerID:  ownerID,
	}
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := svc.Create(ctx, project.CreateInput{
		UIName:   domain.Translations{"en": "second"},
		IDPrefix: "PSVCD",
		OwnerID:  ownerID,
	})
	var verr *project.ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("duplicate Create err = %v, want ErrValidation", err)
	}
	if len(verr.Fields["id_prefix"]) == 0 {
		t.Errorf("expected id_prefix collision error, got %v", verr.Fields)
	}
}
