package release

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Initial column inventory, deliberately limited to the project/model capture
// batch. Complete assembly and portable representation remain S3/S4 work. A new
// schema field needs a disposition here even if sqlc still compiles without it.
type captureInventory struct {
	table, query               string
	live, archive              reflect.Type
	copied                     string
	pending, excluded, derived map[string]string
}

var metadataCaptureInventory = []captureInventory{
	{
		table: "weave_projects", query: "WeaveSnapshotOwnedProject",
		live: reflect.TypeFor[sqlcgen.WeaveProject](), archive: reflect.TypeFor[sqlcgen.WeaveProjectsArchive](),
		copied:   "id created_at updated_at system_name ui_name description namespace parent_project_id staging_id owner_id visibility deprecated license readme topics base_url created_by_id concept_namespace",
		pending:  map[string]string{"enforce_concept_lists": "S1 remaining audit: authored vocabulary enforcement setting; no archive or complete-project claim yet"},
		excluded: map[string]string{"is_core_weave": "Live administration/discovery policy for the parent picker, not released content"},
		derived:  map[string]string{"effective_concept_namespace": "weave_effective_concept_namespace_v1(concept_namespace)"},
	},
	{
		table: "weave_models", query: "WeaveSnapshotOwnedModels",
		live: reflect.TypeFor[sqlcgen.WeaveModel](), archive: reflect.TypeFor[sqlcgen.WeaveModelsArchive](),
		copied: "id created_at updated_at system_name ui_name description project_id ontology_scope staging_id deprecated model_type",
	},
}

func captureColumns(inventory captureInventory) map[string]string {
	columns := map[string]string{"status": "CASE WHEN status = 'deprecated' THEN status ELSE 'published' END", "version_number": "sqlc.arg(version)::text"}
	for _, column := range strings.Fields(inventory.copied) {
		columns[column] = column
	}
	for column, expression := range inventory.derived {
		columns[column] = expression
	}
	return columns
}

func inventoryColumns(inventory captureInventory, archived bool) []string {
	columns := captureColumns(inventory)
	if !archived {
		for column := range inventory.derived {
			delete(columns, column)
		}
		for column, reason := range inventory.pending {
			columns[column] = reason
		}
		for column, reason := range inventory.excluded {
			columns[column] = reason
		}
	}
	result := make([]string, 0, len(columns))
	for column := range columns {
		result = append(result, column)
	}
	slices.Sort(result)
	return result
}

func TestMetadataCaptureColumnInventory(t *testing.T) {
	data, err := os.ReadFile("../../database/queries/weave_release_snapshots.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, inventory := range metadataCaptureInventory {
		t.Run(inventory.table, func(t *testing.T) {
			for _, archived := range []bool{false, true} {
				typ := inventory.live
				if archived {
					typ = inventory.archive
				}
				var actual []string
				for i := range typ.NumField() {
					actual = append(actual, typ.Field(i).Tag.Get("json"))
				}
				slices.Sort(actual)
				if want := inventoryColumns(inventory, archived); !slices.Equal(actual, want) {
					t.Fatalf("%s schema changed: classify capture/transform/deferred/excluded fields; got %v, want %v", typ.Name(), actual, want)
				}
			}
			_, query, ok := strings.Cut(string(data), "-- name: "+inventory.query+" :")
			if !ok {
				t.Fatal("capture query missing")
			}
			query, _, _ = strings.Cut(query, "-- name:")
			cols, ok := insertColumns(query)
			if !ok {
				t.Fatal("cannot parse capture columns")
			}
			values, ok := selectList(query)
			if !ok {
				t.Fatal("cannot parse capture SELECT")
			}
			columns, expressions := strings.Split(cols, ","), strings.Split(values, ",")
			want := captureColumns(inventory)
			if len(columns) != len(expressions) || len(columns) != len(want) {
				t.Fatalf("capture projection omitted/added a column: %s", query)
			}
			for i, column := range columns {
				column = strings.TrimSpace(column)
				if expression, ok := want[column]; !ok || strings.TrimSpace(expressions[i]) != expression {
					t.Errorf("%s capture = %q; want %q", column, strings.TrimSpace(expressions[i]), expression)
				}
				delete(want, column)
			}
			if len(want) != 0 {
				t.Fatalf("uncaptured fields: %v", want)
			}
		})
	}
}
