package gitmaterializer

import "sort"

type RestoreOperationKind string

const (
	RestoreVendoredProject    RestoreOperationKind = "vendored_project"
	RestoreVendoredOntology   RestoreOperationKind = "vendored_ontology"
	RestoreProjectManifest    RestoreOperationKind = "project_manifest"
	RestoreCategory           RestoreOperationKind = "category"
	RestoreField              RestoreOperationKind = "field"
	RestoreModel              RestoreOperationKind = "model"
	RestoreCollection         RestoreOperationKind = "collection"
	RestoreBaseOverride       RestoreOperationKind = "base_override"
	RestoreModelOverride      RestoreOperationKind = "model_override"
	RestoreCollectionOverride RestoreOperationKind = "collection_override"
	RestoreAdoptionReceipt    RestoreOperationKind = "adoption_receipt"
	RestoreForkReceipt        RestoreOperationKind = "fork_receipt"
)

type RestoreOperation struct {
	Kind       RestoreOperationKind
	Path       string
	ProjectID  string
	EntityType string
	EntityID   string
	FieldID    string
	OwnerType  string
	OwnerID    string
	Module     string
	Version    string
	Payload    []byte
}

type RestorePlan struct {
	RootDir    string
	ProjectID  string
	ModulePath string
	Snapshot   *ProjectSnapshot
	Operations []RestoreOperation
}

func BuildRestorePlan(snapshot *ProjectSnapshot) *RestorePlan {
	if snapshot == nil {
		return &RestorePlan{}
	}

	plan := &RestorePlan{
		RootDir:   snapshot.RootDir,
		ProjectID: snapshot.Manifest.Project.ID,
		Snapshot:  snapshot,
	}
	if snapshot.Mod != nil {
		plan.ModulePath = snapshot.Mod.Module.Path
	}

	appendVendorOperations(plan, snapshot.Vendor)
	appendProjectOperations(plan, snapshot)
	return plan
}

func appendVendorOperations(plan *RestorePlan, vendor VendorSnapshotSet) {
	projectDeps := append([]VendoredProjectSnapshot(nil), vendor.Projects...)
	sort.Slice(projectDeps, func(i, j int) bool {
		return projectDeps[i].Module < projectDeps[j].Module
	})
	for _, dep := range projectDeps {
		plan.Operations = append(plan.Operations, RestoreOperation{
			Kind:      RestoreVendoredProject,
			Path:      dep.Snapshot.RootDir,
			ProjectID: dep.Snapshot.Manifest.Project.ID,
			Module:    dep.Module,
		})
		appendProjectOperations(plan, dep.Snapshot)
	}

	ontologyDeps := append([]VendoredOntologySnapshot(nil), vendor.Ontologies...)
	sort.Slice(ontologyDeps, func(i, j int) bool {
		if ontologyDeps[i].Module != ontologyDeps[j].Module {
			return ontologyDeps[i].Module < ontologyDeps[j].Module
		}
		return ontologyDeps[i].Version < ontologyDeps[j].Version
	})
	for _, dep := range ontologyDeps {
		plan.Operations = append(plan.Operations, RestoreOperation{
			Kind:    RestoreVendoredOntology,
			Path:    dep.Snapshot.RootDir,
			Module:  dep.Module,
			Version: dep.Version,
		})
	}
}

func appendProjectOperations(plan *RestorePlan, snapshot *ProjectSnapshot) {
	if snapshot == nil {
		return
	}
	projectID := snapshot.Manifest.Project.ID

	plan.Operations = append(plan.Operations, RestoreOperation{
		Kind:      RestoreProjectManifest,
		Path:      "project.yaml",
		ProjectID: projectID,
	})

	for _, item := range snapshot.Entities.Categories {
		plan.Operations = append(plan.Operations, restoreOpForEntity(projectID, RestoreCategory, item))
	}
	for _, item := range snapshot.Entities.Fields {
		plan.Operations = append(plan.Operations, restoreOpForEntity(projectID, RestoreField, item))
	}
	for _, item := range snapshot.Entities.Models {
		plan.Operations = append(plan.Operations, restoreOpForEntity(projectID, RestoreModel, item))
	}
	for _, item := range snapshot.Entities.Collections {
		plan.Operations = append(plan.Operations, restoreOpForEntity(projectID, RestoreCollection, item))
	}
	for _, item := range snapshot.Entities.BaseOverrides {
		plan.Operations = append(plan.Operations, restoreOpForEntity(projectID, RestoreBaseOverride, item))
	}
	for _, item := range snapshot.Entities.ModelOverrides {
		plan.Operations = append(plan.Operations, restoreOpForEntity(projectID, RestoreModelOverride, item))
	}
	for _, item := range snapshot.Entities.CollectionOverrides {
		plan.Operations = append(plan.Operations, restoreOpForEntity(projectID, RestoreCollectionOverride, item))
	}

	if snapshot.Adoptions != nil {
		receipts := make([]AdoptionsIndexReceiptEntry, 0, len(snapshot.Adoptions.Index.Receipts))
		receipts = append(receipts, snapshot.Adoptions.Index.Receipts...)
		sort.Slice(receipts, func(i, j int) bool {
			if receipts[i].EntityType != receipts[j].EntityType {
				return receipts[i].EntityType < receipts[j].EntityType
			}
			return receipts[i].EntityID < receipts[j].EntityID
		})
		for _, entry := range receipts {
			plan.Operations = append(plan.Operations, RestoreOperation{
				Kind:       RestoreAdoptionReceipt,
				Path:       "adoptions/" + entry.File,
				ProjectID:  projectID,
				EntityType: entry.EntityType,
				EntityID:   entry.EntityID,
			})
		}
	}

	if snapshot.Forks != nil {
		receipts := make([]ForksIndexReceiptEntry, 0, len(snapshot.Forks.Index.Receipts))
		receipts = append(receipts, snapshot.Forks.Index.Receipts...)
		sort.Slice(receipts, func(i, j int) bool {
			if receipts[i].EntityType != receipts[j].EntityType {
				return receipts[i].EntityType < receipts[j].EntityType
			}
			return receipts[i].EntityID < receipts[j].EntityID
		})
		for _, entry := range receipts {
			plan.Operations = append(plan.Operations, RestoreOperation{
				Kind:       RestoreForkReceipt,
				Path:       "forks/" + entry.File,
				ProjectID:  projectID,
				EntityType: entry.EntityType,
				EntityID:   entry.EntityID,
			})
		}
	}
}

func restoreOpForEntity(projectID string, kind RestoreOperationKind, item SnapshotEntityFile) RestoreOperation {
	return RestoreOperation{
		Kind:       kind,
		Path:       item.Path,
		ProjectID:  projectID,
		EntityType: item.EntityType,
		EntityID:   item.EntityID,
		FieldID:    item.FieldID,
		OwnerType:  item.OwnerType,
		OwnerID:    item.OwnerID,
		Payload:    item.Payload,
	}
}
