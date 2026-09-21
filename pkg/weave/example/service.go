package example

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/ids"
)

// expectedValueTypeModel is the ResolvedField.ExpectedValueType value for
// fields that reference a single Model entity.
const expectedValueTypeModel = "Model"

type ViewReader interface {
	ModelView(ctx context.Context, modelID, projectID string) (*domain.ModelView, error)
}

type Service struct {
	store Store
	views ViewReader
}

type conceptValidator interface {
	ConceptURIAllowedForLists(ctx context.Context, uri string, conceptListIDs []string) (bool, error)
}

// modelNamer is satisfied by the real weave store; used to show the target
// model's display name in the example form instead of its id.
type modelNamer interface {
	Models() domain.WeaveModelStore
}

// TargetName returns the model's display name when the view reader can look
// it up, else the id.
func (s *Service) TargetName(ctx context.Context, modelID string) domain.Translations {
	fallback := domain.Translations{"en": modelID}
	namer, ok := s.views.(modelNamer)
	if !ok {
		return fallback
	}
	m, err := namer.Models().GetByID(ctx, modelID)
	if err != nil || m == nil || len(m.UIName) == 0 {
		return fallback
	}
	return m.UIName
}

func NewService(store Store, views ViewReader) *Service {
	return &Service{store: store, views: views}
}

type CreateInput struct {
	EntityType  domain.ExampleEntityType `json:"entity_type"`
	EntityID    string                   `json:"entity_id"`
	Title       domain.Translations      `json:"title,omitempty"`
	Description domain.Translations      `json:"description,omitempty"`
	Values      []domain.ExampleValue    `json:"values,omitempty"`
	// Lang is the form's primary language; stub examples created from a
	// typed label store the label under it. Defaults to "en".
	Lang string `json:"lang,omitempty"`
}

type UpdateInput struct {
	Title       *domain.Translations  `json:"title,omitempty"`
	Description *domain.Translations  `json:"description,omitempty"`
	Values      []domain.ExampleValue `json:"values,omitempty"`
	Lang        string                `json:"lang,omitempty"`
}

type ExampleRecord struct {
	Example    *domain.Example                `json:"example"`
	Values     []domain.ExampleValue          `json:"values"`
	Validation domain.ExampleValidationReport `json:"validation"`
}

type ExampleFormSchema struct {
	Kind      string                     `json:"kind"`
	ExampleID string                     `json:"example_id,omitempty"`
	Target    ExampleFormTarget          `json:"target"`
	Endpoint  *formschema.SchemaEndpoint `json:"endpoint,omitempty"`
	Sections  []ExampleFormSection       `json:"sections"`
	Issues    []domain.ExampleIssue      `json:"issues,omitempty"`
	UI        formschema.SchemaUI        `json:"ui"`
}

type ExampleFormTarget struct {
	EntityType string              `json:"entity_type"`
	EntityID   string              `json:"entity_id"`
	Name       domain.Translations `json:"name,omitempty"`
}

type ExampleFormSection struct {
	ID             string              `json:"id"`
	Label          domain.Translations `json:"label"`
	CanonicalOrder int                 `json:"canonical_order,omitempty"`
	Groups         []ExampleFormGroup  `json:"groups,omitempty"`
	DirectFields   []ExampleFormField  `json:"direct_fields,omitempty"`
}

type ExampleFormGroup struct {
	ID               string               `json:"id"`
	Label            domain.Translations  `json:"label"`
	Position         int                  `json:"position,omitempty"`
	SharedPathPrefix []domain.PathElement `json:"shared_path_prefix,omitempty"`
	Fields           []ExampleFormField   `json:"fields"`
	// Instance is this entry's group instance; a repeatable group appears
	// once per instance, consecutively.
	Instance int `json:"instance"`
	// SlotPrefix is prepended to "<override>:<occurrence>" to form a value's
	// slot_path: "LAC.1:1/" in a collection group, "" for direct fields.
	SlotPrefix string                `json:"slot_prefix"`
	Repeatable bool                  `json:"repeatable,omitempty"`
	MinOccurs  int                   `json:"min_occurs,omitempty"`
	MaxOccurs  *int                  `json:"max_occurs,omitempty"`
	Issues     []domain.ExampleIssue `json:"issues,omitempty"`
}

type ExampleOccurrence struct {
	OccurrenceIndex int                        `json:"occurrence_index"`
	Value           domain.ExampleValuePayload `json:"value,omitempty"`
	Issues          []domain.ExampleIssue      `json:"issues,omitempty"`
}

type ExampleFormField struct {
	OverrideID        int64                   `json:"override_id"`
	FieldID           string                  `json:"field_id"`
	FieldSemanticID   string                  `json:"field_semantic_id"`
	Label             domain.Translations     `json:"label"`
	Help              domain.Translations     `json:"help,omitempty"`
	Widget            string                  `json:"widget"`
	ValueKind         domain.ExampleValueKind `json:"value_kind"`
	ExpectedValueType string                  `json:"expected_value_type,omitempty"`
	Required          bool                    `json:"required,omitempty"`
	Repeatable        bool                    `json:"repeatable,omitempty"`
	MinOccurs         int                     `json:"min_occurs,omitempty"`
	MaxOccurs         *int                    `json:"max_occurs,omitempty"`
	Hidden            bool                    `json:"hidden,omitempty"`
	SetValue          string                  `json:"set_value,omitempty"`
	ResourceModels    []domain.EntityRef      `json:"resource_models,omitempty"`
	CollectionModels  []domain.EntityRef      `json:"collection_models,omitempty"`
	ConceptLists      []domain.EntityRef      `json:"concept_lists,omitempty"`
	ConceptSources    []ConceptListSource     `json:"concept_sources,omitempty"`
	Issues            []domain.ExampleIssue   `json:"issues,omitempty"`
	Occurrences       []ExampleOccurrence     `json:"occurrences,omitempty"`
	SlotPrefix        string                  `json:"slot_prefix,omitempty"`
}

