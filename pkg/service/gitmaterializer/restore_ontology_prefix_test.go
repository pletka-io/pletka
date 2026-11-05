//go:build integration

package gitmaterializer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// TestHydrateProjectOntologies_ByPrefix pins that a linked_versions entry
// naming only prefix + version resolves to the imported version, and that an
// unknown prefix fails before any link is written, naming prefix and version.
func TestHydrateProjectOntologies_ByPrefix(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)
	const (
		ownerID    = "PREFIX_OWNER_ACTOR"
		projectID  = "PREFIX_PROJ"
		badProject = "PREFIX_PROJ_BAD"
		ontologyID = "prefix-ontology"
		versionID  = "prefix-ontology-v1"
	)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = ANY($1)`, []string{projectID, badProject})
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = ANY($1)`, []string{projectID, badProject})
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{projectID, badProject})
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, versionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{ID: ownerID, Type: "organization", DisplayName: "Prefix Org", Slug: "prefix-org", Visibility: "private"}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := queries.WeaveCreateOntology(ctx, sqlcgen.WeaveCreateOntologyParams{ID: ontologyID, Prefix: "rpx", Namespace: "https://example.org/rpx/", Name: "Prefix Ontology", Description: []byte(`{}`), OntologyType: "base"}); err != nil {
		t.Fatalf("create ontology: %v", err)
	}
	if _, err := queries.WeaveCreateOntologyVersion(ctx, sqlcgen.WeaveCreateOntologyVersionParams{
		ID: versionID, OntologyID: ontologyID, VersionString: "1.0.0", IsActive: true, CompatibleBaseVersions: []string{},
		RdfContent: stringPtr("@prefix rpx: <https://example.org/rpx/> .\n"), ParsedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		VersionInfo: []byte(`{}`), ImportedOntologies: []string{}, OntologyLabel: []byte(`{}`), OntologyComment: []byte(`{}`), OntologyMetadata: []byte(`{}`),
	}); err != nil {
		t.Fatalf("create ontology version: %v", err)
	}

	snapshotFor := func(id string, link ProjectManifestLinkedOntology) *ProjectSnapshot {
		return &ProjectSnapshot{
			Manifest: ProjectManifestFile{
				SchemaVersion: 1,
				Project: ProjectManifestProject{
					ID: id, Title: map[string]string{"en": id}, Visibility: "private",
					Owner: &manifestActor{ActorID: ownerID, Type: "organization", Slug: "prefix-org", DisplayName: "Prefix Org"},
				},
				Ontologies: &ProjectManifestOntologies{LinkedVersions: []ProjectManifestLinkedOntology{link}},
				Manifests:  &ProjectManifestManifests{},
			},
			Mod: &PletkaModFile{SchemaVersion: 1, Module: PletkaModModule{Path: "pletka.io/orgs/prefix-org/projects/" + id, ProjectID: id}},
		}
	}
	mat := NewMaterializer(pool, t.TempDir(), nil)

	good := snapshotFor(projectID, ProjectManifestLinkedOntology{Prefix: "rpx", Version: "1.0.0", IsPrimary: true})
	if err := mat.HydrateProjectShell(ctx, BuildRestorePlan(good)); err != nil {
		t.Fatalf("HydrateProjectShell by prefix: %v", err)
	}
	links, err := queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 1 || links[0].OntologyVersionID != versionID {
		t.Fatalf("want one link to %s, got %+v", versionID, links)
	}

	bad := snapshotFor(badProject, ProjectManifestLinkedOntology{Prefix: "nope", Version: "9.9"})
	err = mat.HydrateProjectShell(ctx, BuildRestorePlan(bad))
	if err == nil || !strings.Contains(err.Error(), "nope@9.9") {
		t.Fatalf("want error naming nope@9.9, got %v", err)
	}
	badLinks, _ := queries.WeaveListProjectOntologyVersions(ctx, badProject)
	if len(badLinks) != 0 {
		t.Fatalf("unknown prefix must write no links, got %+v", badLinks)
	}
}
