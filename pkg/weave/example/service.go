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
	CollectionView(ctx context.Context, collectionID, projectID string) ([]domain.ResolvedField, error)
}

// defaultMaxNestingDepth is how many collection levels an example form opens
// inside a Collection-typed field when examples.max_nesting_depth is unset.
const defaultMaxNestingDepth = 2

type Service struct {
	store    Store
	views    ViewReader
	maxDepth int
}

// ServiceOption configures a Service at construction.
type ServiceOption func(*Service)

// WithMaxNestingDepth sets the nesting cap; n <= 0 keeps the default.
func WithMaxNestingDepth(n int) ServiceOption {
	return func(s *Service) {
		if n > 0 {
			s.maxDepth = n
		}
	}
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

// NewService builds the example service; opts adjust defaults such as the
// nesting depth cap.
func NewService(store Store, views ViewReader, opts ...ServiceOption) *Service {
	s := &Service{store: store, views: views, maxDepth: defaultMaxNestingDepth}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	// Nested describes a Collection-typed field's target collection. Present
	// on every Collection-typed field; Expandable false carries a Note.
	Nested *NestedCollection `json:"nested,omitempty"`
	// NestedInstances are the collection instances holding values, in
	// order, each a group entry whose SlotPrefix is
	// "<field slot_prefix><override>:<k>/".
	NestedInstances []ExampleFormGroup `json:"nested_instances,omitempty"`
}

// NestedCollection is the target collection a Collection-typed field
// opens in place.
type NestedCollection struct {
	CollectionID string              `json:"collection_id,omitempty"`
	Label        domain.Translations `json:"label,omitempty"`
	Expandable   bool                `json:"expandable"`
	Note         string              `json:"note,omitempty"`
	// Fields is a blank template of the collection's fields (slot_prefix
	// empty, no occurrences' values), recursively carrying their own Nested
	// templates up to the depth cap. The workspace clones it for "+ Add".
	Fields []ExampleFormField `json:"fields,omitempty"`
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
	resolver, err := s.resolverFor(ctx, projectID, in.EntityID)
	if err != nil {
		return nil, err
	}
	if err := s.materializeStubs(ctx, resolver, in.Lang, in.Values); err != nil {
		return nil, err
	}
	values, err := normalizeValues(in.Values)
	if err != nil {
		return nil, err
	}
	report, err := s.validateValues(ctx, resolver, values, true)
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
	resolver, err := s.resolverFor(ctx, projectID, ex.EntityID)
	if err != nil {
		return nil, err
	}
	if err := s.materializeStubs(ctx, resolver, in.Lang, in.Values); err != nil {
		return nil, err
	}
	values, err := normalizeValues(in.Values)
	if err != nil {
		return nil, err
	}
	report, err := s.validateValues(ctx, resolver, values, true)
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
// when there are drafts to create, every value's slot path is then checked
// the way validateValues checks it on a write (checkStrictPaths), so a bad
// nested path elsewhere in the request fails before any draft exists. Only
// then does a second pass create the drafts and link them.
//
// ponytail: an unlinked draft can still be left by a DB error on the parent
// save, or by a write rejected after this point for a reason other than an
// unresolvable path (wrong or duplicate group segment, a slot_path that
// contradicts override_id/occurrence_index): stubs are created before the
// parent is saved and are not rolled back; they are drafts and harmless. Wrap in one transaction
// if that ever bites.
func (s *Service) materializeStubs(ctx context.Context, resolver *slotResolver, lang string, values []domain.ExampleValue) error {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "en"
	}
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
		rs, err := resolver.resolve(valueSlotPath(values[i]))
		if err != nil {
			return err
		}
		if rs.status != slotOK {
			continue // validation reports the unresolvable path
		}
		target, err := resolveStubTarget(rs.field, p)
		if err != nil {
			return err
		}
		candidates = append(candidates, stubCandidate{index: i, target: target, label: label})
	}
	if len(candidates) == 0 {
		return nil
	}
	if err := resolver.checkStrictPaths(values); err != nil {
		return err
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
				ProjectID:  resolver.projectID,
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
	record, _, err := s.get(ctx, projectID, exampleID)
	return record, err
}

// get is Get, also returning the slot resolver it validated with so the
// edit form reuses its cached views.
func (s *Service) get(ctx context.Context, projectID, exampleID string) (*ExampleRecord, *slotResolver, error) {
	ex, err := s.store.GetByID(ctx, exampleID)
	if err != nil {
		return nil, nil, err
	}
	if ex == nil || ex.ProjectID != projectID {
		return nil, nil, fmt.Errorf("example not found")
	}
	values, err := s.store.ListValues(ctx, exampleID)
	if err != nil {
		return nil, nil, err
	}
	resolver, err := s.resolverFor(ctx, projectID, ex.EntityID)
	if err != nil {
		return nil, nil, err
	}
	report, err := s.validateValues(ctx, resolver, values, false)
	if err != nil {
		return nil, nil, err
	}
	report.ExampleID = ex.ID
	return &ExampleRecord{Example: ex, Values: values, Validation: report}, resolver, nil
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
		resolver, err := s.resolverFor(ctx, projectID, targetID)
		if err != nil {
			return nil, err
		}
		return s.buildModelFormSchema(ctx, resolver, "", targetID, nil, lang, languages)
	case formschema.ModeEdit:
		record, resolver, err := s.get(ctx, projectID, exampleID)
		if err != nil {
			return nil, err
		}
		return s.buildModelFormSchema(ctx, resolver, record.Example.ID, record.Example.EntityID, record, lang, languages)
	default:
		return nil, fmt.Errorf("unsupported form mode: %s", mode)
	}
}

