//go:build integration

package project_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// newFullSaveConflictRouter mounts the model and collection slices
// alongside the project slice on ONE root router, in the same relative
// order pkg/weave/router.Mount uses in production:
// mountSlice(".../models", model.Mount) then
// mountSlice(".../collections", collection.Mount), both BEFORE
// project.Mount(parent, ...) (see router.go lines ~187-192 and ~235).
//
// model.Mount and collection.Mount register their own unlocked,
// fingerprint-blind PUT …/overrides handlers at the exact same path
// project.Mount later registers its locked, fingerprinted handler at.
// Verified empirically (by temporarily swapping the mount order and
// re-running this test, then reverting): chi resolves the two the same
// way regardless of which is mounted first, because project's mount is
// a longer, more specific path (a static "overrides" segment past
// {modelID}) than model's wildcard mount at ".../models" — chi prefers
// the more specific match at that tree position independent of
// registration order. So the real risk this test guards is not mount
// *order* so much as mount *shape*: if project's mountAt for this path
// were ever removed, or model/collection's own route somehow became the
// more specific one, the unlocked handler would start serving the save
// silently. Every other integration test in this file
// (newSaveConflictRouter / newSaveConflictRouterWithPools) mounts ONLY
// project.Mount, so none of them could ever notice that regression —
// the lock and the staleness check would stop running while every other
// gate on the branch stayed green. This constructor closes that gap by
// replicating the real mount shape (chi.Mount with a project-resource
// sub-mux) rather than a hand-rolled subset.
//
// Not replicated: the ~20 other slices router.Mount also registers
// (category, fields, settings, etc.) and the withChangeSetHint /
// ResolveContentVersion middleware mountSlice conditionally installs.
// None of those participate in the routing precedence question this
// test targets — model, collection, and project are the only three
// slices that register a handler at
// /projects/{projectID}/{models,collections}/{id}/overrides — and
// pulling in the rest would mean constructing hosts for slices with no
// bearing on this test, exactly the "elaborate harness" the review
// flagged against. See the report for the confidence caveat.
func newFullSaveConflictRouter(pool *pgxpool.Pool) http.Handler {
	weaveStore := weave.NewPostgresStore(pool)
	overrideStore := override.NewPostgresStore(pool)
	overrideSvc := override.NewService(overrideStore, nil, nil)
	projectStore := project.NewPostgresStore(pool)
	projectSvc := project.NewService(projectStore, nil, nil, nil, nil)

	projects := weaveStore.Projects()
	categories := weaveStore.WeaveCategories()

	// Dependency wiring mirrors pkg/app/weave_slice_hosts.go's
	// buildCoreEntityHosts exactly: same store, overrides, adoptions,
	// forks, projects, hierarchy, numberer, views, categories argument
	// mapping (deps.Weave stands in for both the numberer and the view
	// reader there; weaveStore plays that role here).
	modelSvc := model.NewService(
		model.NewPostgresStore(pool),
		overrideSvc,
		weaveStore.Adoptions(),
		weaveStore.Forks(),
		projects,
		projects,
		weaveStore,
		weaveStore,
		categories,
		nil,
		nil,
	)
	collectionSvc := collection.NewService(
		collection.NewPostgresStore(pool),
		overrideSvc,
		weaveStore.Adoptions(),
		weaveStore.Forks(),
		projects,
		projects,
		weaveStore,
		weaveStore,
		categories,
		nil,
		nil,
	)

	router := chi.NewRouter()

	// Mirrors router.mountSlice: a private sub-mux carrying the project
	// version + project-resource middleware, mounted at the slice's
	// prefix. mountSlice itself is unexported to pkg/weave/router, so
	// this reproduces its two load-bearing middlewares rather than the
	// unrelated withChangeSetHint/ResolveContentVersion ones, which
	// don't affect which handler a request reaches.
	mountAt := func(pattern string, attach func(chi.Router)) {
		sub := chi.NewMux()
		sub.Use(weaveauth.WithProjectVersionContext)
		sub.Use(weaveauth.WithProjectResource(weaveStore))
		attach(sub)
		router.Mount(pattern, sub)
	}

	mountAt("/projects/{projectID}/models", func(r chi.Router) {
		model.Mount(r, model.Host{Service: modelSvc, Projects: projects})
	})
	mountAt("/projects/{projectID}/collections", func(r chi.Router) {
		collection.Mount(r, collection.Host{Service: collectionSvc, Projects: projects})
	})

	// project.Mount runs last, exactly as it does in router.go — its
	// mountAt registers a MORE SPECIFIC route
	// (.../models/{modelID}/overrides) directly on this same root
	// router, alongside the WIDER mount model.Mount just installed at
	// .../models. The assertion below is that chi resolves the deeper,
	// more specific mount over the shallower one.
	project.Mount(router, project.Host{
		Service:   projectSvc,
		Overrides: overrideSvc,
		Weave:     weaveStore,
	})

	return router
}

