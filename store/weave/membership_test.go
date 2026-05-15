package weave

import (
	"context"
	"testing"
	"time"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestMembershipStoreNilGuard(t *testing.T) {
	var store *membershipStore

	if err := store.Upsert(context.Background(), domainweave.Membership{}); err == nil {
		t.Fatalf("nil Upsert() error = nil; want error")
	}
	if err := store.Delete(context.Background(), "actor", "project", "project"); err == nil {
		t.Fatalf("nil Delete() error = nil; want error")
	}
	if _, err := store.ListByActor(context.Background(), "actor"); err == nil {
		t.Fatalf("nil ListByActor() error = nil; want error")
	}
	if _, err := store.ListByScope(context.Background(), "project", "project"); err == nil {
		t.Fatalf("nil ListByScope() error = nil; want error")
	}
	if _, err := store.SnapshotForActor(context.Background(), "actor"); err == nil {
		t.Fatalf("nil SnapshotForActor() error = nil; want error")
	}
}

func TestMembershipRowMapping(t *testing.T) {
	createdAt := time.Date(2026, 5, 15, 8, 30, 0, 0, time.UTC)

	membership := membershipFromRow(sqlcgen.WeaveMembership{
		ActorID:   "actor-1",
		ScopeType: "project",
		ScopeID:   "TPC",
		Role:      "editor",
		CreatedAt: createdAt,
	})
	if membership != (domainweave.Membership{
		ActorID:   "actor-1",
		ScopeType: "project",
		ScopeID:   "TPC",
		Role:      "editor",
		CreatedAt: createdAt,
	}) {
		t.Fatalf("membershipFromRow() = %#v", membership)
	}

	scopeMembership := membershipFromScopeRow(sqlcgen.WeaveMembershipListByScopeRow{
		ActorID:   "actor-2",
		ScopeType: "org",
		ScopeID:   "delving",
		Role:      "owner",
		CreatedAt: createdAt,
	})
	if scopeMembership.ActorID != "actor-2" || scopeMembership.ScopeType != "org" || scopeMembership.ScopeID != "delving" || scopeMembership.Role != "owner" || !scopeMembership.CreatedAt.Equal(createdAt) {
		t.Fatalf("membershipFromScopeRow() = %#v", scopeMembership)
	}
}

func TestSnapshotRowMapping(t *testing.T) {
	snapshot := snapshotFromRow(sqlcgen.WeaveMembershipSnapshotForActorRow{
		Kind:      "owned",
		ScopeType: "project",
		ScopeID:   "TPC",
		Role:      "owner",
	})
	if snapshot != (domainweave.SnapshotRow{
		Kind:      "owned",
		ScopeType: "project",
		ScopeID:   "TPC",
		Role:      "owner",
	}) {
		t.Fatalf("snapshotFromRow() = %#v", snapshot)
	}
}
