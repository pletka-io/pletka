package autocomplete

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

// ----------------------------------------------------------------------------
// fakeProjectReader
// ----------------------------------------------------------------------------

// fakeProjectReader implements ProjectReader with a fixed slice of resolved
// links. SourceProjectID is "" (own links) so inherited-skip never fires
// unless a test sets it explicitly.
type fakeProjectReader struct {
	links []domain.ResolvedOntologyVersion
	err   error
}

func (f *fakeProjectReader) ResolvedOntologyVersions(
	_ context.Context, _ string, _ domain.ResolvedOntologyVersionOpts,
) ([]domain.ResolvedOntologyVersion, error) {
	return f.links, f.err
}

// minimalVersionLink builds a ResolvedOntologyVersion with only the fields
// resolve() reads.
func minimalVersionLink(versionID string, isPrimary bool) domain.ResolvedOntologyVersion {
	return domain.ResolvedOntologyVersion{
		Link:            &domain.ProjectOntologyVersion{OntologyVersionID: versionID, IsPrimary: isPrimary},
		SourceProjectID: "", // own link — not inherited
	}
}

// ----------------------------------------------------------------------------
// countingStore wraps fakeIndexStore and increments builds on every
// ListClassesByVersions call, which buildIndex calls exactly once per build.
// ----------------------------------------------------------------------------

type countingStore struct {
	fakeIndexStore
	builds atomic.Int64
}

func (c *countingStore) ListClassesByVersions(ctx context.Context, ids []string) ([]*domain.OntologyClass, error) {
	c.builds.Add(1)
	return c.fakeIndexStore.ListClassesByVersions(ctx, ids)
}

// ----------------------------------------------------------------------------
// TestIndexCache_TTLRebuild (fake store + injected clock — no DB needed)
// ----------------------------------------------------------------------------

// TestIndexCache_TTLRebuild builds an index with a fake store, advances an
// injected clock past the TTL, then calls For() again and asserts that BuiltAt
// changed (i.e., the index was rebuilt).
func TestIndexCache_TTLRebuild(t *testing.T) {
	const vID = "v-ttl"

	fakeStore := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{
				ID:                "cls-ttl",
				OntologyVersionID: vID,
				Prefix:            "x",
				LocalName:         "TTL",
				URI:               "http://example.org/TTL",
				Qname:             "x:TTL",
				Label:             domain.Translations{"en": "TTL"},
			},
		},
		versions:   map[string]*domain.OntologyVersion{vID: {ID: vID}},
		ontologies: map[string]*domain.Ontology{},
	}

	reader := &fakeProjectReader{
		links: []domain.ResolvedOntologyVersion{minimalVersionLink(vID, true)},
	}

	base := time.Now()
	now := base
	clock := func() time.Time { return now }

	cache := NewIndexCache(fakeStore, reader, nil).
		WithClock(clock).
		WithTTL(5 * time.Minute)

	req := Request{ProjectID: "proj-ttl"}

	// Cold build.
	idx1, err := cache.For(context.Background(), req)
	if err != nil {
		t.Fatalf("For (cold): %v", err)
	}
	if idx1 == nil {
		t.Fatal("For (cold): got nil index")
	}
	builtAt1 := idx1.BuiltAt

	// Advance past TTL.
	now = now.Add(6 * time.Minute)

	// Stale: must rebuild.
	idx2, err := cache.For(context.Background(), req)
	if err != nil {
		t.Fatalf("For (after TTL): %v", err)
	}
	if idx2 == nil {
		t.Fatal("For (after TTL): got nil index")
	}

	if !idx2.BuiltAt.After(builtAt1) {
		t.Errorf("BuiltAt did not advance after TTL expiry: before=%v after=%v", builtAt1, idx2.BuiltAt)
	}
}

// ----------------------------------------------------------------------------
// TestIndexCache_InvalidateForVersion_DropsLive_KeepsRelease
// ----------------------------------------------------------------------------

