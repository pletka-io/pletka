package model

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type SchemaStore interface {
	GetByID(ctx context.Context, id string) (*domain.Model, error)
}

type SchemaProvider struct {
	store SchemaStore
}

func NewSchemaProvider(store SchemaStore) *SchemaProvider {
	return &SchemaProvider{store: store}
}

func (p *SchemaProvider) EntityType() string {
	return "model"
}

func (p *SchemaProvider) BuildEntityListSchema(_ context.Context, req schemaregistry.EntityListRequest) (*formschema.EntityListSchema, error) {
	return formschema.BuildModelEntityListSchema(req.ProjectID, req.Lang, req.Languages), nil
}

func (p *SchemaProvider) BuildFormSchema(ctx context.Context, req schemaregistry.FormRequest) (*formschema.FormSchema, error) {
	var existing *domain.Model
	if p.store != nil && (req.Mode == formschema.ModeEdit || req.Mode == formschema.ModeView) && req.EntityID != "" {
		model, err := p.store.GetByID(ctx, req.EntityID)
		if err != nil {
			return nil, err
		}
		existing = model
	}
	return BuildFormSchema(req.Mode, existing, req.ProjectID, req.Lang, req.Languages), nil
}
