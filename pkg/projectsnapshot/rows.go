package projectsnapshot

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// visitContent uses the existing typed inventory and reviewed coverage registry.
// Every section must match; adding a section cannot silently skip freezing,
// decoding, patching or closure discovery. No separate serialization shape is
// maintained here. A nil slice is left intact for the caller's coverage check.
func visitContent(content *Content, visit func(Section, reflect.Value) error) error {
	value := reflect.ValueOf(content).Elem()
	typ := value.Type()
	sections := Coverage()
	if value.NumField() != len(sections) {
		return fmt.Errorf("project section coverage differs from value contract")
	}
	fields := make(map[string]reflect.Value, value.NumField())
	for i := range value.NumField() {
		name := typ.Field(i).Tag.Get("json")
		if name == "" || fields[name].IsValid() {
			return fmt.Errorf("missing or duplicate content section name")
		}
		fields[name] = value.Field(i)
	}
	for _, section := range sections {
		rows := fields[section.Name]
		if !rows.IsValid() || rows.Kind() != reflect.Slice || rows.Type().Elem().Name() != section.ValueType {
			return fmt.Errorf("typed section %s differs from value contract", section.Name)
		}
		delete(fields, section.Name)
		if err := visit(section, rows); err != nil {
			return err
		}
	}
	return nil
}

// freezeRow owns both its encoded bytes and its field index. Neither buffer is
// exposed: getters always decode into independent typed records. The same row
// encoding is used for draft compare-and-swap, including exact authored bytes.
func freezeRow(value reflect.Value) (record, error) {
	data, err := json.Marshal(value.Interface())
	if err != nil {
		return record{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return record{}, err
	}
	return record{data: data, fields: fields}, nil
}

// decodeContent avoids constructing another whole-project JSON envelope. Row
// order, scalar types and nil/empty cell values still use the existing codec.
// Every output section is explicitly loaded, including empty selections.
func decodeContent(read func(string) []json.RawMessage) (Content, error) {
	var content Content
	err := visitContent(&content, func(section Section, target reflect.Value) error {
		rows := read(section.Name)
		out := reflect.MakeSlice(target.Type(), len(rows), len(rows))
		for i, row := range rows {
			if err := json.Unmarshal(row, out.Index(i).Addr().Interface()); err != nil {
				return err
			}
		}
		target.Set(out)
		return nil
	})
	if err != nil {
		return Content{}, err
	}
	return content, nil
}

func (g *graph) content(id Identity, include func(nodeKey) bool) (Content, error) {
	return decodeContent(func(section string) []json.RawMessage {
		keys := g.projects[id].rows[section]
		rows := make([]json.RawMessage, 0, len(keys))
		for _, key := range keys {
			if include == nil || include(key) {
				rows = append(rows, g.nodes[key].data)
			}
		}
		return rows
	})
}
