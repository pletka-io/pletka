//go:build integration

package database_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

func TestMetadataMigrationPreservesUnknownHistory(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx := t.Context()
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	for version := latest; version >= 31; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO weave_actors(id,display_name,slug) VALUES ('history-owner','Owner','history-owner')`)
	exec(`INSERT INTO weave_projects(id,owner_id,concept_namespace) VALUES ('history','history-owner','https://today.example/')`)
	exec(`INSERT INTO weave_models(id,project_id,model_type) VALUES ('history-model','history','auxiliary')`)
	exec(`INSERT INTO weave_releases(project_id,version,created_by_id,archive_ownership_version) VALUES ('history','1.0.0','history-owner',1)`)
	exec(`INSERT INTO weave_projects_archive(id,owner_id,version_number) VALUES ('history','history-owner','1.0.0')`)
	exec(`INSERT INTO weave_models_archive(id,project_id,version_number) VALUES ('history-model','history','1.0.0')`)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var marker int
	var raw, effective *string
	var modelType string
	if err := pool.QueryRow(ctx, `SELECT r.metadata_capture_version,a.concept_namespace,a.effective_concept_namespace,m.model_type
		FROM weave_releases r JOIN weave_projects_archive a ON a.id=r.project_id AND a.version_number=r.version
		JOIN weave_models_archive m ON m.project_id=r.project_id AND m.version_number=r.version
		WHERE r.project_id='history'`).Scan(&marker, &raw, &effective, &modelType); err != nil {
		t.Fatal(err)
	}
	if marker != 0 || raw != nil || effective != nil || modelType != "core" {
		t.Fatal("migration fabricated historical metadata")
	}
	// Downgrade is reversible until a new cut certifies this contract.
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	authorized := auth.WithSnapshot(ctx, &auth.AuthSnapshot{IsSuperAdmin: true})
	authorized = auth.WithPrincipal(authorized, &auth.Principal{ActorID: "history-owner"})
	if _, err := release.NewService(release.NewPostgresStore(pool), pool, nil).Create(authorized, "history", release.CreateInput{Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	// Reach the content boundary explicitly; later operational migrations
	// may still be reversible on a database without operation records.
	for version := latest; version > 33; version-- {
		if err := database.MigrateDown(db); err != nil {
			t.Fatal(err)
		}
	}
	// The content boundary protects this fresh cut.
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade captured release content") {
		t.Fatalf("certified metadata lost on downgrade: %v", err)
	}
}
