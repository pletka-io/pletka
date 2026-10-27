//go:build integration

package ontologyrelease_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/namespace"
)

func TestProjectCaptureRejectsSourceWithoutSemanticEvidence(t *testing.T) {
	pool := testdb.EmptyPool(t)
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context()) // isolated test transaction
	for _, sql := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('semantic-owner','organization','semantic-owner','Semantic owner')`,
		`INSERT INTO weave_projects(id,owner_id) VALUES ('semantic-source','semantic-owner'),('semantic-consumer','semantic-owner')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('semantic-source','1.0.0','semantic-owner'),('semantic-consumer','1.0.0','semantic-owner')`,
		`UPDATE weave_releases SET ontology_dependencies_version=1 WHERE project_id='semantic-source'`,
		`INSERT INTO weave_release_project_dependencies VALUES ('semantic-consumer','1.0.0','semantic-source','1.0.0')`,
	} {
		if _, err := tx.Exec(t.Context(), sql); err != nil {
			t.Fatal(err)
		}
	}
	err = ontologyrelease.CaptureProject(t.Context(), sqlcgen.New(tx), "semantic-consumer", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "no verified ontology inventory") {
		t.Fatalf("source absence accepted: %v", err)
	}
	if _, err := tx.Exec(t.Context(), `UPDATE weave_releases SET ontology_dependencies_version=1 WHERE project_id='semantic-consumer'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO weave_release_ontology_references VALUES ('semantic-consumer','1.0.0','[]')`); err == nil || !strings.Contains(err.Error(), "source project semantic inventory is unverified") {
		t.Fatalf("raw source absence accepted: %v", err)
	}
}

func TestSemanticStorageMatcherAndMigration(t *testing.T) {
	pool := testdb.EmptyPool(t)
	aliases := []namespace.Binding{{Prefix: "alias", Namespace: "https://old.example/", Weight: 20}, {Prefix: "ex", Namespace: "https://phantom.example/", Weight: 80}}
	bindings, err := json.Marshal(aliases)
	if err != nil {
		t.Fatal(err)
	}
	term := `{"uri":"https://old.example/A","prefix":"ex","local_name":"A"}`
	for _, tc := range []struct {
		reference string
		want      bool
	}{
		{"https://old.example/A", true}, {"ex:A", true}, {"alias:A", true},
		{"https://phantom.example/A", false}, {"unknown:A", false}, {"ex:B", false}, {"", false}, {"ex:", false},
	} {
		var stored bool
		if err := pool.QueryRow(t.Context(), `SELECT weave_semantic_term_matches($1,$2::jsonb,$3::jsonb)`, tc.reference, term, bindings).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		goMatch := namespace.MatchesTerm(tc.reference, "https://old.example/A", "ex", "A", aliases)
		if stored != tc.want || goMatch != tc.want {
			t.Errorf("%q: storage=%v Go=%v want=%v", tc.reference, stored, goMatch, tc.want)
		}
	}
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var column bool
	if err := pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='weave_fields_archive' AND column_name='subfield_paths')`).Scan(&column); err != nil || !column {
		t.Fatalf("subfield migration round trip: %v %v", column, err)
	}
}
