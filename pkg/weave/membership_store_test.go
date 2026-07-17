package weave_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
)

func TestMembershipStore_UpsertAndList(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ms := store.Memberships()
	ctx := context.Background()

	actorID := seedTestActor(t, pool, "testuser_membership")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_memberships WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	m := domain.Membership{
		ActorID:   actorID,
		ScopeType: "project",
		ScopeID:   "LA",
		Role:      "maintainer",
	}
	if err := ms.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	list, err := ms.ListByActor(ctx, actorID)
	if err != nil {
		t.Fatalf("ListByActor: %v", err)
	}
	if len(list) != 1 || list[0].Role != "maintainer" {
		t.Errorf("ListByActor: got %+v want 1 row with role=maintainer", list)
	}

	// Upsert again with a different role → same row updated.
	m.Role = "owner"
	if err := ms.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}
	list, _ = ms.ListByActor(ctx, actorID)
	if list[0].Role != "owner" {
		t.Errorf("after update: got role %q want owner", list[0].Role)
	}
}

func TestMembershipStore_Delete(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ms := store.Memberships()
	ctx := context.Background()

	actorID := seedTestActor(t, pool, "testuser_membership_delete")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_memberships WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	m := domain.Membership{
		ActorID:   actorID,
		ScopeType: "project",
		ScopeID:   "SRDM",
		Role:      "viewer",
	}
	if err := ms.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := ms.Delete(ctx, actorID, "project", "SRDM"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, err := ms.ListByActor(ctx, actorID)
	if err != nil {
		t.Fatalf("ListByActor after delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 memberships after delete, got %d", len(list))
	}
}

func TestMembershipStore_SnapshotForActor(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	ms := store.Memberships()
	ctx := context.Background()

	actorID := seedTestActor(t, pool, "testuser_membership_snapshot")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_memberships WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	m := domain.Membership{
		ActorID:   actorID,
		ScopeType: "project",
		ScopeID:   "LA",
		Role:      "maintainer",
	}
	if err := ms.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	rows, err := ms.SnapshotForActor(ctx, actorID)
	if err != nil {
		t.Fatalf("SnapshotForActor: %v", err)
	}
	if len(rows) == 0 {
		t.Error("expected at least 1 snapshot row, got 0")
	}
	found := false
	for _, r := range rows {
		if r.ScopeID == "LA" && r.Role == "maintainer" && r.Kind == "membership" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected snapshot row for LA/maintainer, got %+v", rows)
	}
}
