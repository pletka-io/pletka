package gitmaterializer

import (
	"context"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/database/projectrestore"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

const completeSourceRelease = "release"

// CompleteRestore keeps format3 out of the lossy legacy DTO/phases. Both values
// were validated before a plan exists; neither grants authorization by itself.
type CompleteRestore struct {
	Project projectbundle.ImportedProject
	Catalog projectbundle.Catalog
}

func projectFileSchema(rootDir string) (int, error) {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	raw, err := readBundleFile(root, projectbundle.ManifestPath, projectbundle.DefaultLimits().MaxBytes)
	if err != nil {
		return 0, err
	}
	var header struct {
		SchemaVersion int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(raw, &header); err != nil {
		return 0, err
	}
	return header.SchemaVersion, nil
}

func prepareCompleteRestore(rootDir, target string) (*RestorePlan, error) {
	bundle, err := LoadCompleteProjectBundle(rootDir, projectbundle.DefaultLimits())
	if err != nil {
		return nil, err
	}
	value := bundle.Project.Snapshot()
	id, err := value.Root()
	if err != nil {
		return nil, err
	}
	if target != "" && target != id.ProjectID() {
		return nil, fmt.Errorf("complete history retains original project identities; retargeting is unsupported")
	}
	catalog, err := projectbundle.DecodeCatalog(value, bundle.Catalog, projectbundle.DefaultLimits().MaxBytes)
	if err != nil {
		return nil, err
	}
	evidence, err := value.Evidence(id)
	if err != nil {
		return nil, err
	}
	plan := &RestorePlan{RootDir: rootDir, ProjectID: id.ProjectID(), ModulePath: evidence.Descriptors().Module(), Complete: &CompleteRestore{Project: bundle.Project, Catalog: catalog}}
	ids, err := value.Projects()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		plan.Operations = append(plan.Operations, RestoreOperation{Kind: RestoreCompleteSnapshot, ProjectID: id.ProjectID(), Version: id.Version()})
	}
	return plan, nil
}

// HydrateCompleteRestore runs one atomic history/working-copy import. It owns
// root-first locking; callers must not acquire a second root lock around it.
func (m *Materializer) HydrateCompleteRestore(ctx context.Context, plan *RestorePlan) error {
	if plan == nil || plan.Complete == nil {
		return fmt.Errorf("missing complete restore plan")
	}
	id, err := plan.Complete.Project.Snapshot().Root()
	if err != nil {
		return err
	}
	if plan.ProjectID != id.ProjectID() {
		return fmt.Errorf("restore plan identity changed")
	}
	result, err := projectrestore.Restore(ctx, m.pool, plan.Complete.Project, plan.Complete.Catalog)
	plan.ImportResult = &result
	if result.Committed && result.DetachedStagingRefs > 0 {
		m.logger.Warn("restore retained staging IDs in history and cleared unavailable working-copy attachments", "project_id", plan.ProjectID, "count", result.DetachedStagingRefs)
	}
	return err
}

func completeRestorePreview(plan *RestorePlan) (*RestorePreview, error) {
	value := plan.Complete.Project.Snapshot()
	root, err := value.Root()
	if err != nil {
		return nil, err
	}
	in, err := value.Input(root)
	if err != nil {
		return nil, err
	}
	ids, err := value.Projects()
	if err != nil {
		return nil, err
	}
	c := in.Content
	preview := &RestorePreview{SnapshotPath: plan.RootDir, ProjectID: plan.ProjectID, ModulePath: plan.ModulePath, SelfContained: true, PreservesHistory: true,
		Counts: RestorePreviewCounts{Categories: len(c.Categories), Fields: len(c.Fields), Models: len(c.Models), Collections: len(c.Collections), AdoptionReceipts: len(c.Adoptions), ForkReceipts: len(c.Forks), VendoredProjects: len(ids) - 1, Operations: len(plan.Operations), Examples: len(c.Examples), Vocabularies: len(c.Vocabularies), ConceptLists: len(c.ConceptLists), Placements: len(c.Placements), Credits: len(c.ProjectActors) + len(c.Attributions)}, OperationCounts: buildOperationStats(plan.Operations)}
	for _, row := range c.Overrides {
		switch row.EntityType {
		case entityTypeModel:
			preview.Counts.ModelOverrides++
		case entityTypeCollection:
			preview.Counts.CollectionOverrides++
		default:
			preview.Counts.BaseOverrides++
		}
	}
	for _, dep := range in.Dependencies {
		e, err := value.Evidence(dep)
		if err != nil {
			return nil, err
		}
		preview.ParentDependencies = append(preview.ParentDependencies, RestorePreviewParent{ProjectID: dep.ProjectID(), SourceMode: completeSourceRelease, SourceVersion: dep.Version(), Module: e.Descriptors().Module()})
	}
	preview.OntologyDependencies, err = completePreviewOntologies(value, ids)
	if err != nil {
		return nil, err
	}
	preview.Counts.VendoredOntologies = len(preview.OntologyDependencies)
	return preview, nil
}

func completePreviewOntologies(value projectsnapshot.CompleteProject, ids []projectsnapshot.Identity) ([]RestorePreviewOntology, error) {
	versions := map[string]RestorePreviewOntology{}
	for _, id := range ids {
		e, err := value.Evidence(id)
		if err != nil {
			return nil, err
		}
		modules := e.Descriptors().OntologyModules()
		if e.Contract() == 2 {
			a, err := e.Legacy()
			if err != nil {
				return nil, err
			}
			doc, err := a.Decode()
			if err != nil {
				return nil, err
			}
			for _, body := range doc.Inventory {
				versions[body.VersionID] = RestorePreviewOntology{OntologyID: body.OntologyID, OntologyVersionID: body.VersionID, Module: modules[body.VersionID]}
			}
		} else {
			publications, err := e.Publications()
			if err != nil {
				return nil, err
			}
			for _, p := range publications {
				doc, err := p.Artifact.Decode()
				if err != nil {
					return nil, err
				}
				versions[doc.VersionID] = RestorePreviewOntology{OntologyID: doc.OntologyID, OntologyVersionID: doc.VersionID, Module: modules[doc.VersionID]}
			}
		}
	}
	out := make([]RestorePreviewOntology, 0, len(versions))
	for _, row := range versions {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OntologyVersionID < out[j].OntologyVersionID })
	return out, nil
}
