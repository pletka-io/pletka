//go:build integration

package gitmaterializer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestWriteProjectTree_ConceptLists pins that a project with a concept list
// materializes concept-lists/<SEMID>.yaml with the canonical keys.
func TestWriteProjectTree_ConceptLists(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)
	const (
		ownerID   = "CLTREE_OWNER_ACTOR"
		projectID = "CLTREE"
		listID    = "CLTREE.CL.1"
	)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_concept_lists WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{ID: ownerID, Type: "organization", DisplayName: "CL Org", Slug: "cl-org", Visibility: "private"}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{ID: projectID, UiName: []byte(`{"en":"CL"}`), Description: []byte(`{}`), Status: "draft", OwnerID: ownerID, Visibility: "private"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	sem, sys := listID, "license"
	if _, err := queries.WeaveCreateConceptList(ctx, sqlcgen.WeaveCreateConceptListParams{ID: listID, SemanticID: &sem, SystemName: &sys, UiName: []byte(`{"en":"License"}`), Description: []byte(`{}`), Status: "draft", ProjectID: projectID}); err != nil {
		t.Fatalf("create concept list: %v", err)
	}
	m := NewMaterializer(pool, t.TempDir(), nil)
	dir := t.TempDir()
	if err := m.writeProjectTree(ctx, dir, projectID); err != nil {
		t.Fatalf("writeProjectTree: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "concept-lists", listID+".yaml"))
	if err != nil {
		t.Fatalf("concept list file not written: %v", err)
	}
	for _, want := range []string{"semantic_id: " + listID, "system_name: license", "is_closed: false"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("payload lacks %q:\n%s", want, b)
		}
	}
}

// canonicalVocabularyFixture is vocabularyFixture rewritten in canonical
// form, so a tree written back from the database can be compared byte for
// byte. It covers every concepts.yaml key: <P>.VOC.1 holds a top concept with
// external_id and exact_match (1), a concept with scope_note, exact_match and
// no external_id (2) and a polyhierarchical concept (3). Lists: <P>.CL.1 on
// the vocabulary with entries [3, 2]; <P>.CL.2 without a vocabulary key (the
// test binds it to the project's local terms); <P>.CL.3 on the vocabulary
// without entries, so without entries.yaml.
func canonicalVocabularyFixture(t *testing.T, projectID string) string {
	t.Helper()
	root := vocabularyFixture(t, projectID)
	vocID := projectID + ".VOC.1"
	write := func(rel string, payload []byte, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, root, rel, payload)
	}
	payload, err := canonical.Vocabulary(&domain.Vocabulary{
		Entity:  domain.Entity{SemanticID: vocID, SystemName: strings.ToLower(projectID) + "_terms", UIName: domain.Translations{"en": "Terms"}, Status: "draft"},
		BaseURI: "http://x/",
	})
	write(domain.FilePath(domain.PathSpec{EntityType: domain.EntityTypeVocabularyPath, EntityID: vocID}), payload, err)
	payload, err = canonical.VocabularyConcepts([]canonical.TreeConcept{
		{URI: "http://x/1", Label: domain.Translations{"en": "One"}, TopConcept: true, ExternalID: "ext-1", ExactMatch: []string{"http://vocab.getty.edu/aat/1", "http://vocab.getty.edu/aat/2"}},
		{URI: "http://x/2", Label: domain.Translations{"en": "Two"}, ScopeNote: domain.Translations{"en": "Second"}, Broader: []string{"http://x/1"}, ExactMatch: []string{"http://vocab.getty.edu/aat/3"}},
		{URI: "http://x/3", Label: domain.Translations{"en": "Three"}, Broader: []string{"http://x/1", "http://x/2"}},
	})
	write(domain.VocabularyConceptsPath(vocID), payload, err)
	for _, l := range []struct {
		n, vocabulary string
		entries       []string
	}{{"1", vocID, []string{"http://x/3", "http://x/2"}}, {"2", "", nil}, {"3", vocID, nil}} {
		listID := projectID + ".CL." + l.n
		payload, err := canonical.ConceptList(&domain.ConceptList{Entity: domain.Entity{SemanticID: listID, SystemName: "terms_" + l.n, UIName: domain.Translations{"en": "Terms " + l.n}, Status: "draft"}}, l.vocabulary)
		write(domain.FilePath(domain.PathSpec{EntityType: domain.EntityTypeConceptListPath, EntityID: listID}), payload, err)
		entriesPath := domain.ConceptListEntriesPath(listID)
		if len(l.entries) == 0 {
			if err := os.Remove(filepath.Join(root, entriesPath)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			continue
		}
		payload, err = canonical.ConceptListEntries(l.entries)
		write(entriesPath, payload, err)
	}
	return root
}

// TestWriteProjectTree_Vocabularies pins the round trip tree -> load-git ->
// init-git for vocabularies, concepts and concept-list entries: every file
// comes back byte for byte. A list bound to the project's local terms (a
// vocabulary outside the tree) keeps its entries in the database: no
// vocabulary key, no entries.yaml, and no vocabularies/<P>.VOCAB.local.
func TestWriteProjectTree_Vocabularies(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	const projectID = "TW"
	vocabularyTestProject(t, pool, projectID)
	if _, err := q.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{ID: projectID, UiName: []byte(`{"en":"TW"}`), Description: []byte(`{}`), Status: "draft", OwnerID: projectID + "_OWNER", Visibility: "private"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	localID, err := vocabulary.NewService(pool, nil).EnsureLocalVocabulary(ctx, projectID)
	if err != nil {
		t.Fatalf("ensure local vocabulary: %v", err)
	}
	root := canonicalVocabularyFixture(t, projectID)
	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}

	// TW.CL.2 holds a local term: bound to TW.VOCAB.local, as the UI does.
	if _, err := pool.Exec(ctx, `UPDATE weave_concept_lists SET vocabulary_id = $1 WHERE id = 'TW.CL.2'`, localID); err != nil {
		t.Fatal(err)
	}
	local, err := q.WeaveCreateVocabularyEntry(ctx, sqlcgen.WeaveCreateVocabularyEntryParams{ID: "TW-local-term", VocabularyID: localID, Uri: "pletka:concept/tw-1", Label: []byte(`{"en":"Local"}`), BroaderPath: []byte(`[]`), BroaderPathItems: []byte(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.WeaveCreateConceptListEntry(ctx, sqlcgen.WeaveCreateConceptListEntryParams{ID: "TW-local-entry", ConceptListID: "TW.CL.2", VocabularyEntryID: local.ID}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := m.writeProjectTree(ctx, dir, projectID); err != nil {
		t.Fatalf("writeProjectTree: %v", err)
	}
	want, err := LoadProjectSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadProjectSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, bucket := range []struct {
		name      string
		want, got []SnapshotEntityFile
	}{
		{"vocabularies", want.Entities.Vocabularies, got.Entities.Vocabularies},
		{"concepts", want.Entities.VocabularyConcepts, got.Entities.VocabularyConcepts},
		{"concept lists", want.Entities.ConceptLists, got.Entities.ConceptLists},
		{"concept-list entries", want.Entities.ConceptListEntries, got.Entities.ConceptListEntries},
	} {
		if len(bucket.got) != len(bucket.want) {
			t.Errorf("%s: wrote %d files, want %d: %v", bucket.name, len(bucket.got), len(bucket.want), snapshotPaths(bucket.got))
			continue
		}
		for i := range bucket.want {
			if bucket.got[i].Path != bucket.want[i].Path || !bytes.Equal(bucket.got[i].Payload, bucket.want[i].Payload) {
				t.Errorf("%s: wrote %s:\n%s\nwant %s:\n%s", bucket.name, bucket.got[i].Path, bucket.got[i].Payload, bucket.want[i].Path, bucket.want[i].Payload)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "vocabularies", projectID+".VOCAB.local")); !os.IsNotExist(err) {
		t.Errorf("vocabularies/%s.VOCAB.local written (stat err %v); local terms stay out of the tree", projectID, err)
	}
}

// TestWriteProjectTreeVersion_Vocabularies pins the release tree: the
// archived tree vocabulary, its concepts and the entries of a list on it are
// written; a list on the local terms has neither vocabulary key nor entries.
// The archive has no vocabulary-level broader edges, so the polyhierarchical
// concept keeps only its first broader (broader_uri).
func TestWriteProjectTreeVersion_Vocabularies(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	const projectID, version = "TWR", "77.0.0"
	vocabularyTestProject(t, pool, projectID)
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM weave_concept_list_entries_archive WHERE version_number = $2 AND concept_list_id LIKE $1 || '.%'`,
			`DELETE FROM weave_vocabulary_entries_archive WHERE version_number = $2 AND vocabulary_id IN (SELECT id FROM weave_vocabularies_archive WHERE project_id = $1)`,
			`DELETE FROM weave_vocabularies_archive WHERE project_id = $1 AND version_number = $2`,
			`DELETE FROM weave_concept_lists_archive WHERE project_id = $1 AND version_number = $2`,
			`DELETE FROM weave_categories_archive WHERE project_id = $1 AND version_number = $2`,
			`DELETE FROM weave_projects_archive WHERE id = $1 AND version_number = $2`,
			`DELETE FROM weave_categories WHERE project_id = $1 AND $2 <> ''`,
			`DELETE FROM weave_change_set WHERE project_id = $1 AND $2 <> ''`,
			`DELETE FROM weave_releases WHERE project_id = $1 AND version = $2`,
		} {
			_, _ = pool.Exec(ctx, stmt, projectID, version)
		}
	})
	// The local terms exist before the load, as for a project made in the UI.
	if _, err := sqlcgen.New(pool).WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{ID: projectID, UiName: []byte(`{"en":"TWR"}`), Description: []byte(`{}`), Status: "draft", OwnerID: projectID + "_OWNER", Visibility: "private"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	localID, err := vocabulary.NewService(pool, nil).EnsureLocalVocabulary(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	mat := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := mat.HydrateProjectSnapshot(ctx, canonicalVocabularyFixture(t, projectID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE weave_concept_lists SET vocabulary_id = $1 WHERE id = $2`, localID, projectID+".CL.2"); err != nil {
		t.Fatal(err)
	}
	// A release needs content beyond concept lists.
	if _, err := pool.Exec(ctx, `INSERT INTO weave_categories (id, semantic_id, project_id) VALUES ($1, $1, $2)`, projectID+".CAT.1", projectID); err != nil {
		t.Fatal(err)
	}
	authCtx := weaveauth.WithPrincipal(weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true}), &weaveauth.Principal{ActorID: projectID + "_OWNER"})
	if _, err := release.NewService(release.NewPostgresStore(pool), pool, nil).Create(authCtx, projectID, release.CreateInput{Version: version}); err != nil {
		t.Fatalf("create release: %v", err)
	}

	dir := t.TempDir()
	if err := mat.writeProjectTreeVersion(ctx, dir, projectID, version); err != nil {
		t.Fatalf("writeProjectTreeVersion: %v", err)
	}
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("%s not written: %v", rel, err)
		}
		return string(b)
	}
	if got := read("vocabularies/TWR.VOC.1/vocabulary.yaml"); !strings.Contains(got, "semantic_id: TWR.VOC.1") || !strings.Contains(got, "base_uri: http://x/") {
		t.Errorf("vocabulary.yaml:\n%s", got)
	}
	wantConcepts, err := canonical.VocabularyConcepts([]canonical.TreeConcept{
		{URI: "http://x/1", Label: domain.Translations{"en": "One"}, TopConcept: true, ExternalID: "ext-1", ExactMatch: []string{"http://vocab.getty.edu/aat/1", "http://vocab.getty.edu/aat/2"}},
		{URI: "http://x/2", Label: domain.Translations{"en": "Two"}, ScopeNote: domain.Translations{"en": "Second"}, Broader: []string{"http://x/1"}, ExactMatch: []string{"http://vocab.getty.edu/aat/3"}},
		{URI: "http://x/3", Label: domain.Translations{"en": "Three"}, Broader: []string{"http://x/1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := read("vocabularies/TWR.VOC.1/concepts.yaml"); got != string(wantConcepts) {
		t.Errorf("concepts.yaml:\n%s\nwant:\n%s", got, wantConcepts)
	}
	if got := read("concept-lists/TWR.CL.1/entries.yaml"); got != "- http://x/3\n- http://x/2\n" {
		t.Errorf("entries.yaml of TWR.CL.1:\n%s", got)
	}
	if got := read("concept-lists/TWR.CL.1.yaml"); !strings.Contains(got, "vocabulary: TWR.VOC.1") {
		t.Errorf("TWR.CL.1 header lacks its vocabulary:\n%s", got)
	}
	if got := read("concept-lists/TWR.CL.2.yaml"); strings.Contains(got, "vocabulary:") {
		t.Errorf("TWR.CL.2 header names a vocabulary outside the tree:\n%s", got)
	}
	for _, rel := range []string{"concept-lists/TWR.CL.2/entries.yaml", "concept-lists/TWR.CL.3/entries.yaml", "vocabularies/TWR.VOCAB.local"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Errorf("%s written (stat err %v)", rel, err)
		}
	}
}

func snapshotPaths(files []SnapshotEntityFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}