// buildModelFormSchema builds the form with the request's slot resolver
// (its model view and cached collection views).
func (s *Service) buildModelFormSchema(ctx context.Context, resolver *slotResolver, exampleID, modelID string, record *ExampleRecord, lang string, languages []formschema.LanguageInfo) (*ExampleFormSchema, error) {
	projectID, view := resolver.projectID, resolver.view
	var values []domain.ExampleValue
	var issues []domain.ExampleIssue
	if record != nil {
		values, issues = record.Values, record.Validation.Issues
	}
	// A value whose field moved to a different group since it was saved does
	// not resolve and stays out of the form, as it stays out of Get's counts.
	slots, err := resolver.resolveAll(values)
	if err != nil {
		return nil, err
	}
	issuesByKey, groupIssues, topIssues := routeIssues(issues)
	builder := newFormBuilder(resolver, values, slots, issuesByKey)
	present := formGroupPresence(values, slots)
	sections := make([]ExampleFormSection, 0, len(view.Categories))
	for _, cat := range view.Categories {
		section := ExampleFormSection{
			ID:             cat.ID,
			Label:          cat.Name,
			CanonicalOrder: cat.Position,
		}
		for _, coll := range cat.Collections {
			entries, err := buildGroupEntries(builder, coll, groupIssues, present[coll.ID])
			if err != nil {
				return nil, err
			}
			section.Groups = append(section.Groups, entries...)
		}
		sections = append(sections, section)
	}
	topIssues = append(topIssues, builder.unplacedIssues(issues)...)
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
func buildGroupEntries(b *formBuilder, coll domain.CollectionGroup, groupIssues map[string][]domain.ExampleIssue, present map[string]bool) ([]ExampleFormGroup, error) {
	instances := formInstances(coll, present)
	out := make([]ExampleFormGroup, 0, len(instances))
	for i, gp := range instances {
		instance := 0
		if gp != "" {
			_, instance, _ = domain.ParseExampleGroupSegment(gp)
		}
		fields := make([]ExampleFormField, 0, len(coll.Fields))
		for _, f := range coll.Fields {
			field, err := b.field(f, gp, 1)
			if err != nil {
				return nil, err
			}
			fields = append(fields, field)
		}
		out = append(out, ExampleFormGroup{
			ID:               coll.ID,
			Label:            coll.Name,
			Position:         coll.Position,
			SharedPathPrefix: coll.SharedPathPrefix,
			Fields:           fields,
			Instance:         instance,
			SlotPrefix:       slotPrefix(gp),
			Repeatable:       groupRepeatable(coll),
			MinOccurs:        placementMin(coll),
			MaxOccurs:        placementMax(coll),
			Issues:           issuesIf(i == 0, groupIssues[coll.ID]),
		})
	}
	return out, nil
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

// settleSlot reconciles SlotPath with OverrideID/OccurrenceIndex. It checks
// the path grammar only; whether the path resolves (including its nesting
// depth) is the slot resolver's concern during validation.
func settleSlot(v *domain.ExampleValue) error {
	if v.SlotPath == "" {
		v.SlotPath = domain.ExampleSlot(v.OverrideID, v.OccurrenceIndex)
		return nil
	}
	oid, occ, ok := domain.ParseExampleSlotLeaf(v.SlotPath)
	if _, _, grammar := slotSegments(v.SlotPath); !ok || !grammar {
		return fmt.Errorf("value for field %s: malformed slot_path %q", v.FieldID, v.SlotPath)
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

// validateModelValues validates leniently, as Get does, with a resolver of
// its own: a value whose stored group no longer matches its override's
// current group is left in place rather than rejected. See validateValues.
func (s *Service) validateModelValues(ctx context.Context, projectID, modelID string, values []domain.ExampleValue) (domain.ExampleValidationReport, error) {
	resolver, err := s.resolverFor(ctx, projectID, modelID)
	if err != nil {
		return domain.ExampleValidationReport{}, err
	}
	return s.validateValues(ctx, resolver, values, false)
}

// validateValues validates values against modelID's current view. strict
// controls placeInGroups: Create/Update pass true and reject a value whose
// slot_path names the wrong group for its field. Get and other reads pass
// false (via validateModelValues) so a field moved to a different group (or
// to/from direct) since the value was saved does not make the example
// unreadable; such a value is left untouched, excluded from group/field
// counts and compaction, and reported as a moved_group_value warning
// instead. Every slot path is resolved through the request's slot resolver
// (the model's current view), nested ones through their container's target
// collection (see checkValue).
func (s *Service) validateValues(ctx context.Context, resolver *slotResolver, values []domain.ExampleValue, strict bool) (domain.ExampleValidationReport, error) {
	if err := placeInGroups(values, resolver.groups, strict); err != nil {
		return domain.ExampleValidationReport{}, err
	}
	slots, err := resolver.resolveAll(values)
	if err != nil {
		return domain.ExampleValidationReport{}, err
	}
	compactInstances(values, slots)
	issues := make([]domain.ExampleIssue, 0)
	counts := map[string]int{}
	present := map[string]map[string]bool{}
	for i, value := range values {
		vIssues, err := s.checkValue(ctx, slots[i], value, strict)
		if err != nil {
			return domain.ExampleValidationReport{}, err
		}
		issues = append(issues, vIssues...)
		if slots[i].status != slotOK {
			continue
		}
		counts[slotCountKey(containerPath(value.SlotPath), value.OverrideID)]++
		gp := groupPart(value.SlotPath)
		if coll, _, ok := domain.ParseExampleGroupSegment(gp); ok {
			if present[coll] == nil {
				present[coll] = map[string]bool{}
			}
			present[coll][gp] = true
		}
	}
	nested, err := resolver.nestedCardinalityIssues(values, slots, counts)
	if err != nil {
		return domain.ExampleValidationReport{}, err
	}
	modelIssues, err := cardinalityIssues(resolver, resolver.view, counts, present)
	if err != nil {
		return domain.ExampleValidationReport{}, err
	}
	issues = append(issues, modelIssues...)
	issues = append(issues, nested...)
	return domain.ExampleValidationReport{
		Valid:  len(issues) == 0,
		Issues: issues,
	}, nil
}

// movedGroupValueIssue reports a value whose stored group segment no longer
// matches its override's current group: left as is by validateValues, out
// of group/field counts, and surfaced as a warning instead of dropped
// silently.
func movedGroupValueIssue(value domain.ExampleValue, groupPath string) domain.ExampleIssue {
	fieldID := value.FieldID
	overrideID := value.OverrideID
	occurrenceIndex := value.OccurrenceIndex
	issue := warningIssue("moved_group_value", &fieldID, &overrideID, &occurrenceIndex, "This value was saved in a group the field no longer belongs to.")
	issue.GroupPath = groupPath
	return issue
}

// valueIssues runs the per-value checks for one value against its resolved
// leaf field.
func (s *Service) valueIssues(ctx context.Context, value domain.ExampleValue, field domain.ResolvedField) ([]domain.ExampleIssue, error) {
	var out []domain.ExampleIssue
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
// instance of its group, and the instance count of each placed group. A
// container that cannot open is never required (see cardinalityField). The
// error is only for a failed collection view read.
func cardinalityIssues(r *slotResolver, view *domain.ModelView, counts map[string]int, present map[string]map[string]bool) ([]domain.ExampleIssue, error) {
	var issues []domain.ExampleIssue
	for _, cat := range view.Categories {
		for _, coll := range cat.Collections {
			instances := []string{""}
			if coll.ID != directGroupID {
				instances = groupInstances(coll.ID, present[coll.ID])
			}
			for _, gp := range instances {
				for _, field := range coll.Fields {
					cf, err := r.cardinalityField(field, 1)
					if err != nil {
						return nil, err
					}
					issues = append(issues, fieldCardinalityIssues(cf, gp, counts[slotCountKey(gp, field.OverrideID)])...)
				}
			}
			if coll.Placement != nil {
				issues = append(issues, groupCardinalityIssues(coll.ID, coll.Placement, len(present[coll.ID]))...)
			}
		}
	}
	return issues, nil
}

// fieldCardinalityIssues checks required/min/max of one field slot given
// its count: values, or nested instances for a Collection container.
func fieldCardinalityIssues(field domain.ResolvedField, groupPath string, count int) []domain.ExampleIssue {
	fieldID, overrideID := field.ID, field.OverrideID
	noun := "value(s)"
	if strings.TrimSpace(field.ExpectedValueType) == expectedValueTypeCollection {
		noun = "instance(s)"
	}
	var out []domain.ExampleIssue
	if field.IsRequired && count == 0 {
		out = append(out, errorIssue("missing_required_value", &fieldID, &overrideID, nil, "This required field has no value."))
	}
	if count < field.MinOccurs {
		out = append(out, errorIssue("min_occurs", &fieldID, &overrideID, nil, fmt.Sprintf("At least %d %s are required.", field.MinOccurs, noun)))
	}
	if field.MaxOccurs != nil && count > *field.MaxOccurs {
		out = append(out, errorIssue("max_occurs", &fieldID, &overrideID, nil, fmt.Sprintf("At most %d %s are allowed.", *field.MaxOccurs, noun)))
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
			// A field bound to a control list always autocompletes against only
			// that list's own entries — never the source vocabulary or another
			// list (#3599). open/sealed governs list-membership editing, not the
			// value picker. The source vocab is a curation aid on the list page.
			SearchURL: fmt.Sprintf("/api/v2/concept-lists/%s/entries/search", ref.ID),
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

// groupMatches reports whether v's stored group segment agrees with its
// anchor override's current group (the anchor equals v.OverrideID for
// depth-0 paths). An override no longer on the model is not this
// function's concern (the stale_override check handles it); it always
// matches here.
func groupMatches(v domain.ExampleValue, groups map[int64]string) bool {
	group, known := groups[anchorOverride(v)]
	if !known {
		return true
	}
	coll, _, ok := domain.ParseExampleGroupSegment(groupPart(v.SlotPath))
	if !ok {
		coll = ""
	}
	return coll == group
}

// placeInGroups gives every value in a collection group its group segment.
// A one-segment path on a grouped field is instance 0 in both modes: rows
// saved before step B.2 and clients that do not know about groups. A
// two-segment path must name the group currently holding the field: strict
// (Create/Update) rejects a mismatch, lenient (Get, via validateModelValues)
// leaves the value exactly as stored so a field moved to another group
// since it was saved does not make the example unreadable — validateValues
// reports it as a moved_group_value warning instead. Values on slots the
// model no longer has are left alone either way; validation reports them as
// stale. Duplicate paths are an input error in both modes. Instance
// numbers are compacted afterwards, once every path is resolved (see
// compactInstances in validateValues).
func placeInGroups(values []domain.ExampleValue, groups map[int64]string, strict bool) error {
	seen := make(map[string]bool, len(values))
	for i := range values {
		v := &values[i]
		group, known := groups[anchorOverride(*v)]
		if known {
			if err := placeValue(v, group, strict); err != nil {
				return err
			}
		}
		if seen[v.SlotPath] {
			return fmt.Errorf("value for field %s: duplicate slot_path %q", v.FieldID, v.SlotPath)
		}
		seen[v.SlotPath] = true
	}
	return nil
}

func placeValue(v *domain.ExampleValue, group string, strict bool) error {
	// A value without a slot path (never after migration 007, but possible
	// for callers that skip normalization) is its depth-0 slot; placing it
	// as is would leave the malformed path "<group>:0/".
	v.SlotPath = valueSlotPath(*v)
	gp := groupPart(v.SlotPath)
	if gp == "" {
		if group != "" {
			v.SlotPath = domain.ExampleGroupSegment(group, 0) + "/" + v.SlotPath
		}
		return nil
	}
	coll, _, ok := domain.ParseExampleGroupSegment(gp)
	if ok && coll == group {
		return nil
	}
	if !strict {
		return nil
	}
	return fmt.Errorf("value for field %s: slot_path %q does not match the group holding field slot %d", v.FieldID, v.SlotPath, anchorOverride(*v))
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
