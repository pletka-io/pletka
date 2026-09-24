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

	var connType, pvStatus string
	if err := pool.QueryRow(ctx, `
SELECT v.connector_type, pv.status
FROM weave_vocabularies v
JOIN weave_project_vocabularies pv ON pv.vocabulary_id = v.id AND pv.project_id = v.project_id
WHERE v.project_id = $1 AND v.connector_type = 'local'`, "LVP").Scan(&connType, &pvStatus); err != nil {
		t.Fatalf("local vocabulary should be provisioned + enabled: %v", err)
	}
	if pvStatus != "active" {
		t.Fatalf("local vocabulary should be active, got %q", pvStatus)
	}
}
