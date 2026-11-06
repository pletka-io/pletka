package gitmaterializer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/domain"
)

// writerOnlyKeys are keys the materializer writes that the restore decoders
// do not read (derived values); they are not unknown keys.
var writerOnlyKeys = map[string]bool{"ontology_path": true}

// loadParents loads each --parent tree keyed by project id, counts vendored
// parent projects, and reports parents that are not declared or not given.
func (v *verifyContext) loadParents(dirs []string) {
	if v.snap.Manifest.Inheritance != nil {
		for _, p := range v.snap.Manifest.Inheritance.Parents {
			v.declared[p.ProjectID] = true
		}
	}
	for _, vp := range v.snap.Vendor.Projects {
		if vp.Snapshot != nil {
			v.parents[vp.Snapshot.Manifest.Project.ID] = vp.Snapshot
		}
	}
	inheritanceLine := yamlKeyLine(v.readRel("project.yaml"), "inheritance")
	for _, dir := range dirs {
		ps, err := loadProjectSnapshot(dir, false)
		if err != nil {
			v.add("project.yaml", inheritanceLine, "parent-tree-missing", "--parent %s: %v", dir, err)
			continue
		}
		id := ps.Manifest.Project.ID
		if !v.declared[id] {
			v.add("project.yaml", inheritanceLine, "parent-not-declared", "--parent %s is project %s, which inheritance.parents does not name", dir, id)
			continue
		}
		v.parents[id] = ps
	}
	for id := range v.declared {
		if v.parents[id] == nil {
			v.add("project.yaml", inheritanceLine, "parent-tree-missing", "parent %s: no tree given with --parent and none vendored; references into it are not checked", id)
		}
	}
}

// checkVendorChecksums verifies pletka.sum against vendor/ when it exists.
func (v *verifyContext) checkVendorChecksums() {
	if _, err := os.Stat(filepath.Join(v.root, "vendor")); err != nil {
		return
	}
	if len(v.snap.Sum) == 0 {
		v.add("pletka.sum", 0, "vendor-checksum", "vendor/ exists but pletka.sum is missing")
		return
	}
	if err := verifyVendorChecksums(v.snap); err != nil {
		v.add("pletka.sum", 0, "vendor-checksum", "%v", err)
	}
}

// checkUnknownFiles reports files under the entity directories that the
// loader would silently ignore.
func (v *verifyContext) checkUnknownFiles() {
	for _, dir := range []string{"categories", "concept-lists", "fields", "models", "collections"} {
		base := filepath.Join(v.root, dir)
		_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // a missing entity dir is fine
			}
			rel, _ := filepath.Rel(v.root, p)
			if _, ok := classifySnapshotEntityFile(filepath.ToSlash(rel)); !ok {
				v.add(rel, 0, "unknown-file", "not a tree file; the loader ignores it")
			}
			return nil
		})
	}
}

// snapshotIDs collects the ids a tree can reference: its own entities,
// adopted ones (markers and receipts), and those of its vendored parents.
func snapshotIDs(ps *ProjectSnapshot, fields, categories, collections, conceptLists map[string]bool) {
	for _, f := range ps.Entities.Fields {
		fields[f.EntityID] = true
	}
	receiptFields := map[string]bool{}
	if ps.Adoptions != nil {
		for _, r := range ps.Adoptions.Index.Receipts {
			if r.EntityType == entityTypeField {
				receiptFields[r.EntityID] = true
			}
		}
	}
	for _, o := range ps.Entities.BaseOverrides {
		// an adopted field is only usable when a receipt backs the marker
		if hasAdoptionMarker(o.Payload) && receiptFields[o.FieldID] {
			fields[o.FieldID] = true
		}
	}
	for _, c := range ps.Entities.Categories {
		categories[c.EntityID] = true
	}
	for _, c := range ps.Entities.Collections {
		collections[c.EntityID] = true
	}
	for _, c := range ps.Entities.ConceptLists {
		conceptLists[c.EntityID] = true
	}
	if ps.Adoptions != nil {
		for _, r := range ps.Adoptions.Index.Receipts {
			switch r.EntityType {
			case entityTypeField:
				fields[r.EntityID] = true
			case entityTypeCategory:
				categories[r.EntityID] = true
			case entityTypeCollection:
				collections[r.EntityID] = true
			case entityTypeConceptList:
				conceptLists[r.EntityID] = true
			}
		}
	}
	for _, vp := range ps.Vendor.Projects {
		if vp.Snapshot != nil {
			snapshotIDs(vp.Snapshot, fields, categories, collections, conceptLists)
		}
	}
}

