//go:build integration

package vocabulary_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/registry"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestRemovedVocabularyIsReadOnly pins what removing a vocabulary from a
// project means, which is the half of enablement that cannot be expressed as
// a delete.
//
// weave_vocabulary_entries cascades from weave_vocabularies, so deleting the
// row empties every concept list built on it; and weave_concept_lists carries
// a plain foreign key with no ON DELETE action, so Postgres refuses the
// delete outright while any list still points at it. Removal is therefore a
// deprecation: the lists keep their pinned entries and keep resolving them,
// and only new pins are refused.
//
// Three behaviours, and the first is the one a delete would have destroyed.
func TestRemovedVocabularyIsReadOnly(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	// A vocabulary service that would answer if it were ever asked. Any hit
	// reaching a caller therefore proves the search ran, rather than proving
	// only that nothing failed.
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"uri":"http://vocab.getty.edu/aat/300999001","id":"300999001","prefLabel":{"en":"a live hit"},"lang":"en"}]}`))
	}))
	defer service.Close()

	reg := registry.New(service.Client(), service.URL)
	svc := vocabulary.NewService(pool, reg)

	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, config, status)
		VALUES ('RMVVOC', 'RMVP', 'aat', 'vocabservice', '{"vocab":"aat"}'::jsonb, 'published')`)
	mustExec(t, pool, `INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label)
		VALUES ('RMVENTRY', 'RMVVOC', 'http://vocab.getty.edu/aat/300033618', '{"en":"paintings (visual works)"}'::jsonb)`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id, semantic_id, system_name, ui_name, vocabulary_id, status)
		VALUES ('RMVLIST', 'RMVP', 'RMVP.CL.1', 'pinned', '{"en":"Pinned"}'::jsonb, 'RMVVOC', 'draft')`)
	mustExec(t, pool, `INSERT INTO weave_concept_list_entries (id, concept_list_id, vocabulary_entry_id, position)
		VALUES ('RMVMEMBER', 'RMVLIST', 'RMVENTRY', 1)`)

	// Remove it, the way the settings screen does.
	mustExec(t, pool, `UPDATE weave_vocabularies SET deprecated = true WHERE id = 'RMVVOC'`)

	t.Run("the pinned entry survives removal", func(t *testing.T) {
		// The point of deprecating rather than deleting. Under the old
		// delete this row was gone via ON DELETE CASCADE — or the delete was
		// refused outright by the concept list's foreign key.
		var members int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM weave_concept_list_entries WHERE concept_list_id = 'RMVLIST'`).Scan(&members); err != nil {
			t.Fatalf("count list members: %v", err)
		}
		if members != 1 {
			t.Errorf("concept list has %d members, want 1 — removal destroyed what was built from the vocabulary", members)
		}
	})

	t.Run("searching a removed vocabulary offers nothing new", func(t *testing.T) {
		r := chi.NewRouter()
		vocabulary.Mount(r, vocabulary.Host{Service: svc, Projects: fakeProjectReader{visibility: "public"}})

		req := httptest.NewRequest(http.MethodGet, "/api/v2/vocabularies/RMVVOC/entries/search?q=painting", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var body struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v (%s)", err, w.Body.String())
		}
		if len(body.Items) != 0 {
			t.Errorf("got %d items from a removed vocabulary, want 0 — it is meant to be read-only", len(body.Items))
		}
	})

	t.Run("a live vocabulary in the same project still answers", func(t *testing.T) {
		// Without this, a check that returned nothing for everything would
		// pass the case above while breaking the feature entirely.
		mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, system_name, connector_type, config, status)
			VALUES ('RMVLIVE', 'RMVP', 'aat_live', 'vocabservice', '{"vocab":"aat"}'::jsonb, 'published')`)

		r := chi.NewRouter()
		vocabulary.Mount(r, vocabulary.Host{Service: svc, Projects: fakeProjectReader{visibility: "public"}})

		req := httptest.NewRequest(http.MethodGet, "/api/v2/vocabularies/RMVLIVE/entries/search?q=painting", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var body struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v (%s)", err, w.Body.String())
		}
		if len(body.Items) == 0 {
			t.Error("a live vocabulary returned no items — the removal gate is rejecting everything")
		}
	})
}
