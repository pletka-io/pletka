//go:build integration

package gitmaterializer

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestHydrateProjectSnapshot_PrunesRemovedListsAndVocabularies pins ruling
// I1: a concept list bound to a tree vocabulary and a <P>.VOC.n vocabulary
// that left the tree are deleted on the next load (the vocabulary with its
// entries, edges and bound lists); the remaining list keeps its entries and
// the project's local terms are not touched.
func TestHydrateProjectSnapshot_PrunesRemovedListsAndVocabularies(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	vocabularyTestProject(t, pool, "TQ")
	root := vocabularyFixture(t, "TQ")
	mustWrite(t, root, "concept-lists/TQ.CL.2.yaml", []byte(
		"description: {}\nis_closed: false\nsemantic_id: TQ.CL.2\nstatus: draft\nsystem_name: terms_two\nui_name:\n  en: Two\nvocabulary: TQ.VOC.1\n"))
	mustWrite(t, root, "concept-lists/TQ.CL.2/entries.yaml", []byte("- http://x/1\n"))
	mustWrite(t, root, "vocabularies/TQ.VOC.2/vocabulary.yaml", []byte(
		"base_uri: http://y/\ndescription: {}\nsemantic_id: TQ.VOC.2\nstatus: draft\nsystem_name: tq_candidates\nui_name:\n  en: Candidates\n"))
	mustWrite(t, root, "vocabularies/TQ.VOC.2/concepts.yaml", []byte(
		"- uri: http://y/1\n  label: {en: Y1}\n  top_concept: true\n- uri: http://y/2\n  label: {en: Y2}\n  broader: [http://y/1]\n"))
	mustWrite(t, root, "concept-lists/TQ.CL.3.yaml", []byte(
		"description: {}\nis_closed: false\nsemantic_id: TQ.CL.3\nstatus: draft\nsystem_name: candidates\nui_name:\n  en: Candidates\nvocabulary: TQ.VOC.2\n"))
	mustWrite(t, root, "concept-lists/TQ.CL.3/entries.yaml", []byte("- http://y/2\n"))

	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	q := sqlcgen.New(pool)
	lists, err := q.WeaveListConceptListsForTree(ctx, "TQ")
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 3 {
		t.Fatalf("first load: %d lists, want 3", len(lists))
	}
	local, err := vocabulary.NewService(pool, nil).EnsureLocalVocabulary(ctx, "TQ")
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"concept-lists/TQ.CL.2.yaml", "concept-lists/TQ.CL.2", "concept-lists/TQ.CL.3.yaml", "concept-lists/TQ.CL.3", "vocabularies/TQ.VOC.2"} {
		if err := os.RemoveAll(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatalf("second load: %v", err)
	}
	lists, _ = q.WeaveListConceptListsForTree(ctx, "TQ")
	var ids []string
	for _, l := range lists {
		ids = append(ids, l.ID)
	}
	if !slices.Equal(ids, []string{"TQ.CL.1"}) {
		t.Fatalf("lists after removal %v, want [TQ.CL.1]", ids)
	}
	uris := listEntryURIs(t, q, "TQ.CL.1")
	if !slices.Equal(uris, []string{"http://x/3", "http://x/2"}) {
		t.Fatalf("remaining list entries %v", uris)
	}
	vocabs, _ := q.WeaveListProjectTreeVocabularies(ctx, "TQ")
	if len(vocabs) != 1 || *vocabs[0].SemanticID != "TQ.VOC.1" {
		t.Fatalf("vocabularies after removal %+v, want only TQ.VOC.1", vocabs)
	}
	var orphans int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_vocabulary_entries WHERE uri LIKE 'http://y/%'`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("entries of the removed vocabulary left: %d (%v)", orphans, err)
	}
	var stillLocal int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM weave_vocabularies WHERE id = $1`, local).Scan(&stillLocal); err != nil || stillLocal != 1 {
		t.Fatalf("local terms vocabulary touched: %d (%v)", stillLocal, err)
	}
}

