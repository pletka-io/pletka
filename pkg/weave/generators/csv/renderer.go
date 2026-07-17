package csv

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// Renderer writes a flat field table from a weave generator snapshot.
type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{
		Format:        generators.FormatCSV,
		ContentType:   "text/csv; charset=utf-8",
		FileExtension: ".csv",
		RequiresTree:  false,
	}
}

func (r *Renderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("csv renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("csv renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for i, field := range snap.Fields {
		if err := cw.Write(row(snap, field)); err != nil {
			return fmt.Errorf("write csv row %d: %w", i, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

var header = []string{
	"root_kind",
	"project_id",
	"root_semantic_id",
	"field_semantic_id",
	"field_system_name",
	"field_label",
	"field_description",
	"field_relative_path",
	"category_id",
	"collection_id",
	"collection_order",
	"position",
	"ontology_scope",
	"ontology_scope_element",
	"ontology_path",
	"expected_value_type",
	"set_value",
	"is_required",
	"is_hidden",
	"ontology_path_elements",
}

func row(snap *generators.Snapshot, node generators.FieldNode) []string {
	field := node.Field
	return []string{
		string(snap.RootKind),
		snap.Project.ID,
		rootSemanticID(snap),
		field.SemanticID,
		field.SystemName,
		marshalTranslations(field.DisplayName),
		marshalTranslations(field.Description),
		node.RelativePath,
		field.CategoryID,
		field.PartOfCollectionID,
		strconv.Itoa(field.CollectionOrder),
		strconv.Itoa(field.Position),
		node.Scope.PrefixedName(),
		marshalPathElement(node.Scope),
		allPathsString(field),
		field.ExpectedValueType,
		field.SetValue,
		strconv.FormatBool(field.IsRequired),
		strconv.FormatBool(field.IsHidden),
		marshalPathElements(field.PathElements),
	}
}

func rootSemanticID(snap *generators.Snapshot) string {
	switch {
	case snap.Model != nil:
		return snap.Model.SemanticID
	case snap.Collection != nil:
		return snap.Collection.SemanticID
	case snap.Field != nil:
		return snap.Field.SemanticID
	default:
		return snap.Project.SemanticID
	}
}

func ontologyPath(elements []domain.PathElement) string {
	if len(elements) == 0 {
		return ""
	}
	f := domain.Field{PathElements: elements}
	return f.OntologyPath()
}

// allPathsString renders a field's primary path plus any legacy subfield paths
// as comma-separated ->prefix:local_name chains (one row per field, spec §E).
func allPathsString(field domain.ResolvedField) string {
	paths := field.AllPaths()
	rendered := make([]string, 0, len(paths))
	for _, elems := range paths {
		rendered = append(rendered, ontologyPath(elems))
	}
	return strings.Join(rendered, ", ")
}

func marshalTranslations(t domain.Translations) string {
	if len(t) == 0 {
		return "{}"
	}
	b, err := json.Marshal(t)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func marshalPathElement(element domain.PathElement) string {
	b, err := json.Marshal(element)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func marshalPathElements(elements []domain.PathElement) string {
	if len(elements) == 0 {
		return "[]"
	}
	b, err := json.Marshal(elements)
	if err != nil {
		return "[]"
	}
	return string(b)
}