func (v *verifyContext) collectIdentities() {
	if v.snap.Adoptions != nil {
		for _, r := range v.snap.Adoptions.Index.Receipts {
			if r.EntityType == entityTypeField {
				v.receipts[r.EntityID] = true
			}
		}
	}
	snapshotIDs(v.snap, v.fields, v.categories, v.collections, v.conceptLists)
	for _, ps := range v.parents {
		snapshotIDs(ps, v.fields, v.categories, v.collections, v.conceptLists)
	}
}

// decodeDoc checks the file's top-level keys against the doc struct's tags,
// decodes it, and reports what fails. ok is false when decoding failed.
func decodeDoc[T any](v *verifyContext, item SnapshotEntityFile) (doc T, ok bool) {
	keys, err := topLevelKeys(item.Payload)
	if err != nil {
		v.add(item.Path, 0, "decode", "%v", err)
		return doc, false
	}
	known := jsonTagNames(doc)
	for _, k := range keys {
		if !known[k] && !writerOnlyKeys[k] {
			v.add(item.Path, yamlKeyLine(item.Payload, k), "unknown-key", "%q is not a key of this file; the loader drops it silently", k)
		}
	}
	doc, err = decodeCanonicalYAML[T](item.Payload)
	if err != nil {
		v.add(item.Path, 0, "decode", "%v", err)
		return doc, false
	}
	return doc, true
}

// checkSemanticID reports an id that is malformed, belongs to another
// project, or disagrees with the file's name.
func (v *verifyContext) checkSemanticID(item SnapshotEntityFile, adoptedFrom, id string) {
	if adoptedFrom != "" {
		return
	}
	line := yamlKeyLine(item.Payload, "semantic_id")
	if id == "" {
		v.add(item.Path, line, "semantic-id", "semantic_id is required")
		return
	}
	parsed := domain.ParseSemanticID(id)
	project := v.snap.Manifest.Project.ID
	switch {
	case !parsed.Valid():
		v.add(item.Path, line, "semantic-id", "%q is not a well-formed semantic id", id)
	case parsed.ProjectID != project:
		v.add(item.Path, line, "semantic-id", "%q belongs to project %s, this tree is %s", id, parsed.ProjectID, project)
	case id != item.EntityID:
		v.add(item.Path, line, "semantic-id", "%q does not match the file name %s", id, item.EntityID)
	}
}

func (v *verifyContext) checkEntities() {
	for _, item := range v.snap.Entities.Categories {
		if doc, ok := decodeDoc[canonicalCategoryDoc](v, item); ok {
			v.checkSemanticID(item, doc.AdoptedFrom, doc.SemanticID)
		}
	}
	for _, item := range v.snap.Entities.ConceptLists {
		if doc, ok := decodeDoc[canonicalConceptListDoc](v, item); ok {
			v.checkSemanticID(item, "", doc.SemanticID)
		}
	}
	for _, item := range v.snap.Entities.Fields {
		if doc, ok := decodeDoc[canonicalFieldDoc](v, item); ok {
			v.checkSemanticID(item, doc.AdoptedFrom, doc.SemanticID)
		}
	}
	for _, item := range v.snap.Entities.Models {
		if doc, ok := decodeDoc[canonicalModelDoc](v, item); ok {
			v.checkSemanticID(item, doc.AdoptedFrom, doc.SemanticID)
		}
	}
	for _, item := range v.snap.Entities.Collections {
		if doc, ok := decodeDoc[canonicalCollectionDoc](v, item); ok {
			v.checkSemanticID(item, doc.AdoptedFrom, doc.SemanticID)
		}
	}
}

// checkOverrides resolves every reference an override carries and reports
// override directories whose owner does not exist.
func (v *verifyContext) checkOverrides() {
	owners := map[string]bool{}
	for _, m := range v.snap.Entities.Models {
		owners[entityTypeModel+"/"+m.EntityID] = true
	}
	for _, c := range v.snap.Entities.Collections {
		owners[entityTypeCollection+"/"+c.EntityID] = true
	}
	orphaned := map[string]bool{}
	groups := []struct {
		items []SnapshotEntityFile
		owned bool
	}{{v.snap.Entities.BaseOverrides, false}, {v.snap.Entities.ModelOverrides, true}, {v.snap.Entities.CollectionOverrides, true}}
	for _, g := range groups {
		for _, item := range g.items {
			if g.owned {
				key := item.OwnerType + "/" + item.OwnerID
				if !owners[key] && !orphaned[key] {
					orphaned[key] = true
					v.add(item.Path, 0, "orphan-overrides", "%s %s has overrides but no %s.yaml", item.OwnerType, item.OwnerID, item.OwnerType)
				}
			}
			v.checkOverrideRefs(item)
		}
	}
}

