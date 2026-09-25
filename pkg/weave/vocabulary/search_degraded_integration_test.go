//go:build integration

package vocabulary_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/registry"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// TestSearchVocabularyEntries_Degraded proves the search response tells a
// failed remote lookup apart from a genuine no-match: both are otherwise a
// 200 with an empty item list, so neither a browser's network tab nor a
// developer without server-log access can tell them apart.
func TestSearchVocabularyEntries_Degraded(t *testing.T) {
	pool := testdb.Pool(t)

	// remoteDown always fails, so the vocabservice connector's call to it
	// degrades every search.
	remoteDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer remoteDown.Close()

	reg := registry.New(remoteDown.Client(), remoteDown.URL)
	svc := vocabulary.NewService(pool, reg)

	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, connector_type, config) VALUES ('DEGSVC', 'vocabservice', '{"vocab":"aat"}'::jsonb)`)
	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, connector_type) VALUES ('DEGLOC', 'local')`)

	r := chi.NewRouter()
	vocabulary.Mount(r, vocabulary.Host{Service: svc, Projects: fakeProjectReader{}})

	search := func(vocabularyID string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/api/v2/vocabularies/"+vocabularyID+"/entries/search?q=term", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response body: %v (body: %s)", err, w.Body.String())
		}
		return w.Code, body
	}

	t.Run("a failed remote lookup reports degraded true with no items", func(t *testing.T) {
		code, body := search("DEGSVC")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		degraded, ok := body["degraded"]
		if !ok {
			t.Fatal(`response has no "degraded" key, want true`)
		}
		if degraded != true {
			t.Fatalf(`"degraded" = %v, want true`, degraded)
		}
		items, ok := body["items"].([]any)
		if !ok || len(items) != 0 {
			t.Fatalf(`"items" = %v, want an empty list`, body["items"])
		}
	})

	// The load-bearing assertion: a vocabulary whose connector does not fail
	// must produce a response with no "degraded" key at all. Anything else
	// would report every other connector in the system as degraded.
	t.Run("a vocabulary whose connector does not fail has no degraded key", func(t *testing.T) {
		code, body := search("DEGLOC")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if _, ok := body["degraded"]; ok {
			t.Fatalf(`response has a "degraded" key = %v, want none`, body["degraded"])
		}
	})
}