// TestSaveRejectsStaleFingerprint_ThroughFullRouterWiring is the
// highest-leverage regression this branch was missing: every other
// save-conflict test in this file mounts only the project slice, so none
// of them would fail if the router ever resolved
// PUT /projects/{projectID}/models/{modelID}/overrides to the model
// slice's OWN SaveOverrides handler instead of project's — that handler
// takes neither the entity lock nor the fingerprint (pkg/weave/model/
// handler.go's SaveOverrides decodes straight from the request body and
// never reads a fingerprint field at all). This test builds the router
// the way pkg/weave/router.Mount actually assembles it — model and
// collection mounted before project, on the same root chi.Router — and
// re-runs the stale-fingerprint case through that full wiring.
//
// Two assertions distinguish "project's handler served this" from "the
// model slice's own handler served this" beyond the status code alone:
//   - the GET response must carry a non-empty "fingerprint" field — the
//     model slice's ListOverrides response has no such field at all, so
//     an empty/missing fingerprint here would mean the wrong handler
//     answered the GET.
//   - the stale PUT must come back 409 with code "conflict" and message
//     "pattern_changed" — the model slice's SaveOverrides always
//     returns 200 because it never checks a fingerprint, so a 200 here
//     would mean the wrong handler answered the PUT.
func TestSaveRejectsStaleFingerprint_ThroughFullRouterWiring(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchModel(t, pool)
	router := newFullSaveConflictRouter(pool)
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
		t.Fatal("expected a non-empty initial fingerprint — an empty one here would mean the model slice's own ListOverrides answered the GET instead of project's")
	}
	staleFingerprint := initial.Fingerprint

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

	secondSave := scPut(t, router, scURL, scOneFieldRequest(fieldID, "Name Type STALE ATTEMPT", staleFingerprint))
	if secondSave.Code != http.StatusConflict {
		t.Fatalf("second (stale) save status = %d, want 409 — a 200 here would mean the model slice's unlocked, fingerprint-blind SaveOverrides handler served this save instead of project's, body = %s", secondSave.Code, secondSave.Body.String())
	}
	var conflictBody scErrorBody
	if err := json.Unmarshal(secondSave.Body.Bytes(), &conflictBody); err != nil {
		t.Fatalf("decode conflict body: %v", err)
	}
	if conflictBody.Code != "conflict" || conflictBody.Message != "pattern_changed" {
		t.Fatalf("conflict body = %+v, want code=conflict message=pattern_changed (body: %s)", conflictBody, secondSave.Body.String())
	}

	count, displayName := scRowState(t, pool, "model", scModelID)
	if count != 1 || displayName != "Name Type" {
		t.Fatalf("override rows after rejected save: count=%d displayName=%q, want count=1 displayName=%q — the stale save must not have mutated rows", count, displayName, "Name Type")
	}
}
