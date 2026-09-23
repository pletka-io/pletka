//go:build integration

package project_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// Scratch coordinates for the save-conflict tests: a scratch model and a
// scratch collection on the real LA fixture project, holding real LA
// fixture fields (LAF.5, LAF.6) — mirrors the scratch-entity convention
// used by pkg/weave/override/fingerprint_integration_test.go and
// replace_integration_test.go, one level up at the HTTP handler.
const (
	scProjectID    = "LA"
	scModelID      = "TSTSC.1"
	scURL          = "/projects/LA/models/TSTSC.1/overrides"
	scCollectionID = "TSTSC.2"
	scCollURL      = "/projects/LA/collections/TSTSC.2/overrides"
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

// seedScratchCollection is seedScratchModel's collection counterpart —
// weave_collections has the same "only id + project_id are NOT NULL
// without a default" shape.
func seedScratchCollection(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_collections (id, project_id) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		scCollectionID, scProjectID); err != nil {
		t.Fatalf("seed scratch collection: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_override_refs WHERE override_id IN (
			SELECT id FROM weave_field_overrides WHERE entity_type = 'collection' AND entity_id = $1)`, scCollectionID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_field_overrides WHERE entity_type = 'collection' AND entity_id = $1`, scCollectionID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_collections WHERE id = $1`, scCollectionID)
	})
}

// newSaveConflictRouter wires the real Handler over the REAL production
// routes via project.Mount — the exact Host/Mount wiring pkg/app uses, not
// a hand-rolled subset. That matters: routes.go's mountAt applies
// auth.WithProjectVersionContext + auth.WithProjectResource, and the
// overrides route groups additionally require auth.RequireProjectRead.
// Reusing Mount (instead of re-declaring a slimmer middleware stack) is
// what makes TestSaveOverridesDeniesOutsiderBeforeReachingTheLock a
// meaningful guard: if the ownership/edit checks in saveOverrides ever
// moved below the lock, this router would still enforce the same gates
// production does, so a regression there has nowhere to hide behind a
// weaker test router. See Task 3 fix round 1, finding 6.
func newSaveConflictRouter(pool *pgxpool.Pool) http.Handler {
	return newSaveConflictRouterWithPools(pool, pool)
}

// newSaveConflictRouterWithPools is newSaveConflictRouter with the override
// store built on its own pool, separate from the one project/weave use.
// TestSaveReportsFailureWhenTheLockIsNeverAcquired needs this: it starves
// ONLY the override store's connection pool (so WithEntityLock's own
// pool.Acquire fails) while the project/model lookups that run before the
// lock keep using a healthy pool.
func newSaveConflictRouterWithPools(pool, overridePool *pgxpool.Pool) http.Handler {
	weaveStore := weave.NewPostgresStore(pool)
	overrideStore := override.NewPostgresStore(overridePool)
	overrideSvc := override.NewService(overrideStore, nil, nil)
	projectStore := project.NewPostgresStore(pool)
	projectSvc := project.NewService(projectStore, nil, nil, nil)

	router := chi.NewRouter()
	project.Mount(router, project.Host{
		Service:   projectSvc,
		Overrides: overrideSvc,
		Weave:     weaveStore,
	})
	return router
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
// collection-grouped) field on the scratch entity, using displayName as its
// override display_name.en — the one property whose value the caller
// varies between saves so the fingerprint changes. Works for both the
// model and the collection editor: the placements-diff path it exercises
// on a model is entityType-gated and simply does not run for a collection.
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

func scGet(t *testing.T, router http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(editorAuthContext(), http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func scPut(t *testing.T, router http.Handler, url string, body scSaveRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal save request: %v", err)
	}
	req := httptest.NewRequestWithContext(editorAuthContext(), http.MethodPut, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// scRowState reads the override rows actually committed for (entityType,
// entityID) directly from Postgres — the row count and the display_name.en
// of the last one scanned. Used to prove "a rejected save touched nothing"
// against the real rows, not against the fingerprint the feature under
// test itself computes (Task 3 fix round 1, finding 4).
func scRowState(t *testing.T, pool *pgxpool.Pool, entityType, entityID string) (count int, displayNameEn string) {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT display_name FROM weave_field_overrides WHERE entity_type = $1 AND entity_id = $2`,
		entityType, entityID)
	if err != nil {
		t.Fatalf("query override rows for %s %s: %v", entityType, entityID, err)
	}
	defer rows.Close()
	for rows.Next() {
		count++
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan override row: %v", err)
		}
		var dn map[string]string
		if err := json.Unmarshal(raw, &dn); err != nil {
			t.Fatalf("unmarshal display_name: %v", err)
		}
		displayNameEn = dn["en"]
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate override rows: %v", err)
	}
	return count, displayNameEn
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

	getResp := scGet(t, router, scURL)
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
	firstSave := scPut(t, router, scURL, scOneFieldRequest(fieldID, "Name Type", staleFingerprint))
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
	secondSave := scPut(t, router, scURL, scOneFieldRequest(fieldID, "Name Type STALE ATTEMPT", staleFingerprint))
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

	// The rejected save must not have touched the rows. Prove it two
	// ways: directly against Postgres (the actual claim), and via a
	// fresh GET's fingerprint (what the feature itself reports) — belt
	// and suspenders, per finding 4: the fingerprint alone would only
	// prove the absence of a change in what the fingerprint happens to
	// cover, using the very mechanism under test.
	count, displayName := scRowState(t, pool, "model", scModelID)
	if count != 1 {
		t.Fatalf("override row count after rejected save = %d, want 1", count)
	}
	if displayName != "Name Type" {
		t.Fatalf("override display_name after rejected save = %q, want %q (the stale save mutated rows)", displayName, "Name Type")
	}

	afterResp := scGet(t, router, scURL)
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

