//go:build integration

package vocabulary

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestMigrationDropsTheGlobalTier asserts the state migration 013 leaves
// behind. The test runs against an already-migrated fixture database, so it
// checks the END state rather than running goose itself: no global
// vocabularies, no join table, project_id NOT NULL, and — the half that
// matters most — that the schema still lets a project-owned vocabulary and
// its own entries live normally after the migration.
func TestMigrationDropsTheGlobalTier(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	var globals int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM weave_vocabularies WHERE project_id IS NULL`).Scan(&globals); err != nil {
		t.Fatalf("count global vocabularies: %v", err)
	}
	if globals != 0 {
		t.Errorf("global vocabularies = %d, want 0", globals)
	}

	var joinTable int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema='public' AND table_name='weave_project_vocabularies'`).Scan(&joinTable); err != nil {
		t.Fatalf("check join table: %v", err)
	}
	if joinTable != 0 {
		t.Errorf("weave_project_vocabularies still exists, want dropped")
	}

	var nullable string
	if err := pool.QueryRow(ctx,
		`SELECT is_nullable FROM information_schema.columns
		 WHERE table_name='weave_vocabularies' AND column_name='project_id'`).Scan(&nullable); err != nil {
		t.Fatalf("check project_id nullability: %v", err)
	}
	if nullable != "NO" {
		t.Errorf("project_id is_nullable = %q, want \"NO\"", nullable)
	}

	// A project-owned vocabulary with its own entry has to keep working after
	// the migration: seed one, an entry on it, and read both back.
	const (
		ownerID   = "tstmig_owner"
		projectID = "TSTMIG"
		vocabID   = "vocab_tstmig"
		entryID   = "entry_tstmig"
	)
	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, ownerID, "TSTMIG Owner", ownerID); err != nil {
		t.Fatalf("seed owner actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, projectID, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, status)
		VALUES ($1,$2,'tstmig','local','draft') ON CONFLICT (id) DO NOTHING`, vocabID, projectID); err != nil {
		t.Fatalf("seed project vocabulary: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ($1,$2,'urn:tstmig:1','{"en":"Term"}'::jsonb) ON CONFLICT (id) DO NOTHING`, entryID, vocabID); err != nil {
		t.Fatalf("seed vocabulary entry: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabulary_entries WHERE id = $1`, entryID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabularies WHERE id = $1`, vocabID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	var gotVocabProject string
	if err := pool.QueryRow(ctx, `SELECT project_id FROM weave_vocabularies WHERE id = $1`, vocabID).Scan(&gotVocabProject); err != nil {
		t.Fatalf("read back project vocabulary: %v", err)
	}
	if gotVocabProject != projectID {
		t.Errorf("survivor vocabulary project_id = %q, want %q", gotVocabProject, projectID)
	}
	var entryCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_vocabulary_entries WHERE id = $1 AND vocabulary_id = $2`,
		entryID, vocabID).Scan(&entryCount); err != nil {
		t.Fatalf("read back vocabulary entry: %v", err)
	}
	if entryCount != 1 {
		t.Errorf("survivor vocabulary entry count = %d, want 1", entryCount)
	}

	// Review Focus 5: a concept list whose source vocabulary was nulled.
	// Nothing produced that state before this migration, so nothing pinned it.
	var nullSourceOK bool
	if err := pool.QueryRow(ctx,
		`SELECT true FROM information_schema.columns
		 WHERE table_name='weave_concept_lists' AND column_name='vocabulary_id' AND is_nullable='YES'`).Scan(&nullSourceOK); err != nil {
		t.Fatalf("concept_lists.vocabulary_id must remain nullable: %v", err)
	}
}

// TestMigrationBoundary_ProjectOwnedDataSurvives replays migration 013's
// destructive statements, verbatim, against a fabricated pre-migration
// dataset holding both a global vocabulary (to be destroyed) and a
// project-owned one (the survivor) wired the same way: an entry, a concept
// list bound to the vocabulary as its source, and a concept list using one of
// the vocabulary's entries as its list_type, plus a concept-list-entry
// membership row. It runs inside a transaction that is always rolled back, so
// nothing it does is visible outside this test — the shared fixture clone (and
// its already-applied migration 013) is untouched.
//
// This is the half of the migration test that a plain end-state check cannot
// give: proof that the destructive statements, when applied to data shaped
// like the pre-migration world, stop exactly at project ownership and do not
// take the survivor's vocabulary, entry, or concept-list wiring with them.
func TestMigrationBoundary_ProjectOwnedDataSurvives(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("exec failed: %v\nSQL: %s", err, sql)
		}
	}

	// Recreate the pre-013 shape inside the transaction only: a nullable
	// project_id and the join table migration 013 drops.
	exec(`ALTER TABLE weave_vocabularies ALTER COLUMN project_id DROP NOT NULL`)
	exec(`CREATE TABLE weave_project_vocabularies (
		project_id text NOT NULL,
		vocabulary_id text NOT NULL,
		selected_version text,
		status text NOT NULL DEFAULT 'active',
		created_at timestamptz NOT NULL DEFAULT NOW(),
		updated_at timestamptz NOT NULL DEFAULT NOW(),
		PRIMARY KEY (project_id, vocabulary_id)
	)`)

	const (
		ownerID   = "tstbnd_owner"
		projectID = "TSTBND"

		survivorVocab = "vocab_tstbnd_own"
		survivorEntry = "entry_tstbnd_own"
		survivorList  = "list_tstbnd_own"
		survivorCLE   = "cle_tstbnd_own"

		globalVocab = "vocab_tstbnd_glob"
		globalEntry = "entry_tstbnd_glob"
		globalList  = "list_tstbnd_glob"
		globalCLE   = "cle_tstbnd_glob"
	)

	exec(`INSERT INTO weave_actors (id, display_name, slug) VALUES ($1,$2,$3)`, ownerID, "TSTBND Owner", ownerID)
	exec(`INSERT INTO weave_projects (id, owner_id) VALUES ($1,$2)`, projectID, ownerID)

	// The survivor: entirely project-owned.
	exec(`INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, status)
		VALUES ($1,$2,'tstbnd_own','local','draft')`, survivorVocab, projectID)
	exec(`INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ($1,$2,'urn:tstbnd:own','{"en":"Survivor term"}'::jsonb)`, survivorEntry, survivorVocab)
	exec(`INSERT INTO weave_concept_lists (id, project_id, vocabulary_id, list_type)
		VALUES ($1,$2,$3,$4)`, survivorList, projectID, survivorVocab, survivorEntry)
	exec(`INSERT INTO weave_concept_list_entries (id, concept_list_id, vocabulary_entry_id)
		VALUES ($1,$2,$3)`, survivorCLE, survivorList, survivorEntry)

	// The global side: what the migration must remove or null out.
	exec(`INSERT INTO weave_vocabularies (id, system_name, connector_type, status)
		VALUES ($1,'tstbnd_glob','local','draft')`, globalVocab)
	exec(`INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ($1,$2,'urn:tstbnd:glob','{"en":"Global term"}'::jsonb)`, globalEntry, globalVocab)
	exec(`INSERT INTO weave_concept_lists (id, project_id, vocabulary_id, list_type)
		VALUES ($1,$2,$3,$4)`, globalList, projectID, globalVocab, globalEntry)
	exec(`INSERT INTO weave_concept_list_entries (id, concept_list_id, vocabulary_entry_id)
		VALUES ($1,$2,$3)`, globalCLE, globalList, globalEntry)
	exec(`INSERT INTO weave_project_vocabularies (project_id, vocabulary_id, status)
		VALUES ($1,$2,'active')`, projectID, globalVocab)

	// Replay migration 013's Up section, verbatim.
	exec(`UPDATE weave_concept_lists SET vocabulary_id = NULL
		WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id IS NULL)`)
	exec(`UPDATE weave_concept_lists SET list_type = NULL
		WHERE list_type IN (
			SELECT e.id FROM weave_vocabulary_entries e
			JOIN weave_vocabularies v ON v.id = e.vocabulary_id
			WHERE v.project_id IS NULL
		)`)
	exec(`DELETE FROM weave_concept_list_entries
		WHERE vocabulary_entry_id IN (
			SELECT e.id FROM weave_vocabulary_entries e
			JOIN weave_vocabularies v ON v.id = e.vocabulary_id
			WHERE v.project_id IS NULL
		)`)
	exec(`DELETE FROM weave_vocabulary_entries
		WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id IS NULL)`)
	exec(`DELETE FROM weave_project_vocabularies
		WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id IS NULL)`)
	exec(`DELETE FROM weave_vocabularies WHERE project_id IS NULL`)
	exec(`DROP TABLE IF EXISTS weave_project_vocabularies`)
	exec(`ALTER TABLE weave_vocabularies ALTER COLUMN project_id SET NOT NULL`)

	// The global side must be gone.
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM weave_vocabularies WHERE id = $1`, globalVocab).Scan(&count); err != nil {
		t.Fatalf("count global vocabulary: %v", err)
	}
	if count != 0 {
		t.Errorf("global vocabulary survived, want deleted")
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM weave_vocabulary_entries WHERE id = $1`, globalEntry).Scan(&count); err != nil {
		t.Fatalf("count global entry: %v", err)
	}
	if count != 0 {
		t.Errorf("global vocabulary entry survived, want deleted")
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM weave_concept_list_entries WHERE id = $1`, globalCLE).Scan(&count); err != nil {
		t.Fatalf("count global concept list entry: %v", err)
	}
	if count != 0 {
		t.Errorf("global concept list entry survived, want deleted")
	}
	var globalListVocab, globalListType *string
	if err := tx.QueryRow(ctx, `SELECT vocabulary_id, list_type FROM weave_concept_lists WHERE id = $1`, globalList).
		Scan(&globalListVocab, &globalListType); err != nil {
		t.Fatalf("read global concept list: %v", err)
	}
	if globalListVocab != nil {
		t.Errorf("global concept list vocabulary_id = %v, want NULL", *globalListVocab)
	}
	if globalListType != nil {
		t.Errorf("global concept list list_type = %v, want NULL", *globalListType)
	}

	// The survivor must be untouched.
	var survivorVocabProject string
	if err := tx.QueryRow(ctx, `SELECT project_id FROM weave_vocabularies WHERE id = $1`, survivorVocab).Scan(&survivorVocabProject); err != nil {
		t.Fatalf("read survivor vocabulary: %v", err)
	}
	if survivorVocabProject != projectID {
		t.Errorf("survivor vocabulary project_id = %q, want %q", survivorVocabProject, projectID)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM weave_vocabulary_entries WHERE id = $1 AND vocabulary_id = $2`,
		survivorEntry, survivorVocab).Scan(&count); err != nil {
		t.Fatalf("count survivor entry: %v", err)
	}
	if count != 1 {
		t.Errorf("survivor vocabulary entry missing, want 1 row")
	}
	var survivorListVocab, survivorListType *string
	if err := tx.QueryRow(ctx, `SELECT vocabulary_id, list_type FROM weave_concept_lists WHERE id = $1`, survivorList).
		Scan(&survivorListVocab, &survivorListType); err != nil {
		t.Fatalf("read survivor concept list: %v", err)
	}
	if survivorListVocab == nil || *survivorListVocab != survivorVocab {
		t.Errorf("survivor concept list vocabulary_id = %v, want %q", survivorListVocab, survivorVocab)
	}
	if survivorListType == nil || *survivorListType != survivorEntry {
		t.Errorf("survivor concept list list_type = %v, want %q", survivorListType, survivorEntry)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM weave_concept_list_entries WHERE id = $1`, survivorCLE).Scan(&count); err != nil {
		t.Fatalf("count survivor concept list entry: %v", err)
	}
	if count != 1 {
		t.Errorf("survivor concept list entry missing, want 1 row")
	}
}
