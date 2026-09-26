//go:build integration

package project_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/project"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestCreate_ProvisionsLocalVocabulary proves a new project gets a local
// (hand-authored) vocabulary provisioned and enabled at create time, so
// concept authoring works without a remote authority (#3599).
func TestCreate_ProvisionsLocalVocabulary(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)`, "lvowner", "Owner", "lvowner"); err != nil {
		t.Fatalf("seed actor: %v", err)
	}

	// vocabulary.Service is the LocalVocabularyProvisioner; registry nil is fine
	// (provisioning never touches a connector).
	svc := project.NewService(project.NewPostgresStore(pool), nil, nil, vocabulary.NewService(pool, nil), nil)

	if _, err := svc.Create(ctx, project.CreateInput{
		UIName:   domain.Translations{"en": "Local Default"},
		IDPrefix: "LVP",
		OwnerID:  "lvowner",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Ownership is the project_id column itself now — there is no separate
	// activation join table (#3599 vocabulary ownership; see
	// vocabulary.Service.EnsureLocalVocabulary).
	var connType, status string
	if err := pool.QueryRow(ctx, `
SELECT connector_type, status
FROM weave_vocabularies
WHERE project_id = $1 AND connector_type = 'local'`, "LVP").Scan(&connType, &status); err != nil {
		t.Fatalf("local vocabulary should be provisioned: %v", err)
	}
	if status != string(domain.StatusPublished) {
		t.Fatalf("local vocabulary should be published, got %q", status)
	}
}
