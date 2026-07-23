package gitmaterializer

import (
	"encoding/json"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Local row-to-domain converters. These mirror the unexported helpers
// in pkg/weave but only populate the fields required by the canonical
// serializers. Keeping them here avoids exporting internals of pkg/weave.

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt32(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func unmarshalTranslations(b []byte) domain.Translations {
	if len(b) == 0 {
		return nil
	}
	var t domain.Translations
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	return t
}

func rowToCategory(row sqlcgen.WeaveCategory) *domain.Category {
	return &domain.Category{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  derefStr(row.SemanticID),
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		CanonicalOrder: int(row.CanonicalOrder),
	}
}

func rowToField(row sqlcgen.WeaveField) *domain.Field {
	f := &domain.Field{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  derefStr(row.SemanticID),
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		ExpectedValueType: derefStr(row.ExpectedValueType),
	}
	// CategoryID and SetValue moved to weave_field_overrides (base
	// override row, entity_type='') in migration 027. Materialiser
	// emits override-level state separately from the field document.
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &f.OntologyScope)
	}
	if len(row.PathElements) > 0 {
		_ = json.Unmarshal(row.PathElements, &f.PathElements)
	}
	if len(row.Examples) > 0 {
		_ = json.Unmarshal(row.Examples, &f.Examples)
	}
	if len(row.SubfieldPaths) > 0 {
		_ = json.Unmarshal(row.SubfieldPaths, &f.SubfieldPaths)
	}
	return f
}

func rowToFieldList(row sqlcgen.WeaveListFieldsRow) *domain.Field {
	f := &domain.Field{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  derefStr(row.SemanticID),
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		ExpectedValueType: derefStr(row.ExpectedValueType),
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &f.OntologyScope)
	}
	if len(row.PathElements) > 0 {
		_ = json.Unmarshal(row.PathElements, &f.PathElements)
	}
	if len(row.Examples) > 0 {
		_ = json.Unmarshal(row.Examples, &f.Examples)
	}
	if len(row.SubfieldPaths) > 0 {
		_ = json.Unmarshal(row.SubfieldPaths, &f.SubfieldPaths)
	}
	return f
}

func rowToModel(row sqlcgen.WeaveModel) *domain.Model {
	m := &domain.Model{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID,
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		StagingID: row.StagingID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &m.OntologyScope)
	}
	return m
}

func rowToModelList(row sqlcgen.WeaveListModelsRow) *domain.Model {
	m := &domain.Model{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID,
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		StagingID: row.StagingID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &m.OntologyScope)
	}
	return m
}

func rowToCollection(row sqlcgen.WeaveCollection) *domain.Collection {
	c := &domain.Collection{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID,
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		CollectionNumber:         derefInt32(row.CollectionNumber),
		CanonicalCollectionOrder: derefInt32(row.CanonicalCollectionOrder),
		StagingID:                row.StagingID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &c.OntologyScope)
	}
	return c
}

func rowToCollectionList(row sqlcgen.WeaveListCollectionsRow) *domain.Collection {
	c := &domain.Collection{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID,
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		CollectionNumber:         derefInt32(row.CollectionNumber),
		CanonicalCollectionOrder: derefInt32(row.CanonicalCollectionOrder),
		StagingID:                row.StagingID,
		DefaultCategoryID:        row.DefaultCategoryID,
	}
	if len(row.OntologyScope) > 0 {
		_ = json.Unmarshal(row.OntologyScope, &c.OntologyScope)
	}
	return c
}

func rowToOverride(row sqlcgen.WeaveFieldOverride) *domain.FieldOverride {
	o := &domain.FieldOverride{
		ID:                 row.ID,
		FieldID:            row.FieldID,
		ProjectID:          row.ProjectID,
		EntityType:         row.EntityType,
		EntityID:           row.EntityID,
		Position:           int(row.Position),
		CollectionOrder:    int(row.CollectionOrder),
		DisplayName:        unmarshalTranslations(row.DisplayName),
		Description:        unmarshalTranslations(row.Description),
		CollectionName:     unmarshalTranslations(row.CollectionName),
		CategoryID:         derefStr(row.CategoryID),
		PartOfCollectionID: derefStr(row.PartOfCollectionID),
		SetValue:           derefStr(row.SetValue),
		IsRequired:         derefBool(row.IsRequired),
		MinOccurs:          derefInt32(row.MinOccurs),
		IsHidden:           derefBool(row.IsHidden),
		Visibility:         derefStr(row.Visibility),
		StagingID:          row.StagingID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.MaxOccurs != nil {
		v := int(*row.MaxOccurs)
		o.MaxOccurs = &v
	}
	return o
}

func rowToOverrideRef(row sqlcgen.WeaveOverrideRef) domain.OverrideRef {
	return domain.OverrideRef{
		OverrideID: row.OverrideID,
		RefType:    row.RefType,
		TargetID:   row.TargetID,
		SemanticID: row.SemanticID,
		Position:   int(row.Position),
	}
}
