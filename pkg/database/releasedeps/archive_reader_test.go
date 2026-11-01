package releasedeps

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	archive "github.com/pletka-io/pletka/pkg/database/readsql/dependencyarchive"
)

type capturedFixture struct {
	archive.Querier // Unexpected query capabilities fail the test immediately.
	members         []archive.WeaveReleaseDependencyEntity
	missing         archive.WeaveReleaseDependencyEntity
	batchErr        error
	batchCalls      int
	projectChecks   int
}

func (f *capturedFixture) WeaveReleaseDependencyState(context.Context, archive.WeaveReleaseDependencyStateParams) (archive.WeaveReleaseDependencyStateRow, error) {
	return archive.WeaveReleaseDependencyStateRow{ArchiveOwnershipVersion: 1, ProjectDependenciesVersion: 1}, nil
}
func (f *capturedFixture) WeaveListReleaseProjectDependencies(_ context.Context, p archive.WeaveListReleaseProjectDependenciesParams) ([]archive.WeaveReleaseProjectDependency, error) {
	if p.ProjectID == "root" {
		return []archive.WeaveReleaseProjectDependency{{SourceProjectID: "source", SourceVersion: "1"}}, nil
	}
	return []archive.WeaveReleaseProjectDependency{}, nil
}
func (f *capturedFixture) WeaveReleaseContainsDependencyEntity(_ context.Context, p archive.WeaveReleaseContainsDependencyEntityParams) (bool, error) {
	if p.EntityType != "project" || p.ProjectID != "source" || p.EntityID != "source" || p.Version != "1" {
		return false, fmt.Errorf("unexpected individual membership query")
	}
	f.projectChecks++
	return true, nil
}
func (f *capturedFixture) WeaveListReleaseDependencyEntities(context.Context, archive.WeaveListReleaseDependencyEntitiesParams) ([]archive.WeaveReleaseDependencyEntity, error) {
	return f.members, nil
}
func (f *capturedFixture) WeaveFirstMissingReleaseDependencyEntity(_ context.Context, p archive.WeaveFirstMissingReleaseDependencyEntityParams) (archive.WeaveReleaseDependencyEntity, error) {
	f.batchCalls++
	if p.ProjectID != "root" || p.Version != "release" {
		return archive.WeaveReleaseDependencyEntity{}, fmt.Errorf("batch escaped consumer selection")
	}
	return f.missing, f.batchErr
}

func TestCapturedMembershipUsesOneBatchRegardlessOfInventorySize(t *testing.T) {
	for _, count := range []int{0, 1, 2500} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := &capturedFixture{batchErr: pgx.ErrNoRows}
			for i := range count {
				f.members = append(f.members, archive.WeaveReleaseDependencyEntity{EntityType: "field", EntityID: fmt.Sprint(i), SourceProjectID: "source", SourceVersion: "1"})
			}
			authorized := false
			plan, err := ReadCaptured(t.Context(), f, "root", "release", func(context.Context, string) error { authorized = true; return nil })
			if err != nil {
				t.Fatal(err)
			}
			wantBatches := 0
			if count > 0 {
				wantBatches = 1
			}
			if !authorized || f.projectChecks != 1 || f.batchCalls != wantBatches || len(plan.Membership()) != count {
				t.Fatalf("membership=%d checks=%d batches=%d authorized=%v", len(plan.Membership()), f.projectChecks, f.batchCalls, authorized)
			}
			for _, member := range plan.Membership() {
				if member.Source != (Source{ProjectID: "source", Version: "1"}) {
					t.Fatal("batch changed source pin")
				}
			}
		})
	}
}

func TestCapturedMembershipFailuresReturnNoPartialPlan(t *testing.T) {
	databaseError := errors.New("database unavailable")
	for _, tc := range []struct {
		name      string
		source    string
		version   string
		batchErr  error
		authorize error
		wantCalls int
		wantError error
	}{
		{"missing body", "source", "1", nil, nil, 1, nil},
		{"source outside closure", "other", "1", pgx.ErrNoRows, nil, 0, nil},
		{"wrong source version", "source", "2", pgx.ErrNoRows, nil, 0, nil},
		{"access revoked", "source", "1", pgx.ErrNoRows, databaseError, 0, databaseError},
		{"query failure", "source", "1", databaseError, nil, 1, databaseError},
		{"cancellation", "source", "1", context.Canceled, nil, 1, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			member := archive.WeaveReleaseDependencyEntity{EntityType: "field", EntityID: "absent", SourceProjectID: tc.source, SourceVersion: tc.version}
			f := &capturedFixture{members: []archive.WeaveReleaseDependencyEntity{member}, missing: member, batchErr: tc.batchErr}
			plan, err := ReadCaptured(t.Context(), f, "root", "release", func(context.Context, string) error { return tc.authorize })
			if tc.wantError != nil {
				if !errors.Is(err, tc.wantError) {
					t.Fatalf("lost query/authorization error: %v", err)
				}
			} else {
				var unavailable *UnavailableError
				if !errors.As(err, &unavailable) || !unavailable.IsNotInRelease() {
					t.Fatalf("missing canonical release failure: %v", err)
				}
			}
			if len(plan.Sources()) != 0 || len(plan.Membership()) != 0 || f.batchCalls != tc.wantCalls {
				t.Fatal("partial plan returned or body queried before source checks")
			}
		})
	}
}
