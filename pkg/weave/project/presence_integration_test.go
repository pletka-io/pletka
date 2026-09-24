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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/ids"
)

// scPresenceURL and scCollPresenceURL are the presence sub-routes of the
// scratch model/collection overrides endpoints declared in
// save_conflict_integration_test.go.
const (
	scPresenceURL     = scURL + "/presence"
	scCollPresenceURL = scCollURL + "/presence"
)

// presenceBody is the presence heartbeat/left request shape (mirrors
// project.presenceRequest, which is unexported).
type presenceBody struct {
	State     string `json:"state,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// presenceEditor and presenceEnvelope mirror override.Editor and
// project.presenceResponse (both unexported outside their packages) so
// this test can decode the real wire response.
type presenceEditor struct {
	ActorID string    `json:"actor_id"`
	Name    string    `json:"name,omitempty"`
	Since   time.Time `json:"since"`
}

type presenceEnvelope struct {
	Editors []presenceEditor `json:"editors"`
}

// presencePost posts a presence request as ctx and returns the raw
// response. body == nil sends a truly empty request body — the shape of a
// browser pagehide/unload beacon sent with no payload (fix round 1,
// finding 5) — rather than an empty JSON object.
func presencePost(t *testing.T, router http.Handler, ctx context.Context, url string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal presence request: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodePresenceBody(t *testing.T, w *httptest.ResponseRecorder) presenceEnvelope {
	t.Helper()
	var env presenceEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode presence response: %v (body: %s)", err, w.Body.String())
	}
	return env
}

// viewerAuthContext is a caller with no roles at all. On a public project
// that resolves to auth's public-read fallback: ProjectRead but not
// ProjectEdit — a real "can see it, can't touch it" viewer, as opposed to
// editorAuthContext (save_conflict_integration_test.go), which is a
// superadmin and can edit anything.
func viewerAuthContext(actorID string) context.Context {
	return weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{ActorID: actorID})
}

// editorAuthContextAs is editorAuthContext parameterised on actor id, so
// two distinct editors can be told apart in a presence list.
func editorAuthContextAs(actorID string) context.Context {
	return weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		IsSuperAdmin: true,
		ActorID:      actorID,
	})
}

// seedPublicProjectWithModel seeds a dedicated PUBLIC project with one
// model, for tests that need a real "viewer can read, can't edit" caller.
// LA (used by save_conflict_integration_test.go's scratch helpers) is
// deliberately not reused here: its visibility is fixture data no test in
// this package assumes one way or the other (see
// TestSaveOverridesDeniesOutsiderBeforeReachingTheLock's doc comment) —
// this test needs to KNOW the project is public, not hope it is.
func seedPublicProjectWithModel(t *testing.T, pool *pgxpool.Pool) (projectID, modelID string) {
	t.Helper()
	ctx := context.Background()
	projectID = ids.GenerateULID()
	modelID = ids.GenerateULID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, visibility, ui_name) VALUES ($1, 'unite', 'public', $2)`,
		projectID, []byte(`{"en":"Presence Viewer Probe"}`)); err != nil {
		t.Fatalf("seed public project: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_models WHERE id = $1`, modelID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, projectID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1, $2)`, modelID, projectID); err != nil {
		t.Fatalf("seed model under public project: %v", err)
	}
	return projectID, modelID
}

