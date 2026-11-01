//go:build integration

package readsession_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

func TestLargeOverrideSelectionKeepsScopeUnderGenericPlans(t *testing.T) {
	a, b := newFixture(t, "SELECTION_A"), newFixture(t, "SELECTION_B")
	a.cut(t)
	b.cut(t)
	config := a.pool.Config()
	config.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
	pool, err := pgxpool.NewWithConfig(a.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	ids := make([]string, 2048, 2051)
	for i := range ids {
		ids[i] = strconv.Itoa(-i - 1)
	}
	ids = append(ids, strconv.FormatInt(a.override, 10), strconv.FormatInt(a.override, 10), strconv.FormatInt(b.override, 10))
	for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
		for _, tc := range []struct {
			name      string
			selection projectsnapshot.Selection
			want      int
		}{
			{"complete", nil, 1},
			{"empty", projectsnapshot.Selection{}, 0},
			{"foreign", projectsnapshot.Selection{"override": {strconv.FormatInt(b.override, 10)}}, 0},
			{"large", projectsnapshot.Selection{"override": ids}, 1},
		} {
			t.Run(fmt.Sprintf("%s/%s", scope.Version(), tc.name), func(t *testing.T) {
				err := factory.Read(a.ctx, scope, a.project, func(s *readsession.Session) error {
					r, err := override.BindReader(s)
					if err != nil {
						return err
					}
					page := readsession.Page{Limit: 256, Selection: tc.selection}
					rows, err := r.ContentOverrides(a.ctx, page)
					if err != nil {
						return err
					}
					refs, err := r.ContentOverrideRefs(a.ctx, page)
					if err != nil {
						return err
					}
					if len(rows) != tc.want || len(refs) != tc.want {
						t.Fatalf("selected rows=%d refs=%d want=%d", len(rows), len(refs), tc.want)
					}
					if tc.want > 0 && (rows[0].ID != a.override || refs[0].OverrideID != a.override) {
						t.Fatal("selection escaped project")
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestVocabularySelectionKeepsScopeUnderGenericPlans(t *testing.T) {
	a, b := newFixture(t, "VOCAB_SELECTION_A"), newFixture(t, "VOCAB_SELECTION_B")
	for _, f := range []fixture{a, b} {
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES($1||'.v',$1,'local')`, f.project); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES ($1||'.e1',$1||'.v','urn:one','{"en":"original"}'),($1||'.e2',$1||'.v','urn:two','{"en":"original"}')`, f.project); err != nil {
			t.Fatal(err)
		}
		f.cut(t)
	}
	if _, err := a.pool.Exec(a.ctx, `UPDATE weave_vocabulary_entries SET uri='urn:changed:'||id WHERE vocabulary_id=$1||'.v'`, a.project); err != nil {
		t.Fatal(err)
	}
	config := a.pool.Config()
	config.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
	pool, err := pgxpool.NewWithConfig(a.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	factory := readsession.NewFactory(pool).WithSourceAuthorization(auth.CheckProjectRead)
	ids := make([]string, 2048, 2051)
	for i := range ids {
		ids[i] = strconv.Itoa(-i - 1)
	}
	ids = append(ids, a.project+".e1", a.project+".e1", b.project+".e1")
	for _, scope := range []auth.ReadScope{auth.Draft(), auth.Release("1.0.0")} {
		for _, tc := range []struct {
			name      string
			selection projectsnapshot.Selection
			want      int
		}{
			{"complete", nil, 2},
			{"empty", projectsnapshot.Selection{}, 0},
			{"foreign", projectsnapshot.Selection{"vocabulary_entry": {b.project + ".e1"}, "vocabulary": {b.project + ".v"}}, 0},
			{"entries", projectsnapshot.Selection{"vocabulary_entry": ids}, 1},
			{"parent", projectsnapshot.Selection{"vocabulary": {a.project + ".v", a.project + ".v", b.project + ".v"}}, 2},
			{"overlap", projectsnapshot.Selection{"vocabulary_entry": ids, "vocabulary": {a.project + ".v"}}, 2},
		} {
			t.Run(fmt.Sprintf("%s/%s", scope.Version(), tc.name), func(t *testing.T) {
				err := factory.Read(a.ctx, scope, a.project, func(s *readsession.Session) error {
					r, err := vocabulary.BindContentReader(s)
					if err != nil {
						return err
					}
					rows, err := r.ContentVocabularyEntries(a.ctx, readsession.Page{Limit: 256, Selection: tc.selection})
					if err != nil {
						return err
					}
					if len(rows) != tc.want {
						t.Fatalf("selected %d rows, want %d", len(rows), tc.want)
					}
					for i, row := range rows {
						if row.ID != fmt.Sprintf("%s.e%d", a.project, i+1) || row.VocabularyID != a.project+".v" {
							t.Fatal("selection escaped project, duplicated or reordered rows")
						}
						if scope.IsRelease() == (strings.HasPrefix(row.URI, "urn:changed:")) {
							t.Fatal("selection escaped draft/release")
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
