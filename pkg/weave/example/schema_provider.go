package example

import (
	"context"

	"github.com/pletka-io/pletka/pkg/formschema"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type SchemaProvider struct{}

func NewSchemaProvider() SchemaProvider {
	return SchemaProvider{}
}

func (SchemaProvider) EntityType() string {
	return "example"
}

func (SchemaProvider) BuildEntityListSchema(_ context.Context, req schemaregistry.EntityListRequest) (*formschema.EntityListSchema, error) {
	return formschema.BuildExampleEntityListSchema(
		req.ProjectID,
		req.Lang,
		req.Languages,
		req.Query.Get("entity_type"),
		req.Query.Get("entity_id"),
	), nil
}
