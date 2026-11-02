package readsession

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

type discoveryProbe struct {
	archive.DBTX
	called int
	args   []any
	rows   []archive.DiscoverContentIdentitiesRow
	err    error
}

func (p *discoveryProbe) Query(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
	p.called++
	p.args = args
	return &discoveryRows{records: p.rows}, p.err
}

type discoveryRows struct {
	pgx.Rows
	records []archive.DiscoverContentIdentitiesRow
	index   int
}

func (r *discoveryRows) Close()     {}
func (r *discoveryRows) Err() error { return nil }
func (r *discoveryRows) Next() bool { r.index++; return r.index <= len(r.records) }
func (r *discoveryRows) Scan(dest ...any) error {
	row := r.records[r.index-1]
	*dest[0].(*string), *dest[1].(*string), *dest[2].(*bool) = row.Kind, row.ID, row.Present
	return nil
}

func TestDiscoveryRequiresLiveExplicitBoundSelection(t *testing.T) {
	for _, tc := range []string{"zero", "closed", "canceled", "nil selection", "unknown selection", "bad page"} {
		t.Run(tc, func(t *testing.T) {
			probe := &discoveryProbe{}
			s := &Session{projectID: "BOUND", scope: auth.Release("1.2.3"), lifetime: &lifetime{}, archive: archive.New(probe)}
			page := Page{Limit: 2, Selection: projectsnapshot.Selection{"field": {"one"}}}
			ctx := t.Context()
			switch tc {
			case "zero":
				s = &Session{}
			case "closed":
				s.lifetime.closed = true
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "nil selection":
				page.Selection = nil
			case "unknown selection":
				page.Selection = projectsnapshot.Selection{"future": {"one"}}
			case "bad page":
				page.Limit = 0
			}
			rows, err := s.DiscoverIdentities(ctx, page)
			if err == nil || rows != nil || probe.called != 0 {
				t.Fatalf("invalid acquisition reached SQL or returned rows: %v", err)
			}
		})
	}
}

func TestDiscoveryDoesNotRetargetOrReturnPartialIdentities(t *testing.T) {
	probe := &discoveryProbe{rows: []archive.DiscoverContentIdentitiesRow{{Kind: "field", ID: "found", Present: true}, {Kind: "future", ID: "unknown"}}}
	s := &Session{projectID: "BOUND", scope: auth.Release("1.2.3"), lifetime: &lifetime{}, archive: archive.New(probe)}
	page := Page{Limit: 2, Selection: projectsnapshot.Selection{"field": {"foreign-looking"}}}
	if rows, err := s.DiscoverIdentities(t.Context(), page); err == nil || rows != nil {
		t.Fatal("unknown edge kind returned partial discovery")
	}
	if probe.args[2] != "BOUND" || probe.args[3] != "1.2.3" || probe.args[6].([]string)[0] != "foreign-looking" {
		t.Fatal("caller changed project/release binding")
	}
	sentinel := errors.New("query failed")
	probe.err = sentinel
	if rows, err := s.DiscoverIdentities(t.Context(), page); !errors.Is(err, sentinel) || rows != nil {
		t.Fatal("query failure returned partial discovery")
	}
	probe.called = 0
	rows, err := s.DiscoverIdentities(t.Context(), Page{Limit: 2, Selection: projectsnapshot.Selection{}})
	if err != nil || rows == nil || len(rows) != 0 || probe.called != 0 {
		t.Fatal("empty selection did not remain empty")
	}
}
