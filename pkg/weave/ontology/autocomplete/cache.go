package autocomplete

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"golang.org/x/sync/singleflight"
)

const (
	defaultLiveTTL = 10 * time.Minute
	defaultSoftCap = 200
)

// entry is one cached index together with its access metadata.
type entry struct {
	index      *Index
	lastAccess time.Time
	projectIDs map[string]struct{} // live projects that resolved to this key
}

// IndexCache builds and caches one Index per IndexKey with singleflight
// deduplication, strict live-TTL expiry, LRU soft cap, and per-version /
// per-project invalidation. Release entries (Lock != "live") are immune to
// invalidation.
//
// Epoch guard: every invalidation (InvalidateForVersion, InvalidateForProject,
// Drop) increments c.epoch under c.mu. For() captures the epoch before the
// singleflight build starts. put() compares the captured epoch to c.epoch
// under c.mu — if they differ, an invalidation raced the in-flight build and
// the freshly-built entry is NOT inserted. The caller still receives the index
// for this request, but stale data is never cached past a concurrent Drop or
// invalidation event.
type IndexCache struct {
	store    indexStore
	projects ProjectReader
	log      *slog.Logger
	nsBinds  NamespaceBindingsFunc
	now      func() time.Time
	ttl      time.Duration
	softCap  int

	mu      sync.Mutex
	entries map[IndexKey]*entry
	epoch   uint64 // incremented by every invalidation; read/written under mu
	sf      singleflight.Group
}

// NewIndexCache constructs a cache with default TTL (10 m) and soft cap (200).
func NewIndexCache(store indexStore, projects ProjectReader, log *slog.Logger) *IndexCache {
	if log == nil {
		log = slog.Default()
	}
	return &IndexCache{
		store:    store,
		projects: projects,
		log:      log,
		now:      time.Now,
		ttl:      defaultLiveTTL,
		softCap:  defaultSoftCap,
		entries:  map[IndexKey]*entry{},
	}
}

// For returns the warm Index for req, building it if cold or TTL-expired.
// All live requests use Lock="live" (MVP). If req.VersionID is set, that
// single version is used and treated as primary.
//
// An epoch is captured before the singleflight build. If any invalidation
// runs while the build is in flight, put() detects the epoch change and
// skips caching the result — preventing stale data from sitting in the cache
// until the TTL expires.
func (c *IndexCache) For(ctx context.Context, req Request) (*Index, error) {
	versionIDs, primary, err := c.resolve(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("resolve versions: %w", err)
	}
	if len(versionIDs) == 0 {
		return nil, nil
	}
	key := keyForRequest(versionIDs, primary, "live") // MVP: live only

	if idx := c.warm(key, req.ProjectID); idx != nil {
		return idx, nil
	}

	// Capture the epoch before entering the build. Any invalidation that
	// runs concurrently will bump the epoch and cause put() to skip the insert.
	capturedEpoch := c.readEpoch()

	v, err, _ := c.sf.Do(cacheKeyString(key), func() (any, error) {
		// Re-check under singleflight to avoid a double build when two
		// goroutines both missed the warm check above.
		if idx := c.warm(key, req.ProjectID); idx != nil {
			return idx, nil
		}
		idx, buildErr := buildIndex(ctx, c.store, c.nsBinds, key, versionIDs, primary, c.log)
		if buildErr != nil {
			return nil, buildErr
		}
		c.put(key, idx, req.ProjectID, capturedEpoch)
		return idx, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Index), nil
}

// InvalidateForProject drops every live entry that the project resolved to.
// It also bumps the epoch so any concurrent in-flight build for this project
// will not cache its (possibly stale) result.
func (c *IndexCache) InvalidateForProject(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++
	for k, e := range c.entries {
		if k.Lock != "live" {
			continue
		}
		if _, ok := e.projectIDs[projectID]; ok {
			delete(c.entries, k)
		}
	}
}

// InvalidateForVersion drops every live entry whose version set contains
// versionID. Release entries (Lock != "live") are immune.
// It also bumps the epoch so any concurrent in-flight build that reads the
// now-invalidated version will not cache its (possibly stale) result.
func (c *IndexCache) InvalidateForVersion(versionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++
	for k, e := range c.entries {
		if k.Lock != "live" {
			continue
		}
		for _, v := range e.index.Versions {
			if v == versionID {
				delete(c.entries, k)
				break
			}
		}
	}
}

// Subscribe wires the cache's invalidation handlers onto bus. Call once at boot
// after constructing the cache and before any requests arrive. The cache will
// automatically invalidate live entries whenever an OntologyVersionImported or
// ProjectOntologyVersionsChanged event is published on the bus.
func (c *IndexCache) Subscribe(bus domain.EventBus) {
	bus.Subscribe(domain.EventOntologyVersionImported, func(_ context.Context, e domain.Event) {
		if e.EntityID != "" {
			c.InvalidateForVersion(e.EntityID)
		}
	})
	bus.Subscribe(domain.EventProjectOntologyVersionsChanged, func(_ context.Context, e domain.Event) {
		if e.ProjectID != "" {
			c.InvalidateForProject(e.ProjectID)
		}
	})
}

// Preload warms the index for a project in the background. Errors are logged,
// not returned — preload is best-effort. Uses a detached context so it
// survives the request that triggered it.
func (c *IndexCache) Preload(projectID string) {
	go func() {
		if _, err := c.For(context.Background(), Request{ProjectID: projectID}); err != nil {
			c.log.Warn("autocomplete preload failed", "project_id", projectID, "err", err)
		}
	}()
}

// Drop clears the entire cache (operator escape hatch).
// It also bumps the epoch so any concurrent in-flight builds will not re-cache
// their results after the drop.
func (c *IndexCache) Drop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++
	c.entries = map[IndexKey]*entry{}
}