type ConceptListSource struct {
	ID         string              `json:"id"`
	SemanticID string              `json:"semantic_id,omitempty"`
	Name       domain.Translations `json:"name,omitempty"`
	URL        string              `json:"url,omitempty"`
	SearchURL  string              `json:"search_url"`
}

func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*ExampleRecord, error) {
	if in.EntityType != domain.ExampleEntityTypeModel {
		return nil, fmt.Errorf("unsupported example target type: %s", in.EntityType)
	}
	if strings.TrimSpace(in.EntityID) == "" {
		return nil, fmt.Errorf("entity_id is required")
	}
	ex := &domain.Example{
		ID:          ids.GenerateULID(),
		ProjectID:   projectID,
		EntityType:  in.EntityType,
		EntityID:    in.EntityID,
		Title:       in.Title,
		Description: in.Description,
		Status:      domain.ExampleStatusDraft,
	}
	if err := s.materializeStubs(ctx, projectID, in.EntityID, in.Lang, in.Values); err != nil {
		return nil, err
	}
	values, err := normalizeValues(in.Values)
	if err != nil {
		return nil, err
	}
	report, err := s.validateModelValues(ctx, projectID, in.EntityID, values)
	if err != nil {
		return nil, err
	}
	report.ExampleID = ex.ID
	ex.Status = deriveStatus(values, report)
	for i := range values {
		values[i].ExampleID = ex.ID
	}
	if err := s.store.CreateWithValues(ctx, ex, values); err != nil {
		return nil, err
	}
	return &ExampleRecord{Example: ex, Values: values, Validation: report}, nil
}

func (s *Service) Update(ctx context.Context, projectID, exampleID string, in UpdateInput) (*ExampleRecord, error) {
	ex, err := s.store.GetByID(ctx, exampleID)
	if err != nil {
		return nil, err
	}
	if ex == nil || ex.ProjectID != projectID {
		return nil, fmt.Errorf("example not found")
	}
	if in.Title != nil {
		ex.Title = *in.Title
	}
	if in.Description != nil {
		ex.Description = *in.Description
	}
	if err := s.materializeStubs(ctx, projectID, ex.EntityID, in.Lang, in.Values); err != nil {
		return nil, err
	}
	values, err := normalizeValues(in.Values)
	if err != nil {
		return nil, err
	}
	report, err := s.validateModelValues(ctx, projectID, ex.EntityID, values)
	if err != nil {
		return nil, err
	}
	report.ExampleID = ex.ID
	ex.Status = deriveStatus(values, report)
	for i := range values {
		values[i].ExampleID = ex.ID
	}
	if err := s.store.UpdateWithValues(ctx, ex, values); err != nil {
		return nil, err
	}
	return &ExampleRecord{Example: ex, Values: values, Validation: report}, nil
}

// stubCandidate is a validated "reference by label" value awaiting draft
// creation: the index into the values slice it came from, the resolved
// target model, and the draft's title.
type stubCandidate struct {
	index  int
	target string
	label  string
}

// materializeStubs turns "reference by label" payloads into real draft
// examples. A value whose payload is example_ref with no example_id but a
// non-blank target_label creates a draft example of the target model
// (title = label, no values) and links it by id. The target model comes
// from target_entity_id, or is inferred when the field allows exactly one
// resource model. Values are mutated in place; callers normalize afterwards
// so the linked_example_id column follows.
//
// Resolution and validation of every stub value happens first, so a guard
// failure on a later value never leaves an earlier value's draft created;
// only then does a second pass create the drafts and link them.
//
// ponytail: only a DB error on the parent save can now leave an unlinked
// draft — stubs are still created before the parent is saved and are not
// rolled back if that save fails afterwards; they are drafts and harmless.
// Wrap in one transaction if that ever bites.
func (s *Service) materializeStubs(ctx context.Context, projectID, modelID, lang string, values []domain.ExampleValue) error {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "en"
	}
	var fieldByOverride map[int64]domain.ResolvedField
	var candidates []stubCandidate
	for i := range values {
		p := &values[i].ValuePayload
		if p.Kind != domain.ExampleValueKindExampleRef || p.ExampleID != nil || p.TargetLabel == nil {
			continue
		}
		label := strings.TrimSpace(*p.TargetLabel)
		if label == "" {
			continue
		}
		if fieldByOverride == nil {
			view, err := s.views.ModelView(ctx, modelID, projectID)
			if err != nil {
				return err
			}
			fieldByOverride = buildFieldByOverride(view)
		}
		field, ok := fieldByOverride[values[i].OverrideID]
		if !ok {
			continue // validation reports stale_override
		}
		target, err := resolveStubTarget(field, p)
		if err != nil {
			return err
		}
		candidates = append(candidates, stubCandidate{index: i, target: target, label: label})
	}
	// One draft per (target model, label) within this save: the same new
	// entity typed into two fields links one record, not two.
	created := map[string]string{}
	for _, c := range candidates {
		key := c.target + "\x00" + c.label
		id, ok := created[key]
		if !ok {
			stub := &domain.Example{
				ID:         ids.GenerateULID(),
				ProjectID:  projectID,
				EntityType: domain.ExampleEntityTypeModel,
				EntityID:   c.target,
				Title:      domain.Translations{lang: c.label},
				Status:     domain.ExampleStatusDraft,
			}
			if err := s.store.CreateWithValues(ctx, stub, nil); err != nil {
				return fmt.Errorf("create draft example for %s: %w", c.target, err)
			}
			id = stub.ID
			created[key] = id
		}
		p := &values[c.index].ValuePayload
		p.ExampleID = &id
		target := c.target
		p.TargetEntityID = &target
	}
	return nil
}

