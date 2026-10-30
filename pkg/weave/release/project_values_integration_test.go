//go:build integration

package release

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func TestProjectValuesAcceptActualCapturedRows(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	seedContentChildren(t, pool)
	if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	content, counts := capturedValueFixture(ctx, t, pool, "9.0.1")
	var evidence projectsnapshot.Evidence
	if err := readsession.NewFactory(pool).Read(ctx, auth.Release("9.0.1"), project, func(s *readsession.Session) error {
		receipt, err := s.Content(ctx)
		if err != nil {
			return err
		}
		descriptors, err := s.Descriptors(ctx)
		if err != nil {
			return err
		}
		reader, err := s.Ontologies(ctx)
		if err != nil {
			return err
		}
		artifacts, err := reader.Artifacts(ctx)
		if err != nil {
			return err
		}
		refs, err := reader.References(ctx)
		if err != nil {
			return err
		}
		evidence, err = projectsnapshot.VerifiedEvidence(receipt, descriptors, artifacts, refs)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	value, err := projectsnapshot.NewComplete(projectsnapshot.ProjectInput{
		Content: content, Evidence: evidence, Counts: counts,
		Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := value.Entity("model", "ZRELM.1")
	if err != nil {
		t.Fatal(err)
	}
	partial, err := fragment.Content(evidence.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.Fields) != 1 || len(partial.Overrides) != 1 || len(partial.ConceptEntries) != 1 || len(partial.VocabularyEntries) != 2 {
		t.Fatalf("captured model lost its field or concept closure: %+v", partial)
	}
}

// This test-only adapter checks storage/value compatibility against an isolated
// single-release database. It is deliberately not a production loader and does
// not establish the assembler's same-transaction guarantee. Use the independent
// capture ledger, not Coverage(), to choose the sections to read.
func capturedValueFixture(ctx context.Context, t *testing.T, pool *pgxpool.Pool, version string) (projectsnapshot.Content, map[string]int64) {
	t.Helper()
	content := projectsnapshot.Content{}
	value := reflect.ValueOf(&content).Elem()
	fields := map[string]reflect.Value{}
	for i := range value.NumField() {
		fields[value.Type().Field(i).Tag.Get("json")] = value.Field(i)
	}
	counts := map[string]int64{}
	for _, section := range loadCaptureLedger(t).Sections {
		field := fields[section.ArchiveTable]
		if !field.IsValid() {
			t.Fatalf("missing value section %s", section.ArchiveTable)
		}
		field.Set(reflect.MakeSlice(field.Type(), 0, 0))
		rows, err := pool.Query(ctx, "SELECT to_jsonb(a) FROM "+pgx.Identifier{section.ArchiveTable}.Sanitize()+" a WHERE version_number=$1", version)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				t.Fatal(err)
			}
			row := reflect.New(field.Type().Elem())
			decodeCapturedValue(t, raw, row)
			field.Set(reflect.Append(field, row.Elem()))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		counts[section.ArchiveTable] = int64(field.Len())
	}
	return content, counts
}

func decodeCapturedValue(t *testing.T, raw []byte, row reflect.Value) {
	t.Helper()
	var columns map[string]json.RawMessage
	if err := json.Unmarshal(raw, &columns); err != nil {
		t.Fatal(err)
	}
	typ := row.Elem().Type()
	for i := range typ.NumField() {
		f := typ.Field(i)
		name := f.Tag.Get("json")
		source, ok := columns[name]
		if !ok {
			t.Fatalf("capture missing %s.%s", typ, name)
		}
		if string(source) == "null" {
			continue
		}
		switch f.Type {
		case reflect.TypeFor[[]byte]():
			// Values retain JSON bytes; their private freeze encoding uses base64.
			columns[name], _ = json.Marshal([]byte(source))
		case reflect.TypeFor[*projectsnapshot.Decimal]():
			if source[0] != '"' {
				columns[name], _ = json.Marshal(string(source))
			}
		}
	}
	converted, err := json.Marshal(columns)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(converted, row.Interface()); err != nil {
		t.Fatal(err)
	}
}
