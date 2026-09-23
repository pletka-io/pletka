//go:build integration

package project_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// Scratch coordinates for the save-conflict tests: a scratch model on the
// real LA fixture project, holding real LA fixture fields (LAF.5) — mirrors
// the scratch-entity convention used by
// pkg/weave/override/fingerprint_integration_test.go and
// replace_integration_test.go, one level up at the HTTP handler.
const (
	scProjectID = "LA"
	scModelID   = "TSTSC.1"
	scURL       = "/projects/LA/models/TSTSC.1/overrides"
)

// scLAFieldID resolves the real weave_fields.id backing a LA fixture field's
// semantic id (the materializer mints the row id at hydrate time, so it
// cannot be hardcoded).
func scLAFieldID(t *testing.T, pool *pgxpool.Pool, semanticID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM weave_fields WHERE project_id = $1 AND semantic_id = $2`,
		scProjectID, semanticID).Scan(&id)
	if err != nil {
		t.Fatalf("resolve field %s: %v", semanticID, err)
	}
	return id
}

// seedScratchModel inserts the TSTSC.1 scratch model under the LA fixture
// project (the minimal row saveOverrides' ownership check needs — see
// weave_models' schema, only id + project_id are NOT NULL without a
// default) and registers cleanup of the model plus every override row and
// ref it accumulates during the test.
func seedScratchModel(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_models (id, project_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		scModelID, scProjectID); err != nil {
		t.Fatalf("seed scratch model: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_override_refs WHERE override_id IN (
			SELECT id FROM weave_field_overrides WHERE entity_type = 'model' AND entity_id = $1)`, scModelID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_field_overrides WHERE entity_type = 'model' AND entity_id = $1`, scModelID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_collection_placements WHERE model_id = $1`, scModelID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_models WHERE id = $1`, scModelID)
	})
}

// newSaveConflictRouter wires the real Handler over the real
// SaveModelOverrides/ModelOverrides HTTP routes against pool, including the
// auth.WithProjectResource middleware routes.go applies at the slice mount
// boundary — override.Service.requireProjectWrite reads the project
// Resource it attaches, not a hand-built one.
func newSaveConflictRouter(pool *pgxpool.Pool) http.Handler {
	weaveStore := weave.NewPostgresStore(pool)
	overrideStore := override.NewPostgresStore(pool)
	overrideSvc := override.NewService(overrideStore, nil, nil)
	projectStore := project.NewPostgresStore(pool)
	projectSvc := project.NewService(projectStore, nil, nil, nil)
	h := project.NewHandler(projectSvc, overrideSvc, weaveStore, nil, nil, nil)

	r := chi.NewRouter()
	r.Route("/projects/{projectID}/models/{modelID}/overrides", func(r chi.Router) {
		r.Use(weaveauth.WithProjectResource(weaveStore))
		r.Get("/", h.ModelOverrides)
		r.Put("/", h.SaveModelOverrides)
	})
	return r
}

// editorAuthContext is the "editor" auth context: a caller authorized to
// edit any project, matching the pattern used throughout this package (see
// override_read_test.go, canedit_org_inheritance_test.go) — IsSuperAdmin
// short-circuits every auth.Snapshot.Can check.
func editorAuthContext() context.Context {
	return weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		IsSuperAdmin: true,
		ActorID:      "save-conflict-tester",
	})
}

type scField struct {
	FieldID     string            `json:"field_id"`
	Position    int               `json:"position"`
	DisplayName map[string]string `json:"display_name"`
}

type scItem struct {
	Widget   string    `json:"widget"`
	ID       string    `json:"id"`
	Position int       `json:"position"`
	Fields   []scField `json:"fields"`
}

type scCategory struct {
	CategoryID   string            `json:"category_id"`
	CategoryName map[string]string `json:"category_name"`
	Position     int               `json:"position"`
	Items        []scItem          `json:"items"`
}

type scSaveRequest struct {
	CommitMessage string       `json:"commit_message"`
	Categories    []scCategory `json:"categories"`
	Fingerprint   string       `json:"fingerprint,omitempty"`
}

// scOneFieldRequest builds a save payload placing one direct (not
// collection-grouped) field on the scratch model, using displayName as its
// override display_name.en — the one property whose value scLAFieldID's
// caller varies between saves so the fingerprint changes.
func scOneFieldRequest(fieldID, displayName, fingerprint string) scSaveRequest {
	return scSaveRequest{
		CommitMessage: "save-conflict test",
		Fingerprint:   fingerprint,
		Categories: []scCategory{
			{
				CategoryID:   "__uncategorized__",
				CategoryName: map[string]string{"en": "Uncategorized"},
				Position:     1,
				Items: []scItem{
					{
						Widget:   "field-group",
						ID:       "__direct__",
						Position: 1,
						Fields: []scField{
							{FieldID: fieldID, Position: 1, DisplayName: map[string]string{"en": displayName}},
						},
					},
				},
			},
		},
	}
}

type scEditorResponse struct {
	Fingerprint string `json:"fingerprint"`
}

type scSaveResponse struct {
	Fingerprint string `json:"fingerprint"`
}

type scErrorBody struct {
	Code    string `json:"code"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func scGet(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(editorAuthContext(), http.MethodGet, scURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func scPut(t *testing.T, router http.Handler, body scSaveRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal save request: %v", err)
	}
	req := httptest.NewRequestWithContext(editorAuthContext(), http.MethodPut, scURL, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestSaveRejectsStaleFingerprint is the RED case: an editor loads the
// pattern, someone else's save (or the same editor, in a second tab)
// changes it, and the editor's own save — carrying the fingerprint it
// originally loaded — must be refused with a 409 instead of silently
// overwriting the newer content.
func TestSaveRejectsStaleFingerprint(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchModel(t, pool)
	router := newSaveConflictRouter(pool)
	fieldID := scLAFieldID(t, pool, "LAF.5")

	getResp := scGet(t, router)
	if getResp.Code != http.StatusOK {
		t.Fatalf("initial GET status = %d, body = %s", getResp.Code, getResp.Body.String())
	}
	var initial scEditorResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial GET body: %v", err)
	}
	if initial.Fingerprint == "" {
		t.Fatal("expected a non-empty initial fingerprint")
	}
	staleFingerprint := initial.Fingerprint

	// First save: succeeds, using the fingerprint the editor loaded.
	firstSave := scPut(t, router, scOneFieldRequest(fieldID, "Name Type", staleFingerprint))
	if firstSave.Code != http.StatusOK {
		t.Fatalf("first save status = %d, body = %s", firstSave.Code, firstSave.Body.String())
	}
	var firstSaveResp scSaveResponse
	if err := json.Unmarshal(firstSave.Body.Bytes(), &firstSaveResp); err != nil {
		t.Fatalf("decode first save body: %v", err)
	}
	if firstSaveResp.Fingerprint == "" || firstSaveResp.Fingerprint == staleFingerprint {
		t.Fatalf("expected a new, non-empty fingerprint after first save, got %q (was %q)", firstSaveResp.Fingerprint, staleFingerprint)
	}

	// Second save: still carries the now-stale fingerprint from the
	// original GET — must be refused, not applied.
	secondSave := scPut(t, router, scOneFieldRequest(fieldID, "Name Type STALE ATTEMPT", staleFingerprint))
	if secondSave.Code != http.StatusConflict {
		t.Fatalf("second (stale) save status = %d, want 409, body = %s", secondSave.Code, secondSave.Body.String())
	}
	var conflictBody scErrorBody
	if err := json.Unmarshal(secondSave.Body.Bytes(), &conflictBody); err != nil {
		t.Fatalf("decode conflict body: %v", err)
	}
	if conflictBody.Code != "conflict" {
		t.Fatalf("conflict body code = %q, want %q (body: %s)", conflictBody.Code, "conflict", secondSave.Body.String())
	}
	if conflictBody.Message != "pattern_changed" {
		t.Fatalf("conflict body message = %q, want %q (body: %s)", conflictBody.Message, "pattern_changed", secondSave.Body.String())
	}

	// The rejected save must not have touched the rows: a fresh GET still
	// reports the fingerprint (and content) the first save produced.
	afterResp := scGet(t, router)
	if afterResp.Code != http.StatusOK {
		t.Fatalf("post-conflict GET status = %d, body = %s", afterResp.Code, afterResp.Body.String())
	}
	var after scEditorResponse
	if err := json.Unmarshal(afterResp.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode post-conflict GET body: %v", err)
	}
	if after.Fingerprint != firstSaveResp.Fingerprint {
		t.Fatalf("fingerprint after rejected save = %q, want unchanged %q — the stale save mutated rows", after.Fingerprint, firstSaveResp.Fingerprint)
	}
}

// TestSaveAcceptsFreshFingerprint proves the fingerprint returned by a save
// is exactly what the next save needs to succeed — the normal edit/save/
// edit/save loop.
func TestSaveAcceptsFreshFingerprint(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchModel(t, pool)
	router := newSaveConflictRouter(pool)
	fieldID := scLAFieldID(t, pool, "LAF.5")

	getResp := scGet(t, router)
	var initial scEditorResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial GET body: %v", err)
	}

	firstSave := scPut(t, router, scOneFieldRequest(fieldID, "Name Type", initial.Fingerprint))
	if firstSave.Code != http.StatusOK {
		t.Fatalf("first save status = %d, body = %s", firstSave.Code, firstSave.Body.String())
	}
	var firstSaveResp scSaveResponse
	if err := json.Unmarshal(firstSave.Body.Bytes(), &firstSaveResp); err != nil {
		t.Fatalf("decode first save body: %v", err)
	}

	// A second save using the fingerprint the FIRST save just returned
	// must succeed — that's the fresh, round-tripped case.
	secondSave := scPut(t, router, scOneFieldRequest(fieldID, "Name Type Updated", firstSaveResp.Fingerprint))
	if secondSave.Code != http.StatusOK {
		t.Fatalf("second (fresh) save status = %d, want 200, body = %s", secondSave.Code, secondSave.Body.String())
	}
	var secondSaveResp scSaveResponse
	if err := json.Unmarshal(secondSave.Body.Bytes(), &secondSaveResp); err != nil {
		t.Fatalf("decode second save body: %v", err)
	}
	if secondSaveResp.Fingerprint == "" || secondSaveResp.Fingerprint == firstSaveResp.Fingerprint {
		t.Fatalf("expected a new fingerprint after the second save, got %q (was %q)", secondSaveResp.Fingerprint, firstSaveResp.Fingerprint)
	}
}

// TestSaveWithoutFingerprintStillWorks proves an empty fingerprint skips
// the staleness comparison entirely rather than being treated as always
// stale — ops tooling, git restore, and forks never load an editor
// payload, so they never have a fingerprint to send.
func TestSaveWithoutFingerprintStillWorks(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchModel(t, pool)
	router := newSaveConflictRouter(pool)
	fieldID := scLAFieldID(t, pool, "LAF.5")

	// Establish some content first, so the no-fingerprint save is
	// genuinely overwriting an existing, different pattern.
	getResp := scGet(t, router)
	var initial scEditorResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial GET body: %v", err)
	}
	firstSave := scPut(t, router, scOneFieldRequest(fieldID, "Name Type", initial.Fingerprint))
	if firstSave.Code != http.StatusOK {
		t.Fatalf("first save status = %d, body = %s", firstSave.Code, firstSave.Body.String())
	}

	noFingerprintSave := scPut(t, router, scOneFieldRequest(fieldID, "Name Type No Fingerprint", ""))
	if noFingerprintSave.Code != http.StatusOK {
		t.Fatalf("no-fingerprint save status = %d, want 200, body = %s", noFingerprintSave.Code, noFingerprintSave.Body.String())
	}
	var noFingerprintResp scSaveResponse
	if err := json.Unmarshal(noFingerprintSave.Body.Bytes(), &noFingerprintResp); err != nil {
		t.Fatalf("decode no-fingerprint save body: %v", err)
	}
	if noFingerprintResp.Fingerprint == "" {
		t.Fatal("expected a non-empty fingerprint in the response even though the request carried none")
	}
}
