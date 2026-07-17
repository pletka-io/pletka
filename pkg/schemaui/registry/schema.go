package registry

import (
	"context"
	"net/url"

	"github.com/pletka-io/pletka/pkg/formschema"
)

type EntityListRequest struct {
	ProjectID  string
	EntityType string
	Lang       string
	Languages  []formschema.LanguageInfo
	Query      url.Values
}

type FormRequest struct {
	ProjectID  string
	EntityType string
	Mode       string
	EntityID   string
	Lang       string
	Languages  []formschema.LanguageInfo
	Query      url.Values
}

type EntityListProvider interface {
	EntityType() string
	BuildEntityListSchema(context.Context, EntityListRequest) (*formschema.EntityListSchema, error)
}

type FormProvider interface {
	EntityType() string
	BuildFormSchema(context.Context, FormRequest) (*formschema.FormSchema, error)
}

type EntityListProviderFunc struct {
	Type  string
	Build func(context.Context, EntityListRequest) (*formschema.EntityListSchema, error)
}

func (p EntityListProviderFunc) EntityType() string {
	return p.Type
}

func (p EntityListProviderFunc) BuildEntityListSchema(ctx context.Context, req EntityListRequest) (*formschema.EntityListSchema, error) {
	return p.Build(ctx, req)
}

type FormProviderFunc struct {
	Type  string
	Build func(context.Context, FormRequest) (*formschema.FormSchema, error)
}

func (p FormProviderFunc) EntityType() string {
	return p.Type
}

func (p FormProviderFunc) BuildFormSchema(ctx context.Context, req FormRequest) (*formschema.FormSchema, error) {
	return p.Build(ctx, req)
}

type SchemaRegistry struct {
	entityLists map[string]EntityListProvider
	forms       map[string]FormProvider
}

func NewSchemaRegistry(providers ...any) *SchemaRegistry {
	r := &SchemaRegistry{
		entityLists: map[string]EntityListProvider{},
		forms:       map[string]FormProvider{},
	}
	for _, provider := range providers {
		if p, ok := provider.(EntityListProvider); ok {
			r.RegisterEntityList(p)
		}
		if p, ok := provider.(FormProvider); ok {
			r.RegisterForm(p)
		}
	}
	return r
}

func (r *SchemaRegistry) RegisterEntityList(provider EntityListProvider) {
	if provider == nil || provider.EntityType() == "" {
		return
	}
	r.entityLists[provider.EntityType()] = provider
}

func (r *SchemaRegistry) RegisterForm(provider FormProvider) {
	if provider == nil || provider.EntityType() == "" {
		return
	}
	r.forms[provider.EntityType()] = provider
}

func (r *SchemaRegistry) EntityListProvider(entityType string) (EntityListProvider, bool) {
	if r == nil {
		return nil, false
	}
	provider, ok := r.entityLists[entityType]
	return provider, ok
}

func (r *SchemaRegistry) FormProvider(entityType string) (FormProvider, bool) {
	if r == nil {
		return nil, false
	}
	provider, ok := r.forms[entityType]
	return provider, ok
}
