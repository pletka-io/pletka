package example

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
)

type valueValidator struct {
	conceptAllowed func(context.Context, string, []domain.EntityRef) (bool, error)
	example        func(context.Context, string) (*domain.Example, error)
}

func (v valueValidator) validateValues(ctx context.Context, resolver *slotResolver, values []domain.ExampleValue, strict bool) (domain.ExampleValidationReport, error) {
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
		vIssues, err := v.checkValue(ctx, slots[i], value, strict)
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

func (v valueValidator) valueIssues(ctx context.Context, value domain.ExampleValue, field domain.ResolvedField) ([]domain.ExampleIssue, error) {
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
		allowed, err := v.conceptAllowed(ctx, *value.ValuePayload.ConceptURI, field.ConceptLists)
		if err != nil {
			return nil, err
		}
		if !allowed {
			fieldID := value.FieldID
			out = append(out, errorIssue("concept_not_in_allowed_list", &fieldID, &value.OverrideID, &value.OccurrenceIndex, "The selected concept is not part of the allowed concept list."))
		}
	}
	if value.ValuePayload.Kind == domain.ExampleValueKindExampleRef && value.ValuePayload.ExampleID != nil {
		linked, err := v.example(ctx, *value.ValuePayload.ExampleID)
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

// checkValue returns the issues of one value given its resolved slot.
// strict (Create/Update) turns an invalid path, or an unknown override on a
// nested path, into an input error; reads report both as warnings. A
// depth-0 value on a removed slot stays a stale_override warning in both
// modes, as before B.3.
func (v valueValidator) checkValue(ctx context.Context, rs resolvedSlot, value domain.ExampleValue, strict bool) ([]domain.ExampleIssue, error) {
	if strict {
		if err := strictSlotError(rs, value); err != nil {
			return nil, err
		}
	}
	cp := containerPath(value.SlotPath)
	var out []domain.ExampleIssue
	switch rs.status {
	case slotOK:
		var err error
		out, err = v.valueIssues(ctx, value, rs.field)
		if err != nil {
			return nil, err
		}
	case slotMoved:
		return []domain.ExampleIssue{movedGroupValueIssue(value, cp)}, nil
	case slotUnknown:
		out = []domain.ExampleIssue{warningIssue("stale_override", nil, &value.OverrideID, &value.OccurrenceIndex, rs.reason)}
	case slotInvalid:
		fieldID := value.FieldID
		out = []domain.ExampleIssue{warningIssue("invalid_nesting", &fieldID, &value.OverrideID, &value.OccurrenceIndex, rs.reason)}
	}
	for i := range out {
		out[i].GroupPath = cp
	}
	return out, nil
}
