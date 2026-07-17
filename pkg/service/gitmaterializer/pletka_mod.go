package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

type pletkaModManifest struct {
	SchemaVersion int              `json:"schema_version"`
	Module        pletkaModModule  `json:"module"`
	Require       pletkaModRequire `json:"require"`
	Replace       []map[string]any `json:"replace"`
}

type pletkaModModule struct {
	Path      string `json:"path"`
	ProjectID string `json:"project_id"`
}

type pletkaModRequire struct {
	Projects   []pletkaProjectRequirement  `json:"projects"`
	Ontologies []pletkaOntologyRequirement `json:"ontologies"`
}

type pletkaProjectRequirement struct {
	Module    string `json:"module"`
	ProjectID string `json:"project_id"`
	Mode      string `json:"mode"`
	Version   string `json:"version,omitempty"`
}

const parentDependencyVersionDraft = "draft"

type pletkaOntologyRequirement struct {
	Module            string `json:"module"`
	OntologyID        string `json:"ontology_id"`
	OntologyVersionID string `json:"ontology_version_id,omitempty"`
	Version           string `json:"version"`
}

func encodePletkaModManifest(manifest pletkaModManifest) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal pletka.mod to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize pletka.mod json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode pletka.mod yaml: %w", err)
	}
	return payload, nil
}

func (m *Materializer) writePletkaMod(ctx context.Context, workDir, projectID string) error {
	manifest, err := m.loadPletkaModManifest(ctx, projectID)
	if err != nil {
		return err
	}
	payload, err := encodePletkaModManifest(manifest)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, "pletka.mod", payload)
}

func (m *Materializer) loadPletkaModManifest(ctx context.Context, projectID string) (pletkaModManifest, error) {
	projectRow, err := m.queries.WeaveGetProjectByID(ctx, projectID)
	if err != nil {
		return pletkaModManifest{}, fmt.Errorf("get project %s for pletka.mod: %w", projectID, err)
	}

	modulePath, err := m.projectModulePath(ctx, projectRow.OwnerID, projectID)
	if err != nil {
		return pletkaModManifest{}, err
	}
	projectReqs, err := m.loadPletkaProjectRequirements(ctx, projectID)
	if err != nil {
		return pletkaModManifest{}, err
	}
	ontologyReqs, err := m.loadPletkaOntologyRequirements(ctx, projectID)
	if err != nil {
		return pletkaModManifest{}, err
	}

	return pletkaModManifest{
		SchemaVersion: 1,
		Module: pletkaModModule{
			Path:      modulePath,
			ProjectID: projectID,
		},
		Require: pletkaModRequire{
			Projects:   projectReqs,
			Ontologies: ontologyReqs,
		},
		Replace: []map[string]any{},
	}, nil
}

func (m *Materializer) loadPletkaProjectRequirements(ctx context.Context, projectID string) ([]pletkaProjectRequirement, error) {
	parents, err := m.loadProjectManifestParents(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load parent requirements: %w", err)
	}
	reqs := make([]pletkaProjectRequirement, 0, len(parents))
	for _, parent := range parents {
		parentProject, err := m.queries.WeaveGetProjectByID(ctx, parent.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("get parent project %s for pletka.mod: %w", parent.ProjectID, err)
		}
		modulePath, err := m.projectModulePath(ctx, parentProject.OwnerID, parentProject.ID)
		if err != nil {
			return nil, err
		}
		version := parentDependencyVersionDraft
		if strings.TrimSpace(parent.SourceMode) == "release" && strings.TrimSpace(parent.SourceVersion) != "" {
			version = strings.TrimSpace(parent.SourceVersion)
		}
		reqs = append(reqs, pletkaProjectRequirement{
			Module:    modulePath,
			ProjectID: parent.ProjectID,
			Mode:      "parent",
			Version:   version,
		})
	}
	sort.Slice(reqs, func(i, j int) bool {
		if reqs[i].Mode != reqs[j].Mode {
			return reqs[i].Mode < reqs[j].Mode
		}
		if reqs[i].Module != reqs[j].Module {
			return reqs[i].Module < reqs[j].Module
		}
		return reqs[i].ProjectID < reqs[j].ProjectID
	})
	return reqs, nil
}

