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
	p.Beat("model", "M1", "alice", t0)
	p.Beat("model", "M1", "bob", t0)

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
	p.Beat("model", "M1", "bob", t0)
	p.Beat("model", "M1", "bob", t0.Add(30*time.Second))
	got := p.Others("model", "M1", "alice", t0.Add(30*time.Second))
	if len(got) != 1 || !got[0].Since.Equal(t0) {
		t.Fatalf("since = %+v, want %v", got, t0)
	}
	p.Leave("model", "M1", "bob")
	if got := p.Others("model", "M1", "alice", t0.Add(30*time.Second)); len(got) != 0 {
		t.Fatalf("others after leave = %+v", got)
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
				p.Beat("model", "M1", actorID, now)
				_ = p.Others("model", "M1", actorID, now)
			}
			p.Leave("model", "M1", actorID)
		}(actorID)
	}
	wg.Wait()
}

func TestPresenceKeysAreSeparate(t *testing.T) {
	p := NewPresence(60 * time.Second)
	t0 := time.Now()
	p.Beat("model", "M1", "bob", t0)
	if got := p.Others("collection", "M1", "alice", t0); len(got) != 0 {
		t.Fatalf("collection M1 saw model M1's editors: %+v", got)
	}
}
