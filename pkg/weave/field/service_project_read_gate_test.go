//go:build integration

package field

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// TestRequireProjectRead_ConstructorGating guards the swap in
// (*Service).requireProjectRead from a hand-built auth.Resource literal to
// auth.ProjectResource(project) (audit L1 / M3): org-inherited read access
// must keep working (OrgID was already threaded through pre-swap), and a
// project with empty visibility must now be treated as private — the
// intended M3 behavior flip. The pre-swap code defaulted an empty
// project.Visibility to "public"; auth.ProjectResource defaults it to
// "private".
func TestRequireProjectRead_ConstructorGating(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const projectID = "FLDGATE"
	// "unite" is a fixture organization actor (Type: "organization") seeded
	// for every package's testdb clone — see internal/testdb/fixture_identities.go.
	_, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, 'unite', 'private')
		ON CONFLICT (id) DO NOTHING`, projectID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_projects WHERE id=$1`, projectID)
	})

	svc := NewService(nil, nil, nil, nil, poolProjectReader{pool: pool}, nil, nil, nil, nil, nil)

	// A user whose ONLY route to this project is an org-level role — no
	// "project:<id>" entry. requireProjectRead must still resolve this via
	// OrgID, exactly as it did before the constructor swap.
	orgMember := auth.WithSnapshot(ctx, &auth.AuthSnapshot{
		ActorID: "org-inherited-user",
		Roles:   map[string]string{"org:unite": "owner"},
	})
	if err := svc.requireProjectRead(orgMember, projectID); err != nil {
		t.Fatalf("org-inherited owner denied requireProjectRead: %v", err)
	}

	// Empty-visibility case: weave_projects_visibility_check forbids ''
	// at the DB layer (only public/internal/private are allowed), so this
	// half can't be seeded via a real row. Exercise the same
	// requireProjectRead/auth.ProjectResource code path against a fake
	// ProjectReader that returns a project with Visibility="" directly,
	// bypassing the DB constraint but not the auth logic under test.
	emptyVisSvc := NewService(nil, nil, nil, nil, fakeProjectReader{
		project: &domain.Project{Entity: domain.Entity{ID: projectID}, OwnerID: "unite", Visibility: ""},
	}, nil, nil, nil, nil, nil)

	anon := auth.WithSnapshot(context.Background(), &auth.AuthSnapshot{IsAnonymous: true})
	err = emptyVisSvc.requireProjectRead(anon, projectID)
	if err == nil {
		t.Fatal("expected anonymous read of an empty-visibility project to be denied (M3: empty visibility now defaults to private, not public)")
	}
	var forbidden *ErrForbidden
	if !errors.As(err, &forbidden) {
		t.Fatalf("expected *ErrForbidden, got %T: %v", err, err)
	}
}

type fakeProjectReader struct {
	project *domain.Project
}

func (f fakeProjectReader) GetByID(context.Context, string) (*domain.Project, error) {
	return f.project, nil
}

// poolProjectReader is a minimal ProjectReader backed directly by a raw SQL
// lookup against weave_projects. field cannot import pkg/weave/project (or
// pkg/weave) for this: both import back into pkg/weave/field, which would
// create an import cycle for this test's build.
type poolProjectReader struct {
	pool *pgxpool.Pool
}

func (r poolProjectReader) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	var p domain.Project
	err := r.pool.QueryRow(ctx, `SELECT id, owner_id, visibility FROM weave_projects WHERE id=$1`, id).
		Scan(&p.ID, &p.OwnerID, &p.Visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