// buildFieldByOverride flattens a model view's category/collection/field
// tree into a lookup keyed by override id, for resolving the field behind a
// value's OverrideID.
func buildFieldByOverride(view *domain.ModelView) map[int64]domain.ResolvedField {
	fieldByOverride := map[int64]domain.ResolvedField{}
	for _, cat := range view.Categories {
		for _, coll := range cat.Collections {
			for _, f := range coll.Fields {
				fieldByOverride[f.OverrideID] = f
			}
		}
	}
	return fieldByOverride
}

// resolveStubTarget returns the model a typed-label stub should be created
// for, or a guard error. Exactly one of the three error messages is kept
// byte-for-byte (tests assert substrings).
func resolveStubTarget(field domain.ResolvedField, p *domain.ExampleValuePayload) (string, error) {
	if strings.TrimSpace(field.ExpectedValueType) != expectedValueTypeModel {
		return "", fmt.Errorf("field %s: a new draft can only be created for Model-typed fields", field.ID)
	}
	target := ""
	if p.TargetEntityID != nil {
		target = strings.TrimSpace(*p.TargetEntityID)
	}
	if target == "" && len(field.ResourceModels) == 1 {
		target = field.ResourceModels[0].ID
	}
	if target == "" {
		return "", fmt.Errorf("field %s: target_entity_id is required to create a draft (field allows %d models)", field.ID, len(field.ResourceModels))
	}
	if len(field.ResourceModels) > 0 && !slices.ContainsFunc(field.ResourceModels, func(ref domain.EntityRef) bool { return ref.ID == target }) {
		return "", fmt.Errorf("field %s: model %s is not an allowed target", field.ID, target)
	}
	return target, nil
}

func (s *Service) Get(ctx context.Context, projectID, exampleID string) (*ExampleRecord, error) {
	ex, err := s.store.GetByID(ctx, exampleID)
	if err != nil {
		return nil, err
	}
	if ex == nil || ex.ProjectID != projectID {
		return nil, fmt.Errorf("example not found")
	}
	values, err := s.store.ListValues(ctx, exampleID)
	if err != nil {
		return nil, err
	}
	report, err := s.validateModelValues(ctx, projectID, ex.EntityID, values)
	if err != nil {
		return nil, err
	}
	report.ExampleID = ex.ID
	return &ExampleRecord{Example: ex, Values: values, Validation: report}, nil
}

func (s *Service) Delete(ctx context.Context, projectID, exampleID string) error {
	ex, err := s.store.GetByID(ctx, exampleID)
	if err != nil {
		return err
	}
	if ex == nil || ex.ProjectID != projectID {
		return fmt.Errorf("example not found")
	}
	return s.store.Delete(ctx, exampleID)
}

func (s *Service) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Example, int64, error) {
	opts = append([]domain.QueryOption{domain.WithProjectID(projectID)}, opts...)
	return s.store.List(ctx, opts...)
}

func (s *Service) BuildFormSchema(ctx context.Context, projectID, mode, targetType, targetID, exampleID, lang string, languages []formschema.LanguageInfo) (*ExampleFormSchema, error) {
	if mode == "" {
		mode = formschema.ModeCreate
	}
	switch mode {
	case formschema.ModeCreate:
		if targetType != string(domain.ExampleEntityTypeModel) || strings.TrimSpace(targetID) == "" {
			return nil, fmt.Errorf("target_type=model and target_id are required")
		}
		return s.buildModelFormSchema(ctx, projectID, "", targetID, nil, lang, languages)
	case formschema.ModeEdit:
		record, err := s.Get(ctx, projectID, exampleID)
		if err != nil {
			return nil, err
		}
		return s.buildModelFormSchema(ctx, projectID, record.Example.ID, record.Example.EntityID, record, lang, languages)
	default:
		return nil, fmt.Errorf("unsupported form mode: %s", mode)
	}
}

