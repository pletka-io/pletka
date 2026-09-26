//go:build integration

package vocabulary_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
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

	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, connector_type, config) VALUES ('DEGSVC', 'DEGP', 'vocabservice', '{"vocab":"aat"}'::jsonb)`)
	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, connector_type) VALUES ('DEGLOC', 'DEGP', 'local')`)

	r := chi.NewRouter()
	// Both vocabularies are project-owned (DEGP) now that there is no global
	// tier; "public" visibility keeps the anonymous request in this test
	// readable, matching the pre-ownership behaviour for a global vocabulary.
	vocabulary.Mount(r, vocabulary.Host{Service: svc, Projects: fakeProjectReader{visibility: "public"}})

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

// TestSearchConceptListSourceEntries_Degraded covers the other route that
// reaches a connector: the concept-list page's "add an entry from the source
// vocabulary" panel. Both of its connector-reaching branches — a list bound to
// one vocabulary, and an unbound list fanned out over every project-scoped
// vocabulary — must report a failed lookup rather than returning an empty list
// indistinguishable from a genuine no-match.
func TestSearchConceptListSourceEntries_Degraded(t *testing.T) {
	pool := testdb.Pool(t)

	remoteDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer remoteDown.Close()

	reg := registry.New(remoteDown.Client(), remoteDown.URL)
	svc := vocabulary.NewService(pool, reg)

	mustExec(t, pool, `INSERT INTO weave_actors (id, display_name, slug) VALUES ('srcown','SrcOwn','srcown')`)
	// SRCP owns one failing (vocabservice) and one local vocabulary; SRCQ owns
	// only a local one, so its unbound list is the negative case for the
	// multi-vocabulary fan-out.
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id) VALUES ('SRCP','srcown')`)
	mustExec(t, pool, `INSERT INTO weave_projects (id, owner_id) VALUES ('SRCQ','srcown')`)
	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, connector_type, config) VALUES ('SRCSVC','SRCP','vocabservice','{"vocab":"aat"}'::jsonb)`)
	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, connector_type) VALUES ('SRCLOC','SRCP','local')`)
	mustExec(t, pool, `INSERT INTO weave_vocabularies (id, project_id, connector_type) VALUES ('SRCLOC2','SRCQ','local')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id, vocabulary_id) VALUES ('CLSRCSVC','SRCP','SRCSVC')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id, vocabulary_id) VALUES ('CLSRCLOC','SRCP','SRCLOC')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id) VALUES ('CLSRCNONE','SRCP')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists (id, project_id) VALUES ('CLSRCQNONE','SRCQ')`)

	r := chi.NewRouter()
	vocabulary.Mount(r, vocabulary.Host{Service: svc, Projects: fakeProjectReader{}})

	// The route is behind RequireProjectEdit; a super-admin snapshot satisfies
	// the gate without standing up a role fixture, which this test is not about.
	searchSource := func(projectID, listID string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/api/v2/projects/"+projectID+"/concept-lists/"+listID+"/source-entries/search?q=term", nil)
		req = req.WithContext(auth.WithSnapshot(req.Context(), &auth.AuthSnapshot{ActorID: "srcown", IsSuperAdmin: true}))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		// Decoding into map[string]any rather than a struct is load-bearing: a
		// struct decode cannot tell an absent "degraded" key from a false one,
		// which is exactly the regression the negative cases below guard.
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response body: %v (body: %s)", err, w.Body.String())
		}
		return w.Code, body
	}

	assertDegraded := func(t *testing.T, projectID, listID string) {
		t.Helper()
		code, body := searchSource(projectID, listID)
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %v)", code, body)
		}
		degraded, ok := body["degraded"]
		if !ok {
			t.Fatalf(`response has no "degraded" key, want true (body: %v)`, body)
		}
		if degraded != true {
			t.Fatalf(`"degraded" = %v, want true`, degraded)
		}
	}

	assertNoDegradedKey := func(t *testing.T, projectID, listID string) {
		t.Helper()
		code, body := searchSource(projectID, listID)
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %v)", code, body)
		}
		if _, ok := body["degraded"]; ok {
			t.Fatalf(`response has a "degraded" key = %v, want none`, body["degraded"])
		}
	}

	t.Run("a list bound to a failing vocabulary reports degraded true", func(t *testing.T) {
		assertDegraded(t, "SRCP", "CLSRCSVC")
	})

	// degraded is the OR across the project's vocabularies: one failing
	// vocabulary degrades the response even though the local one answered.
	t.Run("an unbound list degrades when any project vocabulary fails", func(t *testing.T) {
		assertDegraded(t, "SRCP", "CLSRCNONE")
	})

	// The load-bearing negatives: the key must be absent, not false, or every
	// other connector in the system reads as degraded.
	t.Run("a list bound to a vocabulary that answers has no degraded key", func(t *testing.T) {
		assertNoDegradedKey(t, "SRCP", "CLSRCLOC")
	})

	t.Run("an unbound list whose vocabularies all answer has no degraded key", func(t *testing.T) {
		assertNoDegradedKey(t, "SRCQ", "CLSRCQNONE")
	})
}
