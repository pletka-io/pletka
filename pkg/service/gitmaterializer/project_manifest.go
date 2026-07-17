package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

type projectManifest struct {
	SchemaVersion int                         `json:"schema_version"`
	Project       projectManifestProject      `json:"project"`
	Inheritance   *projectManifestInheritance `json:"inheritance,omitempty"`
	Ontologies    *projectManifestOntologies  `json:"ontologies,omitempty"`
	Namespaces    *projectManifestNamespaces  `json:"namespaces,omitempty"`
	Manifests     *projectManifestManifests   `json:"manifests,omitempty"`
}

type projectManifestProject struct {
	ID          string              `json:"id"`
	Title       domain.Translations `json:"title,omitempty"`
	Description domain.Translations `json:"description,omitempty"`
	Readme      domain.Translations `json:"readme,omitempty"`
	Namespace   string              `json:"namespace,omitempty"`
	Visibility  string              `json:"visibility,omitempty"`
	Owner       *manifestActor      `json:"owner,omitempty"`
	CreatedBy   *manifestActor      `json:"created_by,omitempty"`
	License     string              `json:"license,omitempty"`
	BaseURL     string              `json:"base_url,omitempty"`
	Topics      []string            `json:"topics,omitempty"`
}

type projectManifestInheritance struct {
	Parents []projectManifestParent `json:"parents,omitempty"`
}

type projectManifestParent struct {
	ProjectID      string `json:"project_id"`
	IsPrimary      bool   `json:"is_primary,omitempty"`
	CanonicalOrder int    `json:"canonical_order"`
	SourceMode     string `json:"source_mode,omitempty"`
	SourceVersion  string `json:"source_version,omitempty"`
}

type projectManifestOntologies struct {
	LinkedVersions []projectManifestLinkedOntology `json:"linked_versions,omitempty"`
}

type projectManifestLinkedOntology struct {
	OntologyID        string `json:"ontology_id"`
	OntologyVersionID string `json:"ontology_version_id"`
	Version           string `json:"version"`
	IsPrimary         bool   `json:"is_primary,omitempty"`
}

type projectManifestNamespaces struct {
	EffectiveBindings []projectManifestNamespaceBinding `json:"effective_bindings,omitempty"`
}

type projectManifestManifests struct {
	Adoptions string `json:"adoptions,omitempty"`
	Forks     string `json:"forks,omitempty"`
}

type projectManifestNamespaceBinding struct {
	Prefix    string `json:"prefix"`
	Namespace string `json:"namespace"`
	Source    string `json:"source,omitempty"`
}

func encodeProjectManifest(manifest projectManifest) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal project manifest to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize project manifest json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode project manifest yaml: %w", err)
	}
	return payload, nil
}

func (m *Materializer) writeProjectManifest(ctx context.Context, workDir, projectID string) error {
	manifest, err := m.loadProjectManifest(ctx, projectID)
	if err != nil {
		return err
	}
	payload, err := encodeProjectManifest(manifest)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, domain.FilePath(domain.PathSpec{EntityType: "project"}), payload)
}

func (m *Materializer) loadProjectManifest(ctx context.Context, projectID string) (projectManifest, error) {
	row, err := m.queries.WeaveGetProjectByID(ctx, projectID)
	if err != nil {
		return projectManifest{}, fmt.Errorf("get project %s: %w", projectID, err)
	}

	manifest := projectManifest{
		SchemaVersion: 1,
		Project: projectManifestProject{
			ID:          row.ID,
			Title:       unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Readme:      unmarshalTranslations(row.Readme),
			Namespace:   derefStr(row.Namespace),
			Visibility:  row.Visibility,
			License:     row.License,
			BaseURL:     row.BaseUrl,
			Topics:      append([]string(nil), row.Topics...),
		},
		Manifests: &projectManifestManifests{
			Adoptions: "adoptions/index.yaml",
			Forks:     "forks/index.yaml",
		},
	}
	project := domain.Project{
		Entity: domain.Entity{
			ID: row.ID,
		},
		Namespace: derefStr(row.Namespace),
	}

	if owner := m.loadManifestActor(ctx, &row.OwnerID); owner != nil {
		manifest.Project.Owner = owner
	}
	manifest.Project.CreatedBy = m.loadManifestActor(ctx, row.CreatedByID)

	if parents, err := m.loadProjectManifestParents(ctx, projectID); err != nil {
		return projectManifest{}, err
	} else if len(parents) > 0 {
		manifest.Inheritance = &projectManifestInheritance{Parents: parents}
	}

	if linked, err := m.loadProjectManifestOntologies(ctx, projectID); err != nil {
		return projectManifest{}, err
	} else if len(linked) > 0 {
		manifest.Ontologies = &projectManifestOntologies{LinkedVersions: linked}
	}

	if bindings, err := m.loadProjectManifestNamespaces(ctx, project); err != nil {
		return projectManifest{}, err
	} else if len(bindings) > 0 {
		manifest.Namespaces = &projectManifestNamespaces{EffectiveBindings: bindings}
	}

	return manifest, nil
}

