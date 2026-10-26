//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

func TestOntologyBodyMigrationPreservesUnverifiedOrphans(t *testing.T) {
	pool := testdb.Pool(t)
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
	})
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version >= 23; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	const orphan = "MIGRATION023_ORPHAN"
	if _, err := pool.Exec(t.Context(), `INSERT INTO weave_ontology_relations(source_id,source_kind,rel_type,target_qname) VALUES ($1,'class','subclass_of','missing:target')`, orphan); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Migrate(db); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(context.Background(), `DELETE FROM weave_ontology_relations WHERE source_id=$1`, orphan); err != nil {
			t.Error(err)
		}
	})
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var seals int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weave_ontology_body_seals`).Scan(&seals); err != nil {
		t.Fatal(err)
	}
	if seals != 0 {
		t.Fatal("migration certified historical bodies")
	}
	rows, err := sqlcgen.New(pool).WeaveAuditOrphanOntologyRelations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.SourceID == orphan {
			found = true
		}
	}
	if !found {
		t.Fatal("migration discarded orphan evidence or invented ownership")
	}
	var versionID string
	if err := pool.QueryRow(t.Context(), `SELECT id FROM weave_ontology_versions ORDER BY id LIMIT 1`).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO weave_ontology_classes(id,ontology_version_id,prefix,local_name,uri) VALUES ($1,$2,'migration023','Orphan','migration023:Orphan')`, orphan, versionID); err == nil {
		t.Fatal("term ID reuse reconnected unowned history")
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO weave_ontology_relations(source_id,source_kind,rel_type,target_qname) VALUES ($1,'class','subclass_of','new:target')`, orphan); err == nil {
		t.Fatal("new orphan bypassed NOT VALID constraint")
	}
	// No seals exist, so down/up must remain safe and retain the evidence.
	for version := latest; version >= 23; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
}
