package gitmaterializer

import (
	"testing"
	"time"
)

func TestAcquireProjectLockMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	rel1, err := acquireProjectLock(dir, "LA")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		rel2, err := acquireProjectLock(dir, "LA") // separate fd → flock blocks
		if err != nil {
			t.Error(err)
			close(got)
			return
		}
		close(got)
		rel2()
	}()
	select {
	case <-got:
		t.Fatal("second lock acquired while first held")
	case <-time.After(200 * time.Millisecond):
	}
	rel1()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second lock never acquired after release")
	}
}

func TestAcquireProjectLockDifferentProjectsIndependent(t *testing.T) {
	dir := t.TempDir()

	relLA, err := acquireProjectLock(dir, "LA")
	if err != nil {
		t.Fatal(err)
	}
	defer relLA()

	done := make(chan error, 1)
	go func() {
		relING, err := acquireProjectLock(dir, "ING")
		if err != nil {
			done <- err
			return
		}
		relING()
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("acquireProjectLock for different project failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lock for a different project never acquired — locks are not independent")
	}
}
