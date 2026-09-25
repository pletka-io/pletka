package vocabulary

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// classifyConnectorErr is the decision this task adds: a degraded lookup is an
// empty result the caller is told about, ErrNotImplemented is simply nothing,
// and anything else is a real failure.
func TestClassifyConnectorErr(t *testing.T) {
	for _, tc := range []struct {
		name         string
		err          error
		wantDegraded bool
		wantFail     bool
	}{
		{"nil", nil, false, false},
		{"degraded", fmt.Errorf("suggest: %w: boom", vocabconnector.ErrDegraded), true, false},
		{"not implemented", vocabconnector.ErrNotImplemented, false, false},
		{"real failure", errors.New("database exploded"), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			degraded, fail := classifyConnectorErr(tc.err)
			if degraded != tc.wantDegraded || fail != tc.wantFail {
				t.Fatalf("classifyConnectorErr(%v) = (%v, %v), want (%v, %v)",
					tc.err, degraded, fail, tc.wantDegraded, tc.wantFail)
			}
		})
	}
}

// The throttle exists because autocomplete fires per keystroke: an outage with
// a few curators typing would otherwise write hundreds of identical lines.
func TestDegradeLogThrottlesPerVocabulary(t *testing.T) {
	l := newDegradeLog()
	now := time.Now()
	if report, count := l.shouldLog("voc-1", now); !report || count != 1 {
		t.Fatalf("first failure: report=%v count=%d, want true, 1", report, count)
	}
	for i := 0; i < 5; i++ {
		if report, _ := l.shouldLog("voc-1", now.Add(time.Duration(i)*time.Second)); report {
			t.Fatal("a failure inside the window was reported")
		}
	}
	// A different vocabulary is reported on its own first failure.
	if report, _ := l.shouldLog("voc-2", now); !report {
		t.Fatal("a second vocabulary's first failure was swallowed")
	}
	// After the window, one line carrying everything since the last one.
	report, count := l.shouldLog("voc-1", now.Add(61*time.Second))
	if !report || count != 6 {
		t.Fatalf("after the window: report=%v count=%d, want true, 6", report, count)
	}
}

// TestDegradeLogConcurrent drives shouldLog from many goroutines at once —
// most hammering one vocabulary, one hammering a second — so `go test -race`
// actually has overlapping goroutines to check, and so a lost update (the
// mutex not covering every read/write) is caught by the count itself rather
// than only by the absence of a crash: every call to shouldLog either gets
// reported immediately (its count folded into reportedSum) or sits in
// l.seen waiting for the next report, so reportedSum plus the final l.seen
// value must equal exactly the number of calls made — a lost increment
// would undercount it.
func TestDegradeLogConcurrent(t *testing.T) {
	l := newDegradeLog()
	now := time.Now()

	const workers = 16
	const callsPerWorker = 500

	var wg sync.WaitGroup
	var mu sync.Mutex
	reportedSum := map[string]int{}

	hammer := func(vocabularyID string) {
		defer wg.Done()
		for i := 0; i < callsPerWorker; i++ {
			if report, count := l.shouldLog(vocabularyID, now); report {
				mu.Lock()
				reportedSum[vocabularyID] += count
				mu.Unlock()
			}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go hammer("voc-1")
	}
	wg.Add(1)
	go hammer("voc-2")
	wg.Wait()

	// wg.Wait has already synchronized-after every goroutine, so reading
	// l.seen directly here (rather than through shouldLog) is race-free.
	l.mu.Lock()
	reportedSum["voc-1"] += l.seen["voc-1"]
	reportedSum["voc-2"] += l.seen["voc-2"]
	l.mu.Unlock()

	if got, want := reportedSum["voc-1"], workers*callsPerWorker; got != want {
		t.Fatalf("voc-1: reported+pending count = %d, want %d (a lost update would undercount)", got, want)
	}
	if got, want := reportedSum["voc-2"], callsPerWorker; got != want {
		t.Fatalf("voc-2: reported+pending count = %d, want %d (a lost update would undercount)", got, want)
	}
}
