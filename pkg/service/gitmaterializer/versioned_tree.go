package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func (m *Materializer) writeProjectTreeVersion(ctx context.Context, workDir, projectID, version string) error {
	if err := m.writeProjectManifestVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writeAdoptionManifestsVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writeForkManifestsVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writePletkaModVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writeCategoriesVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writeFieldsVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writeModelsVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writeCollectionsVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	if err := m.writeScopedOverridesByProjectVersion(ctx, workDir, projectID, version, "model"); err != nil {
		return err
	}
	if err := m.writeScopedOverridesByProjectVersion(ctx, workDir, projectID, version, "collection"); err != nil {
		return err
	}
	if err := m.writeAdoptedEntitiesVersion(ctx, workDir, projectID, version); err != nil {
		return err
	}
	return nil
}

func (m *Materializer) writeProjectManifestVersion(ctx context.Context, workDir, projectID, version string) error {
	manifest, err := m.loadProjectManifestAtVersion(ctx, projectID, version)
	if err != nil {
		return err
	}
	payload, err := encodeProjectManifest(manifest)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, domain.FilePath(domain.PathSpec{EntityType: "project"}), payload)
}

func (m *Materializer) loadProjectManifestAtVersion(ctx context.Context, projectID, version string) (projectManifest, error) {
	row, err := m.loadArchivedProject(ctx, projectID, version)
	if err != nil {
		return projectManifest{}, err
	}

	manifest := projectManifest{
		SchemaVersion: 1,
		Project: projectManifestProject{
			ID:          row.ID,
			SystemName:  row.SystemName,
			Title:       row.UIName,
			Description: row.Description,
			Readme:      row.Readme,
			Namespace:   row.Namespace,
			Visibility:  row.Visibility,
			License:     row.License,
			BaseURL:     row.BaseURL,
			Topics:      append([]string(nil), row.Topics...),
		},
		Manifests: &projectManifestManifests{
			Adoptions: "adoptions/index.yaml",
			Forks:     "forks/index.yaml",
		},
	}

	if owner := m.loadManifestActor(ctx, &row.OwnerID); owner != nil {
		manifest.Project.Owner = owner
	}
	manifest.Project.CreatedBy = m.loadManifestActor(ctx, row.CreatedByID)

	if parents, err := m.loadProjectManifestParentsAtVersion(ctx, projectID, version); err != nil {
		return projectManifest{}, err
	} else if len(parents) > 0 {
		manifest.Inheritance = &projectManifestInheritance{Parents: parents}
	}

	if linked, err := m.loadProjectManifestOntologiesAtVersion(ctx, projectID, version); err != nil {
		return projectManifest{}, err
	} else if len(linked) > 0 {
		manifest.Ontologies = &projectManifestOntologies{LinkedVersions: linked}
	}

	project := domain.Project{
		Entity:    domain.Entity{ID: row.ID},
		Namespace: row.Namespace,
	}
	if bindings, err := m.loadProjectManifestNamespacesAtVersion(ctx, project, version); err != nil {
		return projectManifest{}, err
	} else if len(bindings) > 0 {
		manifest.Namespaces = &projectManifestNamespaces{EffectiveBindings: bindings}
	}

	return manifest, nil
}

type archivedProjectManifestRow struct {
	ID          string
	SystemName  string
	UIName      domain.Translations
	Description domain.Translations
	Readme      domain.Translations
	Namespace   string
	Visibility  string
	OwnerID     string
	CreatedByID *string
	License     string
	BaseURL     string
	Topics      []string
}

func (m *Materializer) loadArchivedProject(ctx context.Context, projectID, version string) (*archivedProjectManifestRow, error) {
	row := m.pool.QueryRow(ctx, `
		SELECT id, COALESCE(system_name, ''), ui_name, description, readme, COALESCE(namespace, ''), visibility, owner_id, created_by_id, license, base_url, topics
		FROM weave_projects_archive
		WHERE id = $1 AND version_number = $2
	`, projectID, version)
	var out archivedProjectManifestRow
	var uiName, description, readme []byte
	if err := row.Scan(&out.ID, &out.SystemName, &uiName, &description, &readme, &out.Namespace, &out.Visibility, &out.OwnerID, &out.CreatedByID, &out.License, &out.BaseURL, &out.Topics); err != nil {
		return nil, fmt.Errorf("get archived project %s@%s: %w", projectID, version, err)
	}
	out.UIName = unmarshalTranslations(uiName)
	out.Description = unmarshalTranslations(description)
	out.Readme = unmarshalTranslations(readme)
	return &out, nil
}