// EntryStat is a snapshot row returned by Stats.
type EntryStat struct {
	Key       IndexKey  `json:"key"`
	Versions  int       `json:"versions"`
	BuiltAt   time.Time `json:"built_at"`
	AgeMillis int64     `json:"age_ms"`
}

// Stats returns a snapshot of current cache entries.
func (c *IndexCache) Stats() []EntryStat {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]EntryStat, 0, len(c.entries))
	for k, e := range c.entries {
		out = append(out, EntryStat{
			Key:       k,
			Versions:  len(e.index.Versions),
			BuiltAt:   e.index.BuiltAt,
			AgeMillis: c.now().Sub(e.index.BuiltAt).Milliseconds(),
		})
	}
	return out
}

// WithNamespaceBindings wires runtime namespace bindings into index builds so full
// URI relation targets can be normalised before they become ghost nodes.
func (c *IndexCache) WithNamespaceBindings(namespaceBindings NamespaceBindingsFunc) *IndexCache {
	c.nsBinds = namespaceBindings
	return c
}

// WithClock overrides the cache clock. Intended for tests only.
func (c *IndexCache) WithClock(now func() time.Time) *IndexCache {
	c.now = now
	return c
}

// NowForTest returns the current cache clock time. Intended for tests only.
func (c *IndexCache) NowForTest() time.Time {
	return c.now()
}

// WithTTL overrides the live TTL. Intended for tests only.
func (c *IndexCache) WithTTL(d time.Duration) *IndexCache {
	c.ttl = d
	return c
}

// SeedReleaseForTest inserts a synthetic release entry. The entry's Key.Lock
// is forced to "release:<releaseID>" so invalidation-immunity tests work.
// Intended for tests only.
func (c *IndexCache) SeedReleaseForTest(releaseID string, idx *Index) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx.Key.Lock = "release:" + releaseID
	c.entries[idx.Key] = &entry{
		index:      idx,
		lastAccess: c.now(),
		projectIDs: map[string]struct{}{},
	}
}

// ----------------------------------------------------------------------------
// internal helpers
// ----------------------------------------------------------------------------

// warm returns the cached index for key when it is present and within TTL.
// On a TTL hit the entry is dropped and nil is returned so the caller rebuilds.
// Also records projectID as a resolver of this key.
func (c *IndexCache) warm(key IndexKey, projectID string) *Index {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[key]
	if e == nil {
		return nil
	}
	if key.Lock == "live" && c.now().Sub(e.index.BuiltAt) >= c.ttl {
		delete(c.entries, key)
		return nil
	}
	e.lastAccess = c.now()
	if projectID != "" {
		e.projectIDs[projectID] = struct{}{}
	}
	return e.index
}

// put inserts idx under key, recording projectID as its originating project,
// then enforces the soft cap by evicting least-recently-accessed entries.
//
// capturedEpoch is the epoch read by For() before the build started. If the
// current epoch has advanced (an invalidation ran while the build was in
// flight), the insert is skipped — the caller still receives the freshly-built
// index for this one request, but stale data is not written into the cache.
func (c *IndexCache) put(key IndexKey, idx *Index, projectID string, capturedEpoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != capturedEpoch {
		// An invalidation raced the build. Do not cache the potentially-stale
		// result; the next request will trigger a fresh build.
		c.log.Debug("autocomplete cache: skipping stale build insert", "key", cacheKeyString(key))
		return
	}
	pids := map[string]struct{}{}
	if projectID != "" {
		pids[projectID] = struct{}{}
	}
	c.entries[key] = &entry{index: idx, lastAccess: c.now(), projectIDs: pids}
	c.evictLocked()
}

// readEpoch returns the current epoch under the mutex.
func (c *IndexCache) readEpoch() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch
}

// evictLocked drops least-recently-accessed entries while over soft cap.
// Linear scan is fine for the default cap of 200.
func (c *IndexCache) evictLocked() {
	for len(c.entries) > c.softCap {
		var oldestKey IndexKey
		var oldest time.Time
		first := true
		for k, e := range c.entries {
			if first || e.lastAccess.Before(oldest) {
				oldestKey = k
				oldest = e.lastAccess
				first = false
			}
		}
		delete(c.entries, oldestKey)
	}
}

// resolve returns the version IDs and primary version ID for req. When
// req.VersionID is set, it pins to that single version (acting as its own
// primary). Inherited links are skipped unless req.IncludeParentProjects.
func (c *IndexCache) resolve(ctx context.Context, req Request) ([]string, string, error) {
	if req.VersionID != "" {
		return []string{req.VersionID}, req.VersionID, nil
	}
	if req.ProjectID == "" {
		return nil, "", nil
	}
	resolved, err := c.projects.ResolvedOntologyVersions(ctx, req.ProjectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		return nil, "", fmt.Errorf("resolved ontology versions: %w", err)
	}
	out := make([]string, 0, len(resolved))
	var primary string
	for _, r := range resolved {
		if r.SourceProjectID != "" && !req.IncludeParentProjects {
			continue
		}
		out = append(out, r.Link.OntologyVersionID)
		if r.Link.IsPrimary {
			primary = r.Link.OntologyVersionID
		}
	}
	return out, primary, nil
}

// cacheKeyString converts an IndexKey to a flat string for singleflight.Group.
func cacheKeyString(k IndexKey) string {
	return k.VersionSet + "#" + k.Primary + "#" + k.Lock
}
