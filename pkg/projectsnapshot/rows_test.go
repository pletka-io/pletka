package projectsnapshot

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRowCodecPreservesWholeContentEncoding(t *testing.T) {
	// Populate every retained field independently of Coverage/visitContent. The
	// old whole-content codec is the byte-level oracle for patch comparisons.
	var content Content
	value := reflect.ValueOf(&content).Elem()
	for i := range value.NumField() {
		rows := reflect.MakeSlice(value.Field(i).Type(), 4, 4)
		for n := range rows.Len() {
			fillCodecValue(t, rows.Index(n), n)
		}
		value.Field(i).Set(rows)
	}
	whole, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	var original map[string][]json.RawMessage
	if err := json.Unmarshal(whole, &original); err != nil {
		t.Fatal(err)
	}
	frozen := map[string][]json.RawMessage{}
	for i := range value.NumField() {
		name := value.Type().Field(i).Tag.Get("json")
		for n := range value.Field(i).Len() {
			row, err := freezeRow(value.Field(i).Index(n))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(string(original[name][n]), string(row.data)); diff != "" {
				t.Fatalf("row encoding changed for %s[%d]: %s", name, n, diff)
			}
			frozen[name] = append(frozen[name], row.data)
		}
	}
	got, err := decodeContent(func(section string) []json.RawMessage { return frozen[section] })
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(content, got); diff != "" {
		t.Fatal(diff)
	}
}

// Variants exercise SQL null, allocated empty, JSON null, exact authored bytes,
// nullable false/zero, Unicode/escaping, and integers outside float64 precision.
func fillCodecValue(t *testing.T, value reflect.Value, variant int) {
	t.Helper()
	switch value.Kind() {
	case reflect.Struct:
		for i := range value.NumField() {
			fillCodecValue(t, value.Field(i), variant)
		}
	case reflect.Pointer:
		if variant > 0 {
			value.Set(reflect.New(value.Type().Elem()))
			fillCodecValue(t, value.Elem(), variant-1)
		}
	case reflect.Slice:
		if variant == 0 {
			return
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			value.SetBytes([]byte([]string{"", "", "null", " {\"en\": \"\u00e9 <tag>\", \"number\": 9007199254740993.123456789} \n"}[variant]))
		} else {
			value.Set(reflect.MakeSlice(value.Type(), variant-1, variant-1))
			for i := range value.Len() {
				fillCodecValue(t, value.Index(i), variant)
			}
		}
	case reflect.String:
		switch value.Type().Name() {
		case "Decimal":
			value.SetString("9007199254740993.12345678900")
		case "Date", "Timestamp":
			value.SetString("-infinity")
		default:
			value.SetString([]string{"", "null", "quotes \" \\ \n", "\u00e9 \u2028 <>&"}[variant])
		}
	case reflect.Int32:
		value.SetInt(int64(variant) * -123)
	case reflect.Int64:
		value.SetInt(int64(variant) * 9007199254740993)
	case reflect.Bool:
		value.SetBool(variant%2 == 1)
	default:
		t.Fatalf("uncovered codec field type %s", value.Type())
	}
}

func TestRowDecodeLoadsEmptySectionsAndRejectsPartialContent(t *testing.T) {
	empty, err := decodeContent(func(string) []json.RawMessage { return nil })
	if err != nil {
		t.Fatal(err)
	}
	value := reflect.ValueOf(empty)
	for i := range value.NumField() {
		if value.Field(i).IsNil() || value.Field(i).Len() != 0 {
			t.Fatalf("%s is not loaded-empty", value.Type().Field(i).Name)
		}
	}
	for i := range value.NumField() {
		name := value.Type().Field(i).Tag.Get("json")
		got, err := decodeContent(func(section string) []json.RawMessage {
			if section == name {
				return []json.RawMessage{json.RawMessage(`{`)}
			}
			return []json.RawMessage{json.RawMessage(`{}`)}
		})
		if err == nil || !reflect.ValueOf(got).IsZero() {
			t.Fatalf("invalid %s returned partial content: %v", name, err)
		}
	}
}
