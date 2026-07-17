package gitmaterializer

import "sort"

type RestorePreview struct {
	SnapshotPath         string                        `json:"snapshot_path"`
	ProjectID            string                        `json:"project_id"`
	ModulePath           string                        `json:"module_path,omitempty"`
	HasLockfile          bool                          `json:"has_lockfile"`
	SelfContained        bool                          `json:"self_contained"`
	ParentDependencies   []RestorePreviewParent        `json:"parent_dependencies,omitempty"`
	OntologyDependencies []RestorePreviewOntology      `json:"ontology_dependencies,omitempty"`
	Counts               RestorePreviewCounts          `json:"counts"`
	OperationCounts      []RestorePreviewOperationStat `json:"operation_counts,omitempty"`
}

type RestorePreviewParent struct {
	ProjectID      string `json:"project_id"`
	IsPrimary      bool   `json:"is_primary"`
	CanonicalOrder int    `json:"canonical_order"`
	SourceMode     string `json:"source_mode"`
	SourceVersion  string `json:"source_version,omitempty"`
	Module         string `json:"module,omitempty"`
}

type RestorePreviewOntology struct {
	OntologyID        string `json:"ontology_id"`
	OntologyVersionID string `json:"ontology_version_id,omitempty"`
	Version           string `json:"version,omitempty"`
	Module            string `json:"module,omitempty"`
}

type RestorePreviewCounts struct {
	Categories          int `json:"categories"`
	Fields              int `json:"fields"`
	Models              int `json:"models"`
	Collections         int `json:"collections"`
	BaseOverrides       int `json:"base_overrides"`
	ModelOverrides      int `json:"model_overrides"`
	CollectionOverrides int `json:"collection_overrides"`
	AdoptionReceipts    int `json:"adoption_receipts"`
	ForkReceipts        int `json:"fork_receipts"`
	VendoredProjects    int `json:"vendored_projects"`
	VendoredOntologies  int `json:"vendored_ontologies"`
	Operations          int `json:"operations"`
}

type RestorePreviewOperationStat struct {
	Kind  RestoreOperationKind `json:"kind"`
	Count int                  `json:"count"`
}

func PreviewRestoreSnapshot(rootDir string) (*RestorePreview, error) {
	snapshot, err := LoadProjectSnapshot(rootDir)
	if err != nil {
		return nil, err
	}
	return BuildRestorePreview(snapshot, BuildRestorePlan(snapshot)), nil
}

func BuildRestorePreview(snapshot *ProjectSnapshot, plan *RestorePlan) *RestorePreview {
	if snapshot == nil {
		return &RestorePreview{}
	}
	if plan == nil {
		plan = BuildRestorePlan(snapshot)
	}

	preview := &RestorePreview{
		SnapshotPath:  snapshot.RootDir,
		ProjectID:     snapshot.Manifest.Project.ID,
		HasLockfile:   len(snapshot.Sum) > 0,
		SelfContained: len(snapshot.Vendor.Projects) > 0 || len(snapshot.Vendor.Ontologies) > 0,
		Counts: RestorePreviewCounts{
			Categories:          len(snapshot.Entities.Categories),
			Fields:              len(snapshot.Entities.Fields),
			Models:              len(snapshot.Entities.Models),
			Collections:         len(snapshot.Entities.Collections),
			BaseOverrides:       len(snapshot.Entities.BaseOverrides),
			ModelOverrides:      len(snapshot.Entities.ModelOverrides),
			CollectionOverrides: len(snapshot.Entities.CollectionOverrides),
			VendoredProjects:    len(snapshot.Vendor.Projects),
			VendoredOntologies:  len(snapshot.Vendor.Ontologies),
		},
	}
	if snapshot.Mod != nil {
		preview.ModulePath = snapshot.Mod.Module.Path
		preview.ParentDependencies = buildPreviewParents(snapshot.Manifest, snapshot.Mod)
		preview.OntologyDependencies = buildPreviewOntologies(snapshot.Mod)
	}
	if snapshot.Adoptions != nil {
		preview.Counts.AdoptionReceipts = len(snapshot.Adoptions.Index.Receipts)
	}
	if snapshot.Forks != nil {
		preview.Counts.ForkReceipts = len(snapshot.Forks.Index.Receipts)
	}
	if plan != nil {
		preview.Counts.Operations = len(plan.Operations)
		preview.OperationCounts = buildOperationStats(plan.Operations)
	}
	return preview
}

func buildPreviewParents(manifest ProjectManifestFile, mod *PletkaModFile) []RestorePreviewParent {
	if mod == nil || manifest.Inheritance == nil || len(manifest.Inheritance.Parents) == 0 {
		return nil
	}
	modByProjectID := make(map[string]pletkaProjectRequirement, len(mod.Require.Projects))
	for _, req := range mod.Require.Projects {
		modByProjectID[req.ProjectID] = req
	}
	parents := make([]RestorePreviewParent, 0, len(manifest.Inheritance.Parents))
	for _, parent := range manifest.Inheritance.Parents {
		item := RestorePreviewParent{
			ProjectID:      parent.ProjectID,
			IsPrimary:      parent.IsPrimary,
			CanonicalOrder: parent.CanonicalOrder,
			SourceMode:     parent.SourceMode,
			SourceVersion:  parent.SourceVersion,
		}
		if req, ok := modByProjectID[parent.ProjectID]; ok {
			item.Module = req.Module
		}
		parents = append(parents, item)
	}
	sort.Slice(parents, func(i, j int) bool {
		if parents[i].CanonicalOrder != parents[j].CanonicalOrder {
			return parents[i].CanonicalOrder < parents[j].CanonicalOrder
		}
		return parents[i].ProjectID < parents[j].ProjectID
	})
	return parents
}

func buildPreviewOntologies(mod *PletkaModFile) []RestorePreviewOntology {
	if mod == nil || len(mod.Require.Ontologies) == 0 {
		return nil
	}
	out := make([]RestorePreviewOntology, 0, len(mod.Require.Ontologies))
	for _, req := range mod.Require.Ontologies {
		out = append(out, RestorePreviewOntology{
			OntologyID:        req.OntologyID,
			OntologyVersionID: req.OntologyVersionID,
			Version:           req.Version,
			Module:            req.Module,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OntologyID != out[j].OntologyID {
			return out[i].OntologyID < out[j].OntologyID
		}
		return out[i].Version < out[j].Version
	})
	return out
}

func buildOperationStats(ops []RestoreOperation) []RestorePreviewOperationStat {
	if len(ops) == 0 {
		return nil
	}
	counts := make(map[RestoreOperationKind]int)
	for _, op := range ops {
		counts[op.Kind]++
	}
	stats := make([]RestorePreviewOperationStat, 0, len(counts))
	for kind, count := range counts {
		stats = append(stats, RestorePreviewOperationStat{Kind: kind, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Kind < stats[j].Kind
	})
	return stats
}
