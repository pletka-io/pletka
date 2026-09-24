package override

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPresenceOthersExcludesSelfAndExpired(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	p.Beat("model", "M1", "alice", "s1", t0)
	p.Beat("model", "M1", "bob", "s1", t0)

	if got := p.Others("model", "M1", "alice", t0); len(got) != 1 || got[0].ActorID != "bob" {
		t.Fatalf("others = %+v, want [bob]", got)
	}
	// bob's beat is 61s old: expired.
	if got := p.Others("model", "M1", "alice", t0.Add(61*time.Second)); len(got) != 0 {
		t.Fatalf("others = %+v, want none", got)
	}
}

func TestPresenceBeatKeepsSinceAndLeaveDrops(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	p.Beat("model", "M1", "bob", "s1", t0)
	p.Beat("model", "M1", "bob", "s1", t0.Add(30*time.Second))
	got := p.Others("model", "M1", "alice", t0.Add(30*time.Second))
	if len(got) != 1 || !got[0].Since.Equal(t0) {
		t.Fatalf("since = %+v, want %v", got, t0)
	}
	p.Leave("model", "M1", "bob", "s1")
	if got := p.Others("model", "M1", "alice", t0.Add(30*time.Second)); len(got) != 0 {
		t.Fatalf("others after leave = %+v", got)
	}
}

// TestPresenceKeysAreSeparate confirms two different entities never share a
// presence bucket.
func TestPresenceKeysAreSeparate(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Now()
	p.Beat("model", "M1", "bob", "s1", t0)
	if got := p.Others("collection", "M1", "alice", t0); len(got) != 0 {
		t.Fatalf("collection M1 saw model M1's editors: %+v", got)
	}
}

// TestPresenceAggregatesSessionsPerActorByEarliestSince is fix round 1,
// finding 2: one curator with two tabs open is one editor, not two, and
// the reported "since" is the earliest of their still-live tabs — so
// closing the OLDER tab while a newer one keeps beating does not make the
// curator look like they just started (their reported since simply
// becomes the surviving tab's own start), and closing every tab drops
// them entirely.
func TestPresenceAggregatesSessionsPerActorByEarliestSince(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Second)

	p.Beat("model", "M1", "bob", "tab1", t0)
	p.Beat("model", "M1", "bob", "tab2", t1)

	got := p.Others("model", "M1", "alice", t1)
	if len(got) != 1 || got[0].ActorID != "bob" || !got[0].Since.Equal(t0) {
		t.Fatalf("others = %+v, want one bob entry since %v (earliest of the two tabs)", got, t0)
	}

	// tab1 (the earlier one) leaves — tab2 keeps bob present, and the
	// reported since must move to tab2's own start, not disappear.
	p.Leave("model", "M1", "bob", "tab1")
	got = p.Others("model", "M1", "alice", t1)
	if len(got) != 1 || !got[0].Since.Equal(t1) {
		t.Fatalf("others after tab1 left = %+v, want bob since %v (tab2's own start)", got, t1)
	}

	// tab2 leaves too — bob is gone entirely.
	p.Leave("model", "M1", "bob", "tab2")
	if got := p.Others("model", "M1", "alice", t1); len(got) != 0 {
		t.Fatalf("others after both tabs left = %+v, want none", got)
	}
}

// TestPresenceExcludesAllOfCallersOwnSessions pins the second half of
// finding 2's fix: the caller is excluded by actor id, not by session, so
// a curator with two tabs open never sees themselves via their OTHER tab.
func TestPresenceExcludesAllOfCallersOwnSessions(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Now()
	p.Beat("model", "M1", "carol", "tab1", t0)
	p.Beat("model", "M1", "carol", "tab2", t0)
	if got := p.Others("model", "M1", "carol", t0); len(got) != 0 {
		t.Fatalf("others = %+v, want carol excluded from her own two tabs", got)
	}
}

// TestPresenceLeaveWithEmptySessionDropsEveryActorSession is the other
// half of Leave's contract: an empty sessionID (the shape a body-less
// "left" decodes to) drops every session the actor holds, not just one.
func TestPresenceLeaveWithEmptySessionDropsEveryActorSession(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Now()
	p.Beat("model", "M1", "dave", "tab1", t0)
	p.Beat("model", "M1", "dave", "tab2", t0)

	p.Leave("model", "M1", "dave", "")

	if got := p.Others("model", "M1", "someone-else", t0); len(got) != 0 {
		t.Fatalf("others after empty-session leave = %+v, want dave gone from both tabs", got)
	}
}

