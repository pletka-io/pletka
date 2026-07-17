package collection

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type SchemaStore interface {
	GetByID(ctx context.Context, id string) (*domain.Collection, error)
}

type SchemaProvider struct {
	store SchemaStore
}

func NewSchemaProvider(store SchemaStore) *SchemaProvider {
	return &SchemaProvider{store: store}
}

func (p *SchemaProvider) EntityType() string {
	return "collection"
}

func (p *SchemaProvider) BuildEntityListSchema(_ context.Context, req schemaregistry.EntityListRequest) (*formschema.EntityListSchema, error) {
	return formschema.BuildCollectionEntityListSchema(req.ProjectID, req.Lang, req.Languages), nil
}

func (p *SchemaProvider) BuildFormSchema(ctx context.Context, req schemaregistry.FormRequest) (*formschema.FormSchema, error) {
	var existing *domain.Collection
	if p.store != nil && (req.Mode == formschema.ModeEdit || req.Mode == formschema.ModeView) && req.EntityID != "" {
		collection, err := p.store.GetByID(ctx, req.EntityID)
		if err != nil {
			return nil, err
		}
		existing = collection
	}
	return BuildFormSchema(req.Mode, existing, req.ProjectID, req.Lang, req.Languages), nil
}
