//go:build integration

package readsession_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/weave/override"
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
