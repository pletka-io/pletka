package gitmaterializer

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// pendingOverride is an encoded override waiting for its final file name.
type pendingOverride struct {
	path     string
	position int
	payload  []byte
}

func newPendingOverride(o *domain.FieldOverride, payload []byte, pathType, fieldKey, ownerKey string) pendingOverride {
	spec := domain.PathSpec{EntityType: pathType, FieldID: fieldKey, OwnerID: ownerKey}
	if pathType != entityTypeBaseOverride {
		spec.EntityID = domain.OverrideToken(o.CategoryID, o.PartOfCollectionID)
	}
	return pendingOverride{path: domain.FilePath(spec), position: o.Position, payload: payload}
}

// writeOverrideFiles writes overrides under names derived from their
// placement (domain.OverrideToken). Exact duplicates of one placement on one
// owner get ~2, ~3, ... ordered by position, then bytes, so a name never
// depends on a row id or on the order a query returns rows in.
func writeOverrideFiles(workDir string, files []pendingOverride) error {
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a.path != b.path {
			return a.path < b.path
		}
		if a.position != b.position {
			return a.position < b.position
		}
		return bytes.Compare(a.payload, b.payload) < 0
	})
	seen := map[string]int{}
	for _, f := range files {
		seen[f.path]++
		p := f.path
		if n := seen[f.path]; n > 1 {
			p = strings.TrimSuffix(p, ".yaml") + fmt.Sprintf("~%d.yaml", n)
		}
		if err := writeEntityFile(workDir, p, f.payload); err != nil {
			return err
		}
	}
	return nil
}
