package legacyartifact

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pletka-io/pletka/pkg/database/internal/legacyartifactsql"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

type chunkRow = legacyartifactsql.ReadLegacyArtifactChunksRow

type fakeRows struct {
	pgx.Rows
	values    []chunkRow
	index     int
	buffer    []byte
	closed    bool
	scanErr   error
	readErr   error
	afterScan func()
}

func (r *fakeRows) Next() bool { r.index++; return r.index <= len(r.values) }
func (r *fakeRows) Err() error { return r.readErr }
func (r *fakeRows) Close() {
	clear(r.buffer)
	r.closed = true
}
func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	v := r.values[r.index-1]
	*dest[0].(*string), *dest[1].(*int32), *dest[2].(*int32) = v.Digest, v.ByteLength, v.ByteOffset
	if v.Chunk == nil {
		r.buffer = nil
	} else {
		clear(r.buffer)
		r.buffer = append(r.buffer[:0], v.Chunk...)
		if r.buffer == nil {
			r.buffer = []byte{}
		}
	}
	switch d := dest[3].(type) {
	case *pgtype.DriverBytes:
		*d = r.buffer
	case *[]byte:
		*d = slices.Clone(r.buffer)
	default:
		return errors.New("unexpected scan type")
	}
	if r.afterScan != nil {
		r.afterScan()
	}
	return nil
}

type fakeDB struct {
	legacyartifactsql.DBTX
	rows *fakeRows
	err  error
	sql  string
	args []any
}

func (d *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	d.sql, d.args = sql, args
	return d.rows, d.err
}

func chunkFixture() []chunkRow {
	return []chunkRow{
		{Digest: "same", ByteLength: chunkBytes + 3, ByteOffset: 1, Chunk: []byte(strings.Repeat("x", chunkBytes-1) + "\xc3")},
		{Digest: "same", ByteLength: chunkBytes + 3, ByteOffset: chunkBytes + 1, Chunk: []byte("\xa4yz")},
	}
}

func TestStreamUsesGeneratedBindingsAndOwnsResultBytes(t *testing.T) {
	values := chunkFixture()
	// A UTF-8 character crosses the protocol row boundary.
	streamDB := &fakeDB{rows: &fakeRows{values: values}}
	got, err := New(streamDB, "PROJECT", "1.2.3").Read(t.Context())
	if err != nil || !streamDB.rows.closed {
		t.Fatalf("stream failed or leaked rows: %v", err)
	}
	collectorDB := &fakeDB{rows: &fakeRows{values: values}}
	params := legacyartifactsql.ReadLegacyArtifactChunksParams{ChunkBytes: chunkBytes, ProjectID: "PROJECT", Version: "1.2.3"}
	collected, err := legacyartifactsql.New(collectorDB).ReadLegacyArtifactChunks(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if streamDB.sql != collectorDB.sql {
		t.Fatal("stream no longer uses the generated query")
	}
	if diff := cmp.Diff(collectorDB.args, streamDB.args); diff != "" {
		t.Fatal(diff)
	}
	var want strings.Builder
	for _, row := range collected {
		want.Write(row.Chunk)
	}
	if got.JSON != want.String() || got.Digest != collected[0].Digest {
		t.Fatal("stream differs from generated scan or retained a reused driver buffer")
	}
}

func TestStreamRejectsIncompleteOrInconsistentRows(t *testing.T) {
	cases := []struct {
		name string
		edit func([]chunkRow) []chunkRow
	}{
		{"missing", func([]chunkRow) []chunkRow { return nil }},
		{"truncated", func(v []chunkRow) []chunkRow { return v[:1] }},
		{"out of order", func(v []chunkRow) []chunkRow { return []chunkRow{v[1], v[0]} }},
		{"duplicate", func(v []chunkRow) []chunkRow { return append(v, v[1]) }},
		{"wrong offset", func(v []chunkRow) []chunkRow { v[1].ByteOffset++; return v }},
		{"wrong length", func(v []chunkRow) []chunkRow { v[1].ByteLength++; return v }},
		{"wrong digest", func(v []chunkRow) []chunkRow { v[1].Digest = "other"; return v }},
		{"negative length", func(v []chunkRow) []chunkRow { v[0].ByteLength = -1; return v }},
		{"short chunk", func(v []chunkRow) []chunkRow { v[0].Chunk = v[0].Chunk[:2]; return v }},
		{"oversized chunk", func(v []chunkRow) []chunkRow { v[0].Chunk = append(v[0].Chunk, 'x'); return v }},
		{"null chunk", func(v []chunkRow) []chunkRow { v[0].Chunk = nil; return v }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDB{rows: &fakeRows{values: tc.edit(chunkFixture())}}
			got, err := New(db, "PROJECT", "1.2.3").Read(t.Context())
			if err == nil || got != (ontologyevidence.Artifact{}) || !db.rows.closed {
				t.Fatalf("partial artifact or open rows after failure: %v", err)
			}
			if tc.name == "missing" && !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("missing artifact changed error: %v", err)
			}
		})
	}
}

func TestStreamErrorsAndCancellationReturnNoArtifact(t *testing.T) {
	sentinel := errors.New("transport failure")
	for _, where := range []string{"query", "scan", "read", "cancel"} {
		t.Run(where, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			db := &fakeDB{rows: &fakeRows{values: chunkFixture()}}
			want := sentinel
			switch where {
			case "query":
				db.err = sentinel
			case "scan":
				db.rows.scanErr = sentinel
			case "read":
				db.rows.readErr = sentinel
			case "cancel":
				db.rows.afterScan = cancel
				want = context.Canceled
			}
			got, err := New(db, "PROJECT", "1.2.3").Read(ctx)
			if !errors.Is(err, want) || got != (ontologyevidence.Artifact{}) || (where != "query" && !db.rows.closed) {
				t.Fatalf("partial artifact, lost error or open rows: %v", err)
			}
		})
	}
	var zero Reader
	if _, err := zero.Read(t.Context()); err == nil {
		t.Fatal("zero reader accepted")
	}
	db := &fakeDB{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := New(db, "PROJECT", "1.2.3").Read(ctx); !errors.Is(err, context.Canceled) || db.sql != "" {
		t.Fatal("canceled reader acquired query capability")
	}
}
