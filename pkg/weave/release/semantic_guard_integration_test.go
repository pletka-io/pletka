//go:build integration

package release

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

// A verified cut validates its semantic references inside the database. That
// validation must grow with the inventory, not re-read every published term
// for every reference: forty fields against a 2,000-term ontology sat in one
// statement for minutes before the guard was bounded.
func TestCreateValidatesSemanticReferencesWithinBudget(t *testing.T) {
	pool := releaseTestPool(t)
	base, ownerID, projectID := seedZREL(t, pool)
	store := ontology.NewPostgresStore(pool)

	defaults, err := store.GetOntologyByPrefix(base, "defaults")
	if err != nil || defaults == nil {
		t.Fatalf("defaults: %v", err)
	}
	oldDefaults, err := store.GetActiveVersion(base, defaults.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if oldDefaults != nil {
			if err := store.SetActiveVersion(context.Background(), defaults.ID, oldDefaults.ID); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err := store.ImportVersion(base, ontology.ImportVersionInput{
		Version:   ontology.CreateVersionInput{ID: "guard-defaults-1", OntologyID: defaults.ID, VersionString: "guard-defaults-1", RDFContent: "<defaults/>"},
		SetActive: true,
		Classes:   []ontology.ImportClass{{ID: "guard-string", Prefix: "xsd", LocalName: "string", URI: "http://www.w3.org/2001/XMLSchema#string"}},
	}); err != nil {
		t.Fatal(err)
	}

	const ns = "https://guard.example/"
	const terms = 2000
	owner, versionID := "guard-owner", "guard-version-1"
	if _, err := store.CreateOntology(base, ontology.CreateOntologyInput{ID: owner, Prefix: "gq", Name: "Guard budget", Namespace: ns, OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	comment := domain.Translations{"en": strings.Repeat("Guard description. ", 16)}
	classes := []ontology.ImportClass{{ID: "gq-Thing", Prefix: "gq", LocalName: "Thing", URI: ns + "Thing"}}
	properties := make([]ontology.ImportProperty, 0, terms)
	for i := 1; i <= terms; i++ {
		classes = append(classes, ontology.ImportClass{ID: fmt.Sprintf("gq-T%d", i), Prefix: "gq", LocalName: fmt.Sprintf("T%d", i), URI: fmt.Sprintf("%sT%d", ns, i), Comment: comment})
		properties = append(properties, ontology.ImportProperty{ID: fmt.Sprintf("gq-p%d", i), Prefix: "gq", LocalName: fmt.Sprintf("p%d", i), URI: fmt.Sprintf("%sp%d", ns, i), Comment: comment, PropertyType: "ObjectProperty"})
	}
	if _, err := store.ImportVersion(base, ontology.ImportVersionInput{
		Version:           ontology.CreateVersionInput{ID: versionID, OntologyID: owner, VersionString: "1", RDFContent: "<terms/>"},
		SetActive:         true,
		Classes:           classes,
		Properties:        properties,
		NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "gq", Namespace: ns, Weight: 80}},
	}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, `INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at) VALUES ($1,$2,now())`, projectID, versionID)
	// seedZREL's purge deletes the project; the ontology link and this cut's
	// archive rows must go first or every later ZREL seed fails on insert.
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM weave_releases WHERE project_id='ZREL' AND version='9.0.2'`,
			`DELETE FROM weave_project_ontology_versions_archive WHERE project_id='ZREL'`,
			`DELETE FROM weave_project_ontology_versions WHERE project_id='ZREL'`,
		} {
			if _, err := pool.Exec(context.Background(), sql); err != nil {
				t.Error(err)
			}
		}
	})

	const fields = 40
	for i := 1; i <= fields; i++ {
		scope := fmt.Sprintf(`{"position":0,"type":"class","prefix":"gq","local_name":"Thing","uri":"%sThing","ontology_version_id":"%s"}`, ns, versionID)
		path := fmt.Sprintf(`[%s,{"position":1,"type":"property","prefix":"gq","local_name":"p%d","uri":"%sp%d","ontology_version_id":"%s"},{"position":2,"type":"class","prefix":"gq","local_name":"T%d","uri":"%sT%d","ontology_version_id":"%s"}]`,
			scope, i, ns, i, versionID, i, ns, i, versionID)
		mustExec(t, pool, `INSERT INTO weave_fields (id, semantic_id, system_name, project_id, ui_name, status, ontology_scope, path_elements, created_at, updated_at)
			VALUES ($1,$1,$2,$3,'{"en":"guard"}','draft',$4::jsonb,$5::jsonb,NOW(),NOW())`,
			fmt.Sprintf("ZRELG%d", i), fmt.Sprintf("guard_%d", i), projectID, scope, path)
	}

	ctx, cancel := context.WithTimeout(base, 15*time.Second)
	defer cancel()
	// On failure, name the statement that held the cut, not just the deadline.
	go func() {
		seen := map[string]bool{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			rows, err := pool.Query(base, `SELECT left(query,160), now()-query_start FROM pg_stat_activity WHERE state='active' AND query NOT ILIKE '%pg_stat_activity%'`)
			if err != nil {
				return
			}
			for rows.Next() {
				var q string
				var age time.Duration
				if rows.Scan(&q, &age) == nil && !seen[q] {
					seen[q] = true
					t.Logf("active: %s", q)
				}
			}
			rows.Close()
		}
	}()
	started := time.Now()
	if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, ownerID), projectID, CreateInput{Version: "9.0.2"}); err != nil {
		t.Fatalf("cut did not finish within budget (%s): %v", time.Since(started).Round(time.Millisecond), err)
	}
	t.Logf("cut with %d references took %s", fields*4, time.Since(started).Round(time.Millisecond))

	var resolutions int
	if err := pool.QueryRow(base, `SELECT jsonb_array_length(resolutions) FROM weave_release_ontology_references WHERE project_id=$1 AND version_number='9.0.2'`, projectID).Scan(&resolutions); err != nil {
		t.Fatal(err)
	}
	if resolutions != fields*4 {
		t.Fatalf("captured %d resolutions, want %d", resolutions, fields*4)
	}
}
