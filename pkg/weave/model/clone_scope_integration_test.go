//go:build integration

package model_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

// TestClone_ReadsDraftEvenUnderAReleaseScopedRequest pins that
// ForkFromSource is a write path: it must copy the source model's
// working (draft) overrides regardless of what the request context is
// scoped to. A fork that honoured a release scope instead would copy
// that release's archived overrides into live rows on the new fork — a
// silent corruption that no view test would catch and a user would only
// discover much later as mangled data.
func TestClone_ReadsDraftEvenUnderAReleaseScopedRequest(t *testing.T) {
	pool := testdb.Pool(t)

	const (
		// Deliberately not ending in F/M/C: domain.ParseSemanticID
		// (used by buildAdoptionsFromOverrides on the field_id) treats a
		// trailing F/M/C as a type-code suffix and strips it from the
		// project id it derives — a project id ending in one of those
		// letters would misparse "<projectID>.f1" and break the FK on
		// weave_adoptions.source_project_id.
		srcProjectID = "ZFRKS"
		dstProjectID = "ZFRKD"
		version      = "1.0.0"
	)

	// Source model has 2 live overrides and 1 archived at 1.0.0.
	srcModelID := seedForkFixture(t, pool, srcProjectID, dstProjectID, version)

	// The trap: the request driving the fork is scoped to a RELEASE. If
	// ForkFromSource read at this scope it would copy the single
	// archived override instead of the two live ones.
	ctx := auth.WithReadScope(forkTestContext(t), auth.Release(version))

	svc := newForkTestService(pool)

	forked, err := svc.ForkFromSource(ctx, dstProjectID, srcModelID)
	if err != nil {
		t.Fatalf("ForkFromSource: %v", err)
	}
	t.Cleanup(func() {
		cleanupForkedModel(pool, dstProjectID, forked.ID)
	})

	got := liveOverrideCount(t, pool, "model", forked.ID)
	if got != 2 {
		t.Fatalf("fork produced %d overrides, want 2 — it read the release archive instead of the draft", got)
	}
}

// forkTestContext returns a super-admin context, which satisfies
// model.Service.requireProjectWrite and override.Service's own
// requireProjectRead/Write without needing a real membership row —
// mirrors serviceTestContext in
// pkg/weave/override/service_scope_integration_test.go.
func forkTestContext(t *testing.T) context.Context {
	t.Helper()
	return auth.WithSnapshot(context.Background(), &auth.AuthSnapshot{IsSuperAdmin: true})
}

// newForkTestService wires a model.Service against real Postgres-backed
// stores, mirroring the production wiring in
// pkg/app/weave_slice_hosts.go (buildCoreEntityHosts): the model store,
// the override service, and the shared weave aggregate for
// adoptions/forks/entity-numbering. projects, hierarchy, views and
// categories are left nil — ForkFromSource only touches them behind nil
// checks (ontology-setup gate, category remap), and this test does not
// exercise either.
func newForkTestService(pool *pgxpool.Pool) *model.Service {
	agg := weave.NewPostgresStore(pool)
	overrides := override.NewService(override.NewPostgresStore(pool), nil, nil)
	return model.NewService(
		model.NewPostgresStore(pool),
		overrides,
		agg.Adoptions(),
		agg.Forks(),
		nil, // projects
		nil, // hierarchy
		agg, // numberer
		nil, // views
		nil, // categories
		nil, // log
		nil, // runner
	)
}

