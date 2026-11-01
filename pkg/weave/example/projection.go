package example

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

const (
	exampleKind = "example"
	modelKind   = "model"
)

type exampleProjection struct {
	fragment    projectsnapshot.EntityFragment
	context     projectsnapshot.Identity
	content     map[projectsnapshot.Identity]projectsnapshot.Content
	collections map[string]projectsnapshot.EntityKey
	concepts    map[string][]ConceptChoice
	linked      []Link
}

// ProjectRead builds the existing record and form contracts from detached content.
// It grants no mutation capabilities, regardless of the selected state.
func ProjectRead(ctx context.Context, input browserinput.Detail, maxDepth int, lang string, languages []formschema.LanguageInfo) (*ExampleRecord, *ExampleFormSchema, error) {
	root, err := input.Fragment.Root()
	if err != nil {
		return nil, nil, err
	}
	p, err := newExampleProjection(input.Fragment)
	if err != nil {
		return nil, nil, err
	}
	var record *ExampleRecord
	kind, id := root.Kind, root.ID
	if root.Kind == exampleKind {
		p.context = root.Source
		record, err = p.record(root)
		if err != nil {
			return nil, nil, err
		}
		kind, id = string(record.Example.EntityType), record.Example.EntityID
	}
	if kind != modelKind && kind != "collection" {
		return nil, nil, fmt.Errorf("example schema requires a model, collection or example")
	}
	view, err := p.modelView(kind, id)
	if err != nil {
		return nil, nil, err
	}
	if maxDepth <= 0 {
		maxDepth = defaultMaxNestingDepth
	}
	resolver := &slotResolver{ctx: ctx, projectID: p.context.ProjectID(), views: p, maxDepth: maxDepth, view: view, groups: groupOfOverride(view), model: buildFieldByOverride(view), collections: map[string][]domain.ResolvedField{}}
	var saved []domain.ExampleValue
	if record != nil {
		saved = append([]domain.ExampleValue(nil), record.Values...)
		checks := valueValidator{conceptAllowed: p.conceptAllowed, example: p.linkedExample}
		record.Validation, err = checks.validateValues(ctx, resolver, saved, false)
		if err != nil {
			return nil, nil, err
		}
		record.Validation.ExampleID = record.Example.ID
	}
	target, err := p.fragment.Resolve(p.context, kind, id)
	if err != nil {
		return nil, nil, err
	}
	name, err := p.name(target)
	if err != nil {
		return nil, nil, err
	}
	formRecord := record
	if record != nil {
		copyRecord := *record
		copyRecord.Values = saved
		formRecord = &copyRecord
	}
	exampleID := ""
	if record != nil {
		exampleID = record.Example.ID
	}
	schema, err := buildExampleFormSchema(resolver, exampleID, id, name, formRecord, lang, languages)
	if err != nil {
		return nil, nil, err
	}
	schema.Target.EntityType = kind
	schema.Target.URL = exampleURL(projectsnapshot.EntityKey{Source: p.context, Kind: kind, ID: id})
	if record != nil {
		schema.Example = record.Example
		schema.UnplacedValues, err = unplacedValues(resolver, record.Values, saved)
		if err != nil {
			return nil, nil, err
		}
	}
	schema.LinkedExamples = p.linked
	p.bindConcepts(schema.Sections, lang)
	return record, schema, nil
}

func newExampleProjection(fragment projectsnapshot.EntityFragment) (*exampleProjection, error) {
	consumer, err := fragment.Consumer()
	if err != nil {
		return nil, err
	}
	p := &exampleProjection{fragment: fragment, context: consumer, content: map[projectsnapshot.Identity]projectsnapshot.Content{}, collections: map[string]projectsnapshot.EntityKey{}, concepts: map[string][]ConceptChoice{}}
	ids, err := fragment.Projects()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		c, err := fragment.Content(id)
		if err != nil {
			return nil, err
		}
		p.content[id] = c
	}
	return p, nil
}

