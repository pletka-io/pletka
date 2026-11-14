package gitmaterializer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	nsutil "github.com/pletka-io/pletka/pkg/namespace"
)

type canonicalCategoryDoc struct {
	AdoptedFrom    string              `json:"_adopted_from,omitempty"`
	SemanticID     string              `json:"semantic_id"`
	SystemName     string              `json:"system_name,omitempty"`
	UIName         domain.Translations `json:"ui_name,omitempty"`
	Description    domain.Translations `json:"description,omitempty"`
	Status         string              `json:"status,omitempty"`
	Deprecated     bool                `json:"deprecated,omitempty"`
	CanonicalOrder int                 `json:"canonical_order,omitempty"`
}

// canonicalPathElement is one path/scope element as read from a snapshot.
// Two formats exist: the current lossless full-map form (every JSONB key of
// domain.PathElement) and the legacy compact qname string
// ("crm:E21_Person" / "E29/crmdig:D1"). json.RawMessage defers the choice
// to decode time so old snapshots keep restoring.
type canonicalPathElement = json.RawMessage

type canonicalSubfieldPathDoc struct {
	PathElements      []canonicalPathElement `json:"path_elements"`
	ExpectedValueType string                 `json:"expected_value_type,omitempty"`
	Scope             string                 `json:"scope,omitempty"`
	Source            string                 `json:"source,omitempty"`
}

type canonicalFieldDoc struct {
	AdoptedFrom       string                     `json:"_adopted_from,omitempty"`
	SemanticID        string                     `json:"semantic_id"`
	SystemName        string                     `json:"system_name,omitempty"`
	UIName            domain.Translations        `json:"ui_name,omitempty"`
	Description       domain.Translations        `json:"description,omitempty"`
	OntologyScope     canonicalPathElement       `json:"ontology_scope,omitempty"`
	PathElements      []canonicalPathElement     `json:"path_elements,omitempty"`
	SubfieldPaths     []canonicalSubfieldPathDoc `json:"subfield_paths,omitempty"`
	ExpectedValueType string                     `json:"expected_value_type,omitempty"`
	Status            string                     `json:"status,omitempty"`
	Deprecated        bool                       `json:"deprecated,omitempty"`
	Examples          []domain.FieldExample      `json:"examples,omitempty"`
}

type canonicalModelDoc struct {
	AdoptedFrom   string               `json:"_adopted_from,omitempty"`
	SemanticID    string               `json:"semantic_id"`
	SystemName    string               `json:"system_name,omitempty"`
	UIName        domain.Translations  `json:"ui_name,omitempty"`
	Description   domain.Translations  `json:"description,omitempty"`
	OntologyScope canonicalPathElement `json:"ontology_scope,omitempty"`
	Status        string               `json:"status,omitempty"`
	Deprecated    bool                 `json:"deprecated,omitempty"`
}

type canonicalCollectionDoc struct {
	AdoptedFrom              string               `json:"_adopted_from,omitempty"`
	SemanticID               string               `json:"semantic_id"`
	SystemName               string               `json:"system_name,omitempty"`
	UIName                   domain.Translations  `json:"ui_name,omitempty"`
	Description              domain.Translations  `json:"description,omitempty"`
	OntologyScope            canonicalPathElement `json:"ontology_scope,omitempty"`
	Status                   string               `json:"status,omitempty"`
	Deprecated               bool                 `json:"deprecated,omitempty"`
	CollectionNumber         int                  `json:"collection_number,omitempty"`
	CanonicalCollectionOrder int                  `json:"canonical_collection_order,omitempty"`
}