func (s *Service) buildModelFormSchema(ctx context.Context, projectID, exampleID, modelID string, record *ExampleRecord, lang string, languages []formschema.LanguageInfo) (*ExampleFormSchema, error) {
	view, err := s.views.ModelView(ctx, modelID, projectID)
	if err != nil {
		return nil, err
	}
	byInstance := map[string][]domain.ExampleValue{}
	issuesByKey := map[string][]domain.ExampleIssue{}
	groupIssues := map[string][]domain.ExampleIssue{}
	present := map[string]map[string]bool{}
	var topIssues []domain.ExampleIssue
	if record != nil {
		for _, v := range record.Values {
			gp := instancePath(v.SlotPath)
			byInstance[slotCountKey(gp, v.OverrideID)] = append(byInstance[slotCountKey(gp, v.OverrideID)], v)
			if coll, _, ok := domain.ParseExampleGroupSegment(gp); ok {
				if present[coll] == nil {
					present[coll] = map[string]bool{}
				}
				present[coll][gp] = true
			}
		}
		for _, issue := range record.Validation.Issues {
			switch {
			case issue.CollectionID != nil:
				groupIssues[*issue.CollectionID] = append(groupIssues[*issue.CollectionID], issue)
			case issue.OverrideID == nil:
				topIssues = append(topIssues, issue)
			default:
				idx := -1
				if issue.OccurrenceIndex != nil {
					idx = *issue.OccurrenceIndex
				}
				key := issueKey(issue.GroupPath, *issue.OverrideID, idx)
				issuesByKey[key] = append(issuesByKey[key], issue)
			}
		}
	}
	sections := make([]ExampleFormSection, 0, len(view.Categories))
	for _, cat := range view.Categories {
		section := ExampleFormSection{
			ID:             cat.ID,
			Label:          cat.Name,
			CanonicalOrder: cat.Position,
		}
		for _, coll := range cat.Collections {
			section.Groups = append(section.Groups, buildGroupEntries(coll, byInstance, issuesByKey, groupIssues, present[coll.ID])...)
		}
		sections = append(sections, section)
	}
	schema := &ExampleFormSchema{
		Kind:      "example-form",
		ExampleID: exampleID,
		Target: ExampleFormTarget{
			EntityType: string(domain.ExampleEntityTypeModel),
			EntityID:   modelID,
			Name:       s.TargetName(ctx, modelID),
		},
		Sections: sections,
		Issues:   topIssues,
		UI: formschema.SchemaUI{
			SubmitLabel:    domain.Translations{"en": submitLabel(exampleID)},
			CancelLabel:    domain.Translations{"en": "Cancel"},
			SuccessMessage: domain.Translations{"en": successMessage(exampleID)},
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}
	if exampleID == "" {
		schema.Endpoint = &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/examples", projectID),
		}
	} else {
		schema.Endpoint = &formschema.SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/projects/%s/examples/%s", projectID, exampleID),
		}
	}
	return schema, nil
}

func buildExampleField(f domain.ResolvedField, values []domain.ExampleValue, issuesByKey map[string][]domain.ExampleIssue, groupPath string) ExampleFormField {
	out := ExampleFormField{
		OverrideID:        f.OverrideID,
		FieldID:           f.ID,
		FieldSemanticID:   f.SemanticID,
		Label:             f.DisplayName,
		Help:              f.Description,
		Widget:            widgetForExpectedType(f.ExpectedValueType),
		ValueKind:         valueKindForExpectedType(f.ExpectedValueType),
		ExpectedValueType: f.ExpectedValueType,
		Required:          f.IsRequired,
		Repeatable:        f.MaxOccurs == nil || *f.MaxOccurs > 1 || f.MinOccurs > 1,
		MinOccurs:         f.MinOccurs,
		MaxOccurs:         f.MaxOccurs,
		Hidden:            f.IsHidden,
		SetValue:          f.SetValue,
		ResourceModels:    f.ResourceModels,
		CollectionModels:  f.CollectionModels,
		ConceptLists:      f.ConceptLists,
		ConceptSources:    conceptSources(f.ConceptLists),
		Issues:            issuesByKey[issueKey(groupPath, f.OverrideID, -1)],
	}
	if len(values) == 0 {
		out.Occurrences = []ExampleOccurrence{{OccurrenceIndex: 0}}
		return out
	}
	sort.Slice(values, func(i, j int) bool { return values[i].OccurrenceIndex < values[j].OccurrenceIndex })
	out.Occurrences = make([]ExampleOccurrence, 0, len(values))
	for _, value := range values {
		key := issueKey(groupPath, f.OverrideID, value.OccurrenceIndex)
		out.Occurrences = append(out.Occurrences, ExampleOccurrence{
			OccurrenceIndex: value.OccurrenceIndex,
			Value:           value.ValuePayload,
			Issues:          issuesByKey[key],
		})
	}
	return out
}

// formInstances lists the group instances the form shows: "" for the direct
// bucket; otherwise the validated instances padded to the placement minimum.
func formInstances(coll domain.CollectionGroup, present map[string]bool) []string {
	if coll.ID == directGroupID {
		return []string{""}
	}
	out := groupInstances(coll.ID, present)
	if coll.Placement == nil {
		return out
	}
	_, next, _ := domain.ParseExampleGroupSegment(out[len(out)-1])
	for len(out) < coll.Placement.MinOccurs {
		next++
		out = append(out, domain.ExampleGroupSegment(coll.ID, next))
	}
	return out
}

// buildGroupEntries returns one ExampleFormGroup per instance of coll: one
// entry for the direct bucket, or one per validated/padded instance of a
// collection group. Split out of buildModelFormSchema to keep it within the
// gocyclo limit.
func buildGroupEntries(coll domain.CollectionGroup, byInstance map[string][]domain.ExampleValue, issuesByKey map[string][]domain.ExampleIssue, groupIssues map[string][]domain.ExampleIssue, present map[string]bool) []ExampleFormGroup {
	instances := formInstances(coll, present)
	out := make([]ExampleFormGroup, 0, len(instances))
	for i, gp := range instances {
		prefix, instance := "", 0
		if gp != "" {
			prefix = gp + "/"
			_, instance, _ = domain.ParseExampleGroupSegment(gp)
		}
		fields := make([]ExampleFormField, 0, len(coll.Fields))
		for _, f := range coll.Fields {
			field := buildExampleField(f, byInstance[slotCountKey(gp, f.OverrideID)], issuesByKey, gp)
			field.SlotPrefix = prefix
			fields = append(fields, field)
		}
		out = append(out, ExampleFormGroup{
			ID:               coll.ID,
			Label:            coll.Name,
			Position:         coll.Position,
			SharedPathPrefix: coll.SharedPathPrefix,
			Fields:           fields,
			Instance:         instance,
			SlotPrefix:       prefix,
			Repeatable:       groupRepeatable(coll),
			MinOccurs:        placementMin(coll),
			MaxOccurs:        placementMax(coll),
			Issues:           issuesIf(i == 0, groupIssues[coll.ID]),
		})
	}
	return out
}

