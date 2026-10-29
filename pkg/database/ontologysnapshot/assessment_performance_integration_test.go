//go:build integration

package ontologysnapshot_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
)

func TestLegacyAssessmentPerformanceParity(t *testing.T) {
	p := fixture(t)
	db := stdlib.OpenDB(*p.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	// Retain the actual pre-038 functions as the oracle, including the old
	// context helper's full inventory and per-location assessment calls.
	for version := latest; version >= 38; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, signature := range []string{"weave_legacy_ontology_assessment(jsonb,jsonb,jsonb)", "weave_legacy_context_v1(text,text)"} {
		var definition string
		if err := p.QueryRow(t.Context(), `SELECT pg_get_functiondef($1::regprocedure)`, signature).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		definition = strings.ReplaceAll(definition, "weave_legacy_ontology_assessment", "assessment_before_038")
		definition = strings.ReplaceAll(definition, "weave_legacy_context_v1", "context_before_038")
		exec(t, p, definition)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	// None of these source/descriptor bytes participate in term matching.
	// They must survive capture exactly while avoiding reparsing per term/ref.
	rdf := "<rdf>" + strings.Repeat("<irrelevant>payload</irrelevant>", 8192) + "</rdf>"
	if _, err := p.Exec(t.Context(), `UPDATE weave_ontology_versions SET rdf_content=$1 WHERE id IN ('crm-6.2','crm-7.1.3')`, rdf); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(t.Context(), `UPDATE weave_ontology_classes SET comment=jsonb_build_object('en',$1::text) WHERE ontology_version_id IN ('crm-6.2','crm-7.1.3')`, strings.Repeat("ignored description ", 512)); err != nil {
		t.Fatal(err)
	}
	exec(t, p, `INSERT INTO weave_ontology_properties(id,ontology_version_id,prefix,local_name,uri) VALUES ('property-6.2','crm-6.2','crm','P1','https://example.org/crm/P1'),('property-7.1.3','crm-7.1.3','crm','P1','https://example.org/crm/P1')`)
	exec(t, p, `UPDATE weave_ontology_versions SET property_count=1 WHERE id IN ('crm-6.2','crm-7.1.3')`)
	exec(t, p, `INSERT INTO weave_namespace_bindings(id,prefix,namespace,weight,source,created_at,updated_at) VALUES ('assessment-body-alias','body_alias','https://example.org/crm/',40,'test',now(),now())`)
	// Duplicate normalized references at distinct original locations must all
	// remain in the output, with the original lexical location ordering.
	exec(t, p, `INSERT INTO weave_fields_archive(id,project_id,version_number,path_elements)
		SELECT 'duplicate-ref-'||n,'legacy-project','1','[{"type":"class","uri":"body_alias:E1"},{"type":"property","prefix":"crm","local_name":"P1"},{"type":"class","uri":"https://missing.example/term"}]'::jsonb FROM generate_series(1,12) n`)
	capture(t, p, "source")
	captured := capture(t, p, "legacy-project")

	var inventory, aliases json.RawMessage
	for _, project := range []string{"legacy-project", "source", "empty"} {
		var before, after string
		err := p.QueryRow(t.Context(), `SELECT context_before_038($1,'1')::text, weave_legacy_context_v1($1,'1')::text`, project).Scan(&before, &after)
		if err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatalf("context output changed for %s (old %d bytes, new %d bytes)", project, len(before), len(after))
		}
		if project == "legacy-project" {
			var context struct {
				Inventory   json.RawMessage               `json:"inventory"`
				Bindings    json.RawMessage               `json:"bindings"`
				Assessments []ontologysnapshot.Assessment `json:"assessments"`
			}
			if err := json.Unmarshal([]byte(after), &context); err != nil {
				t.Fatal(err)
			}
			inventory, aliases = context.Inventory, context.Bindings
			if len(context.Assessments) != 43 {
				t.Fatalf("lost semantic locations: got %d, want 43", len(context.Assessments))
			}
		}
	}
	doc, err := captured.Decode()
	if err != nil {
		t.Fatalf("optimized capture disagrees with Go validation: %v", err)
	}
	if len(doc.Inventory) != 2 || len(doc.Sources) != 1 {
		t.Fatal("capture lost inventory or source context")
	}
	for _, body := range doc.Inventory {
		var full struct {
			Version struct {
				RDF string `json:"rdf_content"`
			}
		}
		if err := json.Unmarshal([]byte(body.BodyJSON), &full); err != nil {
			t.Fatal(err)
		}
		if full.Version.RDF != rdf {
			t.Fatal("compact matching inventory leaked into stored full evidence")
		}
	}

	for _, tc := range []struct {
		name, status string
		ref          ontologysnapshot.TermReference
	}{
		{"ambiguous URI", "ambiguous", ontologysnapshot.TermReference{Kind: "class", URI: "https://example.org/crm/E1"}},
		{"explicit version", "resolved", ontologysnapshot.TermReference{Kind: "class", URI: "https://example.org/crm/E1", VersionID: "crm-6.2"}},
		{"body alias", "ambiguous", ontologysnapshot.TermReference{Kind: "class", URI: "body_alias:E1"}},
		{"QName", "ambiguous", ontologysnapshot.TermReference{Kind: "class", Prefix: "crm", LocalName: "E1"}},
		{"property alias", "resolved", ontologysnapshot.TermReference{Kind: "property", Prefix: "body_alias", LocalName: "P1", VersionID: "crm-7.1.3"}},
		{"missing URI", "missing", ontologysnapshot.TermReference{Kind: "class", URI: "https://missing.example/term"}},
		{"wrong context alias", "missing", ontologysnapshot.TermReference{Kind: "class", URI: "source_alias:E1"}},
		{"URI and QName disagree", "missing", ontologysnapshot.TermReference{Kind: "class", URI: "https://example.org/crm/E1", Prefix: "crm", LocalName: "other"}},
		{"unknown version", "invalid", ontologysnapshot.TermReference{Kind: "class", URI: "https://example.org/crm/E1", VersionID: "crm-8"}},
		{"invalid kind", "invalid", ontologysnapshot.TermReference{Kind: "literal", URI: "https://example.org/crm/E1"}},
		{"half QName", "invalid", ontologysnapshot.TermReference{Kind: "class", Prefix: "crm"}},
		{"empty reference", "invalid", ontologysnapshot.TermReference{Kind: "class"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := json.Marshal(tc.ref)
			if err != nil {
				t.Fatal(err)
			}
			var before, after string
			err = p.QueryRow(t.Context(), `SELECT assessment_before_038($1::jsonb,$2::jsonb,$3::jsonb)::text, weave_legacy_ontology_assessment($1::jsonb,$2::jsonb,$3::jsonb)::text`, ref, inventory, aliases).Scan(&before, &after)
			if err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatalf("assessment changed: old %s, new %s", before, after)
			}
			var result ontologysnapshot.Assessment
			if err := json.Unmarshal([]byte(after), &result); err != nil {
				t.Fatal(err)
			}
			if string(result.Status) != tc.status {
				t.Fatalf("got status %s, want %s", result.Status, tc.status)
			}
		})
	}
}
