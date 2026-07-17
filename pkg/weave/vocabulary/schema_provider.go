package vocabulary

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

type ConceptListSchemaStore interface {
	GetByID(ctx context.Context, id string) (*domain.ConceptList, error)
}

type VocabularySchemaStore interface {
	ListVocabularies(ctx context.Context) ([]*domain.Vocabulary, error)
}

type ConceptListSchemaProvider struct {
	lists        ConceptListSchemaStore
	vocabularies VocabularySchemaStore
}

func NewConceptListSchemaProvider(lists ConceptListSchemaStore, vocabularies VocabularySchemaStore) *ConceptListSchemaProvider {
	return &ConceptListSchemaProvider{lists: lists, vocabularies: vocabularies}
}

func (p *ConceptListSchemaProvider) EntityType() string {
	return "concept-list"
}

func (p *ConceptListSchemaProvider) BuildEntityListSchema(_ context.Context, req schemaregistry.EntityListRequest) (*formschema.EntityListSchema, error) {
	return formschema.BuildConceptListEntityListSchema(req.ProjectID, req.Lang, req.Languages), nil
}

func (p *ConceptListSchemaProvider) BuildFormSchema(ctx context.Context, req schemaregistry.FormRequest) (*formschema.FormSchema, error) {
	var existing *ConceptListView
	if p.lists != nil && (req.Mode == formschema.ModeEdit || req.Mode == formschema.ModeView) && req.EntityID != "" {
		list, err := p.lists.GetByID(ctx, req.EntityID)
		if err != nil || list == nil || list.ProjectID != req.ProjectID {
			return nil, err
		}
		existing = conceptListViewFromDomain(list)
	}
	vocabs, err := p.projectVocabularyViews(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	return BuildConceptListFormSchema(req.Mode, existing, vocabs, req.ProjectID, req.Lang, req.Languages), nil
}

func (p *ConceptListSchemaProvider) projectVocabularyViews(ctx context.Context, projectID string) ([]VocabularyView, error) {
	if p.vocabularies == nil {
		return nil, nil
	}
	items, err := p.vocabularies.ListVocabularies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]VocabularyView, 0, len(items))
	for _, item := range items {
		if item.ProjectID != "" && item.ProjectID != projectID {
			continue
		}
		out = append(out, VocabularyView{
			ID:            item.ID,
			SemanticID:    item.SemanticID,
			SystemName:    item.SystemName,
			UIName:        item.UIName,
			Description:   item.Description,
			Status:        string(item.Status),
			ProjectID:     item.ProjectID,
			ConnectorType: item.ConnectorType,
			BaseURI:       item.BaseURI,
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	return out, nil
}

func conceptListViewFromDomain(list *domain.ConceptList) *ConceptListView {
	if list == nil {
		return nil
	}
	return &ConceptListView{
		ID:           list.ID,
		SemanticID:   list.SemanticID,
		SystemName:   list.SystemName,
		UIName:       list.UIName,
		Description:  list.Description,
		Status:       string(list.Status),
		ProjectID:    list.ProjectID,
		ListType:     list.ListType,
		VocabularyID: list.VocabularyID,
		CreatedAt:    list.CreatedAt,
		UpdatedAt:    list.UpdatedAt,
	}
}
