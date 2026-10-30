package ontologyrelease

import (
	"context"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
)

// ProjectReader is the session-bound, verified publication facet. The alias
// preserves the standalone API while assembly uses Session.Ontologies directly.
type ProjectReader = readsession.OntologyReader

// ReadProject opens a standalone project session and binds its ontology facet.
// Inside an existing session use Session.Ontologies; do not nest this factory.
func (f *Factory) ReadProject(ctx context.Context, project, version string, fn func(ProjectReader) error) error {
	if f == nil || f.pool == nil || strings.TrimSpace(project) == "" || strings.TrimSpace(version) == "" || fn == nil {
		return fmt.Errorf("project ontology session requires a project and concrete release")
	}
	return readsession.NewFactory(f.pool).Read(ctx, auth.Release(version), project, func(s *readsession.Session) error {
		r, err := s.Ontologies(ctx)
		if err != nil {
			return err
		}
		return fn(r)
	})
}
