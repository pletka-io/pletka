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

// TestRemoveVocabularyRetiresItAndKeepsWhatWasBuiltFromIt pins the other
// half, and it is deliberately the opposite of what this test asserted
// before. Removing used to delete the row and let the entries cascade away;
// the owner settled the semantics on 2026-09-26 as "the control lists remain,
// you just can't add to it anymore".
//
// A delete cannot express that. weave_vocabulary_entries cascades from
// weave_vocabularies, so deleting empties every list built on the vocabulary,
// and weave_concept_lists.vocabulary_id carries no ON DELETE action, so
// Postgres refuses the delete outright while a list still points at the row.
// Removal is therefore a deprecation.
func TestRemoveVocabularyRetiresItAndKeepsWhatWasBuiltFromIt(t *testing.T) {
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

	var deprecated bool
	if err := pool.QueryRow(ctx, `SELECT deprecated FROM weave_vocabularies WHERE id = $1`, vocabID).Scan(&deprecated); err != nil {
		t.Fatalf("read back vocabulary: %v", err)
	}
	if !deprecated {
		t.Error("vocabulary is not deprecated after removal")
	}

	var entryCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_vocabulary_entries WHERE vocabulary_id = $1`, vocabID).Scan(&entryCount); err != nil {
		t.Fatalf("count vocabulary entries: %v", err)
	}
	if entryCount != 1 {
		t.Errorf("vocabulary entries = %d, want 1 kept — removal must not unmake what was pinned from it", entryCount)
	}
}

// TestRemoveVocabularyBoundToConceptListSucceeds is the case the previous
// behaviour got backwards. A delete had to be refused with in_use whenever a
// concept list pointed at the vocabulary — so the one situation where a
// curator most wants to stop new pins, a source they have already built on,
// was the one situation they could not act on. Retiring the row has no such
// conflict.
func TestRemoveVocabularyBoundToConceptListSucceeds(t *testing.T) {
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

	if _, err := pool.Exec(ctx, `INSERT INTO weave_concept_lists (id, project_id, vocabulary_id) VALUES ($1, $2, $3)`,
		conceptListID, projectID, vocabID); err != nil {
		t.Fatalf("seed concept list: %v", err)
	}

	if err := store.RemoveVocabulary(ctx, projectID, vocabID); err != nil {
		t.Fatalf("removing a vocabulary a concept list points at must succeed: %v", err)
	}

	// The list keeps its source: it still resolves the entries it pinned,
	// and the read-only behaviour is enforced in the vocabulary slice
	// (TestRemovedVocabularyIsReadOnly), not by severing the reference here.
	var boundVocab string
	if err := pool.QueryRow(ctx, `SELECT vocabulary_id FROM weave_concept_lists WHERE id = $1`, conceptListID).Scan(&boundVocab); err != nil {
		t.Fatalf("read back concept list: %v", err)
	}
	if boundVocab != vocabID {
		t.Errorf("concept list vocabulary_id = %q, want %q kept", boundVocab, vocabID)
	}
}

// TestAddingARemovedVocabularyRevivesIt closes a one-way door found by
// clicking it on alpha: removal deprecates the row rather than deleting it,
// so the row keeps holding (project_id, system_name) in migration 014's
// partial unique index. Before the upsert, re-adding a mount the project had
// removed failed with 409 "already added to this project" — an error that
// also contradicted the screen, which showed it as removed.
//
// Reviving is also the behaviour a curator would expect: the entries cached
// under that row come back with it, so terms they had pinned are still there.
func TestAddingARemovedVocabularyRevivesIt(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const (
		ownerID   = "tstrev_owner"
		projectID = "TSTREV"
	)

	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, ownerID, "TSTREV Owner", ownerID); err != nil {
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
	if err := store.AddServiceVocabulary(ctx, projectID, "aat", ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	var vocabID string
	if err := pool.QueryRow(ctx, `SELECT id FROM weave_vocabularies WHERE project_id = $1`, projectID).Scan(&vocabID); err != nil {
		t.Fatalf("read back vocabulary id: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ($1, $2, 'urn:tstrev:1', '{"en":"Pinned"}'::jsonb)`, "entry_tstrev", vocabID); err != nil {
		t.Fatalf("seed vocabulary entry: %v", err)
	}

	// Adding a vocabulary that is genuinely still here is a conflict.
	if err := store.AddServiceVocabulary(ctx, projectID, "aat", ""); err == nil {
		t.Fatal("adding a live vocabulary again must conflict")
	} else if ae := apierror.FromError(err); ae.Status != http.StatusConflict {
		t.Errorf("mapped error = status %d, want 409 (got: %v)", ae.Status, err)
	}

	if err := store.RemoveVocabulary(ctx, projectID, vocabID); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// ...and adding it after removal brings the same row back.
	if err := store.AddServiceVocabulary(ctx, projectID, "aat", ""); err != nil {
		t.Fatalf("re-adding a removed vocabulary must succeed, not conflict: %v", err)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_vocabularies WHERE project_id = $1`, projectID).Scan(&rows); err != nil {
		t.Fatalf("count vocabularies: %v", err)
	}
	if rows != 1 {
		t.Errorf("vocabulary rows = %d, want 1 — reviving must not insert a duplicate", rows)
	}

	var deprecated bool
	if err := pool.QueryRow(ctx, `SELECT deprecated FROM weave_vocabularies WHERE id = $1`, vocabID).Scan(&deprecated); err != nil {
		t.Fatalf("read back vocabulary: %v", err)
	}
	if deprecated {
		t.Error("vocabulary is still deprecated after being re-added")
	}

	var entries int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_vocabulary_entries WHERE vocabulary_id = $1`, vocabID).Scan(&entries); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if entries != 1 {
		t.Errorf("entries = %d, want the pinned one to come back with the row", entries)
	}
}
