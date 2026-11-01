package browserhttp

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

type exampleProjection struct {
	fragment  projectsnapshot.EntityFragment
	content   map[projectsnapshot.Identity]projectsnapshot.Content
	names     map[projectsnapshot.EntityKey]domain.Translations
	overrides map[projectsnapshot.Identity]map[int64]domain.Translations
}

// ProjectExample is a pure projection of the selected example and its retained references.
func ProjectExample(input browserinput.Detail) (browserinput.ExampleView, error) {
	var out browserinput.ExampleView
	root, err := input.Fragment.Root()
	if err != nil {
		return out, err
	}
	consumer, err := input.Fragment.Consumer()
	if err != nil {
		return out, err
	}
	if root.Kind != catalogExample {
		return out, fmt.Errorf("example view requires an example fragment")
	}
	p, err := newExampleProjection(input.Fragment)
	if err != nil {
		return out, err
	}
	out.ProjectID, out.Version, out.ID = consumer.ProjectID(), consumer.Version(), root.ID
	out.Values = []browserinput.ExampleValueView{}
	found := false
	for _, ex := range p.content[root.Source].Examples {
		if ex.ID != root.ID {
			continue
		}
		found = true
		out.Title, err = exampleTranslations(ex.Title)
		if err != nil {
			return out, err
		}
		out.Description, err = exampleTranslations(ex.Description)
		if err != nil {
			return out, err
		}
		out.Status, out.TargetID = ex.Status, ex.EntityID
		key, err := p.fragment.Resolve(root.Source, ex.EntityType, ex.EntityID)
		if err != nil {
			return out, err
		}
		out.TargetName = p.names[key]
		out.TargetURL = exampleEntityURL(projectsnapshot.EntityKey{Source: root.Source, Kind: key.Kind, ID: key.ID})
	}
	if !found {
		return out, fmt.Errorf("example is outside retained input")
	}
	for _, v := range p.content[root.Source].ExampleValues {
		if v.ExampleID != root.ID {
			continue
		}
		row, err := p.value(root.Source, v)
		if err != nil {
			return out, err
		}
		out.Values = append(out.Values, row)
	}
	return out, nil
}
func newExampleProjection(f projectsnapshot.EntityFragment) (*exampleProjection, error) {
	p := &exampleProjection{fragment: f, content: map[projectsnapshot.Identity]projectsnapshot.Content{}, names: map[projectsnapshot.EntityKey]domain.Translations{}, overrides: map[projectsnapshot.Identity]map[int64]domain.Translations{}}
	ids, err := f.Projects()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		c, err := f.Content(id)
		if err != nil {
			return nil, err
		}
		p.content[id] = c
		p.overrides[id] = map[int64]domain.Translations{}
		add := func(kind, key string, raw []byte) error {
			label, err := exampleTranslations(raw)
			if err != nil {
				return err
			}
			p.names[projectsnapshot.EntityKey{Source: id, Kind: kind, ID: key}] = label
			return nil
		}
		for _, r := range c.Models {
			if err := add("model", r.ID, r.UIName); err != nil {
				return nil, err
			}
		}
		for _, r := range c.Collections {
			if err := add("collection", r.ID, r.UIName); err != nil {
				return nil, err
			}
		}
		for _, r := range c.Fields {
			if err := add("field", r.ID, r.UIName); err != nil {
				return nil, err
			}
		}
		for _, r := range c.Overrides {
			label, err := exampleTranslations(r.DisplayName)
			if err != nil {
				return nil, err
			}
			p.overrides[id][r.ID] = label
		}
	}
	return p, nil
}
func (p *exampleProjection) value(context projectsnapshot.Identity, v projectsnapshot.ExampleValueRecord) (browserinput.ExampleValueView, error) {
	row := browserinput.ExampleValueView{ID: v.ID, FieldID: v.FieldID, Label: domain.Translations{"en": v.FieldID}, SlotPath: v.SlotPath, OccurrenceIndex: v.OccurrenceIndex, Kind: v.ValueKind, Payload: json.RawMessage(v.ValuePayload), TextValue: v.TextValue, NumberValue: v.NumberValue, DateValue: v.DateValue, URIValue: v.URIValue, ConceptURI: v.ConceptURI, LinkedExampleID: v.LinkedExampleID}
	if len(row.Payload) == 0 {
		row.Payload = json.RawMessage("null")
	}
	field, fieldErr := p.fragment.Resolve(context, "field", v.FieldID)
	override, overrideErr := p.fragment.Resolve(context, "override", fmt.Sprint(v.OverrideID))
	row.Unresolved = fieldErr != nil || overrideErr != nil
	if fieldErr == nil {
		row.Label = p.names[field]
	}
	if overrideErr == nil {
		if label := p.overrides[override.Source][v.OverrideID]; len(label) > 0 {
			row.Label = label
		}
	}
	if v.LinkedExampleID != nil && *v.LinkedExampleID != "" {
		key, err := p.fragment.Resolve(context, catalogExample, *v.LinkedExampleID)
		if err != nil {
			return row, err
		}
		row.LinkedURL = exampleEntityURL(key)
	}
	return row, nil
}
func exampleTranslations(raw []byte) (domain.Translations, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var label domain.Translations
	err := json.Unmarshal(raw, &label)
	return label, err
}
func exampleEntityURL(key projectsnapshot.EntityKey) string {
	path := "/projects/" + url.PathEscape(key.Source.ProjectID()) + "/" + key.Kind + "s/" + url.PathEscape(key.ID)
	if !key.Source.IsDraft() {
		path += "?version=" + url.QueryEscape(key.Source.Version())
	}
	return path
}