func (m *Materializer) loadProjectManifestParentsAtVersion(ctx context.Context, projectID, version string) ([]projectManifestParent, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT parent_project_id, is_primary, canonical_order, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived project inheritances for manifest: %w", err)
	}
	defer rows.Close()

	parents := make([]projectManifestParent, 0)
	for rows.Next() {
		var parent projectManifestParent
		if err := rows.Scan(&parent.ProjectID, &parent.IsPrimary, &parent.CanonicalOrder, &parent.SourceMode, &parent.SourceVersion); err != nil {
			return nil, fmt.Errorf("scan archived project inheritance for manifest: %w", err)
		}
		parent.SourceMode = strings.TrimSpace(parent.SourceMode)
		parent.SourceVersion = strings.TrimSpace(parent.SourceVersion)
		parents = append(parents, parent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived project inheritances for manifest: %w", err)
	}
	return parents, nil
}

func (m *Materializer) loadProjectManifestOntologiesAtVersion(ctx context.Context, projectID, version string) ([]projectManifestLinkedOntology, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT ontology_version_id, COALESCE(is_primary, false)
		FROM weave_project_ontology_versions_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY COALESCE(is_primary, false) DESC, ontology_version_id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived project ontology versions for manifest: %w", err)
	}
	defer rows.Close()

	linked := make([]projectManifestLinkedOntology, 0)
	for rows.Next() {
		var ontologyVersionID string
		var isPrimary bool
		if err := rows.Scan(&ontologyVersionID, &isPrimary); err != nil {
			return nil, fmt.Errorf("scan archived project ontology version for manifest: %w", err)
		}
		versionRow, err := m.queries.WeaveGetOntologyVersionByID(ctx, ontologyVersionID)
		if err != nil {
			return nil, fmt.Errorf("get ontology version %s for archived manifest: %w", ontologyVersionID, err)
		}
		linked = append(linked, projectManifestLinkedOntology{
			OntologyID:        versionRow.OntologyID,
			OntologyVersionID: versionRow.ID,
			Version:           versionRow.VersionString,
			IsPrimary:         isPrimary,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived project ontology versions for manifest: %w", err)
	}
	return linked, nil
}

func (m *Materializer) loadProjectManifestNamespacesAtVersion(ctx context.Context, project domain.Project, version string) ([]projectManifestNamespaceBinding, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT prefix, namespace, weight, source
		FROM weave_namespace_bindings_archive
		WHERE project_id = $1 AND version_number = $2
	`, project.ID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived namespace bindings for manifest: %w", err)
	}
	defer rows.Close()

	visible := make([]*domain.NamespaceBinding, 0)
	for rows.Next() {
		b := &domain.NamespaceBinding{ProjectID: project.ID}
		if err := rows.Scan(&b.Prefix, &b.Namespace, &b.Weight, &b.Source); err != nil {
			return nil, fmt.Errorf("scan archived namespace binding for manifest: %w", err)
		}
		visible = append(visible, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived namespace bindings for manifest: %w", err)
	}

	set := generators.BuildNamespaceSet(project, visible)
	bindings := make([]projectManifestNamespaceBinding, 0, len(set.Bindings))
	for _, binding := range set.Bindings {
		bindings = append(bindings, projectManifestNamespaceBinding{
			Prefix:    binding.Prefix,
			Namespace: binding.Namespace,
			Source:    binding.Source,
		})
	}
	return bindings, nil
}

func (m *Materializer) writePletkaModVersion(ctx context.Context, workDir, projectID, version string) error {
	manifest, err := m.loadPletkaModManifestAtVersion(ctx, projectID, version)
	if err != nil {
		return err
	}
	payload, err := encodePletkaModManifest(manifest)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, "pletka.mod", payload)
}

func (m *Materializer) loadPletkaModManifestAtVersion(ctx context.Context, projectID, version string) (pletkaModManifest, error) {
	projectRow, err := m.loadArchivedProject(ctx, projectID, version)
	if err != nil {
		return pletkaModManifest{}, err
	}
	modulePath, err := m.projectModulePath(ctx, projectRow.OwnerID, projectID)
	if err != nil {
		return pletkaModManifest{}, err
	}
	projectReqs, err := m.loadPletkaProjectRequirementsAtVersion(ctx, projectID, version)
	if err != nil {
		return pletkaModManifest{}, err
	}
	ontologyReqs, err := m.loadPletkaOntologyRequirementsAtVersion(ctx, projectID, version)
	if err != nil {
		return pletkaModManifest{}, err
	}
	return pletkaModManifest{
		SchemaVersion: 1,
		Module:        pletkaModModule{Path: modulePath, ProjectID: projectID},
		Require: pletkaModRequire{
			Projects:   projectReqs,
			Ontologies: ontologyReqs,
		},
		Replace: []map[string]any{},
	}, nil
}

func (m *Materializer) loadPletkaProjectRequirementsAtVersion(ctx context.Context, projectID, version string) ([]pletkaProjectRequirement, error) {
	parents, err := m.loadProjectManifestParentsAtVersion(ctx, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("load archived parent requirements: %w", err)
	}
	reqs := make([]pletkaProjectRequirement, 0, len(parents))
	for _, parent := range parents {
		parentProject, err := m.loadArchivedProject(ctx, parent.ProjectID, version)
		if err != nil {
			// pinned parent selectors still need the owner/module path; fall back to hot project row lookup
			live, liveErr := m.queries.WeaveGetProjectByID(ctx, parent.ProjectID)
			if liveErr != nil {
				return nil, fmt.Errorf("get parent project %s for archived pletka.mod: %w", parent.ProjectID, err)
			}
			parentProject = &archivedProjectManifestRow{ID: live.ID, OwnerID: live.OwnerID}
		}
		modulePath, err := m.projectModulePath(ctx, parentProject.OwnerID, parent.ProjectID)
		if err != nil {
			return nil, err
		}
		reqVersion := parentDependencyVersionDraft
		if strings.TrimSpace(parent.SourceMode) == "release" && strings.TrimSpace(parent.SourceVersion) != "" {
			reqVersion = strings.TrimSpace(parent.SourceVersion)
		}
		reqs = append(reqs, pletkaProjectRequirement{
			Module:    modulePath,
			ProjectID: parent.ProjectID,
			Mode:      "parent",
			Version:   reqVersion,
		})
	}
	return reqs, nil
}

func (m *Materializer) loadPletkaOntologyRequirementsAtVersion(ctx context.Context, projectID, version string) ([]pletkaOntologyRequirement, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT ontology_version_id
		FROM weave_project_ontology_versions_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY ontology_version_id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived ontology requirements: %w", err)
	}
	defer rows.Close()

	reqs := make([]pletkaOntologyRequirement, 0)
	for rows.Next() {
		var ontologyVersionID string
		if err := rows.Scan(&ontologyVersionID); err != nil {
			return nil, fmt.Errorf("scan archived ontology requirement: %w", err)
		}
		versionRow, err := m.queries.WeaveGetOntologyVersionByID(ctx, ontologyVersionID)
		if err != nil {
			return nil, fmt.Errorf("get ontology version %s for archived pletka.mod: %w", ontologyVersionID, err)
		}
		ontology, err := m.queries.WeaveGetOntologyByID(ctx, versionRow.OntologyID)
		if err != nil {
			return nil, fmt.Errorf("get ontology %s for archived pletka.mod: %w", versionRow.OntologyID, err)
		}
		reqs = append(reqs, pletkaOntologyRequirement{
			Module:            m.ontologyModulePath(ontologyModuleSlug(ontology.Prefix, ontology.Name, ontology.ID)),
			OntologyID:        versionRow.OntologyID,
			OntologyVersionID: versionRow.ID,
			Version:           versionRow.VersionString,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived ontology requirements: %w", err)
	}
	return reqs, nil
}

func (m *Materializer) writeCategoriesVersion(ctx context.Context, workDir, projectID, version string) error {
	categories, err := m.loadArchivedCategories(ctx, projectID, version)
	if err != nil {
		return err
	}
	for _, cat := range categories {
		payload, err := canonical.Category(cat)
		if err != nil {
			return fmt.Errorf("encode archived category %s: %w", cat.ID, err)
		}
		path := domain.FilePath(domain.PathSpec{EntityType: "category", EntityID: identifierFor(cat.SemanticID, cat.ID)})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeFieldsVersion(ctx context.Context, workDir, projectID, version string) error {
	fields, err := m.loadArchivedFields(ctx, projectID, version)
	if err != nil {
		return err
	}
	for _, field := range fields {
		payload, err := canonical.Field(field)
		if err != nil {
			return fmt.Errorf("encode archived field %s: %w", field.ID, err)
		}
		fieldKey := identifierFor(field.SemanticID, field.ID)
		path := domain.FilePath(domain.PathSpec{EntityType: "field", EntityID: fieldKey})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
		base, ok, err := m.loadArchivedBaseOverride(ctx, field.ID, projectID, version)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := m.writeArchivedOverride(ctx, workDir, base, "base_override", fieldKey, "", version); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeModelsVersion(ctx context.Context, workDir, projectID, version string) error {
	models, err := m.loadArchivedModels(ctx, projectID, version)
	if err != nil {
		return err
	}
	for _, mod := range models {
		payload, err := canonical.Model(mod)
		if err != nil {
			return fmt.Errorf("encode archived model %s: %w", mod.ID, err)
		}
		path := domain.FilePath(domain.PathSpec{EntityType: "model", EntityID: identifierFor(mod.SemanticID, mod.ID)})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeCollectionsVersion(ctx context.Context, workDir, projectID, version string) error {
	collections, err := m.loadArchivedCollections(ctx, projectID, version)
	if err != nil {
		return err
	}
	for _, col := range collections {
		payload, err := canonical.Collection(col)
		if err != nil {
			return fmt.Errorf("encode archived collection %s: %w", col.ID, err)
		}
		path := domain.FilePath(domain.PathSpec{EntityType: "collection", EntityID: identifierFor(col.SemanticID, col.ID)})
		if err := writeEntityFile(workDir, path, payload); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeScopedOverridesByProjectVersion(ctx context.Context, workDir, projectID, version, entityType string) error {
	overrides, err := m.loadArchivedOverridesByProjectAndType(ctx, projectID, version, entityType)
	if err != nil {
		return err
	}
	pathType := entityType + "_override"
	for _, o := range overrides {
		if err := m.writeArchivedOverride(ctx, workDir, o, pathType, o.FieldID, o.EntityID, version); err != nil {
			return err
		}
	}
	return nil
}

func (m *Materializer) writeArchivedOverride(ctx context.Context, workDir string, o *domain.FieldOverride, pathType, fieldKey, ownerKey, version string) error {
	refs, err := m.loadArchivedOverrideRefs(ctx, o.ID, version)
	if err != nil {
		return err
	}
	payload, err := canonical.Override(o, refs)
	if err != nil {
		return fmt.Errorf("encode archived override %d: %w", o.ID, err)
	}
	spec := domain.PathSpec{EntityType: pathType, EntityID: fmt.Sprintf("%d", o.ID), FieldID: fieldKey, OwnerID: ownerKey}
	switch pathType {
	case "model_override":
		spec.OwnerType = "model"
	case "collection_override":
		spec.OwnerType = "collection"
	}
	return writeEntityFile(workDir, domain.FilePath(spec), payload)
}

func (m *Materializer) writeAdoptionManifestsVersion(ctx context.Context, workDir, projectID, version string) error {
	adoptions, err := m.loadProjectAdoptionsAtVersion(ctx, projectID, version)
	if err != nil {
		return err
	}
	grouped := groupProjectAdoptions(adoptions)
	index := adoptionsIndexManifest{SchemaVersion: 1, Receipts: make([]adoptionsIndexReceiptEntry, 0, len(grouped))}
	for _, receipt := range grouped {
		file := adoptionReceiptFileName(receipt.EntityType, receipt.EntityID)
		index.Receipts = append(index.Receipts, adoptionsIndexReceiptEntry{EntityType: receipt.EntityType, EntityID: receipt.EntityID, File: file})
		payload, err := m.encodeAdoptionReceipt(ctx, receipt)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, "adoptions/"+file, payload); err != nil {
			return err
		}
	}
	indexPayload, err := encodeAdoptionsIndexManifest(index)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, "adoptions/index.yaml", indexPayload)
}

func (m *Materializer) writeForkManifestsVersion(ctx context.Context, workDir, projectID, version string) error {
	forks, err := m.loadProjectForksAtVersion(ctx, projectID, version)
	if err != nil {
		return err
	}
	index := forksIndexManifest{SchemaVersion: 1, Receipts: make([]forksIndexReceiptEntry, 0, len(forks))}
	for _, fork := range forks {
		file := forkReceiptFileName(fork.EntityType, fork.ForkEntityID)
		index.Receipts = append(index.Receipts, forksIndexReceiptEntry{EntityType: fork.EntityType, EntityID: fork.ForkEntityID, File: file})
		payload, err := m.encodeForkReceipt(ctx, fork)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, "forks/"+file, payload); err != nil {
			return err
		}
	}
	indexPayload, err := encodeForksIndexManifest(index)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, "forks/index.yaml", indexPayload)
}

func (m *Materializer) writeAdoptedEntitiesVersion(ctx context.Context, workDir, projectID, version string) error {
	adoptions, err := m.loadProjectAdoptionsAtVersion(ctx, projectID, version)
	if err != nil {
		return err
	}
	ids := adoptedIDsFromAdoptions(adoptions, projectID)

	for fieldID, ownerProjectID := range ids.fieldIDs {
		sourceVersion := sourceVersionForEntity(adoptions, "field", ownerProjectID, fieldID)
		field, err := m.loadAdoptedFieldAtSource(ctx, ownerProjectID, fieldID, sourceVersion)
		if err != nil {
			return err
		}
		payload, err := canonical.Field(field)
		if err != nil {
			return fmt.Errorf("encode archived adopted field %s: %w", fieldID, err)
		}
		annotated, err := addAdoptionMarker(payload, ownerProjectID)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, domain.FilePath(domain.PathSpec{EntityType: "field", EntityID: fieldID}), annotated); err != nil {
			return err
		}
		if err := m.writeAdoptedBaseOverrideAtSource(ctx, workDir, fieldID, ownerProjectID, sourceVersion); err != nil {
			return err
		}
	}

	for modelID, ownerProjectID := range ids.modelIDs {
		sourceVersion := sourceVersionForEntity(adoptions, "model", ownerProjectID, modelID)
		mod, err := m.loadAdoptedModelAtSource(ctx, ownerProjectID, modelID, sourceVersion)
		if err != nil {
			return err
		}
		payload, err := canonical.Model(mod)
		if err != nil {
			return fmt.Errorf("encode archived adopted model %s: %w", modelID, err)
		}
		annotated, err := addAdoptionMarker(payload, ownerProjectID)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, domain.FilePath(domain.PathSpec{EntityType: "model", EntityID: modelID}), annotated); err != nil {
			return err
		}
	}

	for collectionID, ownerProjectID := range ids.collectionIDs {
		sourceVersion := sourceVersionForEntity(adoptions, "collection", ownerProjectID, collectionID)
		col, err := m.loadAdoptedCollectionAtSource(ctx, ownerProjectID, collectionID, sourceVersion)
		if err != nil {
			return err
		}
		payload, err := canonical.Collection(col)
		if err != nil {
			return fmt.Errorf("encode archived adopted collection %s: %w", collectionID, err)
		}
		annotated, err := addAdoptionMarker(payload, ownerProjectID)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, domain.FilePath(domain.PathSpec{EntityType: "collection", EntityID: collectionID}), annotated); err != nil {
			return err
		}
	}

	return nil
}

func sourceVersionForEntity(adoptions []domain.Adoption, entityType, sourceProjectID, sourceEntityID string) string {
	for _, adoption := range adoptions {
		if adoption.EntityType == entityType && adoption.SourceProjectID == sourceProjectID && adoption.SourceEntityID == sourceEntityID {
			return strings.TrimSpace(adoption.SourceVersion)
		}
	}
	return ""
}

func unmarshalPathElement(raw []byte) domain.PathElement {
	if len(raw) == 0 {
		return domain.PathElement{}
	}
	var out domain.PathElement
	_ = json.Unmarshal(raw, &out)
	return out
}

func unmarshalPathElements(raw []byte) []domain.PathElement {
	if len(raw) == 0 {
		return nil
	}
	var out []domain.PathElement
	_ = json.Unmarshal(raw, &out)
	return out
}

func parseExamples(raw []byte) []domain.FieldExample {
	if len(raw) == 0 {
		return nil
	}
	var out []domain.FieldExample
	_ = json.Unmarshal(raw, &out)
	return out
}
