package releasedeps

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// archivedFixture answers the archive-input queries; live owner lookups always
// miss so the archived owner and the pinned closure are what gets exercised.
type archivedFixture struct {
	*planFixture
	archivedRefs  []sqlcgen.WeaveArchivedDependencyReferencesRow
	archivedOwner map[string]string
	following     []string
	seen          []sqlcgen.WeaveArchivedDependencyReferencesParams
}

func (f *archivedFixture) WeaveArchivedDependencyReferences(_ context.Context, p sqlcgen.WeaveArchivedDependencyReferencesParams) ([]sqlcgen.WeaveArchivedDependencyReferencesRow, error) {
	f.seen = append(f.seen, p)
	return f.archivedRefs, nil
}
func (f *archivedFixture) WeaveArchivedFollowingDependencies(context.Context, sqlcgen.WeaveArchivedFollowingDependenciesParams) ([]string, error) {
	return f.following, nil
}
func (f *archivedFixture) WeaveArchivedDependencyEntityOwner(_ context.Context, p sqlcgen.WeaveArchivedDependencyEntityOwnerParams) (string, error) {
	if owner := f.archivedOwner[p.EntityType+"/"+p.EntityID]; owner != "" {
		return owner, nil
	}
	return "", pgx.ErrNoRows
}

func TestArchivedPlanPinsSourcesTheDraftNoLongerAdopts(t *testing.T) {
	f := &archivedFixture{planFixture: basePlanFixture(), archivedOwner: map[string]string{"field/c-field": "C"}}
	f.latest = map[string]string{"B": "1", "C": "1"}
	// B is adopted in the archive with no pin (as July releases are); C is
	// reached only through an archived override whose field the draft deleted.
	f.archivedRefs = []sqlcgen.WeaveArchivedDependencyReferencesRow{
		{EntityType: "field", EntityID: "b-field", SourceProjectID: "B"},
		{EntityType: "field", EntityID: "c-field"},
	}
	plan, err := planDependencies(context.Background(), archivedDraft{q: f, version: "0.1.0"}, f, "A", "0.1.0", allowSource)
	if err != nil {
		t.Fatal(err)
	}
	want := []Source{{"B", "1"}, {"C", "1"}}
	// B's archived adoption pins latest B@1; C is reached through the archived
	// owner of a field the draft deleted and agrees with B@1's captured closure.
	if got := plan.Sources(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sources %+v", got)
	}
	if s, ok := plan.Entity("field", "c-field"); !ok || s != (Source{"C", "1"}) {
		t.Fatalf("archived owner not used: %+v %v", s, ok)
	}
	if len(f.seen) != 1 || f.seen[0] != (sqlcgen.WeaveArchivedDependencyReferencesParams{ProjectID: "A", Version: "0.1.0"}) {
		t.Fatalf("references not read at the release version: %+v", f.seen)
	}
}

func TestArchivedPlanRefusesDraftFollowingInheritance(t *testing.T) {
	f := &archivedFixture{planFixture: basePlanFixture(), following: []string{"B"}}
	_, err := planDependencies(context.Background(), archivedDraft{q: f, version: "0.1.0"}, f, "A", "0.1.0", allowSource)
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("draft-following archive accepted: %v", err)
	}
}
