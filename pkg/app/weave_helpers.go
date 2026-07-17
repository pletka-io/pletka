package app

import (
	"context"
	"net/http"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	ontologyslice "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// weaveLanguages converts the i18n manager's enabled languages to the
// formschema.LanguageInfo slice that weave slice handlers consume.
func weaveLanguages(mgr i18n.Manager) []formschema.LanguageInfo {
	if mgr == nil {
		return nil
	}
	var langs []formschema.LanguageInfo
	for _, l := range mgr.Languages() {
		if !l.Enabled {
			continue
		}
		flag := ""
		if f, ok := l.Metadata["flag"]; ok {
			if s, ok := f.(string); ok {
				flag = s
			}
		}
		langs = append(langs, formschema.LanguageInfo{
			Code: l.Code,
			Name: l.Name,
			Flag: flag,
		})
	}
	return langs
}

// weaveLangResolver returns a request-to-language resolver backed by the
// session manager. Defaults to "en" when no session is active.
func weaveLangResolver(sm *session.Manager) func(*http.Request) string {
	return func(r *http.Request) string {
		if sm != nil {
			if lang := sm.Language(r.Context()); lang != "" {
				return lang
			}
		}
		return "en"
	}
}

// sliceOntologyReader wraps the pkg/weave/ontology slice's Store for slices
// that need ontology metadata. This is the canonical wiring: slices read the
// master ontology tables (weave_ontologies / weave_ontology_versions), not the
// legacy GORM ontologies table.
type sliceOntologyReader struct {
	store ontologyslice.Store
}

func (a sliceOntologyReader) List(ctx context.Context) ([]*domain.Ontology, error) {
	return a.store.ListOntologies(ctx)
}

func (a sliceOntologyReader) GetByID(ctx context.Context, id string) (*domain.Ontology, error) {
	return a.store.GetOntology(ctx, id)
}

func (a sliceOntologyReader) GetVersionByID(ctx context.Context, id string) (*domain.OntologyVersion, error) {
	return a.store.GetVersion(ctx, id)
}

func (a sliceOntologyReader) ListVersionsByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error) {
	return a.store.ListVersionsByOntology(ctx, ontologyID)
}

// sliceOntologyVersionReader splits the version-side methods of the slice store
// into the OntologyVersionReader interface that the project-ontology-version
// slice consumes.
type sliceOntologyVersionReader struct {
	store ontologyslice.Store
}

func (a sliceOntologyVersionReader) GetByID(ctx context.Context, id string) (*domain.OntologyVersion, error) {
	return a.store.GetVersion(ctx, id)
}

func (a sliceOntologyVersionReader) ListByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error) {
	return a.store.ListVersionsByOntology(ctx, ontologyID)
}