func (m *Materializer) HydrateProjectEntities(ctx context.Context, plan *RestorePlan) error {
	if plan == nil || plan.Snapshot == nil {
		return fmt.Errorf("hydrate project entities: missing snapshot")
	}
	projectID := strings.TrimSpace(plan.ProjectID)
	if projectID == "" {
		return fmt.Errorf("hydrate project entities: missing project id")
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("hydrate project entities: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := m.queries.WithTx(tx)
	expand := snapshotPrefixExpanderWith(plan.Snapshot.Manifest.Namespaces, ontologyPrefixLookup(ctx, q))
	for _, item := range plan.Snapshot.Entities.Categories {
		if err := hydrateCategoryFile(ctx, q, projectID, item); err != nil {
			return err
		}
	}
	if err := hydrateVocabularySlice(ctx, q, projectID, plan.Snapshot.Entities); err != nil {
		return err
	}
	for _, item := range plan.Snapshot.Entities.Fields {
		if err := hydrateFieldFile(ctx, q, projectID, item, expand); err != nil {
			return err
		}
	}
	for _, item := range plan.Snapshot.Entities.Models {
		if err := hydrateModelFile(ctx, q, projectID, item, expand); err != nil {
			return err
		}
	}
	for _, item := range plan.Snapshot.Entities.Collections {
		if err := hydrateCollectionFile(ctx, q, projectID, item, expand); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("hydrate project entities: commit tx: %w", err)
	}
	return nil
}

func hydrateCategoryFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile) error {
	doc, err := decodeTreeFile[canonicalCategoryDoc](item.Path, item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate category %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate category %s: missing semantic_id", item.Path)
	}

	existing, err := q.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
		ProjectID:  projectID,
		SemanticID: nullableString(semanticID),
	})
	createParams := sqlcgen.WeaveCreateCategoryParams{
		ID:             semanticID, // categories are keyed by their semantic id, like fields
		SemanticID:     nullableString(semanticID),
		SystemName:     nullableString(doc.SystemName),
		UiName:         marshalTranslationsForRestore(doc.UIName),
		Description:    marshalTranslationsForRestore(doc.Description),
		Status:         restoreStatus(doc.Status),
		ProjectID:      projectID,
		CanonicalOrder: int32(doc.CanonicalOrder),
	}
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("hydrate category %s: lookup existing: %w", item.Path, err)
		}
		created, err := q.WeaveCreateCategory(ctx, createParams)
		if err != nil {
			return fmt.Errorf("hydrate category %s: create: %w", item.Path, err)
		}
		if err := setDeprecatedStateCategory(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate category %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateCategory(ctx, sqlcgen.WeaveUpdateCategoryParams{
		ID:             existing.ID,
		UiName:         marshalTranslationsForRestore(doc.UIName),
		Description:    marshalTranslationsForRestore(doc.Description),
		SystemName:     nullableString(doc.SystemName),
		Status:         restoreStatus(doc.Status),
		CanonicalOrder: int32(doc.CanonicalOrder),
	}); err != nil {
		return fmt.Errorf("hydrate category %s: update: %w", item.Path, err)
	}
	if err := setDeprecatedStateCategory(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate category %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

type canonicalConceptListDoc struct {
	SemanticID  string            `json:"semantic_id"`
	SystemName  string            `json:"system_name"`
	UIName      map[string]string `json:"ui_name"`
	Description map[string]string `json:"description"`
	Status      string            `json:"status"`
	IsClosed    bool              `json:"is_closed"`
	Vocabulary  string            `json:"vocabulary"`
}

type canonicalVocabularyDoc struct {
	SemanticID  string              `json:"semantic_id"`
	SystemName  string              `json:"system_name"`
	UIName      domain.Translations `json:"ui_name"`
	Description domain.Translations `json:"description"`
	Status      string              `json:"status"`
	BaseURI     string              `json:"base_uri"`
}

type canonicalConceptDoc struct {
	URI        string              `json:"uri"`
	Label      domain.Translations `json:"label"`
	ScopeNote  domain.Translations `json:"scope_note"`
	Broader    []string            `json:"broader"`
	TopConcept bool                `json:"top_concept"`
	ExternalID string              `json:"external_id"`
	ExactMatch []string            `json:"exact_match"`
}

// treeConceptList is a concept list loaded from the tree: its row id and,
// when the header names one, its vocabulary's row id.
type treeConceptList struct {
	id           string
	vocabularyID string
}

// hydrateVocabularySlice loads vocabularies, concept lists, concepts and
// list entries, in an order the foreign keys allow:
//
//  1. vocabularies (claim a same-name row, then upsert by semantic id);
//  2. concept-list headers, binding vocabulary_id, then delete what left
//     the tree (pruneRemovedVocabularySlice);
//  3. delete the entries of every list whose header names a vocabulary
//     (entries.yaml is then their complete membership; an absent file is
//     an empty list). A list without one keeps its entries, as before
//     entries were in the tree;
//  4. concepts per vocabulary: upsert, prune those not in the file, then
//     replace the vocabulary-level broader edges;
//  5. list entries in file order.
//
// Step 3 precedes the prune in step 4 because weave_concept_list_entries
// references weave_vocabulary_entries without ON DELETE.
func hydrateVocabularySlice(ctx context.Context, q *sqlcgen.Queries, projectID string, entities SnapshotEntityTree) error {
	vocabularies := map[string]string{}
	for _, item := range entities.Vocabularies {
		id, err := hydrateVocabularyFile(ctx, q, projectID, item)
		if err != nil {
			return err
		}
		vocabularies[item.EntityID] = id
	}
	lists := map[string]treeConceptList{}
	for _, item := range entities.ConceptLists {
		list, err := hydrateConceptListFile(ctx, q, projectID, item, vocabularies)
		if err != nil {
			return err
		}
		lists[item.EntityID] = list
	}
	if err := pruneRemovedVocabularySlice(ctx, q, projectID, entities, vocabularies); err != nil {
		return err
	}
	// Clearing drops entry custom_label on vocabulary-bound lists: it is not
	// in the tree, and entries.yaml is their whole membership.
	for _, item := range entities.ConceptLists {
		list := lists[item.EntityID]
		if list.vocabularyID == "" {
			continue
		}
		if err := q.WeaveDeleteConceptListEntriesByList(ctx, list.id); err != nil {
			return fmt.Errorf("hydrate concept list %s: clear entries: %w", item.Path, err)
		}
	}
	conceptFiles := map[string]SnapshotEntityFile{}
	for _, item := range entities.VocabularyConcepts {
		if _, ok := vocabularies[item.OwnerID]; !ok {
			return fmt.Errorf("hydrate concepts %s: vocabulary %s has no vocabulary.yaml", item.Path, item.OwnerID)
		}
		conceptFiles[item.OwnerID] = item
	}
	// A vocabulary without concepts.yaml has no concepts: the tree is the
	// whole vocabulary.
	for _, item := range entities.Vocabularies {
		file, ok := conceptFiles[item.EntityID]
		if !ok {
			file = SnapshotEntityFile{Path: domain.VocabularyConceptsPath(item.EntityID), OwnerID: item.EntityID, Payload: []byte("[]\n")}
		}
		if err := hydrateVocabularyConceptsFile(ctx, q, file, vocabularies[item.EntityID]); err != nil {
			return err
		}
	}
	for _, item := range entities.ConceptListEntries {
		list, ok := lists[item.OwnerID]
		if !ok {
			return fmt.Errorf("hydrate concept-list entries %s: concept list %s is not in the tree", item.Path, item.OwnerID)
		}
		if err := hydrateConceptListEntriesFile(ctx, q, item, list); err != nil {
			return err
		}
	}
	return nil
}

// pruneRemovedVocabularySlice deletes what left the tree: the project's
// concept lists bound to a <P>.VOC.n vocabulary but absent from the tree, and
// the project's <P>.VOC.n vocabularies absent from the tree, with their
// entries and broader edges (ON DELETE CASCADE) and bound lists.
//
// This deviates on purpose from the upsert-only load of every other entity
// kind (see HydrateRestorePlan): vocabularies and list members are tree-first,
// so a removed list or vocabulary must not linger in the database and reach
// a generator. Lists bound to <P>.VOCAB.local or to no vocabulary, and
// vocabularies of any other shape, are never touched. A list's entries and
// list-scoped broader edges go with it by ON DELETE CASCADE.
//
// Override refs carry no foreign key. The overrides are reloaded from the
// tree after the entities, so the refs that count are the tree's: a removed
// list a tree override still refers to fails the load, naming both, before
// anything is deleted. So does a removed vocabulary whose concepts another
// list still uses or that a tree list still names.
func pruneRemovedVocabularySlice(ctx context.Context, q *sqlcgen.Queries, projectID string, entities SnapshotEntityTree, vocabularies map[string]string) error {
	inTree := map[string]bool{}
	for _, item := range entities.ConceptLists {
		inTree[item.EntityID] = true
	}
	refs, err := treeConceptListRefs(entities)
	if err != nil {
		return err
	}
	rows, err := q.WeaveListProjectTreeVocabularies(ctx, projectID)
	if err != nil {
		return fmt.Errorf("prune vocabularies: list project vocabularies: %w", err)
	}
	for _, v := range rows {
		semanticID := derefStr(v.SemanticID)
		_, keep := vocabularies[semanticID]
		bound, err := q.WeaveListProjectConceptListsByVocabulary(ctx, sqlcgen.WeaveListProjectConceptListsByVocabularyParams{ProjectID: projectID, VocabularyID: v.ID})
		if err != nil {
			return fmt.Errorf("prune vocabulary %s: list its concept lists: %w", semanticID, err)
		}
		for _, l := range bound {
			if inTree[l.SemanticID] {
				if !keep {
					return fmt.Errorf("prune vocabulary %s: removed from the tree, but concept list %s in the tree names it", semanticID, l.SemanticID)
				}
				continue
			}
			if override, ok := refs[l.SemanticID]; ok {
				return fmt.Errorf("prune concept list %s: removed from the tree, but override %s still refers to it", l.SemanticID, override)
			}
			if err := q.WeaveDeleteConceptList(ctx, sqlcgen.WeaveDeleteConceptListParams{ProjectID: projectID, ID: l.ID}); err != nil {
				return fmt.Errorf("prune concept list %s: %w", l.SemanticID, err)
			}
		}
		if keep {
			continue
		}
		used, err := q.WeaveListVocabularyEntryRefsNotIn(ctx, sqlcgen.WeaveListVocabularyEntryRefsNotInParams{VocabularyID: v.ID})
		if err != nil {
			return fmt.Errorf("prune vocabulary %s: check its concepts: %w", semanticID, err)
		}
		if len(used) > 0 {
			names := make([]string, 0, len(used))
			for _, r := range used {
				names = append(names, fmt.Sprintf("%s (concept list %s)", r.Uri, r.ConceptList))
			}
			return fmt.Errorf("prune vocabulary %s: removed from the tree, but its concepts are still used: %s", semanticID, strings.Join(names, ", "))
		}
		if _, err := q.WeaveDeleteProjectVocabulary(ctx, sqlcgen.WeaveDeleteProjectVocabularyParams{ProjectID: projectID, ID: v.ID}); err != nil {
			return fmt.Errorf("prune vocabulary %s: %w", semanticID, err)
		}
	}
	return nil
}

// treeConceptListRefs maps each concept list a tree override refers to onto
// the path of the first such override. Adopted overrides are skipped, as the
// override load skips them.
func treeConceptListRefs(entities SnapshotEntityTree) (map[string]string, error) {
	refs := map[string]string{}
	for _, group := range [][]SnapshotEntityFile{entities.BaseOverrides, entities.ModelOverrides, entities.CollectionOverrides} {
		for _, item := range group {
			doc, err := decodeTreeFile[canonicalOverrideDoc](item.Path, item.Payload)
			if err != nil {
				return nil, fmt.Errorf("prune concept lists: decode override %s: %w", item.Path, err)
			}
			if strings.TrimSpace(doc.AdoptedFrom) != "" {
				continue
			}
			for _, ref := range doc.Refs {
				id := strings.TrimSpace(ref.SemanticID)
				if strings.TrimSpace(ref.RefType) != entityTypeConceptList || id == "" {
					continue
				}
				if _, seen := refs[id]; !seen {
					refs[id] = item.Path
				}
			}
		}
	}
	return refs, nil
}

// hydrateVocabularyFile upserts a tree vocabulary by (project, semantic id)
// and returns its row id. A row holding the same system_name (unique per
// project) without a semantic id is claimed first, so a vocabulary an older
// import created becomes the tree's; one owned by another semantic id is an
// error rather than a unique-index violation.
func hydrateVocabularyFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile) (string, error) {
	doc, err := decodeTreeFile[canonicalVocabularyDoc](item.Path, item.Payload)
	if err != nil {
		return "", fmt.Errorf("hydrate vocabulary %s: decode payload: %w", item.Path, err)
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return "", fmt.Errorf("hydrate vocabulary %s: missing semantic_id", item.Path)
	}
	if semanticID != item.EntityID {
		return "", fmt.Errorf("hydrate vocabulary %s: semantic_id %q must equal the directory name", item.Path, semanticID)
	}
	systemName := strings.TrimSpace(doc.SystemName)
	if systemName != "" {
		same, err := q.WeaveGetProjectVocabularyBySystemName(ctx, sqlcgen.WeaveGetProjectVocabularyBySystemNameParams{ProjectID: projectID, SystemName: systemName})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return "", fmt.Errorf("hydrate vocabulary %s: lookup system_name %q: %w", item.Path, systemName, err)
		case same.SemanticID == nil:
			if _, err := q.WeaveClaimVocabularySemanticID(ctx, sqlcgen.WeaveClaimVocabularySemanticIDParams{ID: same.ID, SemanticID: semanticID}); err != nil {
				return "", fmt.Errorf("hydrate vocabulary %s: claim vocabulary %s: %w", item.Path, same.ID, err)
			}
		case *same.SemanticID != semanticID:
			return "", fmt.Errorf("hydrate vocabulary %s: vocabulary %s: system_name %q is already used by vocabulary %s in project %s", item.Path, semanticID, systemName, *same.SemanticID, projectID)
		}
	}
	id, err := q.WeaveUpsertProjectVocabulary(ctx, sqlcgen.WeaveUpsertProjectVocabularyParams{
		ID:          ids.GenerateULID(),
		SemanticID:  semanticID,
		SystemName:  systemName,
		UiName:      marshalTranslationsForRestore(doc.UIName),
		Description: marshalTranslationsForRestore(doc.Description),
		Status:      restoreStatus(doc.Status),
		ProjectID:   projectID,
		BaseUri:     strings.TrimSpace(doc.BaseURI),
	})
	if err != nil {
		return "", fmt.Errorf("hydrate vocabulary %s: upsert: %w", item.Path, err)
	}
	return id, nil
}

