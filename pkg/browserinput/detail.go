// Package browserinput defines detached inputs for browser projections. It has
// no readers: content and aggregate queries finish before rendering starts.
package browserinput

import (
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Detail combines bounded structural content with its same-session field usage.
// A fragment is not a complete inverse-usage inventory; field roots require the
// explicit aggregate. Other kinds derive their composition statistics locally.
type Detail struct {
	Fragment   projectsnapshot.EntityFragment
	FieldUsage *domain.FieldUsageCounts
}
