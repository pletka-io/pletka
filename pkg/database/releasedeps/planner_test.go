package releasedeps

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

type planFixture struct {
	refs        []sqlcgen.WeaveDraftDependencyReferencesRow
	latest      map[string]string
	pins        map[Source][]Source
	latestCalls map[string]int
	lookupError error
	following   []string
}

func (f *planFixture) WeaveDraftDependencyReferences(context.Context, string) ([]sqlcgen.WeaveDraftDependencyReferencesRow, error) {
	return slices.Clone(f.refs), nil
}
func (f *planFixture) WeaveDraftFollowingDependencies(context.Context, string) ([]string, error) {
	return f.following, nil
}
func (f *planFixture) WeaveDraftDependencyEntityOwner(_ context.Context, p sqlcgen.WeaveDraftDependencyEntityOwnerParams) (string, error) {
	return "", pgx.ErrNoRows
}
func (f *planFixture) WeaveLatestDependencyRelease(_ context.Context, p string) (string, error) {
	f.latestCalls[p]++
	if f.lookupError != nil {
		return "", f.lookupError
	}
	if v := f.latest[p]; v != "" {
		return v, nil
	}
	return "", pgx.ErrNoRows
}
func (f *planFixture) WeaveReleaseDependencyState(_ context.Context, p sqlcgen.WeaveReleaseDependencyStateParams) (sqlcgen.WeaveReleaseDependencyStateRow, error) {
	if _, ok := f.pins[Source{p.ProjectID, p.Version}]; !ok {
		return sqlcgen.WeaveReleaseDependencyStateRow{}, pgx.ErrNoRows
	}
	return sqlcgen.WeaveReleaseDependencyStateRow{ArchiveOwnershipVersion: 1, ProjectDependenciesVersion: 1}, nil
}
func (f *planFixture) WeaveReleaseContainsDependencyEntity(_ context.Context, p sqlcgen.WeaveReleaseContainsDependencyEntityParams) (bool, error) {
	_, ok := f.pins[Source{p.ProjectID, p.Version}]
	return ok, nil
}
func (f *planFixture) WeaveListReleaseProjectDependencies(_ context.Context, p sqlcgen.WeaveListReleaseProjectDependenciesParams) ([]sqlcgen.WeaveReleaseProjectDependency, error) {
	out := make([]sqlcgen.WeaveReleaseProjectDependency, 0, len(f.pins[Source{p.ProjectID, p.Version}]))
	for _, s := range f.pins[Source{p.ProjectID, p.Version}] {
		out = append(out, sqlcgen.WeaveReleaseProjectDependency{SourceProjectID: s.ProjectID, SourceVersion: s.Version})
	}
	return out, nil
}
func allowSource(context.Context, string) error { return nil }
func basePlanFixture() *planFixture {
	return &planFixture{latest: map[string]string{"B": "1", "C": "2"}, latestCalls: map[string]int{}, pins: map[Source][]Source{{"B", "1"}: {{"C", "1"}}, {"C", "1"}: nil, {"C", "2"}: nil}}
}

func TestPlanningResolvesDirectSourcesBeforeMergingClosures(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		f := basePlanFixture()
		f.refs = []sqlcgen.WeaveDraftDependencyReferencesRow{{EntityType: "field", EntityID: "b", SourceProjectID: "B"}, {EntityType: "field", EntityID: "c", SourceProjectID: "C"}}
		if reversed {
			slices.Reverse(f.refs)
		}
		plan, err := planDependencies(t.Context(), f, f, "A", "", allowSource)
		if err == nil || !strings.Contains(err.Error(), "conflicting release pins") || len(plan.Sources()) != 0 {
			t.Fatalf("conflict returned partial plan: %+v %v", plan.Sources(), err)
		}
		if diff := cmp.Diff(map[string]int{"B": 1, "C": 1}, f.latestCalls); diff != "" {
			t.Fatal(diff)
		}
	}
}
func TestExplicitClosureConstrainsUnpinnedReferences(t *testing.T) {
	f := basePlanFixture()
	f.refs = []sqlcgen.WeaveDraftDependencyReferencesRow{{EntityType: "field", EntityID: "c", SourceProjectID: "C"}, {EntityType: "field", EntityID: "b", SourceProjectID: "B", SourceVersion: "1"}, {EntityType: "field", EntityID: "c2", SourceProjectID: "C"}}
	plan, err := planDependencies(t.Context(), f, f, "A", "", allowSource)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]Source{{"B", "1"}, {"C", "1"}}, plan.Sources()); diff != "" {
		t.Fatal(diff)
	}
	if len(f.latestCalls) != 0 {
		t.Fatalf("explicit closure used latest: %v", f.latestCalls)
	}
	sources := plan.Sources()
	sources[0].Version = "changed"
	if source, ok := plan.Entity("field", "b"); !ok || source.Version != "1" {
		t.Fatal("detached plan was mutated")
	}
}
func TestPlanningRejectsErrorsAndUnauthorizedSourcesBeforeLatest(t *testing.T) {
	f := basePlanFixture()
	f.refs = []sqlcgen.WeaveDraftDependencyReferencesRow{{EntityType: "field", EntityID: "c", SourceProjectID: "C"}}
	sentinel := errors.New("lookup failed")
	f.lookupError = sentinel
	if _, err := planDependencies(t.Context(), f, f, "A", "", allowSource); !errors.Is(err, sentinel) {
		t.Fatalf("lookup error lost: %v", err)
	}
	f.latestCalls = map[string]int{}
	if _, err := planDependencies(t.Context(), f, f, "A", "", func(context.Context, string) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("authorization error lost: %v", err)
	}
	if len(f.latestCalls) != 0 {
		t.Fatal("selected a source release before authorization")
	}
	f.following = []string{"C"}
	if _, err := planDependencies(t.Context(), f, f, "A", "", allowSource); err == nil || !strings.Contains(err.Error(), "explicitly follows source drafts") {
		t.Fatalf("follow draft converted: %v", err)
	}
	if len(f.latestCalls) != 0 {
		t.Fatal("follow draft used latest")
	}
	if _, err := planDependencies(t.Context(), f, f, "A", "", nil); err == nil {
		t.Fatal("missing authorization accepted")
	}
}
func TestClosureRejectsCyclesWithinSources(t *testing.T) {
	f := basePlanFixture()
	f.pins[Source{"C", "1"}] = []Source{{"B", "1"}}
	c := newClosure("A", f, allowSource)
	if err := c.selectSource(t.Context(), "B", "1"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle among foreign projects accepted: %v", err)
	}
}