// TestIndexCache_InvalidateForVersion_DropsLive_KeepsRelease seeds one live
// entry and one synthetic release entry sharing a version. After
// InvalidateForVersion, the live entry must be gone and the release entry must
// survive.
func TestIndexCache_InvalidateForVersion_DropsLive_KeepsRelease(t *testing.T) {
	const sharedVersion = "v-shared"

	fakeStore := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{
				ID:                "cls-1",
				OntologyVersionID: sharedVersion,
				Prefix:            "x",
				LocalName:         "A",
				URI:               "http://example.org/A",
				Qname:             "x:A",
				Label:             domain.Translations{"en": "Class A"},
			},
		},
		versions:   map[string]*domain.OntologyVersion{sharedVersion: {ID: sharedVersion}},
		ontologies: map[string]*domain.Ontology{},
	}

	reader := &fakeProjectReader{
		links: []domain.ResolvedOntologyVersion{minimalVersionLink(sharedVersion, true)},
	}

	cache := NewIndexCache(fakeStore, reader, nil)

	// Populate live entry via For().
	req := Request{ProjectID: "proj-inv"}
	if _, err := cache.For(context.Background(), req); err != nil {
		t.Fatalf("For: %v", err)
	}

	// Seed a synthetic release entry that also carries sharedVersion.
	releaseIdx := &Index{
		Key:      keyForRequest([]string{sharedVersion}, sharedVersion, "live"),
		Versions: []string{sharedVersion},
		Primary:  sharedVersion,
		BuiltAt:  time.Now(),
		ByQname:  map[string]*Node{},
	}
	cache.SeedReleaseForTest("rel-1", releaseIdx)

	statsBefore := cache.Stats()
	if len(statsBefore) != 2 {
		t.Fatalf("expected 2 entries before invalidation, got %d", len(statsBefore))
	}

	// Invalidate by version — live entry must go, release must survive.
	cache.InvalidateForVersion(sharedVersion)

	statsAfter := cache.Stats()
	if len(statsAfter) != 1 {
		t.Fatalf("expected 1 entry after invalidation, got %d", len(statsAfter))
	}
	surviving := statsAfter[0]
	if surviving.Key.Lock == "live" {
		t.Errorf("surviving entry should be the release entry, got Lock=%q", surviving.Key.Lock)
	}
}

// ----------------------------------------------------------------------------
// TestIndexCache_Subscribe_InvalidatesOnBusEvent
// ----------------------------------------------------------------------------

// TestIndexCache_Subscribe_InvalidatesOnBusEvent verifies that Subscribe wires
// the cache's two invalidation handlers onto a SimpleEventBus and that
// publishing the correct events drops the matching live cache entry.
func TestIndexCache_Subscribe_InvalidatesOnBusEvent(t *testing.T) {
	const vID = "v-bus"
	const pID = "proj-bus"

	fakeStore := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{
				ID:                "cls-bus",
				OntologyVersionID: vID,
				Prefix:            "x",
				LocalName:         "Bus",
				URI:               "http://example.org/Bus",
				Qname:             "x:Bus",
				Label:             domain.Translations{"en": "Bus"},
			},
		},
		versions:   map[string]*domain.OntologyVersion{vID: {ID: vID}},
		ontologies: map[string]*domain.Ontology{},
	}

	reader := &fakeProjectReader{
		links: []domain.ResolvedOntologyVersion{minimalVersionLink(vID, true)},
	}

	bus := domain.NewSimpleEventBus()
	cache := NewIndexCache(fakeStore, reader, nil)
	cache.Subscribe(bus)

	ctx := context.Background()
	req := Request{ProjectID: pID}

	// Warm the cache.
	if _, err := cache.For(ctx, req); err != nil {
		t.Fatalf("For (warm): %v", err)
	}
	if n := len(cache.Stats()); n != 1 {
		t.Fatalf("expected 1 entry after warm, got %d", n)
	}

	// Publish ProjectOntologyVersionsChanged → cache must evict the project's entry.
	bus.Publish(ctx, domain.Event{Type: domain.EventProjectOntologyVersionsChanged, ProjectID: pID})
	if n := len(cache.Stats()); n != 0 {
		t.Errorf("expected 0 entries after ProjectOntologyVersionsChanged event, got %d", n)
	}

	// Re-warm, then test InvalidateForVersion via bus.
	if _, err := cache.For(ctx, req); err != nil {
		t.Fatalf("For (re-warm): %v", err)
	}
	bus.Publish(ctx, domain.Event{Type: domain.EventOntologyVersionImported, EntityID: vID})
	if n := len(cache.Stats()); n != 0 {
		t.Errorf("expected 0 entries after OntologyVersionImported event, got %d", n)
	}
}

