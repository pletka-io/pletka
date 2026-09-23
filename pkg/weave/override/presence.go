package override

import (
	"sort"
	"sync"
	"time"
)

// Editor is one other actor currently editing an entity's pattern, as
// reported back to a Presence.Beat/Others caller.
type Editor struct {
	ActorID string    `json:"actor_id"`
	Name    string    `json:"name,omitempty"`
	Since   time.Time `json:"since"`
}

// presenceEntry is one actor's last-known editing state for one entity key.
type presenceEntry struct {
	since    time.Time
	lastSeen time.Time
}

// Presence is an in-memory, notice-only registry of who is editing which
// pattern right now. It blocks nothing — Beat/Leave/Others never fail and
// never take a lock on the entity itself; the save path's advisory lock is
// entirely separate. One process per instance, so a mutex-guarded map is
// enough: a restart forgets every entry, and a 20s client heartbeat refills
// it within one beat interval.
//
// A zero-value Presence is not usable — construct with NewPresence.
type Presence struct {
	ttl time.Duration

	mu      sync.Mutex
	byEntry map[string]map[string]presenceEntry // key(entityType, entityID) -> actorID -> entry
}

// NewPresence constructs a Presence whose entries expire after ttl of
// silence (no Beat) at read time (Others).
func NewPresence(ttl time.Duration) *Presence {
	return &Presence{
		ttl:     ttl,
		byEntry: make(map[string]map[string]presenceEntry),
	}
}

// presenceKey combines entityType and entityID into one map key. Both
// values come from URL path segments (chi params) — neither can contain
// the separator in practice, and even if one did, the worst case is two
// distinct entities sharing a presence bucket, never a security issue.
func presenceKey(entityType, entityID string) string {
	return entityType + "\x00" + entityID
}

// Beat records that actorID is editing (entityType, entityID) as of now.
// The actor's original since is preserved across repeated beats; only
// lastSeen advances.
func (p *Presence) Beat(entityType, entityID, actorID string, now time.Time) {
	key := presenceKey(entityType, entityID)

	p.mu.Lock()
	defer p.mu.Unlock()

	actors := p.byEntry[key]
	if actors == nil {
		actors = make(map[string]presenceEntry)
		p.byEntry[key] = actors
	}
	entry, ok := actors[actorID]
	if !ok {
		entry.since = now
	}
	entry.lastSeen = now
	actors[actorID] = entry
}

// Leave drops actorID's entry for (entityType, entityID), if any.
func (p *Presence) Leave(entityType, entityID, actorID string) {
	key := presenceKey(entityType, entityID)

	p.mu.Lock()
	defer p.mu.Unlock()

	actors := p.byEntry[key]
	if actors == nil {
		return
	}
	delete(actors, actorID)
	if len(actors) == 0 {
		delete(p.byEntry, key)
	}
}

// Others lists the editors of (entityType, entityID) other than actorID,
// oldest since first. Entries whose last beat is older than ttl relative to
// now are purged as a side effect of this read, so a caller reading a stale
// entity never sees a ghost editor.
func (p *Presence) Others(entityType, entityID, actorID string, now time.Time) []Editor {
	key := presenceKey(entityType, entityID)

	p.mu.Lock()
	defer p.mu.Unlock()

	actors := p.byEntry[key]
	if actors == nil {
		return nil
	}
	for id, entry := range actors {
		if now.Sub(entry.lastSeen) > p.ttl {
			delete(actors, id)
		}
	}
	if len(actors) == 0 {
		delete(p.byEntry, key)
		return nil
	}

	others := make([]Editor, 0, len(actors))
	for id, entry := range actors {
		if id == actorID {
			continue
		}
		others = append(others, Editor{ActorID: id, Since: entry.since})
	}
	sort.Slice(others, func(i, j int) bool { return others[i].Since.Before(others[j].Since) })
	return others
}
