package vocabulary

import (
	"errors"
	"fmt"
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