// ----------------------------------------------------------------------------
// TestIndexCache_StaleBuildNotCached
// ----------------------------------------------------------------------------

// TestIndexCache_StaleBuildNotCached verifies the epoch-guard: when an
// invalidation runs while a cold build is in flight, put() must discard the
// result instead of caching it.
//
// Deterministic approach: we directly unit-test put(). We capture the epoch
// before calling Drop() (simulating a build that started before the
// invalidation), then call put() with the stale epoch and assert the entry is
// NOT cached. A subsequent put() with the current epoch IS cached.
func TestIndexCache_StaleBuildNotCached(t *testing.T) {
	const vID = "v-epoch"

	fakeStore := &fakeIndexStore{
		classes: []*domain.OntologyClass{
			{
				ID:                "cls-epoch",
				OntologyVersionID: vID,
				Prefix:            "x",
				LocalName:         "Epoch",
				URI:               "http://example.org/Epoch",
				Qname:             "x:Epoch",
				Label:             domain.Translations{"en": "Epoch"},
			},
		},
		versions:   map[string]*domain.OntologyVersion{vID: {ID: vID}},
		ontologies: map[string]*domain.Ontology{},
	}

	reader := &fakeProjectReader{
		links: []domain.ResolvedOntologyVersion{minimalVersionLink(vID, true)},
	}
	cache := NewIndexCache(fakeStore, reader, nil)

	// Build a synthetic index to simulate the "stale" result of a cold build.
	key := keyForRequest([]string{vID}, vID, "live")
	idx := &Index{
		Key:      key,
		Versions: []string{vID},
		Primary:  vID,
		BuiltAt:  cache.now(),
		ByQname:  map[string]*Node{},
	}

	// Step 1: Capture the epoch as a build would (before the actual build).
	epochBefore := cache.readEpoch()

	// Step 2: Simulate concurrent invalidation bumping the epoch.
	cache.Drop()

	// Step 3: put() with the pre-invalidation epoch — must NOT insert.
	cache.put(key, idx, "proj-epoch", epochBefore)

	if n := len(cache.Stats()); n != 0 {
		t.Errorf("stale build should NOT be cached after concurrent Drop; got %d entries", n)
	}

	// Step 4: Sanity — put() with the current (post-Drop) epoch DOES insert.
	currentEpoch := cache.readEpoch()
	cache.put(key, idx, "proj-epoch", currentEpoch)
	if n := len(cache.Stats()); n != 1 {
		t.Errorf("fresh build with current epoch should be cached; got %d entries", n)
	}
}

// ----------------------------------------------------------------------------
// TestIndexCache_Singleflight_OneBuild
// ----------------------------------------------------------------------------

// TestIndexCache_Singleflight_OneBuild fires N concurrent For() calls on a
// cold key and asserts that exactly one underlying buildIndex occurred.
func TestIndexCache_Singleflight_OneBuild(t *testing.T) {
	const sharedVersion = "v-sf"
	const N = 20

	cs := &countingStore{
		fakeIndexStore: fakeIndexStore{
			classes: []*domain.OntologyClass{
				{
					ID:                "cls-sf",
					OntologyVersionID: sharedVersion,
					Prefix:            "x",
					LocalName:         "SF",
					URI:               "http://example.org/SF",
					Qname:             "x:SF",
					Label:             domain.Translations{"en": "SF"},
				},
			},
			versions:   map[string]*domain.OntologyVersion{sharedVersion: {ID: sharedVersion}},
			ontologies: map[string]*domain.Ontology{},
		},
	}

	reader := &fakeProjectReader{
		links: []domain.ResolvedOntologyVersion{minimalVersionLink(sharedVersion, true)},
	}

	cache := NewIndexCache(cs, reader, nil)
	req := Request{ProjectID: "proj-sf"}

	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = cache.For(context.Background(), req)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	got := cs.builds.Load()
	if got != 1 {
		t.Errorf("expected exactly 1 build under singleflight, got %d", got)
	}
}
