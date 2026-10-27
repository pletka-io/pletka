//go:build integration

package gitmaterializer

// The whole project tree is written twice in this package -- once for the
// draft and once for a release -- by ten paired functions that share no code
// (writeProjectTree/writeProjectTreeVersion and their nine step pairs). This
// test is the first thing to compare their output.
//
// A release cut immediately after a draft materialization records exactly the
// draft's content, so the two trees should describe the same project. Anything
// they disagree about is either a transformation the cut performs on purpose
// or a drift between two implementations of the same thing.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestDraftTreeVersusVersionTreeAfterARealCut(t *testing.T) {
	f := setupMatScopedFixture(t)

	f.category("MATSCOPED_DVCAT")
	f.field("MATSCOPED_DVFIELD")
	f.model("MATSCOPED_DVMODEL")
	f.collection("MATSCOPED_DVCOLL")
	f.placeFieldOnModel("MATSCOPED_DVFIELD", "MATSCOPED_DVMODEL")
	f.placeFieldOnCollection("MATSCOPED_DVFIELD", "MATSCOPED_DVCOLL")

	// A published source is required for a release dependency. The separate
	// dependency tests cover resolving an initially unpinned receipt.
	f.sourceProject("MATSCOPED_SOURCE_DV")
	f.sourceModel("MATSCOPED_SOURCE_DV", "MATSCOPED_SOURCE_DV_MODEL")
	f.adoptSource("model", "MATSCOPED_SOURCE_DV", "MATSCOPED_SOURCE_DV_MODEL")

	// This structural parity fixture has no ontology selection. Semantic scope,
	// alias and subfield fidelity are exercised with published ontologies in
	// TestProjectSemanticReferencesCaptureAndIsolation.
	for _, table := range []string{"weave_fields", "weave_models", "weave_collections"} {
		if _, err := f.pool.Exec(f.ctx, fmt.Sprintf("UPDATE %s SET ontology_scope=NULL WHERE project_id IN ($1,$2)", table), f.projectID, "MATSCOPED_SOURCE_DV"); err != nil {
			t.Fatal(err)
		}
	}

	const version = "1.0.0"
	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_release_project_dependencies WHERE project_id = $1`,
			`DELETE FROM weave_adoptions_archive WHERE project_id = $1`,
			`DELETE FROM weave_override_refs_archive WHERE project_id = $1`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id = $1`,
			`DELETE FROM weave_collection_placements_archive WHERE project_id = $1`,
			`DELETE FROM weave_collections_archive WHERE project_id = $1`,
			`DELETE FROM weave_models_archive WHERE project_id = $1`,
			`DELETE FROM weave_fields_archive WHERE project_id = $1`,
			`DELETE FROM weave_categories_archive WHERE project_id = $1`,
			`DELETE FROM weave_project_inheritance_archive WHERE project_id = $1`,
			`DELETE FROM weave_project_ontology_versions_archive WHERE project_id = $1`,
			`DELETE FROM weave_namespace_bindings_archive WHERE project_id = $1`,
			`DELETE FROM weave_projects_archive WHERE id = $1`,
			`DELETE FROM weave_change_set WHERE project_id = $1`,
			`DELETE FROM weave_releases WHERE project_id = $1`,
		} {
			if _, err := f.pool.Exec(bg, strings.ReplaceAll(stmt, " = $1", " = ANY($1)"), []string{f.projectID, "MATSCOPED_SOURCE_DV"}); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})

	authCtx := weaveauth.WithPrincipal(
		weaveauth.WithSnapshot(f.ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true}),
		&weaveauth.Principal{ActorID: matScopedOwnerActorID},
	)
	rel := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil)
	if _, err := rel.Create(authCtx, "MATSCOPED_SOURCE_DV", release.CreateInput{Version: version}); err != nil {
		t.Fatalf("cut source release: %v", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE weave_adoptions SET source_version=$1 WHERE project_id=$2 AND source_project_id='MATSCOPED_SOURCE_DV'`, version, f.projectID); err != nil {
		t.Fatal(err)
	}

	draftDir := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, draftDir, f.projectID); err != nil {
		t.Fatalf("write draft tree: %v", err)
	}

	if _, err := rel.Create(authCtx, f.projectID, release.CreateInput{
		Version: version,
		Title:   "Draft versus version tree",
	}); err != nil {
		t.Fatalf("cut release %s: %v", version, err)
	}

	versionDir := t.TempDir()
	if err := f.mat.writeProjectTreeVersion(f.ctx, versionDir, f.projectID, version); err != nil {
		t.Fatalf("write version tree: %v", err)
	}

	draftFiles := treeContents(t, draftDir)
	versionFiles := treeContents(t, versionDir)

	t.Logf("draft tree: %d files, version tree: %d files", len(draftFiles), len(versionFiles))

	for path := range draftFiles {
		if _, ok := versionFiles[path]; !ok {
			t.Errorf("PATH ONLY IN DRAFT TREE: %s", path)
		}
	}
	for path := range versionFiles {
		if _, ok := draftFiles[path]; !ok {
			t.Errorf("PATH ONLY IN VERSION TREE: %s", path)
		}
	}

	// project.yaml's namespace map is not transformed by the cut, so the two
	// trees must agree on it exactly. Every generator needs these prefixes to
	// emit Turtle or JSON-LD, and restore reads them back, so a release tree
	// that loses them is not a usable release.
	//
	// The cut DOES archive global bindings -- its insert reads
	// "WHERE project_id = $1 OR project_id IS NULL OR project_id = ''".
	// Whether the version path reads them back is what this pins.
	t.Run("project.yaml namespaces survive the cut", func(t *testing.T) {
		draftNS := namespaceBlockOf(t, draftFiles["project.yaml"])
		versionNS := namespaceBlockOf(t, versionFiles["project.yaml"])
		if draftNS != versionNS {
			t.Errorf("the release tree's namespace map differs from the draft's.\n--- draft ---\n%s\n--- version ---\n%s",
				draftNS, versionNS)
		}
	})

	// The whole contract in one assertion: undo the cut's status rewrite and
	// a release tree must be byte-identical to the draft it was cut from.
	// The rewrite maps draft -> published and published -> published, so
	// applying it to the DRAFT side is the faithful direction.
	//
	// Adopted entities are excluded and checked separately below: they are
	// written from the SOURCE project, so this project's status rewrite never
	// applies to them.
	t.Run("the trees differ only by the status rewrite", func(t *testing.T) {
		for path, draftBody := range draftFiles {
			if strings.Contains(draftBody, "_adopted_from:") {
				continue
			}
			want := strings.ReplaceAll(draftBody, "status: draft", "status: published")
			if got := versionFiles[path]; got != want {
				t.Errorf("%s differs beyond the status rewrite:\n--- draft (status normalized) ---\n%s\n--- version ---\n%s",
					path, want, got)
			}
		}
	})

	// Regression for bug-022: source edits, newer releases and live deletion
	// must leave the consuming release byte-identical.
	t.Run("a release tree does not move when the SOURCE project's draft moves", func(t *testing.T) {
		// Also cut from an unpinned receipt: its release must resolve once,
		// even when the source later publishes the same version string.
		if _, err := f.pool.Exec(f.ctx, `UPDATE weave_adoptions SET source_version='' WHERE project_id=$1`, f.projectID); err != nil {
			t.Fatal(err)
		}
		if _, err := rel.Create(authCtx, f.projectID, release.CreateInput{Version: "2.0.0"}); err != nil {
			t.Fatal(err)
		}
		unpinnedDir := t.TempDir()
		if err := f.mat.writeProjectTreeVersion(f.ctx, unpinnedDir, f.projectID, "2.0.0"); err != nil {
			t.Fatal(err)
		}

		if _, err := f.pool.Exec(f.ctx,
			`UPDATE weave_models SET ui_name = '{"en":"Source edited after the cut"}'::jsonb WHERE id = $1`,
			"MATSCOPED_SOURCE_DV_MODEL"); err != nil {
			t.Fatalf("edit the source project's draft: %v", err)
		}

		if _, err := rel.Create(authCtx, "MATSCOPED_SOURCE_DV", release.CreateInput{Version: "2.0.0"}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(f.ctx, `DELETE FROM weave_models WHERE id='MATSCOPED_SOURCE_DV_MODEL'`); err != nil {
			t.Fatal(err)
		}

		for _, selection := range []struct{ version, before string }{{version, versionDir}, {"2.0.0", unpinnedDir}} {
			again := t.TempDir()
			if err := f.mat.writeProjectTreeVersion(f.ctx, again, f.projectID, selection.version); err != nil {
				t.Fatalf("re-write version tree: %v", err)
			}
			if diff := diffTrees(t, selection.before, again); diff != "" {
				t.Errorf("release %s changed after source edit, release and deletion:\n%s", selection.version, diff)
			}
		}
	})

	if diff := diffTrees(t, draftDir, versionDir); diff != "" {
		t.Logf("RESIDUAL CONTENT DIFF (-draft +version):\n%s", diff)
	}
}

// namespaceBlockOf extracts the namespaces section of a project.yaml so the
// comparison names the field that differs instead of printing the whole file.
func namespaceBlockOf(t *testing.T, manifest string) string {
	t.Helper()
	lines := strings.Split(manifest, "\n")
	var out []string
	inBlock := false
	for _, line := range lines {
		if strings.HasPrefix(line, "namespaces:") {
			inBlock = true
			continue
		}
		if inBlock {
			// The block ends at the next top-level key.
			if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				break
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