// TestSaveCollectionRejectsStaleFingerprint is TestSaveRejectsStaleFingerprint's
// collection counterpart (Task 3 fix round 1, finding 5): the collection
// editor's GET fingerprint and the collection branch of saveOverrides —
// which skips the model-only placements diff entirely — had no coverage.
func TestSaveCollectionRejectsStaleFingerprint(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchCollection(t, pool)
	router := newSaveConflictRouter(pool)
	fieldID := scLAFieldID(t, pool, "LAF.6")

	getResp := scGet(t, router, scCollURL)
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

	firstSave := scPut(t, router, scCollURL, scOneFieldRequest(fieldID, "Collection Field", staleFingerprint))
	if firstSave.Code != http.StatusOK {
		t.Fatalf("first save status = %d, body = %s", firstSave.Code, firstSave.Body.String())
	}
	var firstSaveResp scSaveResponse
	if err := json.Unmarshal(firstSave.Body.Bytes(), &firstSaveResp); err != nil {
		t.Fatalf("decode first save body: %v", err)
	}

	secondSave := scPut(t, router, scCollURL, scOneFieldRequest(fieldID, "Collection Field STALE ATTEMPT", staleFingerprint))
	if secondSave.Code != http.StatusConflict {
		t.Fatalf("second (stale) save status = %d, want 409, body = %s", secondSave.Code, secondSave.Body.String())
	}
	var conflictBody scErrorBody
	if err := json.Unmarshal(secondSave.Body.Bytes(), &conflictBody); err != nil {
		t.Fatalf("decode conflict body: %v", err)
	}
	if conflictBody.Code != "conflict" || conflictBody.Message != "pattern_changed" {
		t.Fatalf("conflict body = %+v, want code=conflict message=pattern_changed (body: %s)", conflictBody, secondSave.Body.String())
	}

	count, displayName := scRowState(t, pool, "collection", scCollectionID)
	if count != 1 || displayName != "Collection Field" {
		t.Fatalf("override rows after rejected save: count=%d displayName=%q, want count=1 displayName=%q", count, displayName, "Collection Field")
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

	getResp := scGet(t, router, scURL)
	var initial scEditorResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial GET body: %v", err)
	}

	firstSave := scPut(t, router, scURL, scOneFieldRequest(fieldID, "Name Type", initial.Fingerprint))
	if firstSave.Code != http.StatusOK {
		t.Fatalf("first save status = %d, body = %s", firstSave.Code, firstSave.Body.String())
	}
	var firstSaveResp scSaveResponse
	if err := json.Unmarshal(firstSave.Body.Bytes(), &firstSaveResp); err != nil {
		t.Fatalf("decode first save body: %v", err)
	}

	// A second save using the fingerprint the FIRST save just returned
	// must succeed — that's the fresh, round-tripped case.
	secondSave := scPut(t, router, scURL, scOneFieldRequest(fieldID, "Name Type Updated", firstSaveResp.Fingerprint))
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
	getResp := scGet(t, router, scURL)
	var initial scEditorResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial GET body: %v", err)
	}
	firstSave := scPut(t, router, scURL, scOneFieldRequest(fieldID, "Name Type", initial.Fingerprint))
	if firstSave.Code != http.StatusOK {
		t.Fatalf("first save status = %d, body = %s", firstSave.Code, firstSave.Body.String())
	}

	noFingerprintSave := scPut(t, router, scURL, scOneFieldRequest(fieldID, "Name Type No Fingerprint", ""))
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