// groupRepeatable reports whether a collection group's form entries allow
// adding another instance: never for the direct bucket, otherwise unless a
// placement caps it at exactly one instance.
func groupRepeatable(coll domain.CollectionGroup) bool {
	if coll.ID == directGroupID {
		return false
	}
	return coll.Placement == nil || coll.Placement.MaxOccurs == nil || *coll.Placement.MaxOccurs > 1
}

func placementMin(coll domain.CollectionGroup) int {
	if coll.Placement == nil {
		return 0
	}
	return coll.Placement.MinOccurs
}

func placementMax(coll domain.CollectionGroup) *int {
	if coll.Placement == nil {
		return nil
	}
	return coll.Placement.MaxOccurs
}

// issuesIf returns issues only when first is true, so a group-level issue
// attaches to a single instance's entry rather than repeating on every one.
func issuesIf(first bool, issues []domain.ExampleIssue) []domain.ExampleIssue {
	if !first {
		return nil
	}
	return issues
}

// normalizeValues copies values, fixes their kinds and columns, and settles
// the slot path: filled from override+occurrence when absent, or the leaf
// override+occurrence derived from it when only the path was sent. A
// malformed path is an input error.
func normalizeValues(values []domain.ExampleValue) ([]domain.ExampleValue, error) {
	out := make([]domain.ExampleValue, len(values))
	copy(out, values)
	for i := range out {
		if err := settleSlot(&out[i]); err != nil {
			return nil, err
		}
		out[i].ValueKind = normalizePayload(&out[i].ValuePayload, out[i].ValueKind)
		projectPayloadColumns(&out[i])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OverrideID != out[j].OverrideID {
			return out[i].OverrideID < out[j].OverrideID
		}
		if out[i].OccurrenceIndex != out[j].OccurrenceIndex {
			return out[i].OccurrenceIndex < out[j].OccurrenceIndex
		}
		return out[i].SlotPath < out[j].SlotPath
	})
	return out, nil
}

// settleSlot reconciles SlotPath with OverrideID/OccurrenceIndex.
func settleSlot(v *domain.ExampleValue) error {
	if v.SlotPath == "" {
		v.SlotPath = domain.ExampleSlot(v.OverrideID, v.OccurrenceIndex)
		return nil
	}
	oid, occ, ok := domain.ParseExampleSlotLeaf(v.SlotPath)
	if !ok {
		return fmt.Errorf("value for field %s: malformed slot_path %q", v.FieldID, v.SlotPath)
	}
	if domain.ExampleSlotDepth(v.SlotPath) > 2 {
		return fmt.Errorf("value for field %s: nested slot_path %q is not supported yet", v.FieldID, v.SlotPath)
	}
	if v.OverrideID == 0 {
		v.OverrideID = oid
		v.OccurrenceIndex = occ
	} else if v.OverrideID != oid || v.OccurrenceIndex != occ {
		return fmt.Errorf("value for field %s: slot_path %q disagrees with override_id %d / occurrence_index %d", v.FieldID, v.SlotPath, v.OverrideID, v.OccurrenceIndex)
	}
	return nil
}

func normalizePayload(payload *domain.ExampleValuePayload, fallback domain.ExampleValueKind) domain.ExampleValueKind {
	if payload == nil {
		return fallback
	}
	if payload.Kind == "" {
		payload.Kind = fallback
	}
	switch payload.Kind {
	case domain.ExampleValueKindString, domain.ExampleValueKindInteger, domain.ExampleValueKindDate, domain.ExampleValueKindURI, domain.ExampleValueKindConcept, domain.ExampleValueKindExampleRef:
		return payload.Kind
	default:
		switch {
		case payload.StringValue != nil:
			payload.Kind = domain.ExampleValueKindString
		case payload.NumberValue != nil:
			payload.Kind = domain.ExampleValueKindInteger
		case payload.DateValue != nil:
			payload.Kind = domain.ExampleValueKindDate
		case payload.URIValue != nil:
			payload.Kind = domain.ExampleValueKindURI
		case payload.ConceptURI != nil:
			payload.Kind = domain.ExampleValueKindConcept
		case payload.ExampleID != nil:
			payload.Kind = domain.ExampleValueKindExampleRef
		default:
			payload.Kind = fallback
		}
		return payload.Kind
	}
}

func projectPayloadColumns(v *domain.ExampleValue) {
	v.TextValue = nil
	v.NumberValue = nil
	v.DateValue = nil
	v.URIValue = nil
	v.ConceptURI = nil
	v.LinkedExampleID = nil
	switch v.ValuePayload.Kind {
	case domain.ExampleValueKindString:
		v.TextValue = v.ValuePayload.StringValue
	case domain.ExampleValueKindInteger:
		v.NumberValue = v.ValuePayload.NumberValue
	case domain.ExampleValueKindDate:
		v.DateValue = v.ValuePayload.DateValue
	case domain.ExampleValueKindURI:
		v.URIValue = v.ValuePayload.URIValue
	case domain.ExampleValueKindConcept:
		v.ConceptURI = v.ValuePayload.ConceptURI
	case domain.ExampleValueKindExampleRef:
		v.LinkedExampleID = v.ValuePayload.ExampleID
	}
}

