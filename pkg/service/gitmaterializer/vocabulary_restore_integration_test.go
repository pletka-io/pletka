//go:build integration

package gitmaterializer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

const vocabularyFixtureConcepts = `- uri: http://x/1
  label: {en: One}
  top_concept: true
  external_id: ext-1
- uri: http://x/2
  label: {en: Two}
  scope_note: {en: Second}
  broader: [http://x/1]
- uri: http://x/3
  label: {en: Three}
  broader: [http://x/1, http://x/2]
`

// conceptsWithout2 is vocabularyFixtureConcepts with concept 2 removed.
const conceptsWithout2 = `- uri: http://x/1
  label: {en: One}
  top_concept: true
  external_id: ext-1
- uri: http://x/3
  label: {en: Three}
  broader: [http://x/1]
`

// vocabularyFixture writes an authored tree for projectID (owner actor
// <projectID>_OWNER, see vocabularyTestProject): one vocabulary <P>.VOC.1 with
// three concepts (1 top, 2 under 1, 3 under both 1 and 2) and one concept
// list <P>.CL.1 on that vocabulary with entries [3, 2].
func vocabularyFixture(t *testing.T, projectID string) string {
	t.Helper()
	root := t.TempDir()
	ownerID := projectID + "_OWNER"
	slug := strings.ToLower(projectID) + "-org"
	manifest, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project: projectManifestProject{
			ID: projectID, Title: map[string]string{"en": projectID}, Namespace: "https://example.org/" + projectID + "/", Visibility: "private",
			Owner: &manifestActor{ActorID: ownerID, Type: "organization", Slug: slug, DisplayName: projectID + " Org"},
		},
		Namespaces: &projectManifestNamespaces{},
		Manifests:  &projectManifestManifests{},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "project.yaml", manifest)
	mod, err := encodePletkaModManifest(pletkaModManifest{SchemaVersion: 1, Module: pletkaModModule{Path: "pletka.io/orgs/" + slug + "/projects/" + projectID, ProjectID: projectID}, Require: pletkaModRequire{Projects: []pletkaProjectRequirement{}, Ontologies: []pletkaOntologyRequirement{}}, Replace: []map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "pletka.mod", mod)
	vocID, listID := projectID+".VOC.1", projectID+".CL.1"
	mustWrite(t, root, "vocabularies/"+vocID+"/vocabulary.yaml", []byte(
		"base_uri: http://x/\ndescription: {}\nsemantic_id: "+vocID+"\nstatus: draft\nsystem_name: "+strings.ToLower(projectID)+"_terms\nui_name:\n  en: Terms\n"))
	mustWrite(t, root, "vocabularies/"+vocID+"/concepts.yaml", []byte(vocabularyFixtureConcepts))
	mustWrite(t, root, "concept-lists/"+listID+".yaml", []byte(
		"description: {}\nis_closed: false\nsemantic_id: "+listID+"\nstatus: draft\nsystem_name: terms\nui_name:\n  en: Terms\nvocabulary: "+vocID+"\n"))
	mustWrite(t, root, "concept-lists/"+listID+"/entries.yaml", []byte("- http://x/3\n- http://x/2\n"))
	return root
}

// vocabularyTestProject creates the owner actor vocabularyFixture names and
// removes everything the load wrote for projectID when the test ends.
func vocabularyTestProject(t *testing.T, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	ctx := context.Background()
	ownerID := projectID + "_OWNER"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_concept_lists WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_vocabularies WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_entity_counters WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})
	if _, err := sqlcgen.New(pool).WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{ID: ownerID, Type: "organization", DisplayName: projectID + " Org", Slug: strings.ToLower(projectID) + "-org", Visibility: "private"}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
}