// hydrateConceptListFile upserts a concept list by (project, semantic id).
// The row id is the semantic id, the convention the vocabulary slice uses.
// A header naming a vocabulary (a tree vocabulary, or one the project
// already has, such as its local terms) binds vocabulary_id.
func hydrateConceptListFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile, vocabularies map[string]string) (treeConceptList, error) {
	doc, err := decodeTreeFile[canonicalConceptListDoc](item.Path, item.Payload)
	if err != nil {
		return treeConceptList{}, fmt.Errorf("hydrate concept list %s: decode payload: %w", item.Path, err)
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return treeConceptList{}, fmt.Errorf("hydrate concept list %s: missing semantic_id", item.Path)
	}
	if semanticID != item.EntityID {
		return treeConceptList{}, fmt.Errorf("hydrate concept list %s: semantic_id %q must equal the file name", item.Path, semanticID)
	}
	row, err := q.WeaveUpsertConceptList(ctx, sqlcgen.WeaveUpsertConceptListParams{
		ID:          semanticID,
		SemanticID:  nullableString(semanticID),
		SystemName:  nullableString(doc.SystemName),
		UiName:      marshalTranslationsForRestore(doc.UIName),
		Description: marshalTranslationsForRestore(doc.Description),
		Status:      restoreStatus(doc.Status),
		ProjectID:   projectID,
		IsClosed:    doc.IsClosed,
	})
	if err != nil {
		return treeConceptList{}, fmt.Errorf("hydrate concept list %s: upsert: %w", item.Path, err)
	}
	list := treeConceptList{id: row.ID}
	vocabulary := strings.TrimSpace(doc.Vocabulary)
	if vocabulary == "" {
		return list, nil
	}
	vocabularyID, ok := vocabularies[vocabulary]
	if !ok {
		vocabularyID, err = q.WeaveGetProjectVocabularyBySemanticID(ctx, sqlcgen.WeaveGetProjectVocabularyBySemanticIDParams{ProjectID: projectID, SemanticID: vocabulary})
		if errors.Is(err, pgx.ErrNoRows) {
			return treeConceptList{}, fmt.Errorf("hydrate concept list %s: vocabulary %s is neither in the tree nor in project %s", item.Path, vocabulary, projectID)
		}
		if err != nil {
			return treeConceptList{}, fmt.Errorf("hydrate concept list %s: lookup vocabulary %s: %w", item.Path, vocabulary, err)
		}
	}
	n, err := q.WeaveSetConceptListVocabulary(ctx, sqlcgen.WeaveSetConceptListVocabularyParams{VocabularyID: vocabularyID, ProjectID: projectID, SemanticID: semanticID})
	if err != nil {
		return treeConceptList{}, fmt.Errorf("hydrate concept list %s: set vocabulary %s: %w", item.Path, vocabulary, err)
	}
	if n == 0 {
		return treeConceptList{}, fmt.Errorf("hydrate concept list %s: set vocabulary %s: no list row %s in project %s", item.Path, vocabulary, semanticID, projectID)
	}
	list.vocabularyID = vocabularyID
	return list, nil
}

