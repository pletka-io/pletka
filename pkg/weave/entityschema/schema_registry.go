package entityschema

import (
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
	"github.com/pletka-io/pletka/pkg/weave/category"
	weavecollection "github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/example"
	weavefield "github.com/pletka-io/pletka/pkg/weave/field"
	weavemodel "github.com/pletka-io/pletka/pkg/weave/model"
	weavevocabulary "github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

func (h *Handler) defaultSchemaRegistry() *schemaregistry.SchemaRegistry {
	return schemaregistry.NewSchemaRegistry(
		weavemodel.NewSchemaProvider(h.weave.Models()),
		weavecollection.NewSchemaProvider(h.weave.Collections()),
		weavefield.NewSchemaProvider(h.weave.WeaveFields(), h.weave.Overrides()),
		category.NewSchemaProvider(h.weave.WeaveCategories()),
		example.NewSchemaProvider(),
		weavevocabulary.NewConceptListSchemaProvider(h.weave.ConceptLists(), h.weave.Vocabularies()),
	)
}