// TestHydrateProjectSnapshot_Vocabularies pins: a vocabulary file creates a
// local vocabulary keyed by its semantic id, concepts become entries with one
// broader edge per broader (top_concept in config), the list is bound to the
// vocabulary with its entries in file order, and a second load removes a
// concept that left the file.
func TestHydrateProjectSnapshot_Vocabularies(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	vocabularyTestProject(t, pool, "TV")
	root := vocabularyFixture(t, "TV")
	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}

	q := sqlcgen.New(pool)
	vocabs, err := q.WeaveListProjectTreeVocabularies(ctx, "TV")
	if err != nil {
		t.Fatal(err)
	}
	if len(vocabs) != 1 || vocabs[0].SemanticID == nil || *vocabs[0].SemanticID != "TV.VOC.1" || vocabs[0].ConnectorType != "local" {
		t.Fatalf("vocabularies %+v", vocabs)
	}
	if vocabs[0].BaseUri == nil || *vocabs[0].BaseUri != "http://x/" || vocabs[0].SystemName == nil || *vocabs[0].SystemName != "tv_terms" {
		t.Fatalf("vocabulary header not loaded: %+v", vocabs[0])
	}
	entries, err := q.WeaveListVocabularyEntriesForTree(ctx, vocabs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries %d", len(entries))
	}
	byURI := map[string]sqlcgen.WeaveVocabularyEntry{}
	for _, e := range entries {
		byURI[e.Uri] = e
	}
	var cfg map[string]any
	if err := json.Unmarshal(byURI["http://x/1"].Config, &cfg); err != nil || cfg["top_concept"] != true {
		t.Fatalf("top concept config %s (%v)", byURI["http://x/1"].Config, err)
	}
	if len(byURI["http://x/2"].Config) != 0 {
		t.Fatalf("non-top concept config %s, want NULL", byURI["http://x/2"].Config)
	}
	if e := byURI["http://x/1"]; e.ExternalID == nil || *e.ExternalID != "ext-1" {
		t.Fatalf("external_id %+v", e.ExternalID)
	}
	if e := byURI["http://x/3"]; e.BroaderUri == nil || *e.BroaderUri != "http://x/1" {
		t.Fatalf("broader_uri of 3 = %v, want the first broader", e.BroaderUri)
	}
	if !strings.Contains(string(byURI["http://x/2"].ScopeNote), "Second") {
		t.Fatalf("scope note %s", byURI["http://x/2"].ScopeNote)
	}
	edges, err := q.WeaveListConceptBroaderByVocabulary(ctx, vocabs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	wantEdges := []sqlcgen.WeaveListConceptBroaderByVocabularyRow{
		{ConceptUri: "http://x/2", BroaderUri: "http://x/1", Position: 0},
		{ConceptUri: "http://x/3", BroaderUri: "http://x/1", Position: 0},
		{ConceptUri: "http://x/3", BroaderUri: "http://x/2", Position: 1},
	}
	if !slices.Equal(edges, wantEdges) {
		t.Fatalf("broader edges %+v, want %+v (two for the polyhierarchical concept)", edges, wantEdges)
	}
	list, err := q.WeaveGetProjectConceptList(ctx, sqlcgen.WeaveGetProjectConceptListParams{ProjectID: "TV", ID: "TV.CL.1"})
	if err != nil {
		t.Fatal(err)
	}
	if list.VocabularyID == nil || *list.VocabularyID != vocabs[0].ID {
		t.Fatalf("list vocabulary_id %v, want %s", list.VocabularyID, vocabs[0].ID)
	}
	uris, err := q.WeaveListConceptListEntryURIs(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(uris, []string{"http://x/3", "http://x/2"}) {
		t.Fatalf("entries %v", uris)
	}

	// Second load after removing concept 2 from concepts.yaml and entries:
	// row, edges and entry are gone, the vocabulary row is the same.
	mustWrite(t, root, "vocabularies/TV.VOC.1/concepts.yaml", []byte(conceptsWithout2))
	mustWrite(t, root, "concept-lists/TV.CL.1/entries.yaml", []byte("- http://x/3\n"))
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	again, _ := q.WeaveListProjectTreeVocabularies(ctx, "TV")
	if len(again) != 1 || again[0].ID != vocabs[0].ID {
		t.Fatalf("vocabulary row not reused: %+v", again)
	}
	entries, _ = q.WeaveListVocabularyEntriesForTree(ctx, vocabs[0].ID)
	if len(entries) != 2 {
		t.Fatalf("entries after removal %d", len(entries))
	}
	edges, _ = q.WeaveListConceptBroaderByVocabulary(ctx, vocabs[0].ID)
	if len(edges) != 1 || edges[0].ConceptUri != "http://x/3" || edges[0].BroaderUri != "http://x/1" {
		t.Fatalf("edges after removal %+v", edges)
	}
	uris, _ = q.WeaveListConceptListEntryURIs(ctx, list.ID)
	if !slices.Equal(uris, []string{"http://x/3"}) {
		t.Fatalf("entries after removal %v", uris)
	}
}

// TestHydrateProjectSnapshot_VocabularySameSystemName pins ruling I3: a
// vocabulary row with the tree vocabulary's system_name and no semantic id is
// claimed (same row id), one with another semantic id fails the load naming
// both semantic ids and the system_name.
func TestHydrateProjectSnapshot_VocabularySameSystemName(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	seed := func(projectID string, semanticID *string) string {
		name := strings.ToLower(projectID) + "_terms"
		v, err := q.WeaveCreateVocabulary(ctx, sqlcgen.WeaveCreateVocabularyParams{
			ID: projectID + "-existing", SemanticID: semanticID, SystemName: &name,
			Status: "published", ProjectID: projectID, ConnectorType: "local",
		})
		if err != nil {
			t.Fatalf("seed vocabulary: %v", err)
		}
		return v.ID
	}

	t.Run("unclaimed row is claimed", func(t *testing.T) {
		vocabularyTestProject(t, pool, "TVA")
		existing := seed("TVA", nil)
		if _, err := NewMaterializer(pool, t.TempDir(), nil).HydrateProjectSnapshot(ctx, vocabularyFixture(t, "TVA")); err != nil {
			t.Fatal(err)
		}
		vocabs, _ := q.WeaveListProjectTreeVocabularies(ctx, "TVA")
		if len(vocabs) != 1 || vocabs[0].ID != existing {
			t.Fatalf("want the existing row %s claimed, got %+v", existing, vocabs)
		}
	})

	t.Run("row owned by another semantic id is an error", func(t *testing.T) {
		vocabularyTestProject(t, pool, "TVB")
		other := "TVB.VOCAB.local"
		seed("TVB", &other)
		_, err := NewMaterializer(pool, t.TempDir(), nil).HydrateProjectSnapshot(ctx, vocabularyFixture(t, "TVB"))
		if err == nil || !strings.Contains(err.Error(), "TVB.VOC.1") || !strings.Contains(err.Error(), other) || !strings.Contains(err.Error(), "tvb_terms") {
			t.Fatalf("want error naming both semantic ids and the system_name, got %v", err)
		}
	})
}

// TestHydrateProjectSnapshot_VocabularyErrors pins the readable load errors:
// a concept removed from the file that a list outside the tree still uses
// (ruling I4), a broader outside the file, and an entry outside the list's
// vocabulary.
func TestHydrateProjectSnapshot_VocabularyErrors(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	vocabularyTestProject(t, pool, "TVE")
	root := vocabularyFixture(t, "TVE")
	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}

	t.Run("broader outside the file", func(t *testing.T) {
		mustWrite(t, root, "vocabularies/TVE.VOC.1/concepts.yaml", []byte(vocabularyFixtureConcepts+"- uri: http://x/4\n  label: {en: Four}\n  broader: [http://x/9]\n"))
		defer mustWrite(t, root, "vocabularies/TVE.VOC.1/concepts.yaml", []byte(vocabularyFixtureConcepts))
		_, err := m.HydrateProjectSnapshot(ctx, root)
		if err == nil || !strings.Contains(err.Error(), "http://x/4") || !strings.Contains(err.Error(), "http://x/9") {
			t.Fatalf("want error naming concept and broader, got %v", err)
		}
	})

	t.Run("entry outside the vocabulary", func(t *testing.T) {
		mustWrite(t, root, "concept-lists/TVE.CL.1/entries.yaml", []byte("- http://x/404\n"))
		defer mustWrite(t, root, "concept-lists/TVE.CL.1/entries.yaml", []byte("- http://x/3\n- http://x/2\n"))
		_, err := m.HydrateProjectSnapshot(ctx, root)
		if err == nil || !strings.Contains(err.Error(), "TVE.CL.1") || !strings.Contains(err.Error(), "http://x/404") {
			t.Fatalf("want error naming list and uri, got %v", err)
		}
	})

	t.Run("removed concept still in a list outside the tree", func(t *testing.T) {
		vocabs, _ := q.WeaveListProjectTreeVocabularies(ctx, "TVE")
		if len(vocabs) != 1 {
			t.Fatalf("vocabularies %+v", vocabs)
		}
		two, err := q.WeaveGetVocabularyEntryByVocabularyURI(ctx, sqlcgen.WeaveGetVocabularyEntryByVocabularyURIParams{VocabularyID: vocabs[0].ID, Uri: "http://x/2"})
		if err != nil {
			t.Fatal(err)
		}
		outside := "TVE.CL.99"
		if _, err := q.WeaveCreateConceptList(ctx, sqlcgen.WeaveCreateConceptListParams{ID: outside, SemanticID: &outside, Status: "draft", ProjectID: "TVE"}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.WeaveCreateConceptListEntry(ctx, sqlcgen.WeaveCreateConceptListEntryParams{ID: "TVE-outside-entry", ConceptListID: outside, VocabularyEntryID: two.ID}); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, root, "vocabularies/TVE.VOC.1/concepts.yaml", []byte(conceptsWithout2))
		mustWrite(t, root, "concept-lists/TVE.CL.1/entries.yaml", []byte("- http://x/3\n"))
		_, err = m.HydrateProjectSnapshot(ctx, root)
		if err == nil || !strings.Contains(err.Error(), "TVE.VOC.1") || !strings.Contains(err.Error(), "http://x/2") || !strings.Contains(err.Error(), outside) {
			t.Fatalf("want error naming vocabulary, concept and list, got %v", err)
		}
	})
}

