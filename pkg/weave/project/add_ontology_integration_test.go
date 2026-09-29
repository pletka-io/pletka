//go:build integration

package project_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// TestAddOntologyToChildren covers the cases that decide whether an operator
// can trust this against 38 production projects — mostly the ones where a
// child must NOT be touched: a draft follower that already resolves the
// ontology, a child that already has it, a child outside --pinned-to, an
// unresolvable ontology, and a prefix that is ambiguous.
func TestAddOntologyToChildren(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const parent = "ZAO"
	const (
		childPinned  = "ZAOPIN"   // release 0.1.0 -> gains it
		childDraft   = "ZAODRAFT" // draft         -> already resolves it
		childHas     = "ZAOHAS"   // release 0.1.0 -> already linked
		childOther   = "ZAOOTHER" // release 0.9.9 -> outside --pinned-to
		ontA         = "ZAO_ONT_V1"
		ontB         = "ZAO_ONT_V2"
		ontologyBase = "ZAO_ONT"
	)

	mustRepinExec(t, pool, `INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
		VALUES ('ZAO_OWNER','organization','AddOnt Org','zao-owner','private',NOW(),NOW())
		ON CONFLICT (id) DO NOTHING`)
	seedAOProject(t, pool, parent)
	for _, id := range []string{childPinned, childDraft, childHas, childOther} {
		seedAOProject(t, pool, id)
	}

	// Two versions of one ontology, so the ambiguous-prefix case is real.
	mustRepinExec(t, pool, `INSERT INTO weave_ontologies (id, prefix, name, namespace, created_at, updated_at)
		VALUES ($1, 'zao-ont', 'AddOnt Test Ontology', 'https://example.org/zao-ont/', NOW(), NOW()) ON CONFLICT (id) DO NOTHING`, ontologyBase)
	mustRepinExec(t, pool, `INSERT INTO weave_ontology_versions (id, ontology_id, version_string, created_at, updated_at)
		VALUES ($1, $3, '1.0', NOW(), NOW()), ($2, $3, '2.0', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, ontA, ontB, ontologyBase)

	link := func(child, mode, version string) {
		t.Helper()
		var v any
		if version != "" {
			v = version
		}
		mustRepinExec(t, pool, `INSERT INTO weave_project_inheritance
			(project_id, parent_project_id, is_primary, canonical_order, source_mode, source_version)
			VALUES ($1, $2, true, 0, $3, $4)`, child, parent, mode, v)
	}
	link(childPinned, "release", "0.1.0")
	link(childDraft, "draft", "")
	link(childHas, "release", "0.1.0")
	link(childOther, "release", "0.9.9")

	mustRepinExec(t, pool, `INSERT INTO weave_project_ontology_versions
		(project_id, ontology_version_id, added_at, is_primary) VALUES ($1, $2, NOW(), false)`, childHas, ontA)

	t.Cleanup(func() {
		bg := context.Background()
		for _, id := range []string{parent, childPinned, childDraft, childHas, childOther} {
			_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id=$1`, id)
		}
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE parent_project_id=$1`, parent)
		for _, id := range []string{parent, childPinned, childDraft, childHas, childOther} {
			_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, id)
		}
		_, _ = pool.Exec(bg, `DELETE FROM weave_ontology_versions WHERE ontology_id=$1`, ontologyBase)
		_, _ = pool.Exec(bg, `DELETE FROM weave_ontologies WHERE id=$1`, ontologyBase)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id='ZAO_OWNER'`)
	})

	t.Run("an unknown ontology is an error", func(t *testing.T) {
		if _, err := project.AddOntologyToChildren(ctx, pool, parent, "nope-not-real", project.AddOntologyOptions{}); !errors.Is(err, project.ErrNoSuchOntology) {
			t.Fatalf("err = %v, want ErrNoSuchOntology", err)
		}
	})

	t.Run("an ambiguous prefix is an error naming the versions", func(t *testing.T) {
		_, err := project.AddOntologyToChildren(ctx, pool, parent, "zao-ont", project.AddOntologyOptions{})
		if !errors.Is(err, project.ErrNoSuchOntology) {
			t.Fatalf("err = %v, want ErrNoSuchOntology — linking the wrong version must not be guessed", err)
		}
		if !strings.Contains(err.Error(), ontA) || !strings.Contains(err.Error(), ontB) {
			t.Errorf("error does not name both candidates: %v", err)
		}
	})

	t.Run("a mistyped parent is an error", func(t *testing.T) {
		if _, err := project.AddOntologyToChildren(ctx, pool, "ZAONOPE", ontA, project.AddOntologyOptions{}); !errors.Is(err, project.ErrNoSuchParent) {
			t.Fatalf("err = %v, want ErrNoSuchParent", err)
		}
	})

	t.Run("a dry run writes nothing", func(t *testing.T) {
		report, err := project.AddOntologyToChildren(ctx, pool, parent, ontA, project.AddOntologyOptions{PinnedTo: "0.1.0"})
		if err != nil {
			t.Fatalf("dry run: %v", err)
		}
		if report.Applied {
			t.Errorf("dry run reported Applied")
		}
		if got := report.Changed(); got != 1 {
			t.Errorf("would change %d children, want 1 (only the pinned one lacking it)", got)
		}
		if hasOntology(t, pool, childPinned, ontA) {
			t.Errorf("%s gained the ontology during a DRY RUN", childPinned)
		}
	})

	t.Run("apply gives it to the pinned child only", func(t *testing.T) {
		report, err := project.AddOntologyToChildren(ctx, pool, parent, ontA, project.AddOntologyOptions{PinnedTo: "0.1.0", Note: "test", Apply: true})
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !report.Applied {
			t.Fatalf("apply reported Applied=false")
		}
		if !hasOntology(t, pool, childPinned, ontA) {
			t.Errorf("%s did not gain the ontology", childPinned)
		}
		// A draft follower already resolves the parent's draft, so a copy of
		// its own buys nothing and is not written.
		if hasOntology(t, pool, childDraft, ontA) {
			t.Errorf("%s (draft follower) was given the ontology without --include-draft", childDraft)
		}
		if hasOntology(t, pool, childOther, ontA) {
			t.Errorf("%s was given the ontology despite --pinned-to 0.1.0", childOther)
		}
	})

	t.Run("re-running changes nothing", func(t *testing.T) {
		report, err := project.AddOntologyToChildren(ctx, pool, parent, ontA, project.AddOntologyOptions{PinnedTo: "0.1.0", Apply: true})
		if err != nil {
			t.Fatalf("second apply: %v", err)
		}
		if got := report.Changed(); got != 0 {
			t.Errorf("second run would change %d children, want 0 — every target already has it", got)
		}
		if n := ontologyLinkCount(t, pool, childPinned, ontA); n != 1 {
			t.Errorf("%s has %d links to the ontology, want 1", childPinned, n)
		}
	})

	t.Run("the report names every child and why it was skipped", func(t *testing.T) {
		report, err := project.AddOntologyToChildren(ctx, pool, parent, ontA, project.AddOntologyOptions{PinnedTo: "0.1.0"})
		if err != nil {
			t.Fatalf("report run: %v", err)
		}
		var sb strings.Builder
		if err := report.Render(&sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		out := sb.String()
		for _, id := range []string{childPinned, childDraft, childHas, childOther} {
			if !strings.Contains(out, id) {
				t.Errorf("report does not name %s:\n%s", id, out)
			}
		}
		if !strings.Contains(out, "already has it") {
			t.Errorf("report does not say a child already had it:\n%s", out)
		}
	})
}

func hasOntology(t *testing.T, pool *pgxpool.Pool, projectID, versionID string) bool {
	t.Helper()
	return ontologyLinkCount(t, pool, projectID, versionID) > 0
}

func ontologyLinkCount(t *testing.T, pool *pgxpool.Pool, projectID, versionID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM weave_project_ontology_versions
		 WHERE project_id=$1 AND ontology_version_id=$2`, projectID, versionID).Scan(&n); err != nil {
		t.Fatalf("count links for %s: %v", projectID, err)
	}
	return n
}

// seedAOProject creates a probe project owned by this test's own actor, so
// the fixture does not depend on a sibling test's owner row existing.
func seedAOProject(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	uiName, _ := json.Marshal(map[string]string{"en": id})
	mustRepinExec(t, pool, `INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
		VALUES ($1, $2::jsonb, $2::jsonb, 'draft', 'ZAO_OWNER', 'private', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, id, string(uiName))
}
