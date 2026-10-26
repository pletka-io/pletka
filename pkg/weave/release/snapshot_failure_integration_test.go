//go:build integration

package release

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestCreateRollsBackOwnedArchivesAndOutbox(t *testing.T) {
	for _, code := range []string{"23514", "40001"} {
		t.Run(code, func(t *testing.T) {
			pool := releaseTestPool(t)
			ctx, ownerID, projectID := seedZREL(t, pool)
			mustExec(t, pool, fmt.Sprintf(`CREATE FUNCTION fail_release_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'injected cut failure' USING ERRCODE = '%s'; END $$`, code))
			t.Cleanup(func() {
				mustExec(t, pool, `DROP TRIGGER IF EXISTS fail_release_outbox ON weave_change_set`)
				mustExec(t, pool, `DROP FUNCTION fail_release_outbox()`)
			})
			mustExec(t, pool, `CREATE TRIGGER fail_release_outbox AFTER INSERT ON weave_change_set
				FOR EACH ROW WHEN (NEW.project_id = 'ZREL' AND NEW.kind = 'release') EXECUTE FUNCTION fail_release_outbox()`)
			svc := NewService(NewPostgresStore(pool), pool, nil)
			_, err := svc.Create(releaseAuthCtx(ctx, ownerID), projectID, CreateInput{Version: "9.0.1"})
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != code {
				t.Fatalf("Create error = %v; want SQLSTATE %s", err, code)
			}
			var count int
			mustScan(t, pool, `SELECT
				(SELECT count(*) FROM weave_releases WHERE project_id='ZREL') +
				(SELECT count(*) FROM weave_projects_archive WHERE id='ZREL') +
				(SELECT count(*) FROM weave_release_namespace_bindings_archive WHERE release_project_id='ZREL') +
				(SELECT count(*) FROM weave_categories_archive WHERE project_id='ZREL') +
				(SELECT count(*) FROM weave_fields_archive WHERE project_id='ZREL') +
				(SELECT count(*) FROM weave_models_archive WHERE project_id='ZREL') +
				(SELECT count(*) FROM weave_field_overrides_archive WHERE project_id='ZREL') +
				(SELECT count(*) FROM weave_change_set WHERE project_id='ZREL')`, nil, &count)
			if count != 0 {
				t.Fatalf("failed cut left %d rows", count)
			}
			mustExec(t, pool, `DROP TRIGGER fail_release_outbox ON weave_change_set`)
			if _, err := svc.Create(releaseAuthCtx(ctx, ownerID), projectID, CreateInput{Version: "9.0.1"}); err != nil {
				t.Fatalf("retry: %v", err)
			}
		})
	}
}