// TestHydrateProjectSnapshot_VocabularyExactMatch pins that exact_match is
// kept in the entry config beside top_concept, each key only when set.
func TestHydrateProjectSnapshot_VocabularyExactMatch(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	vocabularyTestProject(t, pool, "TVX")
	root := vocabularyFixture(t, "TVX")
	mustWrite(t, root, "vocabularies/TVX.VOC.1/concepts.yaml", []byte(`- uri: http://x/1
  label: {en: One}
  top_concept: true
  exact_match: [http://vocab.getty.edu/aat/1, http://vocab.getty.edu/aat/2]
- uri: http://x/2
  label: {en: Two}
  broader: [http://x/1]
  exact_match: [http://vocab.getty.edu/aat/3]
- uri: http://x/3
  label: {en: Three}
  broader: [http://x/1, http://x/2]
`))
	if _, err := NewMaterializer(pool, t.TempDir(), nil).HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	vocabs, _ := q.WeaveListProjectTreeVocabularies(ctx, "TVX")
	if len(vocabs) != 1 {
		t.Fatalf("vocabularies %+v", vocabs)
	}
	entries, _ := q.WeaveListVocabularyEntriesForTree(ctx, vocabs[0].ID)
	want := map[string]map[string]any{
		"http://x/1": {"top_concept": true, "exact_match": []any{"http://vocab.getty.edu/aat/1", "http://vocab.getty.edu/aat/2"}},
		"http://x/2": {"exact_match": []any{"http://vocab.getty.edu/aat/3"}},
		"http://x/3": nil,
	}
	for _, e := range entries {
		var got map[string]any
		if len(e.Config) > 0 {
			if err := json.Unmarshal(e.Config, &got); err != nil {
				t.Fatalf("%s config %s: %v", e.Uri, e.Config, err)
			}
		}
		if !reflect.DeepEqual(got, want[e.Uri]) {
			t.Fatalf("%s config %s, want %v", e.Uri, e.Config, want[e.Uri])
		}
	}
}