// TestPresenceViewerGetsEmptyListAndRecordsNothing is fix round 1, finding
// 1's first bullet: a read-only viewer on a readable (public) project gets
// an empty editor list back, and — the part a status-code check alone
// can't prove — is never actually recorded: a real editor reading right
// after must not see the viewer either.
func TestPresenceViewerGetsEmptyListAndRecordsNothing(t *testing.T) {
	pool := testdb.Pool(t)
	projectID, modelID := seedPublicProjectWithModel(t, pool)
	router := newSaveConflictRouter(pool)
	url := fmt.Sprintf("/projects/%s/models/%s/overrides/presence", projectID, modelID)

	// A curator must already be editing when the viewer reads, or the
	// empty list below proves nothing: an empty registry returns an empty
	// list however broken the gate is. This is the disclosure that matters
	// — who is editing, under what name, since when.
	present := editorAuthContextAs("presence-editor-already-here")
	wPresent := presencePost(t, router, present, url, presenceBody{State: "editing", SessionID: "present-tab"})
	if wPresent.Code != http.StatusOK {
		t.Fatalf("seeding editor presence status = %d, want 200, body = %s", wPresent.Code, wPresent.Body.String())
	}

	viewer := viewerAuthContext("presence-viewer-only")
	w := presencePost(t, router, viewer, url, presenceBody{State: "editing", SessionID: "viewer-tab"})
	if w.Code != http.StatusOK {
		t.Fatalf("viewer presence status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := decodePresenceBody(t, w).Editors; len(got) != 0 {
		t.Fatalf("viewer was told who is editing: %+v — a non-editor must learn nothing about other curators", got)
	}

	editor := editorAuthContextAs("presence-editor-after-viewer")
	w2 := presencePost(t, router, editor, url, presenceBody{State: "editing", SessionID: "editor-tab"})
	if w2.Code != http.StatusOK {
		t.Fatalf("editor presence status = %d, want 200, body = %s", w2.Code, w2.Body.String())
	}
	// The seeded curator is expected here; the viewer is not, because a
	// non-editor's heartbeat must never have been recorded.
	got := decodePresenceBody(t, w2).Editors
	for _, e := range got {
		if e.ActorID == "presence-viewer-only" {
			t.Fatalf("editor saw the viewer in the editors list: %+v — the viewer must not have been recorded", got)
		}
	}
	if len(got) != 1 || got[0].ActorID != "presence-editor-already-here" {
		t.Fatalf("editor presence editors = %+v, want only the curator seeded above", got)
	}
}

// TestPresenceViewerCannotDistinguishEntityExistenceByStatusCode is fix
// round 1, finding 4: the presence gate must run CanEdit BEFORE the
// entity-ownership lookup, exactly like the save's own project -> CanEdit
// -> ownership order — not after it. Reading a real model that belongs to
// the project and a made-up model id under the same project must produce
// the IDENTICAL response for a non-editor. The reversed order (entity
// lookup before CanEdit) lets a viewer fingerprint entity existence purely
// from the status code — 200 (exists, viewer just can't edit it) vs. 404
// (doesn't exist) — with no real security consequence on its own (the
// viewer can already see the whole pattern via the neighbouring GET
// route), but the whole point of this endpoint's gate is that it runs the
// edit check first, and this is the one observable difference that proves
// it actually does.
func TestPresenceViewerCannotDistinguishEntityExistenceByStatusCode(t *testing.T) {
	pool := testdb.Pool(t)
	projectID, modelID := seedPublicProjectWithModel(t, pool)
	router := newSaveConflictRouter(pool)

	viewer := viewerAuthContext("presence-order-viewer")
	realURL := fmt.Sprintf("/projects/%s/models/%s/overrides/presence", projectID, modelID)
	fakeURL := fmt.Sprintf("/projects/%s/models/%s/overrides/presence", projectID, ids.GenerateULID())

	realResp := presencePost(t, router, viewer, realURL, presenceBody{State: "editing", SessionID: "s1"})
	fakeResp := presencePost(t, router, viewer, fakeURL, presenceBody{State: "editing", SessionID: "s1"})

	if realResp.Code != http.StatusOK {
		t.Fatalf("viewer presence for the real model status = %d, want 200, body = %s", realResp.Code, realResp.Body.String())
	}
	if fakeResp.Code != realResp.Code {
		t.Fatalf("viewer presence status differs between a real model (%d) and a made-up model id (%d) — the check order leaks entity existence to a non-editor", realResp.Code, fakeResp.Code)
	}
	if fakeResp.Body.String() != realResp.Body.String() {
		t.Fatalf("viewer presence bodies differ between a real and a made-up model id: %q vs %q", realResp.Body.String(), fakeResp.Body.String())
	}
}

// TestPresenceCrossProjectModelGetsSameNotFoundAsSave is fix round 1,
// finding 1's second bullet: an EDITOR (who can edit project A) hitting
// project B's model through project A's URL still gets the same 404 the
// save gives — the entity-ownership check itself must survive sitting
// after the (now-passing) CanEdit check, not get lost in the reorder.
func TestPresenceCrossProjectModelGetsSameNotFoundAsSave(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	projectAID := ids.GenerateULID()
	projectBID := ids.GenerateULID()
	modelInBID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, visibility, ui_name) VALUES ($1, 'unite', 'private', $3), ($2, 'unite', 'private', $3)`,
		projectAID, projectBID, []byte(`{"en":"Presence Cross-Project Probe"}`)); err != nil {
		t.Fatalf("seed projects A and B: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_models WHERE id = $1`, modelInBID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{projectAID, projectBID})
	})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1, $2)`, modelInBID, projectBID); err != nil {
		t.Fatalf("seed model under project B: %v", err)
	}

	router := newSaveConflictRouter(pool)
	url := fmt.Sprintf("/projects/%s/models/%s/overrides/presence", projectAID, modelInBID)

	w := presencePost(t, router, editorAuthContextAs("presence-cross-project-editor"), url, presenceBody{State: "editing", SessionID: "s1"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project presence status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
}

// TestPresenceEditorSeesOtherEditorNeverSelf is fix round 1, finding 1's
// third bullet: an editor sees another editor, and never themselves —
// exercised with two distinct actor ids, through the real router.
func TestPresenceEditorSeesOtherEditorNeverSelf(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchCollection(t, pool)
	router := newSaveConflictRouter(pool)

	alice := editorAuthContextAs("presence-alice")
	bob := editorAuthContextAs("presence-bob")

	w := presencePost(t, router, alice, scCollPresenceURL, presenceBody{State: "editing", SessionID: "alice-tab"})
	if w.Code != http.StatusOK {
		t.Fatalf("alice presence status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := decodePresenceBody(t, w).Editors; len(got) != 0 {
		t.Fatalf("alice's own presence read = %+v, want none — alice must never see herself", got)
	}

	w2 := presencePost(t, router, bob, scCollPresenceURL, presenceBody{State: "editing", SessionID: "bob-tab"})
	if w2.Code != http.StatusOK {
		t.Fatalf("bob presence status = %d, body = %s", w2.Code, w2.Body.String())
	}
	got2 := decodePresenceBody(t, w2).Editors
	if len(got2) != 1 || got2[0].ActorID != "presence-alice" {
		t.Fatalf("bob's presence read = %+v, want exactly [presence-alice]", got2)
	}

	w3 := presencePost(t, router, alice, scCollPresenceURL, presenceBody{State: "editing", SessionID: "alice-tab"})
	if w3.Code != http.StatusOK {
		t.Fatalf("alice's second presence status = %d, body = %s", w3.Code, w3.Body.String())
	}
	got3 := decodePresenceBody(t, w3).Editors
	if len(got3) != 1 || got3[0].ActorID != "presence-bob" {
		t.Fatalf("alice's second presence read = %+v, want exactly [presence-bob]", got3)
	}
}

// TestPresenceEmptyBodyIsTreatedAsLeft is fix round 1, finding 5: a
// pagehide/unload beacon sent with no payload at all must not 400 — it is
// treated the same as an explicit {"state":"left"}, and it actually drops
// the caller's presence (proven by a separate watcher never seeing them),
// not merely returning an empty-looking response.
func TestPresenceEmptyBodyIsTreatedAsLeft(t *testing.T) {
	pool := testdb.Pool(t)
	seedScratchModel(t, pool)
	router := newSaveConflictRouter(pool)

	editor := editorAuthContextAs("presence-empty-body-editor")
	w := presencePost(t, router, editor, scPresenceURL, presenceBody{State: "editing", SessionID: "s1"})
	if w.Code != http.StatusOK {
		t.Fatalf("beat status = %d, body = %s", w.Code, w.Body.String())
	}

	w2 := presencePost(t, router, editor, scPresenceURL, nil) // no body at all
	if w2.Code != http.StatusOK {
		t.Fatalf("empty-body presence status = %d, want 200 (treated as left), body = %s", w2.Code, w2.Body.String())
	}
	if got := decodePresenceBody(t, w2).Editors; len(got) != 0 {
		t.Fatalf("empty-body presence editors = %+v, want none", got)
	}

	watcher := editorAuthContextAs("presence-empty-body-watcher")
	w3 := presencePost(t, router, watcher, scPresenceURL, presenceBody{State: "editing", SessionID: "watcher-tab"})
	if got := decodePresenceBody(t, w3).Editors; len(got) != 0 {
		t.Fatalf("watcher's presence read = %+v, want none — the empty-body left must have actually dropped the editor, not just answered them with an empty list", got)
	}
}