func (p *exampleProjection) record(key projectsnapshot.EntityKey) (*ExampleRecord, error) {
	out := &ExampleRecord{Values: []domain.ExampleValue{}}
	for _, r := range p.content[key.Source].Examples {
		if r.ID == key.ID {
			ex, err := projectExampleRecord(r)
			if err != nil {
				return nil, err
			}
			out.Example = ex
			break
		}
	}
	if out.Example == nil {
		return nil, fmt.Errorf("example is outside retained input")
	}
	linkedIDs := map[string]bool{}
	for _, r := range p.content[key.Source].ExampleValues {
		if r.ExampleID != key.ID {
			continue
		}
		v, err := projectExampleValue(r)
		if err != nil {
			return nil, err
		}
		out.Values = append(out.Values, v)
		if v.LinkedExampleID != nil && !linkedIDs[*v.LinkedExampleID] {
			linkedIDs[*v.LinkedExampleID] = true
			linked, err := p.fragment.Resolve(key.Source, exampleKind, *v.LinkedExampleID)
			if err != nil {
				return nil, err
			}
			for _, ex := range p.content[linked.Source].Examples {
				if ex.ID != linked.ID {
					continue
				}
				body, err := projectExampleRecord(ex)
				if err != nil {
					return nil, err
				}
				p.linked = append(p.linked, Link{ID: body.ID, EntityType: string(body.EntityType), EntityID: body.EntityID, Title: body.Title, Status: string(body.Status), URL: exampleURL(linked)})
			}
		}
	}
	return out, nil
}

func projectExampleRecord(r projectsnapshot.ExampleRecord) (*domain.Example, error) {
	ex := &domain.Example{ID: r.ID, ProjectID: r.ProjectID, EntityType: domain.ExampleEntityType(r.EntityType), EntityID: r.EntityID, Status: domain.ExampleStatus(r.Status), VersionNumber: r.VersionNumber}
	for _, part := range []struct {
		raw   []byte
		value any
	}{{r.Title, &ex.Title}, {r.Description, &ex.Description}} {
		if err := exampleJSON(part.raw, part.value); err != nil {
			return nil, err
		}
	}
	var err error
	ex.CreatedAt, err = exampleTime(r.CreatedAt)
	if err != nil {
		return nil, err
	}
	ex.UpdatedAt, err = exampleTime(r.UpdatedAt)
	return ex, err
}

func projectExampleValue(r projectsnapshot.ExampleValueRecord) (domain.ExampleValue, error) {
	v := domain.ExampleValue{ID: r.ID, ExampleID: r.ExampleID, OverrideID: r.OverrideID, FieldID: r.FieldID, PartOfCollectionID: exampleValue(r.PartOfCollectionID), OccurrenceIndex: int(r.OccurrenceIndex), ValueKind: domain.ExampleValueKind(r.ValueKind), TextValue: r.TextValue, URIValue: r.URIValue, ConceptURI: r.ConceptURI, LinkedExampleID: r.LinkedExampleID, SlotPath: r.SlotPath}
	if err := exampleJSON(r.ValuePayload, &v.ValuePayload); err != nil {
		return v, err
	}
	if r.NumberValue != nil {
		n, err := strconv.ParseFloat(string(*r.NumberValue), 64)
		if err != nil {
			return v, err
		}
		v.NumberValue = &n
	}
	if r.DateValue != nil {
		d := string(*r.DateValue)
		v.DateValue = &d
	}
	if v.ValuePayload.Kind == "" {
		v.ValuePayload.Kind = v.ValueKind
	}
	if v.ValuePayload.StringValue == nil {
		v.ValuePayload.StringValue = v.TextValue
	}
	if v.ValuePayload.NumberValue == nil {
		v.ValuePayload.NumberValue = v.NumberValue
	}
	if v.ValuePayload.DateValue == nil {
		v.ValuePayload.DateValue = v.DateValue
	}
	if v.ValuePayload.URIValue == nil {
		v.ValuePayload.URIValue = v.URIValue
	}
	if v.ValuePayload.ConceptURI == nil {
		v.ValuePayload.ConceptURI = v.ConceptURI
	}
	if v.ValuePayload.ExampleID == nil {
		v.ValuePayload.ExampleID = v.LinkedExampleID
	}
	var err error
	v.CreatedAt, err = exampleTime(r.CreatedAt)
	if err != nil {
		return v, err
	}
	v.UpdatedAt, err = exampleTime(r.UpdatedAt)
	return v, err
}

func exampleJSON(raw []byte, value any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, value)
}

func exampleValue[T any](p *T) (out T) {
	if p != nil {
		out = *p
	}
	return
}

func exampleInt(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

func exampleTime(t projectsnapshot.Timestamp) (time.Time, error) {
	if t == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, string(t))
}

func exampleURL(key projectsnapshot.EntityKey) string {
	segment := key.Kind + "s"
	if key.Kind == "concept_list" {
		segment = "concept-lists"
	}
	path := "/projects/" + url.PathEscape(key.Source.ProjectID()) + "/" + segment + "/" + url.PathEscape(key.ID)
	if key.Source.Version() != "" {
		path += "?version=" + url.QueryEscape(key.Source.Version())
	}
	return path
}
