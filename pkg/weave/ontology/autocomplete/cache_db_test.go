package autocomplete_test

import (
	"context"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// TestIndexCache_For_AME_Smoke is the live-DB gate: builds the AME index
// through the cache's For() path and asserts basic structural invariants.
// This confirms the cache wires store+projects correctly end-to-end.
// Skipped when the DB is unavailable.
func TestIndexCache_For_AME_Smoke(t *testing.T) {
	pool := testPool(t) // defined in resolver_smoke_test.go (package autocomplete_test)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)

	var now time.Time = time.Now()
	clock := func() time.Time { return now }

	cache := autocomplete.NewIndexCache(store, ws.Projects(), nil).
		WithClock(clock).
		WithTTL(5 * time.Minute)

	req := autocomplete.Request{ProjectID: "AME"}

	// Cold build.
	idx1, err := cache.For(context.Background(), req)
	if err != nil {
		t.Fatalf("For (cold): %v", err)
	}
	if idx1 == nil {
		t.Fatal("For (cold): got nil index")
	}
	if idx1.ByQname["crm:E1_CRM_Entity"] == nil {
		t.Fatal("crm:E1_CRM_Entity missing from index")
	}
	builtAt1 := idx1.BuiltAt

	// Warm hit — same pointer expected.
	idx2, err := cache.For(context.Background(), req)
	if err != nil {
		t.Fatalf("For (warm): %v", err)
	}
	if idx2 != idx1 {
		t.Error("warm hit should return the same *Index pointer")
	}

	// Advance clock past TTL — must rebuild.
	now = now.Add(6 * time.Minute)

	idx3, err := cache.For(context.Background(), req)
	if err != nil {
		t.Fatalf("For (after TTL): %v", err)
	}
	if idx3 == nil {
		t.Fatal("For (after TTL): got nil index")
	}
	if !idx3.BuiltAt.After(builtAt1) {
		t.Errorf("BuiltAt did not advance after TTL expiry: before=%v after=%v", builtAt1, idx3.BuiltAt)
	}
}