func (v *verifyContext) checkOverrideRefs(item SnapshotEntityFile) {
	doc, ok := decodeDoc[canonicalOverrideDoc](v, item)
	if !ok {
		return
	}
	v.checkOverrideLocation(item, doc)
	// restore resolves the field as the written field_id, else the file name
	fieldID := firstNonEmpty(doc.FieldID, item.FieldID)
	fieldLine := yamlKeyLine(item.Payload, "field_id")
	switch {
	case doc.AdoptedFrom != "":
		// restore skips a marker override; its placement comes from the receipt
		if !v.receipts[fieldID] && !v.inLoadedParent(fieldID) {
			v.add(item.Path, yamlKeyLine(item.Payload, "_adopted_from"), "adoption-receipt-missing", "field %s is marked adopted but no receipt in adoptions/ names it; the loader drops this placement", fieldID)
		}
	case fieldID == "":
		v.add(item.Path, 0, "unresolved-field", "no field_id and no field name in the file name")
	case !v.fields[fieldID] && !v.inUnloadedParent(fieldID):
		v.add(item.Path, fieldLine, "unresolved-field", "field %s is neither in this tree, adopted, nor in a parent tree", fieldID)
	}
	if doc.CategoryID != "" && !v.categories[doc.CategoryID] {
		v.add(item.Path, yamlKeyLine(item.Payload, "category_id"), "unresolved-category", "category %s not found", doc.CategoryID)
	}
	if doc.PartOfCollectionID != "" && !v.collections[doc.PartOfCollectionID] {
		v.add(item.Path, yamlKeyLine(item.Payload, "part_of_collection_id"), "unresolved-collection", "collection %s not found", doc.PartOfCollectionID)
	}
	for _, ref := range doc.Refs {
		if ref.RefType == "concept_list" && !v.conceptLists[ref.SemanticID] {
			v.add(item.Path, yamlKeyLine(item.Payload, "refs"), "unresolved-concept-list", "concept list %s not found", ref.SemanticID)
		}
	}
}

// checkOverrideLocation reports an override whose own entity_id,
// entity_type or field_id disagrees with the directory it sits in: restore
// takes the written value and attaches the placement elsewhere.
func (v *verifyContext) checkOverrideLocation(item SnapshotEntityFile, doc canonicalOverrideDoc) {
	if item.EntityType == entityTypeBaseOverride {
		if doc.FieldID != "" && doc.FieldID != item.FieldID {
			v.add(item.Path, yamlKeyLine(item.Payload, "field_id"), "override-location", "field_id %s but the file sits under fields/%s; the loader attaches it to %s", doc.FieldID, item.FieldID, doc.FieldID)
		}
		return
	}
	if doc.EntityID != "" && doc.EntityID != item.OwnerID {
		v.add(item.Path, yamlKeyLine(item.Payload, "entity_id"), "override-location", "entity_id %s but the file sits under %s %s; the loader attaches it to %s", doc.EntityID, item.OwnerType, item.OwnerID, doc.EntityID)
	}
	if doc.EntityType != "" && doc.EntityType != item.OwnerType {
		v.add(item.Path, yamlKeyLine(item.Payload, "entity_type"), "override-location", "entity_type %s but the file sits under a %s", doc.EntityType, item.OwnerType)
	}
}

// inUnloadedParent reports whether the id belongs to a declared parent
// whose tree is not available; such references are reported once as
// parent-tree-missing, never per placement.
func (v *verifyContext) inUnloadedParent(id string) bool {
	pid := domain.ParseSemanticID(id).ProjectID
	return pid != "" && v.declared[pid] && v.parents[pid] == nil
}

// inLoadedParent reports whether a loaded parent tree carries the field.
func (v *verifyContext) inLoadedParent(fieldID string) bool {
	for _, ps := range v.parents {
		fields := map[string]bool{}
		snapshotIDs(ps, fields, map[string]bool{}, map[string]bool{}, map[string]bool{})
		if fields[fieldID] {
			return true
		}
	}
	return false
}

const manifestSectionProject = "project"

