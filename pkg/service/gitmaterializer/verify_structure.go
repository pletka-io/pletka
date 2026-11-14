package gitmaterializer

import (
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

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

const verifyManifestFile = "project.yaml"

// checkNamespaceParents notes every prefix the tree binds to another
// namespace than a parent's tree does. The project's binding wins in its own
// output, also for fields adopted from that parent, so the note is not a
// finding.
func (v *verifyContext) checkNamespaceParents() {
	own := firstBindings(v.snap.Manifest.Namespaces)
	line := yamlKeyLine(v.readRel(verifyManifestFile), "namespaces")
	for _, id := range slices.Sorted(maps.Keys(v.parents)) {
		theirs := firstBindings(v.parents[id].Manifest.Namespaces)
		for _, prefix := range slices.Sorted(maps.Keys(own)) {
			if ns, ok := theirs[prefix]; ok && ns != own[prefix] {
				v.notes = append(v.notes, Finding{File: verifyManifestFile, Line: line, Code: "namespace-diverges-from-parent",
					Message: fmt.Sprintf("%s is bound to %s; parent %s binds it to %s", prefix, own[prefix], id, ns)})
			}
		}
	}
}

// firstBindings maps each prefix to its first namespace, the one the loader
// takes as the project's choice.
func firstBindings(ns *projectManifestNamespaces) map[string]string {
	out := map[string]string{}
	if ns == nil {
		return out
	}
	for _, b := range ns.EffectiveBindings {
		prefix := strings.TrimSpace(b.Prefix)
		if _, seen := out[prefix]; !seen && prefix != "" {
			out[prefix] = strings.TrimSpace(b.Namespace)
		}
	}
	return out
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
	for _, dir := range []string{"categories", "concept-lists", "vocabularies", "fields", "models", "collections"} {
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
func snapshotIDs(ps *ProjectSnapshot, fields, categories, collections, conceptLists, vocabularies map[string]bool) {
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
	addEntityIDs(conceptLists, ps.Entities.ConceptLists)
	addEntityIDs(vocabularies, ps.Entities.Vocabularies)
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
			case entityTypeVocabulary:
				vocabularies[r.EntityID] = true
			}
		}
	}
	for _, vp := range ps.Vendor.Projects {
		if vp.Snapshot != nil {
			snapshotIDs(vp.Snapshot, fields, categories, collections, conceptLists, vocabularies)
		}
	}
}

