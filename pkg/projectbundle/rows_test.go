package projectbundle

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Exercise every actual typed field independently of the codec's section walk.
// The schema/capture conformance test separately checks these record types against
// storage. This fails if transport loses a newly added field or changes null/zero.
func TestEveryCapturedColumnRoundTripsExactly(t *testing.T) {
	contentType := reflect.TypeFor[projectsnapshot.Content]()
	fields := 0
	for i := range contentType.NumField() {
		sectionName := contentType.Field(i).Tag.Get("json")
		sections := projectsnapshot.Coverage()
		index := slices.IndexFunc(sections, func(s projectsnapshot.Section) bool { return s.Name == sectionName })
		if index < 0 {
			t.Fatalf("no section for %s", sectionName)
		}
		section := sections[index]
		rowType := contentType.Field(i).Type.Elem()
		fields += rowType.NumField()
		t.Run(sectionName, func(t *testing.T) {
			for _, populated := range []bool{false, true} {
				row := reflect.New(rowType)
				if populated {
					for j := range rowType.NumField() {
						fillScalar(t, row.Elem().Field(j), rowType.Field(j).Name)
					}
				}
				before := encodeJSON(t, row.Interface())
				wire, _, err := portableRow(before, section)
				if err != nil {
					t.Fatal(err)
				}
				var columns map[string]json.RawMessage
				if err := json.Unmarshal(wire, &columns); err != nil {
					t.Fatal(err)
				}
				if len(columns) != rowType.NumField() {
					t.Fatal("lost portable columns")
				}
				if populated {
					for _, name := range section.JSONColumns {
						var raw string
						if err := json.Unmarshal(columns[name], &raw); err != nil {
							t.Fatal(err)
						}
						if raw != `{ "number": 123456789012345678901234567890.001, "nested": [null,false,0,""], "é": "原文" }` {
							t.Fatalf("authored JSON changed: %s", raw)
						}
					}
				}
				internal, err := internalRow(wire, section)
				if err != nil {
					t.Fatal(err)
				}
				after := reflect.New(rowType)
				if err := json.Unmarshal(internal, after.Interface()); err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(row.Interface(), after.Interface()); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
	if fields != 272 {
		t.Fatalf("review changed captured field count: %d", fields)
	}
}

func fillScalar(t *testing.T, v reflect.Value, name string) {
	t.Helper()
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillScalar(t, v.Elem(), name)
	case reflect.String:
		s := "value/" + name + "/é/原文"
		switch v.Type().Name() {
		case "Timestamp":
			s = "0001-01-01T00:00:00.123456Z BC"
		case "Date":
			s = "infinity"
		case "Decimal":
			s = "123456789012345678901234567890.00100"
		}
		v.SetString(s)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int64:
		v.SetInt(9007199254740993)
	case reflect.Int32:
		v.SetInt(-2147483647)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.String {
			v.Set(reflect.ValueOf([]string{"", "first", "first", "é/原文"}))
			return
		}
		if v.Type().Elem().Kind() != reflect.Uint8 {
			t.Fatalf("unsupported captured slice %s", v.Type())
		}
		v.SetBytes([]byte(`{ "number": 123456789012345678901234567890.001, "nested": [null,false,0,""], "é": "原文" }`))
	default:
		t.Fatalf("unreviewed captured scalar %s", v.Type())
	}
}

func TestRowPathsUseIdentityNotContentOrArrayOrder(t *testing.T) {
	value := fixture(t, false)
	files, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := value.Root()
	in, _ := value.Input(id)
	in.Content.Fields[0].Description = []byte(`{"en":"edited"}`)
	changed, err := projectsnapshot.NewComplete(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := Encode(changed)
	if err != nil {
		t.Fatal(err)
	}
	for path := range files {
		if strings.Contains(path, "/rows/") {
			if _, ok := updated[path]; !ok {
				t.Fatalf("editing content moved identity %s", path)
			}
		}
	}
}

func TestRepeatedEvidenceConsumesWorkBudget(t *testing.T) {
	data := []byte("exact evidence")
	files := Files{"ontologies/a.json": data}
	d := bundleDecoder{files: files, remaining: int64(len(data)), consumed: map[string]bool{}}
	ref := fileRef{Path: "ontologies/a.json", SHA256: hash(data)}
	if _, err := d.read(ref, false); err != nil {
		t.Fatal(err)
	}
	if _, err := d.read(ref, false); err == nil {
		t.Fatal("shared evidence amplified decoding work")
	}
}
