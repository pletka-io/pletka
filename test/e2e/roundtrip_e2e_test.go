//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/app"
)

// TestRoundtripReadAndExport drives one black-box flow over real HTTP against
// the fixture clone:
//
//	register + promote  -> authenticated super_admin session (cookie jar)
//	GET fields list     -> real HTTP -> field service -> DB read (200 + fixture field)
//	GET field turtle     -> real HTTP -> generator service -> RDF export (200 + crm qname)
//
// The clone already contains testdb.FixtureParent (project LA, "Living
// Archives" — a real, self-contained fixture with no inheritance parents of
// its own, linked to the vendored CIDOC-CRM ontology and hundreds of crm:
// fields), so no ontology import happens mid-test — the flow operates
// against the seeded fixture. It is intentionally a READ + EXPORT flow: a
// write would require assembling a valid ontology-path payload, which is
// disproportionate for a starter net. The path is still a full
// HTTP -> service -> DB -> generator roundtrip. LA is used rather than the
// larger AME fixture since this test only needs one project with one
// crm-scoped field, not AME's cross-ontology/vendored-parent breadth.
func TestRoundtripReadAndExport(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	// Boot the real assembled app HTTP surface against the fixture clone.
	// Only Pool + the core frontend manifest are required; every other option
	// falls back to a core default (see pkg/app.New). Logs are discarded to
	// keep the test output focused on the flow's stages.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(ctx, app.Options{
		Pool:                     pool,
		Logger:                   logger,
		RegistrationEnabled:      true,
		CoreFrontendManifestPath: filepath.Join(repoRoot(t), "frontend", "core.frontend.json"),
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = application.Close(context.Background()) })

	srv := httptest.NewServer(application.Handler)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{Jar: jar}

	// --- Auth: register a user, then promote it to super_admin so it can read
	// the private fixture project and request generator exports. The auth
	// snapshot is rebuilt from weave_actors.role on every request, so the SQL
	// promotion takes effect on the next call that carries the session cookie.
	const (
		email    = "e2e-roundtrip@test.local"
		username = "e2e_roundtrip_user"
		password = "e2e_roundtrip_password_secret"
	)
	t.Cleanup(func() { cleanupUser(pool, email, username) })
	cleanupUser(pool, email, username) // in case a prior run left rows in the clone

	regBody, _ := json.Marshal(map[string]any{
		"name": "E2E Roundtrip", "email": email, "username": username, "password": password,
	})
	regResp, err := client.Post(srv.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	regResp.Body.Close()
	if regResp.StatusCode != http.StatusCreated {
		t.Fatalf("register: got %d, want 201", regResp.StatusCode)
	}
	t.Logf("stage register: 201 Created, session cookie issued")

	if _, err := pool.Exec(ctx, `UPDATE weave_actors SET role = 'super_admin' WHERE slug = $1`, username); err != nil {
		t.Fatalf("promote to super_admin: %v", err)
	}

	// Read back the ULID the generator route needs. The /gen/fields/{id} route
	// resolves fields by primary key (ULID), which the HTTP surface never
	// exposes for a fixture semantic id, so this one read is out-of-band.
	fieldULID := fieldULIDBySemanticID(t, pool, "LA", "LAF.230")

	// --- Stage 1 (READ): list the project's fields over HTTP. Real path is
	// HTTP -> field slice handler -> field service -> Postgres clone.
	t.Run("read fields list", func(t *testing.T) {
		// LA has 600+ fields and the list endpoint paginates (default 50/page),
		// so search for the target field by name rather than relying on it
		// landing on an unfiltered first page.
		resp, err := client.Get(srv.URL + "/projects/LA/fields/?search=gender")
		if err != nil {
			t.Fatalf("list fields request: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list fields: got %d, want 200; body=%s", resp.StatusCode, truncate(body))
		}
		// The seeded fixture field LAF.230 has system_name "gender" — its
		// presence proves the DB read returned the hydrated fixture data.
		if !bytes.Contains(body, []byte("gender")) {
			t.Fatalf("list fields: response missing fixture field %q; body=%s", "gender", truncate(body))
		}
		t.Logf("stage read: 200 OK, fields list includes fixture field gender")
	})

	// --- Stage 2 (EXPORT): request a turtle RDF export of the field. Real path
	// is HTTP -> visualization slice handler -> generator service -> RDF.
	t.Run("export field turtle", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/gen/fields/" + fieldULID + "/turtle")
		if err != nil {
			t.Fatalf("export turtle request: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("export turtle: got %d, want 200; body=%s", resp.StatusCode, truncate(body))
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "turtle") {
			t.Fatalf("export turtle: content-type %q does not contain %q", ct, "turtle")
		}
		if len(bytes.TrimSpace(body)) == 0 {
			t.Fatal("export turtle: empty body")
		}
		// The fixture field's ontology path is rooted at crm:E21_Person via the
		// vendored CIDOC-CRM ontology, so the serialized RDF must reference the
		// CRM namespace.
		if !bytes.Contains(body, []byte("cidoc-crm")) {
			t.Fatalf("export turtle: RDF missing CIDOC-CRM namespace; body=%s", truncate(body))
		}
		t.Logf("stage export: 200 OK, %d bytes of turtle referencing the CRM namespace", len(body))
	})
}

// fieldULIDBySemanticID reads the generated primary-key ULID for a fixture
// field, keyed by its stable project + semantic id.
func fieldULIDBySemanticID(t *testing.T, pool *pgxpool.Pool, projectID, semanticID string) string {
	t.Helper()
	var ulid string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM weave_fields WHERE project_id = $1 AND semantic_id = $2`,
		projectID, semanticID).Scan(&ulid)
	if err != nil {
		t.Fatalf("look up field ULID for %s/%s: %v", projectID, semanticID, err)
	}
	if ulid == "" {
		t.Fatalf("field ULID for %s/%s is empty", projectID, semanticID)
	}
	return ulid
}

// cleanupUser removes the registered test user (auth row first for the FK) so a
// re-run against the same clone does not hit a unique-email/slug collision.
func cleanupUser(pool *pgxpool.Pool, email, username string) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
}

// truncate bounds an error-path body dump so a large RDF payload does not flood
// the test log.
func truncate(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// repoRoot walks up from the test working directory to the module root (the
// directory holding go.mod) so the frontend manifest can be resolved by an
// absolute path regardless of where `go test` runs.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("could not find repo root (go.mod)")
		}
		wd = parent
	}
}
