//go:build integration

package legacyartifact_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/internal/legacyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func TestPostgresArtifactTransportPreservesBytesAndScope(t *testing.T) {
	const chunkBytes = 256 << 10
	pool := testdb.EmptyPool(t)
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	// Transport fixtures shadow only the query's two relations on this private
	// connection. Actual capture/immutability is exercised by read-session tests.
	_, err = conn.Exec(t.Context(), `CREATE TEMP TABLE weave_legacy_ontology_snapshots(project_id text,version_number text,document text,digest text);
CREATE TEMP TABLE weave_releases(project_id text,version text,content_capture_version smallint,content_capture_receipt jsonb);`)
	if err != nil {
		t.Fatal(err)
	}
	payloads := []string{"", "x", strings.Repeat("a", chunkBytes-1), strings.Repeat("b", chunkBytes), strings.Repeat("c", chunkBytes+1), strings.Repeat("ä🙂\\\r\n\"", chunkBytes/2)}
	for i, document := range payloads {
		project := fmt.Sprintf("BYTES%d", i)
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(document)))
		if _, err := conn.Exec(t.Context(), `INSERT INTO weave_legacy_ontology_snapshots VALUES($1,'1.0.0',$2,$3);
`, project, document, digest); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(t.Context(), `INSERT INTO weave_releases VALUES($1,'1.0.0',2,jsonb_build_object('ontology_snapshot_digest',$2::text))`, project, digest); err != nil {
			t.Fatal(err)
		}
	}
	_, err = conn.Exec(t.Context(), `INSERT INTO weave_releases VALUES
('UNFINISHED','1.0.0',0,'{"ontology_snapshot_digest":"same"}'),
('WRONG_RECEIPT','1.0.0',2,'{"ontology_snapshot_digest":"other"}'),
('MISSING_BODY','1.0.0',2,'{"ontology_snapshot_digest":"same"}');
INSERT INTO weave_legacy_ontology_snapshots VALUES
('UNFINISHED','1.0.0','x','same'),('WRONG_RECEIPT','1.0.0','x','same'),('MISSING_RELEASE','1.0.0','x','same'),
('BYTES1','2.0.0','other version','different');
INSERT INTO weave_releases VALUES('BYTES1','2.0.0',2,'{"ontology_snapshot_digest":"different"}');`)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			tx, err := conn.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := tx.Rollback(t.Context()); err != nil {
					t.Error(err)
				}
			}()
			if _, err := tx.Exec(t.Context(), `SELECT set_config('plan_cache_mode',$1,true)`, mode); err != nil {
				t.Fatal(err)
			}
			for i, document := range payloads {
				got, err := legacyartifact.New(tx, fmt.Sprintf("BYTES%d", i), "1.0.0").Read(t.Context())
				if err != nil || got.JSON != document || got.Digest != fmt.Sprintf("%x", sha256.Sum256([]byte(document))) {
					t.Fatalf("payload %d lost exact bytes: %v", i, err)
				}
			}
			got, err := legacyartifact.New(tx, "BYTES1", "2.0.0").Read(t.Context())
			if err != nil || got.JSON != "other version" {
				t.Fatalf("selected version replaced: %v", err)
			}
			for _, project := range []string{"ABSENT", "UNFINISHED", "WRONG_RECEIPT", "MISSING_BODY", "MISSING_RELEASE"} {
				got, err := legacyartifact.New(tx, project, "1.0.0").Read(t.Context())
				if !errors.Is(err, pgx.ErrNoRows) || got != (ontologyevidence.Artifact{}) {
					t.Fatalf("%s returned unmatched evidence: %v", project, err)
				}
			}
			if _, err := legacyartifact.New(tx, "BYTES1", "absent").Read(t.Context()); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("missing version substituted: %v", err)
			}
		})
	}
}
