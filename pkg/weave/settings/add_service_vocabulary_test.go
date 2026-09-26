//go:build integration

package settings

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// TestAddServiceVocabularyWritesTheRow pins what "enabling" means after this
// work: a project's vocabulary row IS the enablement, carrying the mount name
// in its config, and removing it takes the row and its cached entries.
func TestAddServiceVocabularyWritesTheRow(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const (
		ownerID   = "tstadd_owner"
		projectID = "TSTADD"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, ownerID, "TSTADD Owner", ownerID); err != nil {
		t.Fatalf("seed owner actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, projectID, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabularies WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	store := NewPostgresStore(pool)
	if err := store.AddServiceVocabulary(ctx, projectID, "aat", "nl"); err != nil {
		t.Fatalf("add: %v", err)
	}

	var connectorType, config string
	if err := pool.QueryRow(ctx,
		`SELECT connector_type, config::text FROM weave_vocabularies WHERE project_id = $1`,
		projectID).Scan(&connectorType, &config); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if connectorType != "vocabservice" {
		t.Errorf("connector_type = %q, want \"vocabservice\"", connectorType)
	}
	if !strings.Contains(config, `"vocab": "aat"`) && !strings.Contains(config, `"vocab":"aat"`) {
		t.Errorf("config = %s, want it to name the aat mount", config)
	}

	// Review Focus 4: a second add of the same mount is rejected, not silently
	// duplicated — two rows naming one mount is coherent but useless. Assert
	// the mapped shape, not merely "an error": if the constraint-name match
	// in AddServiceVocabulary ever stopped matching (a renamed index, a
	// Postgres version quirk), the code falls through to a generic wrapped
	// error, apierror.FromError would map that to a plain 500, and a bare
	// `err == nil` check would not notice. Going through apierror.FromError
	// — the same call the handler makes — pins the actual "clean 409, not a
	// raw constraint failure" requirement.
	err := store.AddServiceVocabulary(ctx, projectID, "aat", "nl")
	if err == nil {
		t.Fatal("adding the same mount twice must be rejected")
	}
	if ae := apierror.FromError(err); ae.Status != http.StatusConflict || ae.Code != apierror.CodeConflict {
		t.Errorf("mapped error = status %d code %q, want 409/conflict (got: %v)", ae.Status, ae.Code, err)
	}
}

// TestRemoveVocabularyDeletesTheRowAndCascadesEntries pins the other half:
// removing IS deleting the row, and the entries cache — which re-resolves
// from the service — goes with it via
// weave_vocabulary_entries_vocabulary_id_fkey's ON DELETE CASCADE, with no
// explicit entry delete in RemoveVocabulary.
func TestRemoveVocabularyDeletesTheRowAndCascadesEntries(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const (
		ownerID   = "tstrem_owner"
		projectID = "TSTREM"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, ownerID, "TSTREM Owner", ownerID); err != nil {
		t.Fatalf("seed owner actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, projectID, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabularies WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	store := NewPostgresStore(pool)
	if err := store.AddServiceVocabulary(ctx, projectID, "fish-monument-type", ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	var vocabID string
	if err := pool.QueryRow(ctx, `SELECT id FROM weave_vocabularies WHERE project_id = $1`, projectID).Scan(&vocabID); err != nil {
		t.Fatalf("read back vocabulary id: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ($1, $2, 'urn:tstrem:1', '{"en":"Term"}'::jsonb)`, "entry_tstrem", vocabID); err != nil {
		t.Fatalf("seed vocabulary entry: %v", err)
	}

	if err := store.RemoveVocabulary(ctx, projectID, vocabID); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var vocabCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_vocabularies WHERE id = $1`, vocabID).Scan(&vocabCount); err != nil {
		t.Fatalf("count vocabulary: %v", err)
	}
	if vocabCount != 0 {
		t.Errorf("vocabulary row = %d, want removed", vocabCount)
	}
	var entryCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_vocabulary_entries WHERE vocabulary_id = $1`, vocabID).Scan(&entryCount); err != nil {
		t.Fatalf("count vocabulary entries: %v", err)
	}
	if entryCount != 0 {
		t.Errorf("vocabulary entries = %d, want cascaded away with the row", entryCount)
	}
}

// TestRemoveVocabularyBoundToConceptListIsInUse pins the fix for review
// item 1: weave_concept_lists.vocabulary_id has no ON DELETE clause
// (RESTRICT, unlike the entries FK), so a vocabulary a concept list still
// points at cannot simply be deleted. RemoveVocabulary must turn that
// foreign-key violation into a typed in-use error rather than a bare wrapped
// error, and the assertion goes through apierror.FromError — the same call
// DeleteVocabulary makes — so it pins the actual 409/in_use response, not
// just "some error came back".
func TestRemoveVocabularyBoundToConceptListIsInUse(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const (
		ownerID       = "tstinu_owner"
		projectID     = "TSTINU"
		conceptListID = "cl_tstinu"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, ownerID, "TSTINU Owner", ownerID); err != nil {
		t.Fatalf("seed owner actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, projectID, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_concept_lists WHERE id = $1`, conceptListID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabularies WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	store := NewPostgresStore(pool)
	if err := store.AddServiceVocabulary(ctx, projectID, "aat", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	var vocabID string
	if err := pool.QueryRow(ctx, `SELECT id FROM weave_vocabularies WHERE project_id = $1`, projectID).Scan(&vocabID); err != nil {
		t.Fatalf("read back vocabulary id: %v", err)
	}

	// A curator later points a concept list at this project-owned
	// vocabulary — the design doc's normal case, reachable today because
	// validateVocabularyInProject accepts any project-scoped vocabulary
	// regardless of connector_type.
	if _, err := pool.Exec(ctx, `INSERT INTO weave_concept_lists (id, project_id, vocabulary_id) VALUES ($1, $2, $3)`,
		conceptListID, projectID, vocabID); err != nil {
		t.Fatalf("seed concept list: %v", err)
	}

	err := store.RemoveVocabulary(ctx, projectID, vocabID)
	if err == nil {
		t.Fatal("removing a vocabulary a concept list still points at must be rejected")
	}
	if ae := apierror.FromError(err); ae.Status != http.StatusConflict || ae.Code != apierror.CodeInUse {
		t.Errorf("mapped error = status %d code %q, want 409/in_use (got: %v)", ae.Status, ae.Code, err)
	}

	var vocabCount int
	if scanErr := pool.QueryRow(ctx, `SELECT count(*) FROM weave_vocabularies WHERE id = $1`, vocabID).Scan(&vocabCount); scanErr != nil {
		t.Fatalf("count vocabulary: %v", scanErr)
	}
	if vocabCount != 1 {
		t.Errorf("vocabulary row = %d, want left in place after a blocked delete", vocabCount)
	}
}
