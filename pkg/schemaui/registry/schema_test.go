package registry

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/formschema"
)

func TestSchemaRegistryRegistersFunctionProviders(t *testing.T) {
	r := NewSchemaRegistry(
		EntityListProviderFunc{
			Type: "model",
			Build: func(context.Context, EntityListRequest) (*formschema.EntityListSchema, error) {
				return &formschema.EntityListSchema{EntityType: "model"}, nil
			},
		},
		FormProviderFunc{
			Type: "model",
			Build: func(context.Context, FormRequest) (*formschema.FormSchema, error) {
				return &formschema.FormSchema{EntityType: "model"}, nil
			},
		},
	)

	listProvider, ok := r.EntityListProvider("model")
	if !ok {
		t.Fatal("expected entity list provider")
	}
	listSchema, err := listProvider.BuildEntityListSchema(context.Background(), EntityListRequest{})
	if err != nil {
		t.Fatalf("BuildEntityListSchema() error = %v", err)
	}
	if listSchema.EntityType != "model" {
		t.Fatalf("entity list schema entity type = %q, want model", listSchema.EntityType)
	}

	formProvider, ok := r.FormProvider("model")
	if !ok {
		t.Fatal("expected form provider")
	}
	formSchema, err := formProvider.BuildFormSchema(context.Background(), FormRequest{})
	if err != nil {
		t.Fatalf("BuildFormSchema() error = %v", err)
	}
	if formSchema.EntityType != "model" {
		t.Fatalf("form schema entity type = %q, want model", formSchema.EntityType)
	}
}
