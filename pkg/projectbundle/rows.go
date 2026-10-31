package projectbundle

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Authored JSON is transported as exact UTF-8 text, not decoded JSON numbers or
// base64. SQL NULL is null; JSON null is the string "null". Never normalize it.
func portableRow(data json.RawMessage, section projectsnapshot.Section) ([]byte, string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, "", err
	}
	key := make([]json.RawMessage, 0, len(section.Key))
	for _, name := range section.Key {
		key = append(key, fields[name])
	}
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return nil, "", err
	}
	for _, name := range section.JSONColumns {
		var raw []byte
		if err := json.Unmarshal(fields[name], &raw); err != nil {
			return nil, "", err
		}
		if raw == nil {
			continue
		}
		if !utf8.Valid(raw) {
			return nil, "", fmt.Errorf("%s.%s is not UTF-8", section.Name, name)
		}
		fields[name], err = json.Marshal(string(raw))
		if err != nil {
			return nil, "", err
		}
	}
	out, err := canonical(fields)
	return out, hash(keyJSON), err
}

func internalRow(data []byte, section projectsnapshot.Section) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if len(fields) != len(section.Columns) {
		return nil, fmt.Errorf("%s field coverage differs", section.Name)
	}
	for _, name := range section.Columns {
		if _, ok := fields[name]; !ok {
			return nil, fmt.Errorf("%s lacks %s", section.Name, name)
		}
	}
	for _, name := range section.JSONColumns {
		if string(fields[name]) == "null" {
			continue
		}
		var raw string
		if err := json.Unmarshal(fields[name], &raw); err != nil {
			return nil, err
		}
		if !json.Valid([]byte(raw)) {
			return nil, fmt.Errorf("%s.%s has invalid JSON text", section.Name, name)
		}
		encoded, err := json.Marshal([]byte(raw))
		if err != nil {
			return nil, err
		}
		fields[name] = encoded
	}
	return json.Marshal(fields)
}
