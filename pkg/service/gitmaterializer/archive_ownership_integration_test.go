//go:build integration

package gitmaterializer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/releasearchive"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/namespacebinding"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func ownershipFixture(t *testing.T) (*matScopedFixture, context.Context, string) {
	t.Helper()
	f := setupMatScopedFixture(t)
	const source = "MATSCOPED_SOURCE_OWNERSHIP"
	f.category("MATSCOPED_OWNCAT")
	f.sourceProject(source)
	f.sourceModel(source, "MATSCOPED_SOURCE_OWNMODEL")
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM weave_project_inheritance WHERE project_id = ANY($1)`,
			`DELETE FROM weave_project_inheritance_archive WHERE project_id = ANY($1)`,
			`DELETE FROM weave_project_ontology_versions_archive WHERE project_id = ANY($1)`,
			`DELETE FROM weave_projects_archive WHERE id = ANY($1)`,
			`DELETE FROM weave_categories_archive WHERE project_id = ANY($1)`,
			`DELETE FROM weave_models_archive WHERE project_id = ANY($1)`,
			`DELETE FROM weave_change_set WHERE project_id = ANY($1)`,
			`DELETE FROM weave_releases WHERE project_id = ANY($1)`,
		} {
			if _, err := f.pool.Exec(context.Background(), stmt, []string{f.projectID, source, "MATSCOPED_SOURCE_OWNGRAND"}); err != nil {
				t.Error(err)
			}
		}
	})
	ctx := auth.WithPrincipal(auth.WithSnapshot(f.ctx, &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: matScopedOwnerActorID})
	return f, ctx, source
}

func cutOwnedRelease(ctx context.Context, t *testing.T, f *matScopedFixture, projectID, version string) {
	t.Helper()
	if _, err := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil).Create(ctx, projectID, release.CreateInput{Version: version}); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseNamespaceOwnershipAcrossSameVersion(t *testing.T) {
	f, ctx, other := ownershipFixture(t)
	t.Cleanup(func() {
		if _, err := f.pool.Exec(f.ctx, `DELETE FROM weave_namespace_bindings WHERE id IN ('OWN_NS_OLD', 'OWN_NS_NEW')`); err != nil {
			t.Error(err)
		}
	})
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO weave_namespace_bindings (id, prefix, namespace, weight, source, created_at, updated_at)
		VALUES ('OWN_NS_OLD', 'owned_old', 'https://before.example/', 80, 'user', now(), now())`); err != nil {
		t.Fatal(err)
	}
	cutOwnedRelease(ctx, t, f, f.projectID, "71.2.3")
	beforeDir := t.TempDir()
	if err := f.mat.writeProjectTreeVersion(ctx, beforeDir, f.projectID, "71.2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE weave_namespace_bindings SET namespace='https://after.example/' WHERE id='OWN_NS_OLD'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO weave_namespace_bindings (id, project_id, prefix, namespace, weight, source, created_at, updated_at)
		VALUES ('OWN_NS_NEW', '', 'owned_new', 'https://new.example/', 80, 'user', now(), now())`); err != nil {
		t.Fatal(err)
	}
	cutOwnedRelease(ctx, t, f, other, "71.2.3")
	svc := namespacebinding.NewService(namespacebinding.NewPostgresStore(f.pool), nil, nil)
	for _, tc := range []struct {
		project, oldURI string
		wantNew         bool
	}{
		{f.projectID, "https://before.example/", false}, {other, "https://after.example/", true},
	} {
		bindings, err := svc.List(auth.WithProjectVersion(ctx, "71.2.3"), tc.project)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]string{}
		for _, b := range bindings {
			found[b.Prefix] = b.Namespace
		}
		if found["owned_old"] != tc.oldURI || (found["owned_new"] != "") != tc.wantNew {
			t.Fatalf("%s namespaces: %v", tc.project, found)
		}
	}
	afterDir := t.TempDir()
	if err := f.mat.writeProjectTreeVersion(ctx, afterDir, f.projectID, "71.2.3"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(treeContents(t, beforeDir), treeContents(t, afterDir)); diff != "" {
		t.Fatal(diff)
	}
	otherDir := t.TempDir()
	if err := f.mat.writeProjectTreeVersion(ctx, otherDir, other, "71.2.3"); err != nil {
		t.Fatal(err)
	}
	manifest := treeContents(t, otherDir)["project.yaml"]
	if !strings.Contains(manifest, "https://after.example/") || !strings.Contains(manifest, "https://new.example/") {
		t.Fatalf("second release manifest: %s", manifest)
	}
}

func TestChildCutDoesNotPrepopulateParentRelease(t *testing.T) {
	f, ctx, parent := ownershipFixture(t)
	const grand = "MATSCOPED_SOURCE_OWNGRAND"
	f.sourceProject(grand)
	f.sourceModel(grand, "MATSCOPED_SOURCE_OWNGRANDMODEL")
	cutOwnedRelease(ctx, t, f, grand, "80.0.0")
	if _, err := f.pool.Exec(ctx, `INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode, source_version)
		VALUES ($1, $2, true, 'release', '80.0.0')`, parent, grand); err != nil {
		t.Fatal(err)
	}
	var ontologyVersion string
	if err := f.pool.QueryRow(ctx, `SELECT id FROM weave_ontology_versions ORDER BY id LIMIT 1`).Scan(&ontologyVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO weave_project_ontology_versions (project_id, ontology_version_id, added_at, usage_notes) VALUES ($1, $2, now(), 'Before')`, parent, ontologyVersion); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id=$1`, parent); err != nil {
			t.Error(err)
		}
	})
	cutOwnedRelease(ctx, t, f, parent, "81.0.0")
	if _, err := f.pool.Exec(ctx, `UPDATE weave_project_ontology_versions SET usage_notes='After' WHERE project_id=$1`, parent); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE weave_project_inheritance SET canonical_order=4 WHERE project_id=$1`, parent); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE weave_projects SET ui_name='{"en":"After"}', namespace='https://parent-after.example/' WHERE id=$1`, parent); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode, source_version)
		VALUES ($1, $2, true, 'release', '81.0.0')`, f.projectID, parent); err != nil {
		t.Fatal(err)
	}
	cutOwnedRelease(ctx, t, f, f.projectID, "82.0.0")
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT
		(SELECT count(*) FROM weave_projects_archive WHERE id=$1 AND version_number='82.0.0') +
		(SELECT count(*) FROM weave_project_inheritance_archive WHERE project_id=$1 AND version_number='82.0.0') +
		(SELECT count(*) FROM weave_project_ontology_versions_archive WHERE project_id=$1 AND version_number='82.0.0')`, parent).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("child cut prepopulated parent's archive key")
	}
	cutOwnedRelease(ctx, t, f, parent, "82.0.0")
	archived, err := f.mat.loadArchivedProject(ctx, parent, "82.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if archived.UIName.Get("en", "") != "After" {
		t.Fatalf("parent's new release has old metadata: %+v", archived)
	}
	var notes string
	var order int
	if err := f.pool.QueryRow(ctx, `SELECT usage_notes FROM weave_project_ontology_versions_archive WHERE project_id=$1 AND version_number='82.0.0'`, parent).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT canonical_order FROM weave_project_inheritance_archive WHERE project_id=$1 AND version_number='82.0.0'`, parent).Scan(&order); err != nil {
		t.Fatal(err)
	}
	if notes != "After" || order != 4 {
		t.Fatalf("parent release retained foreign copy: notes=%q order=%d", notes, order)
	}
	parents, err := releasearchive.Parents(ctx, f.queries, f.projectID, "82.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 1 || *parents[0].SourceVersion != "81.0.0" {
		t.Fatalf("parent pins = %+v", parents)
	}
	reqs, err := f.mat.loadPletkaProjectRequirementsAtVersion(ctx, f.projectID, "82.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 || reqs[0].Version != "81.0.0" {
		t.Fatalf("requirements: %+v", reqs)
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM weave_projects_archive WHERE id=$1 AND version_number='81.0.0'`, parent); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mat.loadPletkaProjectRequirementsAtVersion(ctx, f.projectID, "82.0.0"); err == nil {
		t.Fatal("missing pinned parent archive fell back to live metadata")
	}
}

func TestLegacyReleaseOwnershipFailsClosed(t *testing.T) {
	f, ctx, _ := ownershipFixture(t)
	if _, err := f.pool.Exec(ctx, `INSERT INTO weave_releases (project_id, version, created_by_id) VALUES ($1, '90.0.0', $2)`, f.projectID, matScopedOwnerActorID); err != nil {
		t.Fatal(err)
	}
	svc := namespacebinding.NewService(namespacebinding.NewPostgresStore(f.pool), nil, nil)
	_, err := svc.List(auth.WithProjectVersion(ctx, "90.0.0"), f.projectID)
	var unavailable *releasearchive.UnavailableError
	if !errors.As(err, &unavailable) || apierror.FromError(err).Code != apierror.CodeNotInRelease {
		t.Fatalf("legacy namespace read: %v", err)
	}
	if err := f.mat.writeProjectTreeVersion(ctx, t.TempDir(), f.projectID, "90.0.0"); !errors.As(err, &unavailable) {
		t.Fatalf("legacy materialization: %v", err)
	}
	if _, err := f.mat.loadProjectManifestNamespacesAtVersion(ctx, domain.Project{Entity: domain.Entity{ID: f.projectID}}, "90.0.0"); !errors.As(err, &unavailable) {
		t.Fatalf("legacy manifest namespaces: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO weave_projects_archive (id, owner_id, version_number) VALUES ($1, $2, '91.0.0')`, f.projectID, matScopedOwnerActorID); err != nil {
		t.Fatal(err)
	}
	report, err := release.AuditArchiveOwnership(ctx, f.pool)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, row := range report {
		if row.ProjectID == f.projectID {
			found[row.Issue+":"+row.Version] = true
		}
	}
	if !found["unverified_release:90.0.0"] || !found["orphan_project_archive:91.0.0"] {
		t.Fatalf("audit missed legacy/orphan rows: %+v", report)
	}
	_, err = release.NewService(release.NewPostgresStore(f.pool), f.pool, nil).Create(ctx, f.projectID, release.CreateInput{Version: "91.0.0"})
	if err == nil {
		t.Fatal("cut silently accepted an archive key prepopulated by history")
	}
	if _, err := f.mat.loadArchivedProject(ctx, f.projectID, "91.0.0"); !errors.As(err, &unavailable) {
		t.Fatalf("failed cut acquired verified ownership: %v", err)
	}
}
