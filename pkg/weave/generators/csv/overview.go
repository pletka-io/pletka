package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
)

var modelOverviewHeader = []string{
	"id",
	"semantic_id",
	"system_name",
	"ui_name",
	"description",
	"status",
	"project_id",
	"deprecated",
	"ontology_scope",
	"ontology_scope_element",
}

var collectionOverviewHeader = []string{
	"id",
	"semantic_id",
	"system_name",
	"ui_name",
	"description",
	"status",
	"project_id",
	"deprecated",
	"ontology_scope",
	"ontology_scope_element",
	"collection_number",
	"canonical_collection_order",
	"default_category_id",
}

var fieldOverviewHeader = []string{
	"id",
	"semantic_id",
	"system_name",
	"ui_name",
	"description",
	"status",
	"project_id",
	"deprecated",
	"ontology_scope",
	"ontology_scope_element",
	"ontology_path",
	"ontology_path_elements",
	"expected_value_type",
	"default_value",
}

func WriteModelOverview(w io.Writer, models []*domain.Model) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(modelOverviewHeader); err != nil {
		return fmt.Errorf("write model overview header: %w", err)
	}
	for i, model := range models {
		if model == nil {
			continue
		}
		if err := cw.Write([]string{
			model.ID,
			model.SemanticID,
			model.SystemName,
			marshalTranslations(model.UIName),
			marshalTranslations(model.Description),
			string(model.Status),
			model.ProjectID,
			strconv.FormatBool(model.Deprecated),
			model.OntologyScope.PrefixedName(),
			marshalPathElement(model.OntologyScope),
		}); err != nil {
			return fmt.Errorf("write model overview row %d: %w", i, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func WriteCollectionOverview(w io.Writer, collections []*domain.Collection) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(collectionOverviewHeader); err != nil {
		return fmt.Errorf("write collection overview header: %w", err)
	}
	for i, collection := range collections {
		if collection == nil {
			continue
		}
		if err := cw.Write([]string{
			collection.ID,
			collection.SemanticID,
			collection.SystemName,
			marshalTranslations(collection.UIName),
			marshalTranslations(collection.Description),
			string(collection.Status),
			collection.ProjectID,
			strconv.FormatBool(collection.Deprecated),
			collection.OntologyScope.PrefixedName(),
			marshalPathElement(collection.OntologyScope),
			strconv.Itoa(collection.CollectionNumber),
			strconv.Itoa(collection.CanonicalCollectionOrder),
			dbutil.NilToEmpty(collection.DefaultCategoryID),
		}); err != nil {
			return fmt.Errorf("write collection overview row %d: %w", i, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func WriteFieldOverview(w io.Writer, fields []*domain.Field) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(fieldOverviewHeader); err != nil {
		return fmt.Errorf("write field overview header: %w", err)
	}
	for i, field := range fields {
		if field == nil {
			continue
		}
		if err := cw.Write([]string{
			field.ID,
			field.SemanticID,
			field.SystemName,
			marshalTranslations(field.UIName),
			marshalTranslations(field.Description),
			string(field.Status),
			field.ProjectID,
			strconv.FormatBool(field.Deprecated),
			field.OntologyScope.PrefixedName(),
			marshalPathElement(field.OntologyScope),
			field.OntologyPath(),
			marshalPathElements(field.PathElements),
			field.ExpectedValueType,
			field.DefaultValue,
		}); err != nil {
			return fmt.Errorf("write field overview row %d: %w", i, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