// TestSaveOverridesDeniesOutsiderBeforeReachingTheLock is Task 3 fix round
// 1, finding 6: the previous test router mounted only
// auth.WithProjectResource and always ran as a superadmin, so no test
// would have failed if the ownership/edit checks in saveOverrides were
// ever moved below the lock — exactly the property this task most needs
// to keep. newSaveConflictRouter now mounts the real production routes
// (project.Mount), including auth.RequireProjectRead; a caller with no
// role on a private project must be turned back by that middleware, with
// a 404 that carries no fingerprint — proving the gate still runs, and
// runs first.
func TestSaveOverridesDeniesOutsiderBeforeReachingTheLock(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	// A dedicated private project + model, rather than reusing LA:
	// LA's own visibility is fixture data this test shouldn't have to
	// assume, and a private project with no membership grants for the
	// outsider guarantees RequireProjectRead denies regardless.
	projectID := ids.GenerateULID()
	modelID := ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, visibility, ui_name) VALUES ($1, 'unite', 'private', $2)`,
		projectID, []byte(`{"en":"Save Conflict Outsider Probe"}`)); err != nil {
		t.Fatalf("seed private project: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_models WHERE id = $1`, modelID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, projectID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1, $2)`, modelID, projectID); err != nil {
		t.Fatalf("seed model under private project: %v", err)
	}

	router := newSaveConflictRouter(pool)
	url := fmt.Sprintf("/projects/%s/models/%s/overrides", projectID, modelID)

	outsider := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{ActorID: "outsider-no-roles"})
	body, err := json.Marshal(scOneFieldRequest("irrelevant-field-id", "Outsider Attempt", ""))
	if err != nil {
		t.Fatalf("marshal outsider save body: %v", err)
	}
	req := httptest.NewRequestWithContext(outsider, http.MethodPut, url, bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("outsider save status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("fingerprint")) {
		t.Fatalf("outsider save response unexpectedly carries a fingerprint: %s", w.Body.String())
	}
}

// TestSaveOverridesDeniesCrossProjectModel is Task 3 fix round 2, finding
// 2: TestSaveOverridesDeniesOutsiderBeforeReachingTheLock proves
// auth.RequireProjectRead turns back a caller with no role at all, but
// that gate runs in middleware, before saveOverrides is ever entered — so
// it would still pass even if the handler's OWN checks moved below the
// lock or were removed.
//
// An initial attempt at this used a viewer role — passes
// auth.RequireProjectRead (viewer has ProjectRead) but should fail
// h.svc.CanEdit (viewer lacks ProjectEdit). That turned out not to
// isolate the handler's own check at all: override.Service.SaveForEntity
// independently enforces ProjectEdit on the target project
// (requireProjectWrite), so disabling h.svc.CanEdit alone left the test
// green — the service's own gate caught it regardless, same 403.
// Confirmed by temporarily disabling h.svc.CanEdit and re-running: still
// PASS (see fix round 2 report for the transcript).
//
// This test targets a check with NO such second line of defense: the
// "model belongs to the URL's project" ownership check in saveOverrides'
// switch statement. override.Service.SaveForEntity only ever checks
// ProjectEdit on the projectID it's given — it has no idea entityID
// might belong to a different project, and if that URL/model mismatch
// went uncaught, flattenOverrideDraft would happily stamp the foreign
// model's overrides with the URL project's id. A superadmin (who passes
// every role/permission gate that exists) saving project B's model
// through project A's URL must still get a 404 here, and only the
// ownership check in saveOverrides can produce it.
func TestSaveOverridesDeniesCrossProjectModel(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	projectAID := ids.GenerateULID()
	projectBID := ids.GenerateULID()
	modelInBID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, visibility, ui_name) VALUES ($1, 'unite', 'private', $3), ($2, 'unite', 'private', $3)`,
		projectAID, projectBID, []byte(`{"en":"Save Conflict Cross-Project Probe"}`)); err != nil {
		t.Fatalf("seed projects A and B: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_field_overrides WHERE entity_type = 'model' AND entity_id = $1`, modelInBID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_models WHERE id = $1`, modelInBID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{projectAID, projectBID})
	})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1, $2)`, modelInBID, projectBID); err != nil {
		t.Fatalf("seed model under project B: %v", err)
	}

	router := newSaveConflictRouter(pool)
	// The URL names project A, but modelInBID actually belongs to
	// project B — that mismatch is exactly what the ownership check
	// must catch.
	url := fmt.Sprintf("/projects/%s/models/%s/overrides", projectAID, modelInBID)
	fieldID := scLAFieldID(t, pool, "LAF.5")

	body, err := json.Marshal(scOneFieldRequest(fieldID, "Cross Project Attempt", ""))
	if err != nil {
		t.Fatalf("marshal cross-project save body: %v", err)
	}
	req := httptest.NewRequestWithContext(editorAuthContext(), http.MethodPut, url, bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project save status = %d, want 404, body = %s", w.Code, w.Body.String())
	}

	count, _ := scRowState(t, pool, "model", modelInBID)
	if count != 0 {
		t.Fatalf("override row count after denied cross-project save = %d, want 0 — the save must not have run", count)
	}
}

// newSingleConnPool opens a fresh pool against the same database as pool,
// capped at one connection, so a test can starve a specific store's pool
// (by holding that one connection) without touching the shared pool other
// stores use. Mirrors
// pkg/weave/override/fingerprint_integration_test.go's
// TestWithAdvisoryLockResetsLockTimeout, which uses the same
// ParseConfig(pool.Config().ConnString())+MaxConns=1 trick.
func newSingleConnPool(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(pool.Config().ConnString())
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.MaxConns = 1
	single, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open single-conn pool: %v", err)
	}
	t.Cleanup(single.Close)
	return single
}

// TestSaveReportsFailureWhenTheLockIsNeverAcquired is Task 3 fix round 2,
// finding 1: WithAdvisoryLock has three failure paths that return before
// ever calling the callback — acquiring the lock's own connection,
// setting lock_timeout on it, or a lock-acquisition error that isn't
// SQLSTATE 55P03 — and none of them is ErrLockBusy. The round-1 fix
// mistook "not ErrLockBusy, and the callback never set an error" for "the
// callback ran and succeeded", and reported 200 with an empty fingerprint
// for a save that wrote nothing (worse: the empty fingerprint then makes
// the client's NEXT save skip the staleness check entirely).
//
// Forces exactly the first of those three paths: the override store's
// pool has its one and only connection held for the whole test, so
// WithEntityLock's own pool.Acquire blocks past the request's short
// deadline and returns — without the callback ever running — a
// "context deadline exceeded" error that is not ErrLockBusy. The
// project/model lookups that run before the lock keep using the normal,
// healthy pool, so this isolates the failure to lock acquisition alone.
func TestSaveReportsFailureWhenTheLockIsNeverAcquired(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchModel(t, pool)
	fieldID := scLAFieldID(t, pool, "LAF.5")

	overridePool := newSingleConnPool(t, pool)
	heldConn, err := overridePool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire the exhausted pool's only connection: %v", err)
	}
	t.Cleanup(heldConn.Release)

	router := newSaveConflictRouterWithPools(pool, overridePool)

	ctx, cancel := context.WithTimeout(editorAuthContext(), 500*time.Millisecond)
	defer cancel()

	body, err := json.Marshal(scOneFieldRequest(fieldID, "Never Ran", ""))
	if err != nil {
		t.Fatalf("marshal save body: %v", err)
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPut, scURL, bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (lock connection never acquired, callback never ran), body = %s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("fingerprint")) {
		t.Fatalf("response unexpectedly carries a fingerprint: %s", w.Body.String())
	}

	count, _ := scRowState(t, pool, "model", scModelID)
	if count != 0 {
		t.Fatalf("override row count = %d, want 0 — nothing should have been written", count)
	}
}
