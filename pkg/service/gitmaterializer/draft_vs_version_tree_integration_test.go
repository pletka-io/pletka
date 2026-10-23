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

	// A cross-project adoption that records NO source version -- the case
	// loadAdoptedModelAtSource resolves by reading the source's LIVE table.
	f.sourceProject("MATSCOPED_SOURCE_DV")
	f.sourceModel("MATSCOPED_SOURCE_DV", "MATSCOPED_SOURCE_DV_MODEL")
	f.adoptSource("model", "MATSCOPED_SOURCE_DV", "MATSCOPED_SOURCE_DV_MODEL")

	const version = "1.0.0"
	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
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
			if _, err := f.pool.Exec(bg, stmt, f.projectID); err != nil {
				t.Errorf("cleanup %q: %v", stmt, err)
			}
		}
	})

	draftDir := t.TempDir()
	if err := f.mat.writeProjectTree(f.ctx, draftDir, f.projectID); err != nil {
		t.Fatalf("write draft tree: %v", err)
	}

	authCtx := weaveauth.WithPrincipal(
		weaveauth.WithSnapshot(f.ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true}),
		&weaveauth.Principal{ActorID: matScopedOwnerActorID},
	)
	rel := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil)
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

	// bug-022: loadAdoptedModelAtSource falls back to the source project's
	// LIVE table when the adoption records no source version. A release is
	// supposed to be immutable, so re-materializing it after the source's
	// draft moves must produce the same bytes.
	t.Run("a release tree does not move when the SOURCE project's draft moves", func(t *testing.T) {
		t.Skip("KNOWN DEFECT, reproduces: loadAdoptedModelAtSource (versioned_loaders.go:335) " +
			"and its field/collection/base-override siblings (:300, :362, :396) read the source's LIVE " +
			"table when the adoption records no source version, and the cut copies that empty " +
			"source_version through verbatim (release/service.go:456). Unskip when the cut pins " +
			"source_version and the empty-version branch becomes an error instead of a live read; " +
			"this subtest is that work's acceptance test.")

		before := treeContents(t, versionDir)

		if _, err := f.pool.Exec(f.ctx,
			`UPDATE weave_models SET ui_name = '{"en":"Source edited after the cut"}'::jsonb WHERE id = $1`,
			"MATSCOPED_SOURCE_DV_MODEL"); err != nil {
			t.Fatalf("edit the source project's draft: %v", err)
		}

		again := t.TempDir()
		if err := f.mat.writeProjectTreeVersion(f.ctx, again, f.projectID, version); err != nil {
			t.Fatalf("re-write version tree: %v", err)
		}
		after := treeContents(t, again)

		for path, bodyBefore := range before {
			if bodyAfter := after[path]; bodyAfter != bodyBefore {
				t.Errorf("release %s changed in %s after an edit to the SOURCE project's DRAFT.\n"+
					"A released tree must be immutable; this one re-reads the source's live rows.\n"+
					"--- before ---\n%s\n--- after ---\n%s", version, path, bodyBefore, bodyAfter)
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