func addEntityIDs(into map[string]bool, items []SnapshotEntityFile) {
	for _, c := range items {
		into[c.EntityID] = true
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
	snapshotIDs(v.snap, v.fields, v.categories, v.collections, v.conceptLists, v.vocabularies)
	for _, ps := range v.parents {
		snapshotIDs(ps, v.fields, v.categories, v.collections, v.conceptLists, v.vocabularies)
	}
}

// valueTypeCollection is a field's expected_value_type for a stub (lower-cased).
const valueTypeCollection = "collection"

// collectStubs records the fields whose expected value is a collection (a
// stub grafts the collection its collection_model ref names), from this
// tree, its loaded parents and their vendored projects.
func (v *verifyContext) collectStubs() {
	v.stubFields = map[string]bool{}
	var walk func(ps *ProjectSnapshot)
	walk = func(ps *ProjectSnapshot) {
		if ps == nil {
			return
		}
		for _, item := range ps.Entities.Fields {
			doc, err := decodeCanonicalYAML[canonicalFieldDoc](item.Payload)
			if err != nil {
				continue // the schema or decode pass reports it
			}
			switch strings.ToLower(strings.TrimSpace(doc.ExpectedValueType)) {
			case valueTypeCollection, valueTypeCollection + " model":
				v.stubFields[item.EntityID] = true
			}
		}
		for _, vp := range ps.Vendor.Projects {
			walk(vp.Snapshot)
		}
	}
	walk(v.snap)
	for _, ps := range v.parents {
		walk(ps)
	}
}

// decodeDoc decodes an entity file. A file the schema pass already flagged
// is not reported again. ok is false when decoding failed.
func decodeDoc[T any](v *verifyContext, item SnapshotEntityFile) (doc T, ok bool) {
	doc, err := decodeCanonicalYAML[T](item.Payload)
	if err != nil {
		if !v.schemaFailed[item.Path] {
			v.add(item.Path, 0, "decode", "%v", err)
		}
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
	parsed := domain.ParseSemanticID(id)
	if (id == "" || !parsed.Valid()) && v.schemaFailed[item.Path] {
		return // the schema pass reported the missing or malformed id
	}
	if id == "" {
		v.add(item.Path, line, "semantic-id", "semantic_id is required")
		return
	}
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
	}{{v.snap.Entities.BaseOverrides, false}, {v.snap.Entities.ModelOverrides, true}, {v.snap.Entities.CollectionOverrides, true}, {v.snap.Entities.Grafts, true}}
	for _, g := range groups {
		for _, item := range g.items {
			if g.owned {
				key := item.OwnerType + "/" + item.OwnerID
				if !owners[key] && !orphaned[key] {
					orphaned[key] = true
					v.add(item.Path, 0, "orphan-overrides", "%s %s has overrides but no %s.yaml", item.OwnerType, item.OwnerID, item.OwnerType)
				}
			}
			if item.EntityType == entityTypeModelGrafts {
				continue // checkGrafts
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
	hasCollectionRef := false
	for _, ref := range doc.Refs {
		switch ref.RefType {
		case "concept_list":
			if !v.conceptLists[ref.SemanticID] {
				v.add(item.Path, yamlKeyLine(item.Payload, "refs"), "unresolved-concept-list", "concept list %s not found", ref.SemanticID)
			}
		case "collection_model":
			hasCollectionRef = true
			if !v.collections[ref.SemanticID] && !v.inUnloadedParent(ref.SemanticID) {
				v.add(item.Path, yamlKeyLine(item.Payload, "refs"), "unresolved-collection-ref", "collection %s not found", ref.SemanticID)
			}
		}
	}
	// refs belong to the winning override, so a placement of a stub names
	// its collection itself; base overrides place nothing
	if item.EntityType != entityTypeBaseOverride && doc.AdoptedFrom == "" && v.stubFields[fieldID] && !hasCollectionRef {
		v.add(item.Path, fieldLine, "stub-without-collection", "field %s holds a collection but this placement names none in refs; the Arches pack grafts nothing here", fieldID)
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
		snapshotIDs(ps, fields, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{})
		if fields[fieldID] {
			return true
		}
	}
	return false
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
		none := map[string]bool{}
		switch receipt.Adoption.EntityType {
		case entityTypeField:
			snapshotIDs(parent, ids, none, none, none, none)
		case entityTypeCategory:
			snapshotIDs(parent, none, ids, none, none, none)
		case entityTypeCollection:
			snapshotIDs(parent, none, none, ids, none, none)
		case entityTypeVocabulary:
			snapshotIDs(parent, none, none, none, none, ids)
		default:
			snapshotIDs(parent, none, none, none, ids, none)
		}
		if !ids[src.EntityID] {
			v.add(file, line, "adoption-missing-source", "%s %s is not in parent %s", receipt.Adoption.EntityType, src.EntityID, src.ProjectID)
		}
	}
}

// checkPlacements reports placement files whose ids disagree with their path,
// whose collection the model does not place, whose category is unknown, or
// whose range is invalid.
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
		collectionID, categoryID, _ := parsePlacementFileName(path.Base(item.Path))
		if doc.ModelID != item.OwnerID || doc.CollectionID != collectionID || doc.CategoryID != categoryID {
			v.add(item.Path, yamlKeyLine(item.Payload, "model_id"), "placement-path",
				"model_id %s, collection_id %s and category_id %q must match the file's place in the tree", doc.ModelID, doc.CollectionID, doc.CategoryID)
		}
		if !placed[item.OwnerID][doc.CollectionID] {
			v.add(item.Path, yamlKeyLine(item.Payload, "collection_id"), "placement-orphan",
				"model %s places no field of collection %s", item.OwnerID, doc.CollectionID)
		}
		if doc.CategoryID != "" && !v.categories[doc.CategoryID] {
			v.add(item.Path, yamlKeyLine(item.Payload, "category_id"), "placement-category",
				"category %s is not in the tree, its adoptions or its parents", doc.CategoryID)
		}
		if doc.MinOccurs < 0 || doc.MinOccurs > math.MaxInt32 ||
			(doc.MaxOccurs != nil && (*doc.MaxOccurs < doc.MinOccurs || *doc.MaxOccurs > math.MaxInt32)) {
			maxOccurs := "unbounded"
			if doc.MaxOccurs != nil {
				maxOccurs = strconv.Itoa(*doc.MaxOccurs)
			}
			v.add(item.Path, yamlKeyLine(item.Payload, "max_occurs"), "placement-range",
				"min_occurs %d and max_occurs %s do not form a range within 0..%d", doc.MinOccurs, maxOccurs, math.MaxInt32)
		}
	}
}

// checkGrafts reports grafts.yaml addresses whose segments name no field the
// tree, its vendored projects or its parents define. Whether the address is
// reached in the composed model needs the database: compose warns
// (compose.unresolved_graft).
func (v *verifyContext) checkGrafts() {
	for _, item := range v.snap.Entities.Grafts {
		if v.schemaFailed[item.Path] {
			continue
		}
		doc, err := decodeTreeFile[map[string]graftDoc](item.Path, item.Payload)
		if err != nil {
			continue // checkSchemas reported it
		}
		for _, addr := range slices.Sorted(maps.Keys(doc)) {
			for _, seg := range strings.Split(addr, "/") {
				if !v.fields[seg] && !v.inUnloadedParent(seg) {
					v.add(item.Path, yamlKeyLine(item.Payload, addr), "unresolved-graft-address",
						"%s: field %s is not defined in the tree, its vendored projects or its parents", addr, seg)
					break
				}
			}
		}
	}
}

// checkVocabularies mirrors what load-git rejects for vocabularies and concept
// lists, so verify reports it first: broader URIs outside the vocabulary,
// broader cycles, list headers naming a vocabulary the tree lacks, entries
// outside the list's vocabulary, and entries without a list header.
func (v *verifyContext) checkVocabularies() {
	concepts := map[string]map[string]bool{} // own vocabulary -> its concept URIs
	for _, item := range v.snap.Entities.Vocabularies {
		concepts[item.EntityID] = map[string]bool{}
	}
	for _, item := range v.snap.Entities.VocabularyConcepts {
		if _, ok := concepts[item.OwnerID]; !ok {
			v.add(item.Path, 0, "unresolved-vocabulary", "vocabulary %s has no vocabulary.yaml; load-git rejects its concepts", item.OwnerID)
			continue
		}
		list, err := decodeCanonicalYAML[[]canonicalConceptDoc](item.Payload)
		if err != nil {
			if !v.schemaFailed[item.Path] {
				v.add(item.Path, 0, "decode", "%v", err)
			}
			continue
		}
		for _, c := range list {
			concepts[item.OwnerID][c.URI] = true
		}
		v.checkConceptEdges(item, list, concepts[item.OwnerID])
	}

	headers := map[string]canonicalConceptListDoc{}
	for _, item := range v.snap.Entities.ConceptLists {
		if doc, ok := decodeDoc[canonicalConceptListDoc](v, item); ok {
			headers[item.EntityID] = doc
			if name := strings.TrimSpace(doc.Vocabulary); name != "" && !v.vocabularies[name] {
				v.add(item.Path, yamlKeyLine(item.Payload, "vocabulary"), "unresolved-vocabulary", "vocabulary %s is neither in this tree, adopted, nor in a vendored parent", name)
			}
		}
	}
	for _, item := range v.snap.Entities.ConceptListEntries {
		doc, ok := headers[item.OwnerID]
		if !ok {
			v.add(item.Path, 0, "entries-without-list", "concept list %s has no header file", item.OwnerID)
			continue
		}
		name := strings.TrimSpace(doc.Vocabulary)
		if name == "" {
			v.add(item.Path, 0, "unresolved-vocabulary", "concept list %s names no vocabulary; load-git rejects its entries", item.OwnerID)
			continue
		}
		own, isOwn := concepts[name]
		if !isOwn {
			continue // adopted or vendored: its concepts are not in this tree
		}
		uris, err := decodeCanonicalYAML[[]string](item.Payload)
		if err != nil {
			if !v.schemaFailed[item.Path] {
				v.add(item.Path, 0, "decode", "%v", err)
			}
			continue
		}
		for i, uri := range uris {
			if !own[uri] {
				v.add(item.Path, i+1, "unresolved-concept", "%s is not a concept of vocabulary %s", uri, name)
			}
		}
	}
}

// checkConceptEdges reports a broader URI outside the vocabulary and any
// broader chain that loops (a self-broader included).
func (v *verifyContext) checkConceptEdges(item SnapshotEntityFile, list []canonicalConceptDoc, known map[string]bool) {
	broader := map[string][]string{}
	for _, c := range list {
		for _, b := range c.Broader {
			if !known[b] {
				v.add(item.Path, yamlKeyLine(item.Payload, "broader"), "unresolved-concept", "concept %s has broader %s, which is not in vocabulary %s", c.URI, b, item.OwnerID)
				continue
			}
			broader[c.URI] = append(broader[c.URI], b)
		}
	}
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var walk func(uri string) bool
	walk = func(uri string) bool {
		switch state[uri] {
		case visiting:
			return true
		case done:
			return false
		}
		state[uri] = visiting
		for _, b := range broader[uri] {
			if walk(b) {
				return true
			}
		}
		state[uri] = done
		return false
	}
	for _, c := range list {
		if walk(c.URI) {
			v.add(item.Path, yamlKeyLine(item.Payload, "broader"), "concept-cycle", "the broader chain of %s loops", c.URI)
			return
		}
	}
}