// TestPresenceSweepAcrossWholeRegistryOnAnyRead is fix round 1, finding 3:
// Others purges expired sessions registry-wide, not only for the key
// being read, so a crashed browser's entry on a pattern nobody reopens
// still gets swept the next time ANY Others call runs. sessionCount is a
// test-only hook (see presence.go) because there is no public way to
// observe a key that was never read directly.
func TestPresenceSweepAcrossWholeRegistryOnAnyRead(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)

	p.Beat("model", "M1", "alice", "s1", t0) // will go stale, never read again
	p.Beat("collection", "C1", "bob", "s1", t0)

	if got := p.sessionCount(); got != 2 {
		t.Fatalf("sessionCount = %d, want 2", got)
	}

	later := t0.Add(61 * time.Second)
	p.Beat("collection", "C1", "bob", "s1", later) // refresh bob so he survives the sweep
	_ = p.Others("collection", "C1", "someone-else", later)

	if got := p.sessionCount(); got != 1 {
		t.Fatalf("sessionCount after reading a DIFFERENT key = %d, want 1 (alice's stale model M1 session must be swept)", got)
	}
}

// TestPresenceConcurrentAccess exercises the one piece of shared mutable
// state in this slice under concurrent Beat/Others/Leave from many
// goroutines. It asserts nothing about ordering — the point is for
// `go test -race` to find no data race on the guarded map.
func TestPresenceConcurrentAccess(t *testing.T) {
	p := NewPresence(60 * time.Second)
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		actorID := fmt.Sprintf("actor-%d", i)
		wg.Add(1)
		go func(actorID string) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				p.Beat("model", "M1", actorID, "s1", now)
				_ = p.Others("model", "M1", actorID, now)
			}
			p.Leave("model", "M1", actorID, "s1")
		}(actorID)
	}
	wg.Wait()
}

// TestPresenceNilReceiverIsSafe is fix round 1, finding 7: several
// existing tests in pkg/weave/project build Handler as a bare struct
// literal (no NewHandler call), which leaves its *Presence field nil.
// Every method must be safe to call on that nil pointer.
func TestPresenceNilReceiverIsSafe(t *testing.T) {
	var p *Presence
	p.Beat("model", "M1", "alice", "s1", time.Now()) // must not panic
	p.Leave("model", "M1", "alice", "s1")            // must not panic
	if got := p.Others("model", "M1", "alice", time.Now()); got != nil {
		t.Fatalf("Others on nil Presence = %+v, want nil", got)
	}
}

// TestPresenceCapsSessionsPerActor pins the bound on the registry. A
// session id is whatever the client sends, so a client minting a fresh one
// on every heartbeat would otherwise hold one entry per beat until it
// expired — and because the sweep walks the whole registry under the one
// lock, that bloat becomes lock-hold time for every other curator in the
// process. Requires edit rights, so it is a buggy or noisy client, not an
// anonymous flood, but the registry should not depend on clients behaving.
func TestPresenceCapsSessionsPerActor(t *testing.T) {
	p := NewPresence(time.Minute)
	start := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 50; i++ {
		p.Beat("model", "M1", "actor-1", fmt.Sprintf("session-%d", i), start.Add(time.Duration(i)*time.Second))
	}

	if got := p.sessionCount(); got > maxSessionsPerActor {
		t.Fatalf("session count = %d, want at most %d — a fresh session id per heartbeat grows the registry unbounded", got, maxSessionsPerActor)
	}

	// The survivors must be the most recent ones: evicting the newest
	// would drop the tab that is actually open.
	others := p.Others("model", "M1", "someone-else", start.Add(49*time.Second))
	if len(others) != 1 {
		t.Fatalf("others = %+v, want the one actor", others)
	}
	if want := start.Add(42 * time.Second); !others[0].Since.Equal(want) {
		t.Errorf("earliest live session = %s, want %s — the oldest sessions should be the evicted ones", others[0].Since, want)
	}
}
