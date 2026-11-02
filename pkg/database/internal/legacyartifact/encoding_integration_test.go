//go:build integration

package legacyartifact_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/internal/legacyartifact"
)

func TestPostgresArtifactLengthUsesTransportEncoding(t *testing.T) {
	config := testdb.Pool(t).Config().ConnConfig.Copy()
	config.Database = "postgres"
	admin, err := pgx.ConnectConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := fmt.Sprintf("artifact_encoding_%d", time.Now().UnixNano())
	if _, err := admin.Exec(t.Context(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0 ENCODING 'LATIN1' LC_COLLATE 'C' LC_CTYPE 'C'"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	}()
	config = config.Copy()
	config.Database = name
	config.RuntimeParams["client_encoding"] = "UTF8"
	conn, err := pgx.ConnectConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(t.Context(), `CREATE TEMP TABLE weave_legacy_ontology_snapshots(project_id text,version_number text,document text,digest text);
CREATE TEMP TABLE weave_releases(project_id text,version text,content_capture_version smallint,content_capture_receipt jsonb);`); err != nil {
		t.Fatal(err)
	}
	document := strings.Repeat("äéñ", 100000)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(document)))
	if _, err := conn.Exec(t.Context(), `INSERT INTO weave_legacy_ontology_snapshots VALUES('ENCODING','1.0.0',$1,$2)`, document, digest); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO weave_releases VALUES('ENCODING','1.0.0',2,jsonb_build_object('ontology_snapshot_digest',$1::text))`, digest); err != nil {
		t.Fatal(err)
	}
	var storedBytes int
	if err := conn.QueryRow(t.Context(), `SELECT octet_length(document) FROM weave_legacy_ontology_snapshots`).Scan(&storedBytes); err != nil || storedBytes*2 != len(document) {
		t.Fatalf("fixture does not distinguish stored and UTF-8 byte length: %d %v", storedBytes, err)
	}
	tx, err := conn.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	got, err := legacyartifact.New(tx, "ENCODING", "1.0.0").Read(t.Context())
	if err != nil || got.JSON != document || got.Digest != digest {
		t.Fatalf("artifact truncated or transcoded twice: %v", err)
	}
}
