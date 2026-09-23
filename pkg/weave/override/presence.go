package override

import (
	"sort"
	"sync"
	"time"
)

// Editor is one other actor currently editing an entity's pattern, as
// reported back to a Presence.Beat/Others caller. Since is the earliest
// still-live session that actor holds for the entity — see Others.
type Editor struct {
	ActorID string    `json:"actor_id"`
	Name    string    `json:"name,omitempty"`
	Since   time.Time `json:"since"`
}

// presenceSession is one browser tab's last-known editing state for one
// (entity, actor) pair.
type presenceSession struct {
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
// Entries are keyed by entity, then actor, then a client-generated session
// id (one per open browser tab) — so one curator with two tabs open is one
// editor with two live sessions, not two editors, and closing one tab
// doesn't drop the other (fix round 1, finding 2).
//
// A zero-value Presence is not usable — construct with NewPresence. Every
// method is nil-safe (a nil *Presence silently does nothing / reports no
// editors) so a Handler built as a bare struct literal in a narrow test —
// see project.Handler's existing test callers — never panics just because
// it never went through NewHandler (fix round 1, finding 7).
type Presence struct {
	ttl time.Duration

	mu sync.Mutex
	// byEntry[key(entityType,entityID)][actorID][sessionID] = session
	byEntry map[string]map[string]map[string]presenceSession
}

// NewPresence constructs a Presence whose sessions expire after ttl of
// silence (no Beat), purged at read time (Others).
func NewPresence(ttl time.Duration) *Presence {
	return &Presence{
		ttl:     ttl,
		byEntry: make(map[string]map[string]map[string]presenceSession),
	}
}

// presenceKey combines entityType and entityID into one map key. Both
// values come from URL path segments (chi params) — neither can contain
// the separator in practice, and even if one did, the worst case is two
// distinct entities sharing a presence bucket, never a security issue.
func presenceKey(entityType, entityID string) string {
	return entityType + "\x00" + entityID
}

// Beat records that actorID's session sessionID is editing (entityType,
// entityID) as of now. The session's original since is preserved across
// repeated beats; only lastSeen advances. A session is identified by
// sessionID alone within (entity, actor) — an empty sessionID is a valid,
// if degenerate, single session (see Leave for why that matters).
func (p *Presence) Beat(entityType, entityID, actorID, sessionID string, now time.Time) {
	if p == nil {
		return
	}
	key := presenceKey(entityType, entityID)

	p.mu.Lock()
	defer p.mu.Unlock()

	actors := p.byEntry[key]
	if actors == nil {
		actors = make(map[string]map[string]presenceSession)
		p.byEntry[key] = actors
	}
	sessions := actors[actorID]
	if sessions == nil {
		sessions = make(map[string]presenceSession)
		actors[actorID] = sessions
	}
	sess, ok := sessions[sessionID]
	if !ok {
		sess.since = now
		evictOldestSession(sessions, maxSessionsPerActor-1)
	}
	sess.lastSeen = now
	sessions[sessionID] = sess
}

// maxSessionsPerActor caps how many open sessions one actor keeps for one
// entity. Nobody edits a pattern in nine tabs; the cap is there because a
// session id is whatever the client sends, so a client minting a fresh one
// per heartbeat would otherwise hold an entry per beat until it expired,
// and the sweep walks the whole registry under the one lock.
const maxSessionsPerActor = 8

// evictOldestSession drops least-recently-seen sessions until at most keep
// remain. The caller holds p.mu.
func evictOldestSession(sessions map[string]presenceSession, keep int) {
	for len(sessions) > keep {
		var oldestID string
		var oldestSeen time.Time
		for id, sess := range sessions {
			if oldestID == "" || sess.lastSeen.Before(oldestSeen) {
				oldestID, oldestSeen = id, sess.lastSeen
			}
		}
		delete(sessions, oldestID)
	}
}

// Leave drops one of actorID's sessions for (entityType, entityID). A
// non-empty sessionID drops only that session — the actor's other open
// tabs are untouched, so closing one tab never drops a curator still
// editing in another. An empty sessionID drops every session the actor
// holds for this entity: the fallback for a "left" signal that cannot name
// a session at all — a browser pagehide/unload beacon sent with no body
// decodes to exactly this (see project.Handler's presence endpoint,
// decodePresenceRequest, fix round 1 finding 5).
func (p *Presence) Leave(entityType, entityID, actorID, sessionID string) {
	if p == nil {
		return
	}
	key := presenceKey(entityType, entityID)

	p.mu.Lock()
	defer p.mu.Unlock()

	actors := p.byEntry[key]
	if actors == nil {
		return
	}
	if sessionID == "" {
		delete(actors, actorID)
	} else if sessions := actors[actorID]; sessions != nil {
		delete(sessions, sessionID)
		if len(sessions) == 0 {
			delete(actors, actorID)
		}
	}
	if len(actors) == 0 {
		delete(p.byEntry, key)
	}
}

// Others lists the editors of (entityType, entityID) other than actorID,
// one per actor (never per session), aggregated to the earliest still-live
// session's since, oldest first. actorID is excluded by actor id, not by
// session, so a curator with two tabs open never sees themselves via
// their OTHER tab either.
//
// As a side effect, every session whose last beat is older than ttl
// relative to now is purged — across the WHOLE registry, not just the key
// being read, so an actor whose browser crashed on a pattern nobody reads
// again is not retained for the life of the process (fix round 1, finding
// 3). The map is small enough that this costs nothing.
func (p *Presence) Others(entityType, entityID, actorID string, now time.Time) []Editor {
	if p == nil {
		return nil
	}
	key := presenceKey(entityType, entityID)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.sweepLocked(now)

	actors := p.byEntry[key]
	if actors == nil {
		return nil
	}

	others := make([]Editor, 0, len(actors))
	for id, sessions := range actors {
		if id == actorID {
			continue
		}
		var earliest time.Time
		for _, sess := range sessions {
			if earliest.IsZero() || sess.since.Before(earliest) {
				earliest = sess.since
			}
		}
		others = append(others, Editor{ActorID: id, Since: earliest})
	}
	sort.Slice(others, func(i, j int) bool { return others[i].Since.Before(others[j].Since) })
	return others
}

// sweepLocked purges every session whose last beat is more than p.ttl
// behind now, across every entity key, and drops any actor/entity bucket
// left empty by that purge. Callers must hold p.mu.
func (p *Presence) sweepLocked(now time.Time) {
	for key, actors := range p.byEntry {
		for actorID, sessions := range actors {
			for sessionID, sess := range sessions {
				if now.Sub(sess.lastSeen) > p.ttl {
					delete(sessions, sessionID)
				}
			}
			if len(sessions) == 0 {
				delete(actors, actorID)
			}
		}
		if len(actors) == 0 {
			delete(p.byEntry, key)
		}
	}
}

// sessionCount returns the total number of tracked sessions across the
// whole registry. Unexported: it exists only so presence_test.go can
// observe the cross-key sweep in Others (finding 3) directly, without
// widening the public API for a test-only concern.
func (p *Presence) sessionCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, actors := range p.byEntry {
		for _, sessions := range actors {
			n += len(sessions)
		}
	}
	return n
}
