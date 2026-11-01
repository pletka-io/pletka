package generators

import (
	"context"
	"io"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/generators/skos"
)

// GenerateConceptListSKOS uses the same scoped source as schema generators.
// SKOS consumes the retained concept graph directly, not the schema Snapshot.
func (s *Service) GenerateConceptListSKOS(ctx context.Context, scope auth.ReadScope, project, id string, w io.Writer) error {
	value, err := s.fragment(ctx, scope, project, "concept_list", id)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return skos.RenderFragment(w, value)
}
