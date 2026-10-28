//go:build integration

package release

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// A release-row lock alone cannot protect a row absent from an older RR snapshot.
func TestMetadataRejectsWritersWithPreCutSnapshots(t *testing.T) {
	for _, statement := range []string{
		`INSERT INTO weave_models_archive(id,project_id,version_number) VALUES ('late-extra','ZREL','9.0.1')`,
		`TRUNCATE weave_models_archive`,
	} {
		t.Run(statement, func(t *testing.T) {
			pool := releaseTestPool(t)
			ctx, owner, project := seedZREL(t, pool)
			stale, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				t.Fatal(err)
			}
			defer stale.Rollback(context.Background())
			var before int
			if err := stale.QueryRow(ctx, `SELECT count(*) FROM weave_releases WHERE project_id='ZREL'`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"}); err != nil {
				t.Fatal(err)
			}
			_, err = stale.Exec(ctx, statement)
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || (pgerr.Code != "23514" && pgerr.Code != "40001") {
				t.Fatalf("pre-cut snapshot bypassed archive protection: %v", err)
			}
		})
	}
}

func TestMetadataRejectsCutWithPreArchiveSnapshot(t *testing.T) {
	pool := releaseTestPool(t)
	ctx, owner, _ := seedZREL(t, pool)
	stale, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Rollback(context.Background())
	var before int
	if err := stale.QueryRow(ctx, `SELECT count(*) FROM weave_releases WHERE project_id='ZREL'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// A legacy/raw writer wins before the cut creates its release record.
	mustExec(t, pool, `INSERT INTO weave_models_archive(id,project_id,version_number) VALUES ('early-extra','ZREL','9.0.1')`)
	_, err = stale.Exec(ctx, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ZREL','9.0.1',$1)`, owner)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
		t.Fatalf("cut could miss a committed archive writer: %v", err)
	}
}