// TestHydrateProjectSnapshot_VocabularyKeepsSchemeEdges pins that a load
// replaces only vocabulary-level broader edges: an edge scoped to a list
// (scheme_id set, made in the UI) survives and is not listed for the tree.
func TestHydrateProjectSnapshot_VocabularyKeepsSchemeEdges(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	vocabularyTestProject(t, pool, "TVS")
	root := vocabularyFixture(t, "TVS")
	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	vocabs, _ := q.WeaveListProjectTreeVocabularies(ctx, "TVS")
	if len(vocabs) != 1 {
		t.Fatalf("vocabularies %+v", vocabs)
	}
	entry := func(uri string) string {
		e, err := q.WeaveGetVocabularyEntryByVocabularyURI(ctx, sqlcgen.WeaveGetVocabularyEntryByVocabularyURIParams{VocabularyID: vocabs[0].ID, Uri: uri})
		if err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	scheme := "TVS.CL.1"
	if err := q.WeaveAddConceptBroader(ctx, sqlcgen.WeaveAddConceptBroaderParams{ID: "TVS-scoped-edge", ConceptID: entry("http://x/2"), BroaderID: entry("http://x/3"), SchemeID: &scheme}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_concept_broader WHERE id = 'TVS-scoped-edge'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("scoped edge after load: count %d (%v)", n, err)
	}
	edges, _ := q.WeaveListConceptBroaderByVocabulary(ctx, vocabs[0].ID)
	for _, e := range edges {
		if e.ConceptUri == "http://x/2" && e.BroaderUri == "http://x/3" {
			t.Fatalf("scoped edge listed as vocabulary-level: %+v", edges)
		}
	}
	if len(edges) != 3 {
		t.Fatalf("edges %+v, want the three from the file", edges)
	}
}

// TestHydrateProjectSnapshot_VocabularyIdempotent pins that loading the same
// tree twice leaves entries, edges and list membership as they were.
func TestHydrateProjectSnapshot_VocabularyIdempotent(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	vocabularyTestProject(t, pool, "TVI")
	root := vocabularyFixture(t, "TVI")
	m := NewMaterializer(pool, t.TempDir(), nil)
	type state struct {
		entryIDs map[string]string
		edges    []sqlcgen.WeaveListConceptBroaderByVocabularyRow
		members  []string
	}
	read := func() state {
		vocabs, _ := q.WeaveListProjectTreeVocabularies(ctx, "TVI")
		if len(vocabs) != 1 {
			t.Fatalf("vocabularies %+v", vocabs)
		}
		st := state{entryIDs: map[string]string{}}
		entries, _ := q.WeaveListVocabularyEntriesForTree(ctx, vocabs[0].ID)
		for _, e := range entries {
			st.entryIDs[e.Uri] = e.ID
		}
		st.edges, _ = q.WeaveListConceptBroaderByVocabulary(ctx, vocabs[0].ID)
		rows, err := pool.Query(ctx, `SELECT e.uri || '@' || le.position FROM weave_concept_list_entries le JOIN weave_vocabulary_entries e ON e.id = le.vocabulary_entry_id WHERE le.concept_list_id = 'TVI.CL.1' ORDER BY le.position`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			st.members = append(st.members, s)
		}
		return st
	}
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	first := read()
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	second := read()
	if !reflect.DeepEqual(first.entryIDs, second.entryIDs) || len(first.entryIDs) != 3 {
		t.Fatalf("entry ids changed: %v -> %v", first.entryIDs, second.entryIDs)
	}
	if !slices.Equal(first.edges, second.edges) || len(first.edges) != 3 {
		t.Fatalf("edges changed: %+v -> %+v", first.edges, second.edges)
	}
	if !slices.Equal(first.members, second.members) || !slices.Equal(first.members, []string{"http://x/3@0", "http://x/2@1"}) {
		t.Fatalf("list entries changed: %v -> %v", first.members, second.members)
	}
}

// TestHydrateProjectSnapshot_VocabularyWithoutConcepts pins that a
// vocabulary without concepts.yaml loads as zero concepts.
func TestHydrateProjectSnapshot_VocabularyWithoutConcepts(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	q := sqlcgen.New(pool)
	vocabularyTestProject(t, pool, "TVN")
	root := vocabularyFixture(t, "TVN")
	m := NewMaterializer(pool, t.TempDir(), nil)
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"vocabularies/TVN.VOC.1/concepts.yaml", "concept-lists/TVN.CL.1/entries.yaml"} {
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.HydrateProjectSnapshot(ctx, root); err != nil {
		t.Fatal(err)
	}
	vocabs, _ := q.WeaveListProjectTreeVocabularies(ctx, "TVN")
	if len(vocabs) != 1 {
		t.Fatalf("vocabularies %+v", vocabs)
	}
	if entries, _ := q.WeaveListVocabularyEntriesForTree(ctx, vocabs[0].ID); len(entries) != 0 {
		t.Fatalf("entries %d, want 0", len(entries))
	}
}
