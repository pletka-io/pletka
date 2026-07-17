package field

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type SchemaStore interface {
	GetByID(ctx context.Context, id string) (*domain.Field, error)
}

type SchemaOverrideStore interface {
	GetBase(ctx context.Context, fieldID, projectID string) (*domain.FieldOverride, error)
	GetRefs(ctx context.Context, overrideID int64) ([]domain.OverrideRef, error)
}

type SchemaProvider struct {
	fields    SchemaStore
	overrides SchemaOverrideStore
}

func NewSchemaProvider(fields SchemaStore, overrides SchemaOverrideStore) *SchemaProvider {
	return &SchemaProvider{fields: fields, overrides: overrides}
}

func (p *SchemaProvider) EntityType() string {
	return "field"
}

func (p *SchemaProvider) BuildEntityListSchema(_ context.Context, req schemaregistry.EntityListRequest) (*formschema.EntityListSchema, error) {
	return formschema.BuildFieldEntityListSchema(req.ProjectID, req.Lang, req.Languages), nil
}

func (p *SchemaProvider) BuildFormSchema(ctx context.Context, req schemaregistry.FormRequest) (*formschema.FormSchema, error) {
	var existing *formschema.ComposedFieldInput
	if p.fields != nil && (req.Mode == formschema.ModeEdit || req.Mode == formschema.ModeView) && req.EntityID != "" {
		field, err := p.fields.GetByID(ctx, req.EntityID)
		if err != nil {
			return nil, err
		}
		existing, err = p.composedFieldInput(ctx, field, req.ProjectID)
		if err != nil {
			return nil, err
		}
	}
	return BuildFormSchema(req.Mode, existing, req.ProjectID, req.Lang, req.Languages), nil
}

func (p *SchemaProvider) composedFieldInput(ctx context.Context, field *domain.Field, projectID string) (*formschema.ComposedFieldInput, error) {
	if field == nil {
		return nil, nil
	}
	var overrideInput *formschema.FieldOverrideInput
	if p.overrides != nil {
		base, err := p.overrides.GetBase(ctx, field.ID, projectID)
		if err != nil {
			return nil, err
		}
		if base != nil {
			overrideInput = &formschema.FieldOverrideInput{
				ID:                base.ID,
				FieldID:           base.FieldID,
				ProjectID:         base.ProjectID,
				EntityType:        base.EntityType,
				EntityID:          base.EntityID,
				ExpectedValueType: field.ExpectedValueType,
				CategoryID:        base.CategoryID,
				SetValue:          base.SetValue,
			}
			refs, err := p.overrides.GetRefs(ctx, base.ID)
			if err != nil {
				return nil, err
			}
			for _, ref := range refs {
				switch ref.RefType {
				case "resource_model":
					overrideInput.ExpectedModelIDs = append(overrideInput.ExpectedModelIDs, ref.TargetID)
				case "collection_model":
					overrideInput.ExpectedCollectionIDs = append(overrideInput.ExpectedCollectionIDs, ref.TargetID)
				}
			}
		}
	}
	return ComposedFieldInputFromDomain(field, overrideInput), nil
}