// hydrateVocabularyConceptsFile makes the vocabulary's entries exactly the
// file's concepts. broader_uri keeps the first broader; every broader is an
// edge in weave_concept_broader (scheme NULL, position = index). The
// broader_path columns are not maintained here.
func hydrateVocabularyConceptsFile(ctx context.Context, q *sqlcgen.Queries, item SnapshotEntityFile, vocabularyID string) error {
	concepts, err := decodeTreeFile[[]canonicalConceptDoc](item.Path, item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate concepts %s: decode payload: %w", item.Path, err)
	}
	uris := make([]string, 0, len(concepts))
	inFile := map[string]bool{}
	for _, c := range concepts {
		if strings.TrimSpace(c.URI) == "" {
			return fmt.Errorf("hydrate concepts %s: concept without uri", item.Path)
		}
		uris = append(uris, c.URI)
		inFile[c.URI] = true
	}
	for _, c := range concepts {
		for _, b := range c.Broader {
			if b == c.URI {
				return fmt.Errorf("hydrate concepts %s: concept %s lists itself as broader", item.Path, c.URI)
			}
			if !inFile[b] {
				return fmt.Errorf("hydrate concepts %s: concept %s has broader %s, which is not in this file", item.Path, c.URI, b)
			}
		}
	}

	entryIDs := make(map[string]string, len(concepts))
	for _, c := range concepts {
		var broaderURI string
		if len(c.Broader) > 0 {
			broaderURI = c.Broader[0]
		}
		config, err := conceptConfig(c)
		if err != nil {
			return fmt.Errorf("hydrate concepts %s: config of %s: %w", item.Path, c.URI, err)
		}
		row, err := q.WeaveUpsertTreeVocabularyEntry(ctx, sqlcgen.WeaveUpsertTreeVocabularyEntryParams{
			ID:           ids.GenerateULID(),
			VocabularyID: vocabularyID,
			Uri:          c.URI,
			Label:        marshalTranslationsForRestore(c.Label),
			ScopeNote:    marshalTranslationsForRestore(c.ScopeNote),
			BroaderUri:   broaderURI,
			ExternalID:   c.ExternalID,
			Config:       config,
		})
		if err != nil {
			return fmt.Errorf("hydrate concepts %s: upsert %s: %w", item.Path, c.URI, err)
		}
		entryIDs[c.URI] = row.ID
	}

	refs, err := q.WeaveListVocabularyEntryRefsNotIn(ctx, sqlcgen.WeaveListVocabularyEntryRefsNotInParams{VocabularyID: vocabularyID, Uris: uris})
	if err != nil {
		return fmt.Errorf("hydrate concepts %s: check removed concepts: %w", item.Path, err)
	}
	if len(refs) > 0 {
		used := make([]string, 0, len(refs))
		for _, r := range refs {
			used = append(used, fmt.Sprintf("%s (concept list %s)", r.Uri, r.ConceptList))
		}
		return fmt.Errorf("hydrate concepts %s: vocabulary %s: concepts removed from the file are still used: %s", item.Path, item.OwnerID, strings.Join(used, ", "))
	}
	if err := q.WeaveDeleteVocabularyEntriesNotIn(ctx, sqlcgen.WeaveDeleteVocabularyEntriesNotInParams{VocabularyID: vocabularyID, Uris: uris}); err != nil {
		return fmt.Errorf("hydrate concepts %s: remove concepts not in the file: %w", item.Path, err)
	}

	if err := q.WeaveDeleteConceptBroaderByVocabulary(ctx, vocabularyID); err != nil {
		return fmt.Errorf("hydrate concepts %s: clear broader edges: %w", item.Path, err)
	}
	for _, c := range concepts {
		for i, b := range c.Broader {
			if err := q.WeaveAddConceptBroader(ctx, sqlcgen.WeaveAddConceptBroaderParams{
				ID:        ids.GenerateULID(),
				ConceptID: entryIDs[c.URI],
				BroaderID: entryIDs[b],
				Position:  int32(i),
			}); err != nil {
				return fmt.Errorf("hydrate concepts %s: broader %s of %s: %w", item.Path, b, c.URI, err)
			}
		}
	}
	return nil
}

