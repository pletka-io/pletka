package category

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type SchemaStore interface {
	GetByID(ctx context.Context, id string) (*domain.Category, error)
}

type SchemaProvider struct {
	store SchemaStore
}

func NewSchemaProvider(store SchemaStore) *SchemaProvider {
	return &SchemaProvider{store: store}
}

func (p *SchemaProvider) EntityType() string {
	return "category"
}

func (p *SchemaProvider) BuildFormSchema(ctx context.Context, req schemaregistry.FormRequest) (*formschema.FormSchema, error) {
	if req.Mode == formschema.ModeEdit || req.Mode == formschema.ModeView {
		if p.store == nil || req.EntityID == "" {
			return nil, nil
		}
		cat, err := p.store.GetByID(ctx, req.EntityID)
		if err != nil || cat == nil {
			return nil, err
		}
		return BuildEditForm(req.ProjectID, cat, req.Lang, req.Languages), nil
	}
	return BuildCreateForm(req.ProjectID, req.Lang, req.Languages), nil
}