// seedForkFixture creates two scratch projects (source and destination,
// sharing one owner actor), a source model with two live overrides and
// exactly one archived override at version, and the adoption row that
// lets the destination project fork the source model. Returns the
// source model's id. Registers t.Cleanup scoped by project id, never by
// version_number alone, since this package's fixtures share one
// database and several tests reuse version "1.0.0".
func seedForkFixture(t *testing.T, pool *pgxpool.Pool, srcProjectID, dstProjectID, version string) string {
	t.Helper()
	ctx := context.Background()
	ownerID := srcProjectID + "_OWNER"
	srcModelID := srcProjectID + "M.1"

	mustFork(t, pool, ctx, `INSERT INTO weave_actors (id, display_name, slug)
		VALUES ($1, $1, $1) ON CONFLICT (id) DO NOTHING`, ownerID)

	mustFork(t, pool, ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, $2)
		ON CONFLICT (id) DO NOTHING`, srcProjectID, ownerID)
	mustFork(t, pool, ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, $2)
		ON CONFLICT (id) DO NOTHING`, dstProjectID, ownerID)

	uiName, err := json.Marshal(map[string]string{"en": "Fork Source Model"})
	if err != nil {
		t.Fatalf("marshal ui_name: %v", err)
	}
	// ontology_scope must carry a local_name — Create's validateCreate
	// (reached via ForkFromSource, which copies source.OntologyScope
	// onto the fork) rejects an empty one.
	ontologyScope, err := json.Marshal(map[string]string{
		"type": "class", "prefix": "crm", "local_name": "E1_CRM_Entity", "uri": "crm:E1_CRM_Entity",
	})
	if err != nil {
		t.Fatalf("marshal ontology_scope: %v", err)
	}
	mustFork(t, pool, ctx, `INSERT INTO weave_models (id, project_id, system_name, ui_name, ontology_scope, status)
		VALUES ($1, $2, 'fork_source_model', $3::jsonb, $4::jsonb, 'draft')
		ON CONFLICT (id) DO NOTHING`, srcModelID, srcProjectID, string(uiName), string(ontologyScope))

	// Two live overrides on the source model.
	mustFork(t, pool, ctx, `INSERT INTO weave_field_overrides
			(field_id, project_id, entity_type, entity_id, position)
		VALUES
			($1 || '.f1', $1, 'model', $2, 0),
			($1 || '.f2', $1, 'model', $2, 1)`, srcProjectID, srcModelID)

	// Exactly one archived override for the same model, at version — so
	// a fork that mistakenly read the release archive would produce 1
	// override, never 2.
	mustFork(t, pool, ctx, `INSERT INTO weave_field_overrides_archive
			(field_id, project_id, entity_type, entity_id, position, version_number)
		VALUES
			($1 || '.f1', $1, 'model', $2, 0, $3)`, srcProjectID, srcModelID, version)

	// The adoption row ForkFromSource requires before it will fork
	// anything: (dst project, model context, src project, src model).
	mustFork(t, pool, ctx, `INSERT INTO weave_adoptions
			(project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id)
		VALUES ($1, 'project', $1, 'model', $2, $3)`, dstProjectID, srcProjectID, srcModelID)

	t.Cleanup(func() {
		bg := context.Background()
		mustCleanup(pool, bg, `DELETE FROM weave_entity_forks WHERE project_id = $1`, dstProjectID)
		mustCleanup(pool, bg, `DELETE FROM weave_adoptions WHERE project_id = $1`, dstProjectID)
		mustCleanup(pool, bg, `DELETE FROM weave_override_refs WHERE override_id IN (
			SELECT id FROM weave_field_overrides WHERE project_id IN ($1, $2))`, srcProjectID, dstProjectID)
		mustCleanup(pool, bg, `DELETE FROM weave_field_overrides_archive WHERE project_id = $1`, srcProjectID)
		mustCleanup(pool, bg, `DELETE FROM weave_field_overrides WHERE project_id IN ($1, $2)`, srcProjectID, dstProjectID)
		mustCleanup(pool, bg, `DELETE FROM weave_models WHERE project_id IN ($1, $2)`, srcProjectID, dstProjectID)
		mustCleanup(pool, bg, `DELETE FROM weave_projects WHERE id IN ($1, $2)`, srcProjectID, dstProjectID)
		mustCleanup(pool, bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	return srcModelID
}

// cleanupForkedModel removes the model row ForkFromSource created in
// dstProjectID, ahead of seedForkFixture's own t.Cleanup (which deletes
// every weave_models/weave_field_overrides row scoped to dstProjectID
// regardless of id, so this is redundant-but-explicit belt-and-braces
// for the row this test itself is responsible for).
func cleanupForkedModel(pool *pgxpool.Pool, dstProjectID, forkedModelID string) {
	bg := context.Background()
	mustCleanup(pool, bg, `DELETE FROM weave_field_overrides WHERE project_id = $1 AND entity_id = $2`, dstProjectID, forkedModelID)
	mustCleanup(pool, bg, `DELETE FROM weave_models WHERE id = $1`, forkedModelID)
}

// liveOverrideCount returns the number of live (weave_field_overrides)
// rows for (entityType, entityID).
func liveOverrideCount(t *testing.T, pool *pgxpool.Pool, entityType, entityID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM weave_field_overrides WHERE entity_type = $1 AND entity_id = $2`,
		entityType, entityID).Scan(&n); err != nil {
		t.Fatalf("count live overrides for %s/%s: %v", entityType, entityID, err)
	}
	return n
}

func mustFork(t *testing.T, pool *pgxpool.Pool, ctx context.Context, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func mustCleanup(pool *pgxpool.Pool, ctx context.Context, sql string, args ...any) {
	_, _ = pool.Exec(ctx, sql, args...)
}