// conceptConfig is the entry config holding what concepts.yaml carries
// beyond the entry columns: top_concept and exact_match, each only when set;
// nil (NULL) when neither is.
func conceptConfig(c canonicalConceptDoc) ([]byte, error) {
	config := map[string]any{}
	if c.TopConcept {
		config["top_concept"] = true
	}
	if len(c.ExactMatch) > 0 {
		config["exact_match"] = c.ExactMatch
	}
	if len(config) == 0 {
		return nil, nil
	}
	return json.Marshal(config)
}

// hydrateConceptListEntriesFile recreates a list's entries in file order,
// each URI resolved in the list's vocabulary. The list's old entries were
// deleted before the concepts were loaded (see hydrateVocabularySlice).
func hydrateConceptListEntriesFile(ctx context.Context, q *sqlcgen.Queries, item SnapshotEntityFile, list treeConceptList) error {
	if list.vocabularyID == "" {
		return fmt.Errorf("hydrate concept-list entries %s: concept list %s names no vocabulary", item.Path, item.OwnerID)
	}
	uris, err := decodeTreeFile[[]string](item.Path, item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate concept-list entries %s: decode payload: %w", item.Path, err)
	}
	for i, uri := range uris {
		entry, err := q.WeaveGetVocabularyEntryByVocabularyURI(ctx, sqlcgen.WeaveGetVocabularyEntryByVocabularyURIParams{VocabularyID: list.vocabularyID, Uri: uri})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("hydrate concept-list entries %s: concept list %s: %s is not a concept of its vocabulary", item.Path, item.OwnerID, uri)
		}
		if err != nil {
			return fmt.Errorf("hydrate concept-list entries %s: resolve %s: %w", item.Path, uri, err)
		}
		if _, err := q.WeaveCreateConceptListEntry(ctx, sqlcgen.WeaveCreateConceptListEntryParams{
			ID:                ids.GenerateULID(),
			ConceptListID:     list.id,
			VocabularyEntryID: entry.ID,
			Position:          int32(i),
		}); err != nil {
			return fmt.Errorf("hydrate concept-list entries %s: add %s: %w", item.Path, uri, err)
		}
	}
	return nil
}

// prefixExpander resolves a prefix to its namespace base. Compact path
// elements in an authored tree carry qnames; the expander turns them into the
// full IRIs the rest of the system stores, so a loaded field is
// indistinguishable from one built in the UI.
type prefixExpander func(prefix string) (string, bool)

// snapshotPrefixExpander builds the expander for a tree without a database
// fallback; see snapshotPrefixExpanderWith.
func snapshotPrefixExpander(ns *projectManifestNamespaces) prefixExpander {
	return snapshotPrefixExpanderWith(ns, nil)
}

// snapshotPrefixExpanderWith resolves a prefix in this order: the project's
// effective bindings from project.yaml (first binding of a prefix wins;
// several prefixes may share one namespace), then lookup (the linked
// ontologies' own prefixes, which are not always binding rows), then the
// standard set.
func snapshotPrefixExpanderWith(ns *projectManifestNamespaces, lookup func(prefix string) (string, bool)) prefixExpander {
	bound := map[string]string{}
	if ns != nil {
		for _, b := range ns.EffectiveBindings {
			prefix, base := strings.TrimSpace(b.Prefix), strings.TrimSpace(b.Namespace)
			if prefix == "" || base == "" {
				continue
			}
			if _, seen := bound[prefix]; !seen {
				bound[prefix] = base
			}
		}
	}
	standard := nsutil.NewDefaultManager()
	return func(prefix string) (string, bool) {
		if base, ok := bound[prefix]; ok {
			return base, true
		}
		if lookup != nil {
			if base, ok := lookup(prefix); ok {
				bound[prefix] = base
				return base, true
			}
		}
		n, err := standard.GetWithPrefix(prefix)
		if err != nil || n == nil || n.URI == "" {
			return "", false
		}
		return n.URI, true
	}
}

