package generators

import (
	"fmt"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func projectionEntity(id, project, semantic, system, status, version string, deprecated bool, created, updated projectsnapshot.Timestamp, name, description []byte) (domain.Entity, error) {
	out := domain.Entity{ID: id, ProjectID: project, SemanticID: semantic, SystemName: system, Status: domain.Status(status), VersionNumber: version, Deprecated: deprecated}
	if err := decodeProjection(name, &out.UIName); err != nil {
		return out, err
	}
	if err := decodeProjection(description, &out.Description); err != nil {
		return out, err
	}
	var err error
	if created != "" {
		out.CreatedAt, err = time.Parse(time.RFC3339Nano, string(created))
		if err != nil {
			return out, err
		}
	}
	if updated != "" {
		out.UpdatedAt, err = time.Parse(time.RFC3339Nano, string(updated))
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func projectEntity(r projectsnapshot.ProjectRecord) (domain.Project, error) {
	e, err := projectionEntity(r.ID, r.ID, "", projectionValue(r.SystemName), r.Status, r.VersionNumber, r.Deprecated, r.CreatedAt, r.UpdatedAt, r.UIName, r.Description)
	if err != nil {
		return domain.Project{}, err
	}
	p := domain.Project{Entity: e, Namespace: projectionValue(r.Namespace), ParentProjectID: r.ParentProjectID, StagingID: r.StagingID, OwnerID: r.OwnerID, CreatedByID: r.CreatedByID, Visibility: r.Visibility, License: r.License, Topics: r.Topics, BaseURL: r.BaseURL}
	err = decodeProjection(r.README, &p.README)
	return p, err
}
func (p *projection) model(key projectsnapshot.EntityKey) (domain.Model, error) {
	for _, r := range p.content[key.Source].Models {
		if r.ID == key.ID {
			e, err := projectionEntity(r.ID, r.ProjectID, r.ID, projectionValue(r.SystemName), r.Status, r.VersionNumber, r.Deprecated, r.CreatedAt, r.UpdatedAt, r.UIName, r.Description)
			if err != nil {
				return domain.Model{}, err
			}
			m := domain.Model{Entity: e, ModelType: r.ModelType, StagingID: r.StagingID}
			m.OntologyScope, err = p.pathElement(key, "/ontology_scope", r.OntologyScope)
			return m, err
		}
	}
	return domain.Model{}, fmt.Errorf("model is outside retained input")
}
func (p *projection) collection(key projectsnapshot.EntityKey) (domain.Collection, error) {
	for _, r := range p.content[key.Source].Collections {
		if r.ID == key.ID {
			e, err := projectionEntity(r.ID, r.ProjectID, r.ID, projectionValue(r.SystemName), r.Status, r.VersionNumber, r.Deprecated, r.CreatedAt, r.UpdatedAt, r.UIName, r.Description)
			if err != nil {
				return domain.Collection{}, err
			}
			c := domain.Collection{Entity: e, CollectionNumber: int(projectionValue(r.CollectionNumber)), CanonicalCollectionOrder: int(projectionValue(r.CanonicalCollectionOrder)), DefaultCategoryID: r.DefaultCategoryID, StagingID: r.StagingID}
			c.OntologyScope, err = p.pathElement(key, "/ontology_scope", r.OntologyScope)
			return c, err
		}
	}
	return domain.Collection{}, fmt.Errorf("collection is outside retained input")
}
func (p *projection) field(key projectsnapshot.EntityKey) (domain.Field, error) {
	for _, r := range p.content[key.Source].Fields {
		if r.ID == key.ID {
			e, err := projectionEntity(r.ID, r.ProjectID, projectionValue(r.SemanticID), projectionValue(r.SystemName), r.Status, r.VersionNumber, r.Deprecated, r.CreatedAt, r.UpdatedAt, r.UIName, r.Description)
			if err != nil {
				return domain.Field{}, err
			}
			f := domain.Field{Entity: e, ExpectedValueType: projectionValue(r.ExpectedValueType)}
			if err := decodeProjection(r.Examples, &f.Examples); err != nil {
				return f, err
			}
			f.OntologyScope, err = p.pathElement(key, "/ontology_scope", r.OntologyScope)
			if err != nil {
				return f, err
			}
			if err := decodeProjection(r.PathElements, &f.PathElements); err != nil {
				return f, err
			}
			for i := range f.PathElements {
				if err := p.resolveElement(key, fmt.Sprintf("/path_elements/%d", i), &f.PathElements[i]); err != nil {
					return f, err
				}
			}
			if err := decodeProjection(r.SubfieldPaths, &f.SubfieldPaths); err != nil {
				return f, err
			}
			for i := range f.SubfieldPaths {
				for j := range f.SubfieldPaths[i].PathElements {
					if err := p.resolveElement(key, fmt.Sprintf("/subfield_paths/%d/path_elements/%d", i, j), &f.SubfieldPaths[i].PathElements[j]); err != nil {
						return f, err
					}
				}
			}
			return f, nil
		}
	}
	return domain.Field{}, fmt.Errorf("field is outside retained input")
}