func (m *Materializer) loadPletkaOntologyRequirements(ctx context.Context, projectID string) ([]pletkaOntologyRequirement, error) {
	rows, err := m.queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project ontology requirements: %w", err)
	}
	reqs := make([]pletkaOntologyRequirement, 0, len(rows))
	for _, row := range rows {
		version, err := m.queries.WeaveGetOntologyVersionByID(ctx, row.OntologyVersionID)
		if err != nil {
			return nil, fmt.Errorf("get ontology version %s for pletka.mod: %w", row.OntologyVersionID, err)
		}
		ontology, err := m.queries.WeaveGetOntologyByID(ctx, version.OntologyID)
		if err != nil {
			return nil, fmt.Errorf("get ontology %s for pletka.mod: %w", version.OntologyID, err)
		}
		reqs = append(reqs, pletkaOntologyRequirement{
			Module:            m.ontologyModulePath(ontologyModuleSlug(ontology.Prefix, ontology.Name, ontology.ID)),
			OntologyID:        version.OntologyID,
			OntologyVersionID: version.ID,
			Version:           version.VersionString,
		})
	}
	sort.Slice(reqs, func(i, j int) bool {
		if reqs[i].Module != reqs[j].Module {
			return reqs[i].Module < reqs[j].Module
		}
		if reqs[i].Version != reqs[j].Version {
			return reqs[i].Version < reqs[j].Version
		}
		return reqs[i].OntologyVersionID < reqs[j].OntologyVersionID
	})
	return reqs, nil
}

func (m *Materializer) projectModulePath(ctx context.Context, ownerID, projectID string) (string, error) {
	ownerID = strings.TrimSpace(ownerID)
	projectID = strings.TrimSpace(projectID)
	if ownerID == "" || projectID == "" {
		return "", fmt.Errorf("build project module path: missing owner or project id")
	}
	actor, err := m.queries.WeaveGetActorByID(ctx, ownerID)
	if err != nil {
		return fmt.Sprintf("%s/actors/%s/projects/%s", m.moduleHost, ownerID, projectID), nil
	}
	slug := strings.TrimSpace(actor.Slug)
	if slug == "" {
		slug = actor.ID
	}
	switch actor.Type {
	case "organization":
		return fmt.Sprintf("%s/orgs/%s/projects/%s", m.moduleHost, slug, projectID), nil
	case "person":
		return fmt.Sprintf("%s/users/%s/projects/%s", m.moduleHost, slug, projectID), nil
	default:
		return fmt.Sprintf("%s/actors/%s/projects/%s", m.moduleHost, slug, projectID), nil
	}
}

// ontologyModulePath builds the module path for a vendored ontology, rooted
// at m.ontologyHost (defaultOntologyHost unless overridden via
// WithModuleHosts).
func (m *Materializer) ontologyModulePath(ontologySlug string) string {
	ontologySlug = strings.TrimSpace(ontologySlug)
	if ontologySlug == "" {
		return fmt.Sprintf("%s/unknown", m.ontologyHost)
	}
	return fmt.Sprintf("%s/%s", m.ontologyHost, ontologySlug)
}

func ontologyModuleSlug(prefix, name, fallback string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name != "" {
		replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-", ":", "-")
		name = replacer.Replace(name)
		name = strings.Trim(name, "-")
		if name != "" {
			return name
		}
	}
	prefix = strings.TrimSpace(prefix)
	if prefix != "" {
		return prefix
	}
	return strings.TrimSpace(fallback)
}

func toProjectRowProject(row domain.Project) domain.Project {
	return row
}