func (m *Materializer) loadProjectManifestParents(ctx context.Context, projectID string) ([]projectManifestParent, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT project_id, parent_project_id, is_primary, canonical_order, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project inheritances for manifest: %w", err)
	}
	defer rows.Close()

	parents := make([]projectManifestParent, 0)
	for rows.Next() {
		var linkProjectID string
		var parent projectManifestParent
		if err := rows.Scan(&linkProjectID, &parent.ProjectID, &parent.IsPrimary, &parent.CanonicalOrder, &parent.SourceMode, &parent.SourceVersion); err != nil {
			return nil, fmt.Errorf("scan project inheritance for manifest: %w", err)
		}
		parent.SourceMode = strings.TrimSpace(parent.SourceMode)
		parent.SourceVersion = strings.TrimSpace(parent.SourceVersion)
		parents = append(parents, parent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project inheritances for manifest: %w", err)
	}
	return parents, nil
}

func (m *Materializer) loadProjectManifestOntologies(ctx context.Context, projectID string) ([]projectManifestLinkedOntology, error) {
	rows, err := m.queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project ontology versions for manifest: %w", err)
	}

	linked := make([]projectManifestLinkedOntology, 0, len(rows))
	for _, row := range rows {
		version, err := m.queries.WeaveGetOntologyVersionByID(ctx, row.OntologyVersionID)
		if err != nil {
			return nil, fmt.Errorf("get ontology version %s for manifest: %w", row.OntologyVersionID, err)
		}
		linked = append(linked, projectManifestLinkedOntology{
			OntologyID:        version.OntologyID,
			OntologyVersionID: version.ID,
			Version:           version.VersionString,
			IsPrimary:         derefBool(row.IsPrimary),
		})
	}
	sort.Slice(linked, func(i, j int) bool {
		if linked[i].IsPrimary != linked[j].IsPrimary {
			return linked[i].IsPrimary
		}
		if linked[i].OntologyID != linked[j].OntologyID {
			return linked[i].OntologyID < linked[j].OntologyID
		}
		if linked[i].Version != linked[j].Version {
			return linked[i].Version < linked[j].Version
		}
		return linked[i].OntologyVersionID < linked[j].OntologyVersionID
	})
	return linked, nil
}

func (m *Materializer) loadProjectManifestNamespaces(ctx context.Context, project domain.Project) ([]projectManifestNamespaceBinding, error) {
	projectID := project.ID
	rows, err := m.queries.WeaveListNamespaceBindings(ctx, &projectID)
	if err != nil {
		return nil, fmt.Errorf("list namespace bindings for manifest: %w", err)
	}
	visible := make([]*domain.NamespaceBinding, 0, len(rows))
	for _, row := range rows {
		visible = append(visible, &domain.NamespaceBinding{
			Prefix:    row.Prefix,
			Namespace: row.Namespace,
			Weight:    row.Weight,
			Source:    manifestNamespaceSource(row, projectID),
		})
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
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Prefix != bindings[j].Prefix {
			return bindings[i].Prefix < bindings[j].Prefix
		}
		if bindings[i].Namespace != bindings[j].Namespace {
			return bindings[i].Namespace < bindings[j].Namespace
		}
		return bindings[i].Source < bindings[j].Source
	})
	return bindings, nil
}

func manifestNamespaceSource(row sqlcgen.WeaveNamespaceBinding, projectID string) string {
	switch {
	case row.ProjectID != nil && strings.TrimSpace(*row.ProjectID) == projectID:
		return "project"
	case row.OntologyID != nil && strings.TrimSpace(*row.OntologyID) != "":
		return "ontology"
	case row.Source == "system":
		return "system"
	default:
		return "global"
	}
}