// TestHydrateProjectSnapshot_PruneRefusesReferencedList pins: a list removed
// from the tree while an override in the tree still refers to it fails the
// load naming the list and the override file, and nothing is deleted.
func TestHydrateProjectSnapshot_PruneRefusesReferencedList(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)
	const projectID, ownerID = "TPR", "TPR_OWNER_ACTOR"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_override_refs WHERE override_id IN (SELECT id FROM weave_field_overrides WHERE project_id = $1)`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_concept_lists WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabularies WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{ID: ownerID, Type: "organization", DisplayName: "CL Org", Slug: "cl-org", Visibility: "private"}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	root := conceptListFixture(t, projectID, ownerID)
	rewrite(t, filepath.Join(root, "fields", "TPRF.1", "base-override.yaml"), "refs: []", "refs:\n  - ref_type: concept_list\n    semantic_id: TPR.CL.1\n    position: 0")
	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatalf("first load: %v", err)
	}
	for _, rel := range []string{"concept-lists/TPR.CL.1.yaml", "concept-lists/TPR.CL.1"} {
		if err := os.RemoveAll(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := m.HydrateProjectSnapshot(ctx, root)
	if err == nil || !strings.Contains(err.Error(), "TPR.CL.1") || !strings.Contains(err.Error(), "base-override.yaml") || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("want an error naming the removed list and the override, got %v", err)
	}
	if _, err := queries.WeaveGetProjectConceptList(ctx, sqlcgen.WeaveGetProjectConceptListParams{ProjectID: projectID, ID: "TPR.CL.1"}); err != nil {
		t.Fatalf("list deleted despite the error: %v", err)
	}
}

// TestWriteProjectTree_SkipsForeignListEntries pins ruling I3: an entry of a
// tree-bound list whose concept belongs to another vocabulary (here the
// local terms) is left out of entries.yaml with a warning naming the list
// and the URI; the database row stays.
func TestWriteProjectTree_SkipsForeignListEntries(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	vocabularyTestProject(t, pool, "TF")
	var logs bytes.Buffer
	m := NewMaterializer(pool, t.TempDir(), slog.New(slog.NewTextHandler(&logs, nil)))
	if _, err := m.HydrateProjectSnapshot(ctx, vocabularyFixture(t, "TF")); err != nil {
		t.Fatal(err)
	}
	localID, err := vocabulary.NewService(pool, nil).EnsureLocalVocabulary(ctx, "TF")
	if err != nil {
		t.Fatal(err)
	}
	local, err := q.WeaveCreateVocabularyEntry(ctx, sqlcgen.WeaveCreateVocabularyEntryParams{ID: "TF-local-term", VocabularyID: localID, Uri: "pletka:concept/tf-1", Label: []byte(`{"en":"Local"}`), BroaderPath: []byte(`[]`), BroaderPathItems: []byte(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.WeaveCreateConceptListEntry(ctx, sqlcgen.WeaveCreateConceptListEntryParams{ID: "TF-local-entry", ConceptListID: "TF.CL.1", VocabularyEntryID: local.ID, Position: 9}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := m.writeProjectTree(ctx, dir, "TF"); err != nil {
		t.Fatalf("writeProjectTree: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "concept-lists", "TF.CL.1", "entries.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "- http://x/3\n- http://x/2\n" {
		t.Fatalf("entries.yaml:\n%s\nwant only the vocabulary's own concepts", got)
	}
	if !strings.Contains(logs.String(), "TF.CL.1") || !strings.Contains(logs.String(), "pletka:concept/tf-1") {
		t.Fatalf("no warning naming the list and the foreign URI; log:\n%s", logs.String())
	}
}

// listEntryURIs is a concept list's entry URIs in order.
func listEntryURIs(t *testing.T, q *sqlcgen.Queries, listID string) []string {
	t.Helper()
	rows, err := q.WeaveListConceptListEntryURIs(context.Background(), listID)
	if err != nil {
		t.Fatal(err)
	}
	uris := make([]string, 0, len(rows))
	for _, r := range rows {
		uris = append(uris, r.Uri)
	}
	return uris
}