func deriveStatus(values []domain.ExampleValue, report domain.ExampleValidationReport) domain.ExampleStatus {
	if len(values) == 0 {
		return domain.ExampleStatusDraft
	}
	if len(report.Issues) == 0 {
		return domain.ExampleStatusValid
	}
	return domain.ExampleStatusHasIssues
}

func (s *Service) validateModelValues(ctx context.Context, projectID, modelID string, values []domain.ExampleValue) (domain.ExampleValidationReport, error) {
	view, err := s.views.ModelView(ctx, modelID, projectID)
	if err != nil {
		return domain.ExampleValidationReport{}, err
	}
	if err := placeInGroups(values, groupOfOverride(view)); err != nil {
		return domain.ExampleValidationReport{}, err
	}
	fieldByOverride := map[int64]domain.ResolvedField{}
	for _, cat := range view.Categories {
		for _, coll := range cat.Collections {
			for _, f := range coll.Fields {
				fieldByOverride[f.OverrideID] = f
			}
		}
	}
	issues := make([]domain.ExampleIssue, 0)
	counts := map[string]int{}
	present := map[string]map[string]bool{}
	for _, value := range values {
		gp := instancePath(value.SlotPath)
		counts[slotCountKey(gp, value.OverrideID)]++
		if coll, _, ok := domain.ParseExampleGroupSegment(gp); ok {
			if present[coll] == nil {
				present[coll] = map[string]bool{}
			}
			present[coll][gp] = true
		}
		vIssues, err := s.valueIssues(ctx, value, fieldByOverride)
		if err != nil {
			return domain.ExampleValidationReport{}, err
		}
		for i := range vIssues {
			vIssues[i].GroupPath = gp
		}
		issues = append(issues, vIssues...)
	}
	issues = append(issues, cardinalityIssues(view, counts, present)...)
	return domain.ExampleValidationReport{
		Valid:  len(issues) == 0,
		Issues: issues,
	}, nil
}

// valueIssues runs the per-value checks for one value.
func (s *Service) valueIssues(ctx context.Context, value domain.ExampleValue, fieldByOverride map[int64]domain.ResolvedField) ([]domain.ExampleIssue, error) {
	var out []domain.ExampleIssue
	field, ok := fieldByOverride[value.OverrideID]
	if !ok {
		out = append(out, warningIssue("stale_override", nil, &value.OverrideID, &value.OccurrenceIndex, "This field slot no longer exists on the target model."))
		return out, nil
	}
	if field.ID != value.FieldID {
		fieldID := value.FieldID
		out = append(out, errorIssue("field_override_mismatch", &fieldID, &value.OverrideID, &value.OccurrenceIndex, "This value is attached to the wrong field slot."))
	}
	expected := valueKindForExpectedType(field.ExpectedValueType)
	if expected != "" && value.ValuePayload.Kind != expected {
		fieldID := value.FieldID
		out = append(out, errorIssue("wrong_value_kind", &fieldID, &value.OverrideID, &value.OccurrenceIndex, fmt.Sprintf("Expected %s but got %s.", expected, value.ValuePayload.Kind)))
	}
	if field.IsHidden {
		fieldID := value.FieldID
		out = append(out, warningIssue("hidden_field_value", &fieldID, &value.OverrideID, &value.OccurrenceIndex, "This field is hidden in the current model configuration."))
	}
	if field.SetValue != "" && value.ValuePayload.Kind == domain.ExampleValueKindConcept {
		if value.ValuePayload.ConceptURI == nil || *value.ValuePayload.ConceptURI != field.SetValue {
			fieldID := value.FieldID
			out = append(out, errorIssue("set_value_mismatch", &fieldID, &value.OverrideID, &value.OccurrenceIndex, "The selected concept does not match the field's fixed value constraint."))
		}
	}
	if value.ValuePayload.Kind == domain.ExampleValueKindConcept && value.ValuePayload.ConceptURI != nil && len(field.ConceptLists) > 0 {
		allowed, err := s.conceptURIAllowed(ctx, *value.ValuePayload.ConceptURI, field.ConceptLists)
		if err != nil {
			return nil, err
		}
		if !allowed {
			fieldID := value.FieldID
			out = append(out, errorIssue("concept_not_in_allowed_list", &fieldID, &value.OverrideID, &value.OccurrenceIndex, "The selected concept is not part of the allowed concept list."))
		}
	}
	if value.ValuePayload.Kind == domain.ExampleValueKindExampleRef && value.ValuePayload.ExampleID != nil {
		linked, err := s.store.GetByID(ctx, *value.ValuePayload.ExampleID)
		if err != nil {
			return nil, err
		}
		if linked == nil {
			fieldID := value.FieldID
			out = append(out, errorIssue("missing_linked_example", &fieldID, &value.OverrideID, &value.OccurrenceIndex, "The referenced example no longer exists."))
		} else if !linkedTargetAllowed(linked, field) {
			fieldID := value.FieldID
			out = append(out, errorIssue("linked_example_not_allowed", &fieldID, &value.OverrideID, &value.OccurrenceIndex, "The referenced example does not match the allowed target models or collections."))
		}
	}
	return out, nil
}