// ontologyPrefixLookup resolves a prefix to the namespace of the ontology
// that owns it, for prefixes an authored tree uses without binding them.
func ontologyPrefixLookup(ctx context.Context, q *sqlcgen.Queries) func(prefix string) (string, bool) {
	return func(prefix string) (string, bool) {
		ont, err := q.WeaveGetOntologyByPrefix(ctx, prefix)
		if err != nil || strings.TrimSpace(ont.Namespace) == "" {
			return "", false
		}
		return ont.Namespace, true
	}
}

func hydrateFieldFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile, expand prefixExpander) error {
	doc, err := decodeTreeFile[canonicalFieldDoc](item.Path, item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate field %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate field %s: missing semantic_id", item.Path)
	}
	scope, err := decodePathElementWith(doc.OntologyScope, "class", 0, expand)
	if err != nil {
		return fmt.Errorf("hydrate field %s: parse ontology_scope: %w", item.Path, err)
	}
	pathElements, err := decodePathElementsWith(doc.PathElements, expand)
	if err != nil {
		return fmt.Errorf("hydrate field %s: parse path_elements: %w", item.Path, err)
	}
	subfieldPaths, err := decodeSubfieldPathsWith(doc.SubfieldPaths, expand)
	if err != nil {
		return fmt.Errorf("hydrate field %s: parse subfield_paths: %w", item.Path, err)
	}

	existing, err := q.WeaveGetFieldByIdentifier(ctx, sqlcgen.WeaveGetFieldByIdentifierParams{
		ProjectID:  projectID,
		SemanticID: nullableString(semanticID),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("hydrate field %s: lookup existing: %w", item.Path, err)
	}

	params := sqlcgen.WeaveCreateFieldParams{
		ID:                semanticID, // fields are keyed by their semantic id, like models and collections
		SemanticID:        nullableString(semanticID),
		SystemName:        nullableString(doc.SystemName),
		UiName:            marshalTranslationsForRestore(doc.UIName),
		Description:       marshalTranslationsForRestore(doc.Description),
		Status:            restoreStatus(doc.Status),
		ProjectID:         projectID,
		OntologyScope:     marshalJSONForRestore(scope),
		OntologyPath:      nullableString(pathString(pathElements)),
		PathElements:      marshalJSONForRestore(pathElements),
		ExpectedValueType: nullableString(doc.ExpectedValueType),
		Examples:          marshalJSONForRestore(doc.Examples),
	}

	if errors.Is(err, pgx.ErrNoRows) {
		created, err := q.WeaveCreateField(ctx, params)
		if err != nil {
			return fmt.Errorf("hydrate field %s: create: %w", item.Path, err)
		}
		if err := setFieldSubfieldPaths(ctx, q, created.ID, subfieldPaths); err != nil {
			return fmt.Errorf("hydrate field %s: set subfield_paths: %w", item.Path, err)
		}
		if err := setDeprecatedStateField(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate field %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateField(ctx, sqlcgen.WeaveUpdateFieldParams{
		ID:                existing.ID,
		UiName:            marshalTranslationsForRestore(doc.UIName),
		Description:       marshalTranslationsForRestore(doc.Description),
		SystemName:        nullableString(doc.SystemName),
		Status:            restoreStatus(doc.Status),
		OntologyScope:     marshalJSONForRestore(scope),
		OntologyPath:      nullableString(pathString(pathElements)),
		PathElements:      marshalJSONForRestore(pathElements),
		ExpectedValueType: nullableString(doc.ExpectedValueType),
		Examples:          marshalJSONForRestore(doc.Examples),
	}); err != nil {
		return fmt.Errorf("hydrate field %s: update: %w", item.Path, err)
	}
	if err := setFieldSubfieldPaths(ctx, q, existing.ID, subfieldPaths); err != nil {
		return fmt.Errorf("hydrate field %s: set subfield_paths: %w", item.Path, err)
	}
	if err := setDeprecatedStateField(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate field %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

func hydrateModelFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile, expand prefixExpander) error {
	doc, err := decodeTreeFile[canonicalModelDoc](item.Path, item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate model %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate model %s: missing semantic_id", item.Path)
	}
	scope, err := decodePathElementWith(doc.OntologyScope, "class", 0, expand)
	if err != nil {
		return fmt.Errorf("hydrate model %s: parse ontology_scope: %w", item.Path, err)
	}

	existing, err := q.WeaveGetModelByIdentifier(ctx, sqlcgen.WeaveGetModelByIdentifierParams{
		ProjectID:  projectID,
		Identifier: semanticID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("hydrate model %s: lookup existing: %w", item.Path, err)
	}

	params := sqlcgen.WeaveCreateModelParams{
		ID:            semanticID,
		SystemName:    nullableString(doc.SystemName),
		UiName:        marshalTranslationsForRestore(doc.UIName),
		Description:   marshalTranslationsForRestore(doc.Description),
		Status:        restoreStatus(doc.Status),
		ProjectID:     projectID,
		OntologyScope: marshalJSONForRestore(scope),
	}
	if errors.Is(err, pgx.ErrNoRows) {
		created, err := q.WeaveCreateModel(ctx, params)
		if err != nil {
			return fmt.Errorf("hydrate model %s: create: %w", item.Path, err)
		}
		if err := setDeprecatedStateModel(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate model %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateModel(ctx, sqlcgen.WeaveUpdateModelParams{
		ID:            existing.ID,
		UiName:        marshalTranslationsForRestore(doc.UIName),
		Description:   marshalTranslationsForRestore(doc.Description),
		SystemName:    nullableString(doc.SystemName),
		Status:        restoreStatus(doc.Status),
		OntologyScope: marshalJSONForRestore(scope),
	}); err != nil {
		return fmt.Errorf("hydrate model %s: update: %w", item.Path, err)
	}
	if err := setDeprecatedStateModel(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate model %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

func hydrateCollectionFile(ctx context.Context, q *sqlcgen.Queries, projectID string, item SnapshotEntityFile, expand prefixExpander) error {
	doc, err := decodeTreeFile[canonicalCollectionDoc](item.Path, item.Payload)
	if err != nil {
		return fmt.Errorf("hydrate collection %s: decode payload: %w", item.Path, err)
	}
	if strings.TrimSpace(doc.AdoptedFrom) != "" {
		return nil
	}
	semanticID := strings.TrimSpace(doc.SemanticID)
	if semanticID == "" {
		return fmt.Errorf("hydrate collection %s: missing semantic_id", item.Path)
	}
	scope, err := decodePathElementWith(doc.OntologyScope, "class", 0, expand)
	if err != nil {
		return fmt.Errorf("hydrate collection %s: parse ontology_scope: %w", item.Path, err)
	}

	existing, err := q.WeaveGetCollectionByIdentifier(ctx, sqlcgen.WeaveGetCollectionByIdentifierParams{
		ProjectID:  projectID,
		Identifier: semanticID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("hydrate collection %s: lookup existing: %w", item.Path, err)
	}

	params := sqlcgen.WeaveCreateCollectionParams{
		ID:                       semanticID,
		SystemName:               nullableString(doc.SystemName),
		UiName:                   marshalTranslationsForRestore(doc.UIName),
		Description:              marshalTranslationsForRestore(doc.Description),
		Status:                   restoreStatus(doc.Status),
		ProjectID:                projectID,
		OntologyScope:            marshalJSONForRestore(scope),
		CollectionNumber:         int32Ptr(doc.CollectionNumber),
		CanonicalCollectionOrder: int32Ptr(doc.CanonicalCollectionOrder),
	}
	if errors.Is(err, pgx.ErrNoRows) {
		created, err := q.WeaveCreateCollection(ctx, params)
		if err != nil {
			return fmt.Errorf("hydrate collection %s: create: %w", item.Path, err)
		}
		if err := setDeprecatedStateCollection(ctx, q, created.ID, doc.Deprecated); err != nil {
			return fmt.Errorf("hydrate collection %s: set deprecated: %w", item.Path, err)
		}
		return nil
	}

	if _, err := q.WeaveUpdateCollection(ctx, sqlcgen.WeaveUpdateCollectionParams{
		ID:                       existing.ID,
		UiName:                   marshalTranslationsForRestore(doc.UIName),
		Description:              marshalTranslationsForRestore(doc.Description),
		SystemName:               nullableString(doc.SystemName),
		Status:                   restoreStatus(doc.Status),
		OntologyScope:            marshalJSONForRestore(scope),
		CollectionNumber:         int32Ptr(doc.CollectionNumber),
		CanonicalCollectionOrder: int32Ptr(doc.CanonicalCollectionOrder),
	}); err != nil {
		return fmt.Errorf("hydrate collection %s: update: %w", item.Path, err)
	}
	if err := setDeprecatedStateCollection(ctx, q, existing.ID, doc.Deprecated); err != nil {
		return fmt.Errorf("hydrate collection %s: set deprecated: %w", item.Path, err)
	}
	return nil
}

// setFieldSubfieldPaths persists the restored subfield paths; nil keeps the
// column NULL (the common non-legacy case).
func setFieldSubfieldPaths(ctx context.Context, q *sqlcgen.Queries, fieldID string, subfields []domain.SubfieldPath) error {
	if subfields == nil {
		return nil
	}
	return q.WeaveSetFieldSubfieldPaths(ctx, sqlcgen.WeaveSetFieldSubfieldPathsParams{
		ID:            fieldID,
		SubfieldPaths: marshalJSONForRestore(subfields),
	})
}

func marshalTranslationsForRestore(t domain.Translations) []byte {
	if t == nil {
		return nil
	}
	raw, _ := json.Marshal(t)
	return raw
}

func marshalJSONForRestore(v any) []byte {
	if v == nil {
		return nil
	}
	raw, _ := json.Marshal(v)
	return raw
}

func restoreStatus(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "draft"
	}
	return v
}

// decodePathElement restores one snapshot element. The lossless full-map
// form decodes straight into domain.PathElement — every stored attribute
// (type, position, class_code, instance_id, datatype, additional_types,
// sub_property_of, complete) survives verbatim. The legacy compact string
// form falls back to parsePathElement's best-effort reconstruction.
func decodePathElement(raw canonicalPathElement, defaultType string, position int) (domain.PathElement, error) {
	return decodePathElementWith(raw, defaultType, position, nil)
}

// decodePathElementWith decodes one element. A full map is taken verbatim.
// A compact qname string is typed by defaultType and, when an expander is
// given, gets its IRI from the prefix; an unbound prefix is an error. With a
// nil expander the IRI stays the qname (legacy callers and tests).
func decodePathElementWith(raw canonicalPathElement, defaultType string, position int, expand prefixExpander) (domain.PathElement, error) {
	if len(raw) == 0 {
		return domain.PathElement{}, nil
	}
	var compact string
	if err := json.Unmarshal(raw, &compact); err == nil {
		pe, err := parsePathElement(compact, defaultType, position)
		if err != nil {
			return domain.PathElement{}, err
		}
		if expand != nil {
			base, ok := expand(pe.Prefix)
			if !ok {
				return domain.PathElement{}, fmt.Errorf("unbound prefix %q in %q", pe.Prefix, compact)
			}
			pe.URI = base + pe.LocalName
		}
		return pe, nil
	}
	var pe domain.PathElement
	if err := json.Unmarshal(raw, &pe); err != nil {
		return domain.PathElement{}, fmt.Errorf("decode path element: %w", err)
	}
	return pe, nil
}

func decodePathElements(items []canonicalPathElement) ([]domain.PathElement, error) {
	return decodePathElementsWith(items, nil)
}

// decodePathElementsWith restores a field's path. The full map form carries
// its own type and IRI; the compact qname form (an authored tree) is typed by
// position: a path alternates property, class, property, ... and a literal
// terminal (xsd:*, the rdf datatypes, rdfs:Literal, rdfs:Resource) is a
// literal whatever its position.
func decodePathElementsWith(items []canonicalPathElement, expand prefixExpander) ([]domain.PathElement, error) {
	out := make([]domain.PathElement, 0, len(items))
	for i, item := range items {
		defaultType := "property"
		if i%2 == 1 {
			defaultType = "class"
		}
		pe, err := decodePathElementWith(item, defaultType, i, expand)
		if err != nil {
			return nil, err
		}
		if isCompactPathElement(item) && isLiteralPathElement(pe) {
			pe.Type = "literal"
		}
		out = append(out, pe)
	}
	return out, nil
}

// isCompactPathElement reports whether a raw element is the authored qname
// string form rather than the full map the materializer writes.
func isCompactPathElement(raw canonicalPathElement) bool {
	var compact string
	return json.Unmarshal(raw, &compact) == nil
}

// isLiteralPathElement reports whether a compact path element names a literal
// type rather than a class: any xsd datatype, the rdf datatypes, the generic
// rdfs:Literal, or rdfs:Resource (an IRI value, which the path builder also
// ends a path with). rdf:type and rdf:value are properties and stay typed by
// position.
func isLiteralPathElement(pe domain.PathElement) bool {
	switch pe.Prefix {
	case "xsd":
		return true
	case "rdf":
		switch pe.LocalName {
		case "langString", "HTML", "XMLLiteral", "JSON", "PlainLiteral":
			return true
		}
		return false
	case "rdfs":
		return pe.LocalName == "Literal" || pe.LocalName == "Resource"
	}
	return false
}

// decodeSubfieldPaths restores the legacy subfield paths with the same
// element fidelity as the primary path. Returns nil for none, so the
// subfield_paths column stays NULL rather than becoming an empty array.
func decodeSubfieldPaths(items []canonicalSubfieldPathDoc) ([]domain.SubfieldPath, error) {
	return decodeSubfieldPathsWith(items, nil)
}

func decodeSubfieldPathsWith(items []canonicalSubfieldPathDoc, expand prefixExpander) ([]domain.SubfieldPath, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]domain.SubfieldPath, 0, len(items))
	for _, item := range items {
		elements, err := decodePathElementsWith(item.PathElements, expand)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.SubfieldPath{
			PathElements:      elements,
			ExpectedValueType: item.ExpectedValueType,
			Scope:             item.Scope,
			Source:            item.Source,
		})
	}
	return out, nil
}

func parsePathElement(input, defaultType string, position int) (domain.PathElement, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return domain.PathElement{}, nil
	}
	parts := strings.Split(input, "/")
	primary, err := parseSingleTypeRef(parts[0])
	if err != nil {
		return domain.PathElement{}, err
	}
	pe := domain.PathElement{
		Type:      defaultType,
		URI:       primary.PrefixedName(),
		Prefix:    primary.Prefix,
		LocalName: primary.LocalName,
		ClassCode: primary.ClassCode,
		Position:  position,
	}
	for _, raw := range parts[1:] {
		ref, err := parseSingleTypeRef(raw)
		if err != nil {
			return domain.PathElement{}, err
		}
		pe.AdditionalTypes = append(pe.AdditionalTypes, ref)
	}
	if pe.Type == "" {
		switch pe.Prefix {
		case "xsd", "rdf":
			pe.Type = "literal"
		default:
			pe.Type = "property"
		}
	}
	return pe, nil
}

func parseSingleTypeRef(input string) (domain.TypeRef, error) {
	input = strings.TrimSpace(input)
	parts := strings.SplitN(input, ":", 2)
	if len(parts) != 2 {
		return domain.TypeRef{}, fmt.Errorf("invalid prefixed name %q", input)
	}
	return domain.TypeRef{
		URI:       input,
		Prefix:    strings.TrimSpace(parts[0]),
		LocalName: strings.TrimSpace(parts[1]),
		ClassCode: strings.TrimSpace(parts[1]),
	}, nil
}

// pathString rebuilds the derived ontology_path cache column in the exact
// format the importer writes: "->qname" per element, with the legacy
// instance id appended in brackets ("crm:E55_Type[AME.1_1]") when present —
// so a restored row's cache is byte-identical to the source row's.
func pathString(elements []domain.PathElement) string {
	if len(elements) == 0 {
		return ""
	}
	var b strings.Builder
	for _, pe := range elements {
		b.WriteString("->")
		b.WriteString(pe.PrefixedName())
		if pe.InstanceID != "" {
			b.WriteString("[")
			b.WriteString(pe.InstanceID)
			b.WriteString("]")
		}
	}
	return b.String()
}

func setDeprecatedStateCategory(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateCategory(ctx, id)
	}
	return q.WeaveActivateCategory(ctx, id)
}

func setDeprecatedStateField(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateField(ctx, id)
	}
	return q.WeaveActivateField(ctx, id)
}

func setDeprecatedStateModel(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateModel(ctx, id)
	}
	return q.WeaveActivateModel(ctx, id)
}

func setDeprecatedStateCollection(ctx context.Context, q *sqlcgen.Queries, id string, deprecated bool) error {
	if deprecated {
		return q.WeaveDeprecateCollection(ctx, id)
	}
	return q.WeaveActivateCollection(ctx, id)
}

func int32Ptr(v int) *int32 {
	val := int32(v)
	return &val
}

func hasAdoptionMarker(payload []byte) bool {
	var raw map[string]any
	if err := yaml.Unmarshal(payload, &raw); err != nil {
		return false
	}
	_, ok := raw["_adopted_from"]
	return ok
}
