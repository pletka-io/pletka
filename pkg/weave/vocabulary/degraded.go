package vocabulary

import (
	"errors"
	"sync"
	"time"

	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// degradeLogWindow bounds how often one vocabulary's failures are logged.
const degradeLogWindow = time.Minute

// classifyConnectorErr decides what a connector's error means to the caller:
// degraded says "treat as empty, but say so", fail says "this is a real
// failure". ErrNotImplemented is neither — it simply means this connector has
// nothing to add.
func classifyConnectorErr(err error) (degraded, fail bool) {
	switch {
	case err == nil:
		return false, false
	case errors.Is(err, vocabconnector.ErrDegraded):
		return true, false
	case errors.Is(err, vocabconnector.ErrNotImplemented):
		return false, false
	default:
		return false, true
	}
}

// degradeLog throttles the warning a degraded lookup writes. Autocomplete
// fires per keystroke, so an outage would otherwise flood the log with
// identical lines.
type degradeLog struct {
	mu   sync.Mutex
	last map[string]time.Time
	seen map[string]int
}

func newDegradeLog() *degradeLog {
	return &degradeLog{last: map[string]time.Time{}, seen: map[string]int{}}
}

// shouldLog reports whether to write a line now, and how many failures that
// line covers, including this one.
func (d *degradeLog) shouldLog(vocabularyID string, now time.Time) (bool, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[vocabularyID]++
	last, ok := d.last[vocabularyID]
	if ok && now.Sub(last) < degradeLogWindow {
		return false, d.seen[vocabularyID]
	}
	count := d.seen[vocabularyID]
	d.last[vocabularyID] = now
	d.seen[vocabularyID] = 0
	return true, count
}