func slotCountKey(groupPath string, overrideID int64) string {
	return groupPath + "|" + strconv.FormatInt(overrideID, 10)
}

// groupInstances lists the instance segments of a collection group that
// validation and the form cover: every instance holding values plus
// instance 0, by index.
func groupInstances(collectionID string, present map[string]bool) []string {
	idx := map[int]bool{0: true}
	for seg := range present {
		if _, n, ok := domain.ParseExampleGroupSegment(seg); ok {
			idx[n] = true
		}
	}
	keys := slices.Sorted(maps.Keys(idx))
	out := make([]string, 0, len(keys))
	for _, n := range keys {
		out = append(out, domain.ExampleGroupSegment(collectionID, n))
	}
	return out
}

// cardinalityIssues checks required/min/max of every field slot within each
// instance of its group, and the instance count of each placed group.
func cardinalityIssues(view *domain.ModelView, counts map[string]int, present map[string]map[string]bool) []domain.ExampleIssue {
	var issues []domain.ExampleIssue
	for _, cat := range view.Categories {
		for _, coll := range cat.Collections {
			instances := []string{""}
			if coll.ID != directGroupID {
				instances = groupInstances(coll.ID, present[coll.ID])
			}
			for _, gp := range instances {
				for _, field := range coll.Fields {
					issues = append(issues, fieldCardinalityIssues(field, gp, counts[slotCountKey(gp, field.OverrideID)])...)
				}
			}
			if coll.Placement != nil {
				issues = append(issues, groupCardinalityIssues(coll.ID, coll.Placement, len(present[coll.ID]))...)
			}
		}
	}
	return issues
}

func fieldCardinalityIssues(field domain.ResolvedField, groupPath string, count int) []domain.ExampleIssue {
	fieldID, overrideID := field.ID, field.OverrideID
	var out []domain.ExampleIssue
	if field.IsRequired && count == 0 {
		out = append(out, errorIssue("missing_required_value", &fieldID, &overrideID, nil, "This required field has no value."))
	}
	if count < field.MinOccurs {
		out = append(out, errorIssue("min_occurs", &fieldID, &overrideID, nil, fmt.Sprintf("At least %d value(s) are required.", field.MinOccurs)))
	}
	if field.MaxOccurs != nil && count > *field.MaxOccurs {
		out = append(out, errorIssue("max_occurs", &fieldID, &overrideID, nil, fmt.Sprintf("At most %d value(s) are allowed.", *field.MaxOccurs)))
	}
	for i := range out {
		out[i].GroupPath = groupPath
	}
	return out
}

func groupCardinalityIssues(collectionID string, pl *domain.CollectionPlacement, count int) []domain.ExampleIssue {
	var out []domain.ExampleIssue
	if pl.MinOccurs > 1 && count < pl.MinOccurs {
		out = append(out, errorIssue("group_min_occurs", nil, nil, nil, fmt.Sprintf("At least %d instance(s) of this group are required.", pl.MinOccurs)))
	}
	if pl.MaxOccurs != nil && count > *pl.MaxOccurs {
		out = append(out, errorIssue("group_max_occurs", nil, nil, nil, fmt.Sprintf("At most %d instance(s) of this group are allowed.", *pl.MaxOccurs)))
	}
	for i := range out {
		id := collectionID
		out[i].CollectionID = &id
	}
	return out
}

func (s *Service) conceptURIAllowed(ctx context.Context, uri string, refs []domain.EntityRef) (bool, error) {
	validator, ok := s.store.(conceptValidator)
	if !ok {
		return true, nil
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.ID) != "" {
			ids = append(ids, ref.ID)
		}
		if strings.TrimSpace(ref.SemanticID) != "" && ref.SemanticID != ref.ID {
			ids = append(ids, ref.SemanticID)
		}
	}
	if len(ids) == 0 {
		return true, nil
	}
	return validator.ConceptURIAllowedForLists(ctx, uri, ids)
}

func conceptSources(refs []domain.EntityRef) []ConceptListSource {
	if len(refs) == 0 {
		return nil
	}
	out := make([]ConceptListSource, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.ID) == "" {
			continue
		}
		out = append(out, ConceptListSource{
			ID:         ref.ID,
			SemanticID: ref.SemanticID,
			Name:       ref.Name,
			URL:        ref.URL,
			SearchURL:  fmt.Sprintf("/api/v2/concept-lists/%s/entries/search", ref.ID),
		})
	}
	return out
}

func linkedTargetAllowed(linked *domain.Example, field domain.ResolvedField) bool {
	switch linked.EntityType {
	case domain.ExampleEntityTypeModel:
		if len(field.ResourceModels) == 0 {
			return field.ExpectedValueType == expectedValueTypeModel
		}
		return slices.ContainsFunc(field.ResourceModels, func(ref domain.EntityRef) bool { return ref.ID == linked.EntityID })
	case domain.ExampleEntityTypeCollection:
		if len(field.CollectionModels) == 0 {
			return field.ExpectedValueType == "Collection"
		}
		return slices.ContainsFunc(field.CollectionModels, func(ref domain.EntityRef) bool { return ref.ID == linked.EntityID })
	default:
		return false
	}
}