// checkProjectManifestKeys reports unknown keys in project.yaml, at the top
// level and inside its sections, which the loader drops silently.
func (v *verifyContext) checkProjectManifestKeys() {
	payload := v.readRel("project.yaml")
	if payload == nil {
		return
	}
	keys, err := topLevelKeys(payload)
	if err != nil {
		return // the loader already decoded it
	}
	known := jsonTagNames(projectManifest{})
	for _, k := range keys {
		if !known[k] {
			v.add("project.yaml", yamlKeyLine(payload, k), "unknown-key", "%q is not a key of project.yaml; the loader drops it silently", k)
		}
	}
	var doc map[string]any
	if err := yaml.Unmarshal(payload, &doc); err != nil {
		return
	}
	sections := map[string]any{
		manifestSectionProject: projectManifestProject{}, "inheritance": projectManifestInheritance{},
		"ontologies": projectManifestOntologies{}, "namespaces": projectManifestNamespaces{},
		"manifests": projectManifestManifests{},
	}
	for name, zero := range sections {
		section, ok := doc[name].(map[string]any)
		if !ok {
			continue
		}
		known := jsonTagNames(zero)
		for k := range section {
			if !known[k] {
				v.add("project.yaml", yamlNestedKeyLine(payload, k), "unknown-key", "%q is not a key of %s; the loader drops it silently", k, name)
			}
		}
	}
}

// checkAdoptions checks every receipt's source against the declared parents
// and, when that parent's tree is loaded, against its entities.
func (v *verifyContext) checkAdoptions() {
	if v.snap.Adoptions == nil {
		return
	}
	for name, receipt := range v.snap.Adoptions.Receipts {
		file := filepath.ToSlash(filepath.Join("adoptions", name)) // index entries are relative to adoptions/
		src := receipt.Adoption.Source
		line := yamlKeyLine(v.readRel(file), "adoption")
		if !v.declared[src.ProjectID] {
			v.add(file, line, "adoption-unknown-parent", "source project %s is not a declared parent", src.ProjectID)
			continue
		}
		parent := v.parents[src.ProjectID]
		if parent == nil {
			continue // reported once as parent-tree-missing
		}
		ids := map[string]bool{}
		switch receipt.Adoption.EntityType {
		case entityTypeField:
			snapshotIDs(parent, ids, map[string]bool{}, map[string]bool{}, map[string]bool{})
		case entityTypeCategory:
			snapshotIDs(parent, map[string]bool{}, ids, map[string]bool{}, map[string]bool{})
		case entityTypeCollection:
			snapshotIDs(parent, map[string]bool{}, map[string]bool{}, ids, map[string]bool{})
		default:
			snapshotIDs(parent, map[string]bool{}, map[string]bool{}, map[string]bool{}, ids)
		}
		if !ids[src.EntityID] {
			v.add(file, line, "adoption-missing-source", "%s %s is not in parent %s", receipt.Adoption.EntityType, src.EntityID, src.ProjectID)
		}
	}
}

// checkPlacements reports placement files whose collection the model does not
// place, whose category is unknown, or whose range is invalid.
func (v *verifyContext) checkPlacements() {
	if len(v.snap.Entities.Placements) == 0 {
		return
	}
	placed, err := placedCollectionsByModel(v.snap)
	if err != nil {
		return // the override checks already report undecodable override files
	}
	for _, item := range v.snap.Entities.Placements {
		doc, ok := decodeDoc[canonicalPlacementDoc](v, item)
		if !ok {
			continue
		}
		if !placed[item.OwnerID][doc.CollectionID] {
			v.add(item.Path, yamlKeyLine(item.Payload, "collection_id"), "placement-orphan",
				"model %s places no field of collection %s", item.OwnerID, doc.CollectionID)
		}
		if doc.CategoryID != "" && !v.categories[doc.CategoryID] {
			v.add(item.Path, yamlKeyLine(item.Payload, "category_id"), "placement-category",
				"category %s is not in the tree, its adoptions or its parents", doc.CategoryID)
		}
		if doc.MinOccurs < 0 || (doc.MaxOccurs != nil && *doc.MaxOccurs < doc.MinOccurs) {
			maxOccurs := "unbounded"
			if doc.MaxOccurs != nil {
				maxOccurs = strconv.Itoa(*doc.MaxOccurs)
			}
			v.add(item.Path, yamlKeyLine(item.Payload, "max_occurs"), "placement-range",
				"min_occurs %d and max_occurs %s do not form a range", doc.MinOccurs, maxOccurs)
		}
	}
}
