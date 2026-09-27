//go:build integration

package field

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// TestServiceGetByIdentifierHonoursThePinnedVersion pins the SERVICE branch,
// not the store method. The store can read the archive correctly and the bug
// still be present, because the bug was that GetByIdentifier never asked it
// to — Get had the version branch and GetByIdentifier did not, and the MCP
// surface reaches a field through the latter (Redmine #3610).
//
// Deliberately separate from the store test: reverting the service branch
// while leaving the store method in place must fail this and pass that.
func TestServiceGetByIdentifierHonoursThePinnedVersion(t *testing.T) {
	pool := batchUsageRefsTestPool(t)
	ctx := context.Background()
	const (
		projectID = "IDVERSVC"
		version   = "1.0.0"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO weave_fields (id, project_id, semantic_id, system_name, ui_name, expected_value_type, status)
		VALUES ('idversvc_f', $1, 'IDVERSVCF.1', 'svc_field', '{"en":"Svc"}'::jsonb, 'Model', 'published')`, projectID); err != nil {
		t.Fatalf("seed live field: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_fields_archive (id, project_id, semantic_id, system_name, ui_name, expected_value_type, status, version_number)
		VALUES ('idversvc_f', $1, 'IDVERSVCF.1', 'svc_field', '{"en":"Svc"}'::jsonb, 'String', 'published', $2)`, projectID, version); err != nil {
		t.Fatalf("seed archived field: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields_archive WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
	})

	svc := newIdentifierVersionTestService(t, pool)

	t.Run("a pinned context reads the archive", func(t *testing.T) {
		got, err := svc.GetByIdentifier(auth.WithProjectVersion(ctx, version), projectID, "IDVERSVCF.1")
		if err != nil {
			t.Fatalf("GetByIdentifier: %v", err)
		}
		if got == nil {
			t.Fatal("field not found")
		}
		if got.ExpectedValueType != "String" {
			t.Errorf("expected_value_type = %q, want %q — the service ignored the pinned version and returned the draft", got.ExpectedValueType, "String")
		}
	})

	t.Run("an unpinned context still reads live", func(t *testing.T) {
		got, err := svc.GetByIdentifier(ctx, projectID, "IDVERSVCF.1")
		if err != nil {
			t.Fatalf("GetByIdentifier: %v", err)
		}
		if got == nil || got.ExpectedValueType != "Model" {
			t.Errorf("unversioned read = %v, want the live Model row", got)
		}
	})
}

// newIdentifierVersionTestService wires the real postgres store behind the
// service, with a project reader that grants read access — the gate under
// test is the version branch, not authorization.
func newIdentifierVersionTestService(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()
	return NewService(NewPostgresStore(pool), nil, nil, nil, alwaysReadableProjects{}, nil, nil, nil, nil, nil)
}

// alwaysReadableProjects satisfies ProjectReader for a public project, so
// requireProjectRead passes without seeding an actor.
type alwaysReadableProjects struct{}

func (alwaysReadableProjects) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	return &domain.Project{Entity: domain.Entity{ID: id}, Visibility: domain.VisibilityPublic}, nil
}