func valueKindForExpectedType(expected string) domain.ExampleValueKind {
	switch strings.TrimSpace(expected) {
	case "String", "Text":
		return domain.ExampleValueKindString
	case "Integer":
		return domain.ExampleValueKindInteger
	case "Date":
		return domain.ExampleValueKindDate
	case "URI":
		return domain.ExampleValueKindURI
	case "Concept":
		return domain.ExampleValueKindConcept
	case expectedValueTypeModel, "Collection":
		return domain.ExampleValueKindExampleRef
	default:
		return domain.ExampleValueKindString
	}
}

func widgetForExpectedType(expected string) string {
	switch strings.TrimSpace(expected) {
	case "Integer":
		return formschema.WidgetNumber
	case "Date":
		return "date"
	case "URI":
		return "url"
	case "Concept", expectedValueTypeModel, "Collection":
		return formschema.WidgetSearchSelect
	default:
		return formschema.WidgetText
	}
}

// directGroupID is the resolver's bucket for fields placed on the model
// outside any collection (pkg/weave resolve.go directKey).
const directGroupID = "__direct__"

// groupOfOverride maps every field slot on the model to the id of the
// collection group holding it; "" for direct fields.
func groupOfOverride(view *domain.ModelView) map[int64]string {
	out := map[int64]string{}
	for _, cat := range view.Categories {
		for _, coll := range cat.Collections {
			group := coll.ID
			if group == directGroupID {
				group = ""
			}
			for _, f := range coll.Fields {
				out[f.OverrideID] = group
			}
		}
	}
	return out
}

// instancePath is the group segment of a two-segment slot path ("C1:1" for
// "C1:1/21:0"); "" for a one-segment path.
func instancePath(slotPath string) string {
	if i := strings.Index(slotPath, "/"); i >= 0 {
		return slotPath[:i]
	}
	return ""
}

// compactGroupInstances renumbers each collection group's instances to
// 0..n-1 in their existing order. The form always shows instance 0, so a
// removed first instance must not leave a gap behind.
func compactGroupInstances(values []domain.ExampleValue) {
	seen := map[string]map[int]bool{}
	for _, v := range values {
		if coll, n, ok := domain.ParseExampleGroupSegment(instancePath(v.SlotPath)); ok {
			if seen[coll] == nil {
				seen[coll] = map[int]bool{}
			}
			seen[coll][n] = true
		}
	}
	renumber := make(map[string]map[int]int, len(seen))
	for coll, set := range seen {
		m := make(map[int]int, len(set))
		for i, n := range slices.Sorted(maps.Keys(set)) {
			m[n] = i
		}
		renumber[coll] = m
	}
	for i := range values {
		v := &values[i]
		gp := instancePath(v.SlotPath)
		coll, n, ok := domain.ParseExampleGroupSegment(gp)
		if !ok || renumber[coll][n] == n {
			continue
		}
		v.SlotPath = domain.ExampleGroupSegment(coll, renumber[coll][n]) + v.SlotPath[len(gp):]
	}
}

// placeInGroups gives every value in a collection group its group segment.
// A one-segment path on a grouped field is instance 0: rows saved before
// step B.2 and clients that do not know about groups. A two-segment path
// must name the group holding the field. Values on slots the model no
// longer has are left alone; validation reports them as stale. Duplicate
// paths are an input error. Group instances are then renumbered 0..n-1.
func placeInGroups(values []domain.ExampleValue, groups map[int64]string) error {
	seen := make(map[string]bool, len(values))
	for i := range values {
		v := &values[i]
		group, known := groups[v.OverrideID]
		if known {
			if err := placeValue(v, group); err != nil {
				return err
			}
		}
		if seen[v.SlotPath] {
			return fmt.Errorf("value for field %s: duplicate slot_path %q", v.FieldID, v.SlotPath)
		}
		seen[v.SlotPath] = true
	}
	compactGroupInstances(values)
	return nil
}

func placeValue(v *domain.ExampleValue, group string) error {
	gp := instancePath(v.SlotPath)
	if gp == "" {
		if group != "" {
			v.SlotPath = domain.ExampleGroupSegment(group, 0) + "/" + v.SlotPath
		}
		return nil
	}
	coll, _, ok := domain.ParseExampleGroupSegment(gp)
	if !ok || coll != group {
		return fmt.Errorf("value for field %s: slot_path %q does not match the group holding field slot %d", v.FieldID, v.SlotPath, v.OverrideID)
	}
	return nil
}

func issueKey(groupPath string, overrideID int64, occurrenceIndex int) string {
	return fmt.Sprintf("%s|%d:%d", groupPath, overrideID, occurrenceIndex)
}

func errorIssue(code string, fieldID *string, overrideID *int64, occurrenceIndex *int, msg string) domain.ExampleIssue {
	return domain.ExampleIssue{
		Severity:        domain.ExampleIssueError,
		Code:            code,
		FieldID:         fieldID,
		OverrideID:      overrideID,
		OccurrenceIndex: occurrenceIndex,
		Message:         domain.Translations{"en": msg},
	}
}

func warningIssue(code string, fieldID *string, overrideID *int64, occurrenceIndex *int, msg string) domain.ExampleIssue {
	return domain.ExampleIssue{
		Severity:        domain.ExampleIssueWarning,
		Code:            code,
		FieldID:         fieldID,
		OverrideID:      overrideID,
		OccurrenceIndex: occurrenceIndex,
		Message:         domain.Translations{"en": msg},
	}
}

func submitLabel(exampleID string) string {
	if exampleID == "" {
		return "Create Example"
	}
	return "Save Example"
}

func successMessage(exampleID string) string {
	if exampleID == "" {
		return "Example created successfully"
	}
	return "Example updated successfully"
}
