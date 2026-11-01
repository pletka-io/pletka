package csvexport

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func decode(raw []byte, into any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("decode retained CSV metadata: %w", err)
	}
	return nil
}

func entity(id, semantic string, system *string, name, description []byte, status string) (domain.Entity, error) {
	e := domain.Entity{ID: id, SemanticID: semantic, SystemName: derefString(system), Status: domain.Status(status)}
	if err := decode(name, &e.UIName); err != nil {
		return e, err
	}
	if err := decode(description, &e.Description); err != nil {
		return e, err
	}
	return e, nil
}

func value[T any](p *T) (out T) {
	if p != nil {
		return *p
	}
	return out
}

func (e *Export) entities(c projectsnapshot.Content) error {
	var err error
	if e.fields, err = projectFields(c.Fields); err != nil {
		return err
	}
	if e.models, err = projectModels(c.Models); err != nil {
		return err
	}
	if e.collections, err = projectCollections(c.Collections); err != nil {
		return err
	}
	e.categories, err = projectCategories(c.Categories)
	return err
}

func projectFields(records []projectsnapshot.FieldRecord) ([]*domain.Field, error) {
	out := make([]*domain.Field, 0, len(records))
	for _, r := range records {
		common, err := entity(r.ID, value(r.SemanticID), r.SystemName, r.UIName, r.Description, r.Status)
		if err != nil {
			return nil, err
		}
		f := &domain.Field{Entity: common, ExpectedValueType: value(r.ExpectedValueType)}
		if err := decode(r.OntologyScope, &f.OntologyScope); err != nil {
			return nil, err
		}
		if err := decode(r.PathElements, &f.PathElements); err != nil {
			return nil, err
		}
		if err := decode(r.SubfieldPaths, &f.SubfieldPaths); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b *domain.Field) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func projectModels(records []projectsnapshot.ModelRecord) ([]*domain.Model, error) {
	out := make([]*domain.Model, 0, len(records))
	for _, r := range records {
		common, err := entity(r.ID, r.ID, r.SystemName, r.UIName, r.Description, r.Status)
		if err != nil {
			return nil, err
		}
		m := &domain.Model{Entity: common}
		if err := decode(r.OntologyScope, &m.OntologyScope); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b *domain.Model) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func projectCollections(records []projectsnapshot.CollectionRecord) ([]*domain.Collection, error) {
	out := make([]*domain.Collection, 0, len(records))
	for _, r := range records {
		common, err := entity(r.ID, r.ID, r.SystemName, r.UIName, r.Description, r.Status)
		if err != nil {
			return nil, err
		}
		coll := &domain.Collection{Entity: common, CollectionNumber: int(value(r.CollectionNumber)), CanonicalCollectionOrder: int(value(r.CanonicalCollectionOrder))}
		if err := decode(r.OntologyScope, &coll.OntologyScope); err != nil {
			return nil, err
		}
		out = append(out, coll)
	}
	slices.SortFunc(out, func(a, b *domain.Collection) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func projectCategories(records []projectsnapshot.CategoryRecord) ([]*domain.Category, error) {
	out := make([]*domain.Category, 0, len(records))
	for _, r := range records {
		common, err := entity(r.ID, value(r.SemanticID), r.SystemName, r.UIName, r.Description, r.Status)
		if err != nil {
			return nil, err
		}
		out = append(out, &domain.Category{Entity: common, CanonicalOrder: int(r.CanonicalOrder)})
	}
	slices.SortFunc(out, func(a, b *domain.Category) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}
