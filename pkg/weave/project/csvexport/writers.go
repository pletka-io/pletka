// Package csvexport provides pure CSV writer functions for domain entity metadata.
// Writers take domain types and write CSV to an io.Writer with no database access.
package csvexport

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
)

// allPathsString renders a field's primary path plus any legacy subfield paths
// (AllPaths) as comma-separated ->prefix:local_name chains. For non-legacy
// fields this is identical to OntologyPath().
func allPathsString(f *domain.Field) string {
	paths := f.AllPaths()
	rendered := make([]string, 0, len(paths))
	for _, elems := range paths {
		tmp := domain.Field{PathElements: elems}
		rendered = append(rendered, tmp.OntologyPath())
	}
	return strings.Join(rendered, ", ")
}

// marshalTranslations converts a Translations map to a JSON string for CSV cells.
// Returns "{}" for nil or empty maps.
func marshalTranslations(t domain.Translations) string {
	if t == nil {
		return "{}"
	}
	b, err := json.Marshal(t)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// derefString returns the string value of a pointer, or empty string if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// FieldOverrideMeta carries the base-override-derived columns that used
// to live on the field itself (set_value, category_id) but moved to
// weave_field_overrides in migration 027.
type FieldOverrideMeta struct {
	CategoryID string
	SetValue   string
}

// WriteFields writes field metadata as CSV to w.
//   - expectedRefs: pre-resolved base override refs per field ID
//     (read-only column, skipped on import)
//   - baseMeta: per-field {category_id, set_value} from the base override
//     row (entity_type=”); set_value/category_id no longer live on the
//     field after migration 027
func WriteFields(w io.Writer, fields []*domain.Field, expectedRefs map[string]string, baseMeta map[string]FieldOverrideMeta) error {
	cw := csv.NewWriter(w)

	header := []string{
		"semantic_id", "system_name", "ui_name", "description",
		"ontology_path", "ontology_scope", "expected_value_type",
		"set_value", "category_id", "status",
		"_expected_refs",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write field header: %w", err)
	}

	for i, f := range fields {
		meta := baseMeta[f.ID]
		row := []string{
			f.SemanticID,
			f.SystemName,
			marshalTranslations(f.UIName),
			marshalTranslations(f.Description),
			allPathsString(f),
			f.OntologyScope.PrefixedName(),
			f.ExpectedValueType,
			meta.SetValue,
			meta.CategoryID,
			string(f.Status),
			expectedRefs[f.ID],
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write field row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// WriteModels writes model metadata as CSV to w.
func WriteModels(w io.Writer, models []*domain.Model) error {
	cw := csv.NewWriter(w)

	header := []string{
		"semantic_id", "system_name", "ui_name", "description",
		"ontology_scope", "status",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write model header: %w", err)
	}

	for i, m := range models {
		row := []string{
			m.SemanticID,
			m.SystemName,
			marshalTranslations(m.UIName),
			marshalTranslations(m.Description),
			m.OntologyScope.PrefixedName(),
			string(m.Status),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write model row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// WriteCollections writes collection metadata as CSV to w.
func WriteCollections(w io.Writer, collections []*domain.Collection) error {
	cw := csv.NewWriter(w)

	header := []string{
		"semantic_id", "system_name", "ui_name", "description",
		"ontology_scope", "status", "collection_number", "canonical_collection_order",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write collection header: %w", err)
	}

	for i, c := range collections {
		row := []string{
			c.SemanticID,
			c.SystemName,
			marshalTranslations(c.UIName),
			marshalTranslations(c.Description),
			c.OntologyScope.PrefixedName(),
			string(c.Status),
			fmt.Sprintf("%d", c.CollectionNumber),
			fmt.Sprintf("%d", c.CanonicalCollectionOrder),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write collection row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// WriteCategories writes category metadata as CSV to w.
func WriteCategories(w io.Writer, categories []*domain.Category) error {
	cw := csv.NewWriter(w)

	header := []string{
		"semantic_id", "system_name", "ui_name", "description",
		"canonical_order", "status",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write category header: %w", err)
	}

	for i, cat := range categories {
		row := []string{
			cat.SemanticID,
			cat.SystemName,
			marshalTranslations(cat.UIName),
			marshalTranslations(cat.Description),
			fmt.Sprintf("%d", cat.CanonicalOrder),
			string(cat.Status),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write category row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// OverrideRow is a pre-processed override ready for CSV serialization.
// The caller maps domain.FieldOverride to OverrideRow before writing.
type OverrideRow struct {
	EntitySemanticID   string
	FieldSemanticID    string
	Position           int
	CollectionOrder    int
	DisplayName        domain.Translations
	Description        domain.Translations
	CollectionName     domain.Translations
	CategoryID         string
	ExpectedValueType  string
	SetValue           string
	PartOfCollectionID string
	IsRequired         bool
	MinOccurs          int
	MaxOccurs          *int
	IsHidden           bool
	Visibility         string
	Refs               string // pre-formatted: "LAM.15,LAC.8"
}

// OntologyRow is a pre-processed ontology link ready for CSV serialization.
type OntologyRow struct {
	OntologyName  string
	OntologyLabel string
	Version       string
	OntologyURI   string
	VersionIRI    string
	IsPrimary     bool
	IsActive      bool
	ClassCount    int64
	PropertyCount int64
	AddedAt       time.Time
	UsageNotes    string
}

// formatMaxOccurs returns the string representation of max_occurs, or empty for nil (unbounded).
func formatMaxOccurs(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// WriteModelFieldOverrides writes model-field override rows as CSV to w.
func WriteModelFieldOverrides(w io.Writer, overrides []OverrideRow) error {
	cw := csv.NewWriter(w)

	header := []string{
		"model_semantic_id", "field_semantic_id", "position",
		"collection_order", "display_name", "description", "collection_name",
		"category_id", "expected_value_type", "set_value",
		"is_required", "min_occurs", "max_occurs", "is_hidden",
		"visibility", "refs",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write model-field override header: %w", err)
	}

	for i, o := range overrides {
		row := []string{
			o.EntitySemanticID,
			o.FieldSemanticID,
			strconv.Itoa(o.Position),
			strconv.Itoa(o.CollectionOrder),
			marshalTranslations(o.DisplayName),
			marshalTranslations(o.Description),
			marshalTranslations(o.CollectionName),
			o.CategoryID,
			o.ExpectedValueType,
			o.SetValue,
			strconv.FormatBool(o.IsRequired),
			strconv.Itoa(o.MinOccurs),
			formatMaxOccurs(o.MaxOccurs),
			strconv.FormatBool(o.IsHidden),
			o.Visibility,
			o.Refs,
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write model-field override row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// WriteCollectionFieldOverrides writes collection-field override rows as CSV to w.
func WriteCollectionFieldOverrides(w io.Writer, overrides []OverrideRow) error {
	cw := csv.NewWriter(w)

	header := []string{
		"collection_semantic_id", "field_semantic_id", "position",
		"display_name", "description", "category_id",
		"part_of_collection_id", "expected_value_type", "set_value",
		"visibility", "refs",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write collection-field override header: %w", err)
	}

	for i, o := range overrides {
		row := []string{
			o.EntitySemanticID,
			o.FieldSemanticID,
			strconv.Itoa(o.Position),
			marshalTranslations(o.DisplayName),
			marshalTranslations(o.Description),
			o.CategoryID,
			o.PartOfCollectionID,
			o.ExpectedValueType,
			o.SetValue,
			o.Visibility,
			o.Refs,
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write collection-field override row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// WriteBaseFieldOverrides writes base (entity_type="") override rows as CSV to w.
func WriteBaseFieldOverrides(w io.Writer, overrides []OverrideRow) error {
	cw := csv.NewWriter(w)

	header := []string{
		"field_semantic_id", "position",
		"display_name", "description", "category_id",
		"expected_value_type", "set_value",
		"is_required", "visibility", "refs",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write base-field override header: %w", err)
	}

	for i, o := range overrides {
		row := []string{
			o.FieldSemanticID,
			strconv.Itoa(o.Position),
			marshalTranslations(o.DisplayName),
			marshalTranslations(o.Description),
			o.CategoryID,
			o.ExpectedValueType,
			o.SetValue,
			strconv.FormatBool(o.IsRequired),
			o.Visibility,
			o.Refs,
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write base-field override row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// WriteOntologies writes project ontology metadata as CSV to w.
func WriteOntologies(w io.Writer, ontologies []OntologyRow) error {
	cw := csv.NewWriter(w)

	header := []string{
		"ontology_name", "ontology_label", "version",
		"ontology_uri", "version_iri",
		"is_primary", "is_active",
		"class_count", "property_count",
		"added_at", "usage_notes",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write ontology header: %w", err)
	}

	for i, o := range ontologies {
		row := []string{
			o.OntologyName,
			o.OntologyLabel,
			o.Version,
			o.OntologyURI,
			o.VersionIRI,
			strconv.FormatBool(o.IsPrimary),
			strconv.FormatBool(o.IsActive),
			strconv.FormatInt(o.ClassCount, 10),
			strconv.FormatInt(o.PropertyCount, 10),
			o.AddedAt.Format(time.RFC3339),
			o.UsageNotes,
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write ontology row %d: %w", i, err)
		}
	}

	cw.Flush()
	return cw.Error()
}
